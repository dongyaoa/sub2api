package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type asyncImageMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*service.ImageTaskRecord
}

func (s *asyncImageMemoryStore) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	s.tasks[task.ID] = &copy
	return nil
}

func (s *asyncImageMemoryStore) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[id]
	if task == nil {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	return &copy, nil
}

func (s *asyncImageMemoryStore) List(_ context.Context, owner service.ImageTaskOwner, limit int) ([]*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]*service.ImageTaskRecord, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
			continue
		}
		copy := *task
		copy.Result = append(json.RawMessage(nil), task.Result...)
		copy.Error = append(json.RawMessage(nil), task.Error...)
		tasks = append(tasks, &copy)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt > tasks[j].CreatedAt })
	if limit > 0 && len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

func (s *asyncImageMemoryStore) Clear(_ context.Context, owner service.ImageTaskOwner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, task := range s.tasks {
		if task.UserID == owner.UserID && task.APIKeyID == owner.APIKeyID {
			delete(s.tasks, id)
		}
	}
	return nil
}

func (s *asyncImageMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; !ok {
		return service.ErrImageTaskNotFound
	}
	delete(s.tasks, id)
	return nil
}

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var accepted struct {
		TaskID  string `json:"task_id"`
		Status  string `json:"status"`
		PollURL string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	require.Equal(t, service.ImageTaskStatusProcessing, accepted.Status)
	require.Equal(t, "/v1/images/tasks/"+accepted.TaskID, accepted.PollURL)
	require.Equal(t, accepted.PollURL, w.Header().Get("Location"))

	// The detached background request must survive completion of/cancellation
	// from the short submission request.
	cancelRequest()
	close(release)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)

	pollReq := httptest.NewRequest(http.MethodGet, accepted.PollURL, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Equal(t, "no-store", pollWriter.Header().Get("Cache-Control"))
	require.Empty(t, pollWriter.Header().Get("Retry-After"))
	require.Contains(t, pollWriter.Body.String(), "https://example.test/image.png")
}

func TestAsyncImageHandlerSubmitMultipleImagesFansOutAndMerges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	type capturedCall struct {
		quantity  int
		requestID string
		err       error
	}
	captured := make(chan capturedCall, 3)
	var callSequence atomic.Int32
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		var payload struct {
			Quantity int `json:"n"`
		}
		if err == nil {
			err = json.Unmarshal(body, &payload)
		}
		requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		captured <- capturedCall{quantity: payload.Quantity, requestID: requestID, err: err}
		index := callSequence.Add(1)
		c.JSON(http.StatusOK, gin.H{
			"created": 123,
			"data":    []gin.H{{"url": fmt.Sprintf("https://example.test/image-%d.png", index)}},
		})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations/async",
		strings.NewReader("{\"model\":\"gpt-image-2\",\"prompt\":\"three cats\",\"n\":3}"),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	var completed *service.ImageTask
	require.Eventually(t, func() bool {
		current, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		if err != nil || current.Status != service.ImageTaskStatusCompleted {
			return false
		}
		completed = current
		return true
	}, time.Second, 10*time.Millisecond)

	var merged struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completed.Result, &merged))
	require.Len(t, merged.Data, 3)

	requestIDs := make(map[string]struct{}, 3)
	for range 3 {
		call := <-captured
		require.NoError(t, call.err)
		require.Equal(t, 1, call.quantity)
		require.NotEmpty(t, call.requestID)
		requestIDs[call.requestID] = struct{}{}
	}
	require.Len(t, requestIDs, 3, "each image call needs a unique billing request id")
	require.Equal(t, int32(3), callSequence.Load())
}

func TestSetAsyncImageRequestQuantityRewritesMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace the background"))
	require.NoError(t, writer.WriteField("n", "3"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-png-content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rewritten, err := setAsyncImageRequestQuantity(writer.FormDataContentType(), body.Bytes(), 1)
	require.NoError(t, err)
	metadata := parseAsyncImageTaskMetadata("/v1/images/edits/async", writer.FormDataContentType(), rewritten)
	require.Equal(t, 1, metadata.Quantity)
	require.Equal(t, "gpt-image-2", metadata.Model)
	require.Equal(t, "replace the background", metadata.Prompt)
	require.Contains(t, string(rewritten), "source.png")
	require.Contains(t, string(rewritten), "fake-png-content")
}
func TestAsyncImageHandlerSubmitMultipartEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	type capturedRequest struct {
		path        string
		contentType string
		body        string
		err         error
	}
	captured := make(chan capturedRequest, 1)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		captured <- capturedRequest{
			path:        c.Request.URL.Path,
			contentType: c.GetHeader("Content-Type"),
			body:        string(body),
			err:         err,
		}
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/edited.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/edits/async", h.Submit)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace the background"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-png-content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits/async", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case got := <-captured:
		require.NoError(t, got.err)
		require.Equal(t, "/v1/images/edits", got.path)
		require.Equal(t, writer.FormDataContentType(), got.contentType)
		require.Contains(t, got.body, "gpt-image-2")
		require.Contains(t, got.body, "replace the background")
		require.Contains(t, got.body, "source.png")
		require.Contains(t, got.body, "fake-png-content")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for asynchronous edit request")
	}
}

func TestAsyncImageHandlerSubmitGeminiAndNormalizeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	executed := make(chan struct{}, 1)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(platform string, c *gin.Context) {
		require.Equal(t, service.PlatformGemini, platform)
		executed <- struct{}{}
		c.JSON(http.StatusOK, gin.H{
			"candidates": []gin.H{{
				"content": gin.H{"parts": []gin.H{{
					"inlineData": gin.H{"mimeType": "image/png", "data": "aW1hZ2U="},
				}}},
			}},
		})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations/async",
		strings.NewReader(`{"model":"gemini-3.1-flash-image","prompt":"cat","n":1,"aspect_ratio":"1:1","resolution":"1K"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	require.NotEmpty(t, accepted.TaskID)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		if err != nil || got.Status != service.ImageTaskStatusCompleted {
			return false
		}
		return strings.Contains(string(got.Result), `"b64_json":"aW1hZ2U="`)
	}, time.Second, 10*time.Millisecond)
	select {
	case <-executed:
	default:
		t.Fatal("Gemini image request was not executed")
	}
}

func TestParseAsyncImageTaskMetadata(t *testing.T) {
	jsonMetadata := parseAsyncImageTaskMetadata(
		"/v1/images/generations/async",
		"application/json",
		[]byte(`{"model":"gpt-image-2","prompt":"city at night","n":3,"size":"1536x1024","quality":"high","aspect_ratio":"3:2","resolution":"2K"}`),
	)
	require.Equal(t, service.ImageTaskMetadata{
		Operation:   "generate",
		Model:       "gpt-image-2",
		Prompt:      "city at night",
		Quantity:    3,
		Size:        "1536x1024",
		Quality:     "high",
		AspectRatio: "3:2",
		Resolution:  "2K",
	}, jsonMetadata)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "remove the sign"))
	require.NoError(t, writer.WriteField("n", "2"))
	require.NoError(t, writer.WriteField("size", "1024x1536"))
	require.NoError(t, writer.WriteField("quality", "medium"))
	require.NoError(t, writer.Close())

	editMetadata := parseAsyncImageTaskMetadata(
		"/v1/images/edits/async",
		writer.FormDataContentType(),
		body.Bytes(),
	)
	require.Equal(t, service.ImageTaskMetadata{
		Operation: "edit",
		Model:     "gpt-image-2",
		Prompt:    "remove the sign",
		Quantity:  2,
		Size:      "1024x1536",
		Quality:   "medium",
	}, editMetadata)
}

func TestAsyncImageHandlerListAndClear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, 7*24*time.Hour, time.Minute)
	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := tasks.CreateWithMetadata(context.Background(), owner, service.ImageTaskMetadata{
		Operation:  "generate",
		Model:      "gpt-image-2",
		Prompt:     "a glass city",
		Quantity:   1,
		Resolution: "1K",
	})
	require.NoError(t, err)
	other, err := tasks.CreateWithMetadata(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 10}, service.ImageTaskMetadata{
		Prompt: "private task",
	})
	require.NoError(t, err)

	h := &AsyncImageHandler{tasks: tasks}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7})
		c.Next()
	})
	router.GET("/v1/images/tasks", h.List)
	router.DELETE("/v1/images/tasks", h.Clear)
	router.DELETE("/v1/images/tasks/:task_id", h.Delete)

	listReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks?limit=10", nil)
	listWriter := httptest.NewRecorder()
	router.ServeHTTP(listWriter, listReq)
	require.Equal(t, http.StatusOK, listWriter.Code)
	var response struct {
		Data          []service.ImageTask `json:"data"`
		RetentionDays int                 `json:"retention_days"`
	}
	require.NoError(t, json.Unmarshal(listWriter.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	require.Equal(t, 7, response.RetentionDays)
	require.Equal(t, created.ID, response.Data[0].ID)
	require.Equal(t, "a glass city", response.Data[0].Metadata.Prompt)

	clearReq := httptest.NewRequest(http.MethodDelete, "/v1/images/tasks", nil)
	clearWriter := httptest.NewRecorder()
	router.ServeHTTP(clearWriter, clearReq)
	require.Equal(t, http.StatusNoContent, clearWriter.Code)

	listWriter = httptest.NewRecorder()
	router.ServeHTTP(listWriter, listReq)
	require.Equal(t, http.StatusOK, listWriter.Code)
	require.NoError(t, json.Unmarshal(listWriter.Body.Bytes(), &response))
	require.Empty(t, response.Data)

	_, err = tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 10}, other.ID)
	require.NoError(t, err)
}

// When object storage is not configured the feature is fully disabled: the
// endpoints must return 404 without creating a task or writing to Redis.
func TestAsyncImageHandlerDisabledReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithOptions(store, time.Hour, time.Minute) // enabled == false
	h := &AsyncImageHandler{tasks: tasks}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not enabled")

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_missing", nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusNotFound, pollWriter.Code)

	// No task was created / persisted.
	require.Empty(t, store.tasks)
}
