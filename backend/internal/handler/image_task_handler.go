package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AsyncImageHandler struct {
	tasks   *service.ImageTaskService
	openAI  *OpenAIGatewayHandler
	gemini  *GatewayHandler
	execute func(platform string, c *gin.Context)
}

const maxAsyncImageQuantity = 4

func NewAsyncImageHandler(tasks *service.ImageTaskService, openAI *OpenAIGatewayHandler) *AsyncImageHandler {
	h := &AsyncImageHandler{tasks: tasks, openAI: openAI}
	h.execute = h.executeWithGateway
	return h
}

// enabled reports whether the async image task feature is available. Object
// storage is the enablement gate: without it the endpoints are fully disabled
// so that large base64 results never land in Redis.
func (h *AsyncImageHandler) enabled() bool {
	return h != nil && h.tasks != nil && h.tasks.Enabled()
}

// pollable reports whether task lookups can be served. It is deliberately weaker
// than enabled(): results already written to Redis stay readable after the
// feature is switched off, so an in-flight task is never stranded.
func (h *AsyncImageHandler) pollable() bool {
	return h != nil && h.tasks != nil && h.tasks.Pollable()
}

// Submit accepts the same payload as the synchronous Images endpoint and
// returns before the upstream image generation begins.
func (h *AsyncImageHandler) Submit(c *gin.Context) {
	if !h.enabled() {
		imageTaskJSONError(c, http.StatusNotFound, "not_found_error", "async image tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		imageTaskError(c, service.ErrImageTaskForbidden)
		return
	}
	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	if platform != service.PlatformOpenAI && platform != service.PlatformGrok && platform != service.PlatformGemini {
		imageTaskJSONError(c, http.StatusNotFound, "not_found_error", "Images API is not supported for this platform")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		imageTaskJSONError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if h == nil || h.tasks == nil || h.execute == nil {
		imageTaskError(c, service.ErrImageTaskUnavailable)
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			imageTaskJSONError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if asyncImageRequestStreams(c.GetHeader("Content-Type"), body) {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "streaming image requests cannot be submitted as asynchronous tasks")
		return
	}
	if err := h.validateRequest(c, platform, body); err != nil {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if !h.checkSecurityAuditBeforeSubmit(c, apiKey, platform, body) {
		return
	}

	metadata := parseAsyncImageTaskMetadata(c.Request.URL.Path, c.GetHeader("Content-Type"), body)
	if metadata.Quantity > maxAsyncImageQuantity {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "n must be between 1 and 4 for asynchronous image tasks")
		return
	}
	unitBody, err := setAsyncImageRequestQuantity(c.GetHeader("Content-Type"), body, 1)
	if err != nil {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	task, err := h.tasks.CreateWithMetadata(
		c.Request.Context(),
		service.ImageTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID},
		metadata,
	)
	if err != nil {
		imageTaskError(c, err)
		return
	}
	baseContext := c.Copy()

	pollURL := imageTaskPollURL(c.Request.URL.Path, task.ID)
	c.Header("Cache-Control", "no-store")
	c.Header("Location", pollURL)
	c.Header("Retry-After", "3")
	c.JSON(http.StatusAccepted, gin.H{
		"id":         task.ID,
		"task_id":    task.TaskID,
		"object":     task.Object,
		"status":     task.Status,
		"created_at": task.CreatedAt,
		"expires_at": task.ExpiresAt,
		"poll_url":   pollURL,
	})

	go h.run(task.ID, platform, baseContext, unitBody, metadata.Quantity, h.tasks.ExecutionTimeout())
}

func (h *AsyncImageHandler) checkSecurityAuditBeforeSubmit(c *gin.Context, apiKey *service.APIKey, platform string, body []byte) bool {
	if h == nil || h.openAI == nil {
		return true
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		imageTaskJSONError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return false
	}
	model := ""
	moderationBody := body
	moderationProtocol := service.ContentModerationProtocolOpenAIImages
	if platform == service.PlatformGemini {
		var err error
		model, moderationBody, err = buildGeminiStudioImageRequest(c.Request.URL.Path, c.GetHeader("Content-Type"), body)
		if err != nil {
			imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return false
		}
		moderationProtocol = service.ContentModerationProtocolGemini
	} else if platform == service.PlatformGrok {
		parsed := service.ParseGrokMediaRequest(c.GetHeader("Content-Type"), body)
		model, moderationBody = parsed.Model, parsed.ModerationBody()
	} else if h.openAI.gatewayService != nil {
		parsed, err := h.openAI.gatewayService.ParseOpenAIImagesRequest(c, body)
		if err != nil {
			imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return false
		}
		model, moderationBody = parsed.Model, parsed.ModerationBody()
	}
	if len(moderationBody) == 0 {
		c.Set(securityAuditCompletedContextKey, true)
		return true
	}
	reqLog := requestLogger(c, "handler.async_image.security_audit",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.String("model", model))
	decision := h.openAI.checkSecurityAudit(c, reqLog, apiKey, subject, moderationProtocol, model, moderationBody)
	if decision != nil && !decision.AllowNextStage {
		h.openAI.openAISecurityAuditError(c, decision)
		return false
	}
	return true
}

func (h *AsyncImageHandler) Get(c *gin.Context) {
	// Polling deliberately does not require the feature to be enabled, only that
	// the task store is reachable. Turning the switch off in the admin UI must not
	// strand tasks that were already accepted — their results are still in Redis
	// and their submitters are still polling.
	if !h.pollable() {
		imageTaskJSONError(c, http.StatusNotFound, "not_found_error", "async image tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		imageTaskError(c, service.ErrImageTaskForbidden)
		return
	}
	task, err := h.tasks.Get(c.Request.Context(), service.ImageTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID}, c.Param("task_id"))
	if err != nil {
		imageTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if task.Status == service.ImageTaskStatusProcessing {
		c.Header("Retry-After", "3")
	}
	c.JSON(http.StatusOK, task)
}

func (h *AsyncImageHandler) List(c *gin.Context) {
	if !h.pollable() {
		imageTaskJSONError(c, http.StatusNotFound, "not_found_error", "async image tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		imageTaskError(c, service.ErrImageTaskForbidden)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	tasks, err := h.tasks.List(
		c.Request.Context(),
		service.ImageTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID},
		limit,
	)
	if err != nil {
		imageTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"object":         "list",
		"data":           tasks,
		"retention_days": h.tasks.RetentionDays(),
	})
}

func (h *AsyncImageHandler) Clear(c *gin.Context) {
	if !h.pollable() {
		imageTaskJSONError(c, http.StatusNotFound, "not_found_error", "async image tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		imageTaskError(c, service.ErrImageTaskForbidden)
		return
	}
	if err := h.tasks.Clear(
		c.Request.Context(),
		service.ImageTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID},
	); err != nil {
		imageTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

func parseAsyncImageTaskMetadata(path, contentType string, body []byte) service.ImageTaskMetadata {
	metadata := service.ImageTaskMetadata{Operation: "generate", Quantity: 1}
	if strings.Contains(path, "/edits") {
		metadata.Operation = "edit"
	}
	if isMultipartImagesContentType(contentType) {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil || strings.TrimSpace(params["boundary"]) == "" {
			return metadata
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				break
			}
			if part.FileName() == "" {
				value, _ := io.ReadAll(io.LimitReader(part, 64<<10))
				setAsyncImageMetadataField(&metadata, part.FormName(), strings.TrimSpace(string(value)))
			}
			_ = part.Close()
		}
		return metadata
	}
	var payload struct {
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
		Quantity    int    `json:"n"`
		Size        string `json:"size"`
		Quality     string `json:"quality"`
		AspectRatio string `json:"aspect_ratio"`
		Resolution  string `json:"resolution"`
	}
	if json.Unmarshal(body, &payload) == nil {
		metadata.Model = strings.TrimSpace(payload.Model)
		metadata.Prompt = strings.TrimSpace(payload.Prompt)
		metadata.Quantity = payload.Quantity
		metadata.Size = strings.TrimSpace(payload.Size)
		metadata.Quality = strings.TrimSpace(payload.Quality)
		metadata.AspectRatio = strings.TrimSpace(payload.AspectRatio)
		metadata.Resolution = strings.TrimSpace(payload.Resolution)
	}
	if metadata.Quantity <= 0 {
		metadata.Quantity = 1
	}
	return metadata
}

func setAsyncImageMetadataField(metadata *service.ImageTaskMetadata, name, value string) {
	switch name {
	case "model":
		metadata.Model = value
	case "prompt":
		metadata.Prompt = value
	case "n":
		if quantity, err := strconv.Atoi(value); err == nil && quantity > 0 {
			metadata.Quantity = quantity
		}
	case "size":
		metadata.Size = value
	case "quality":
		metadata.Quality = value
	case "aspect_ratio":
		metadata.AspectRatio = value
	case "resolution":
		metadata.Resolution = value
	}
}

func setAsyncImageRequestQuantity(contentType string, body []byte, quantity int) ([]byte, error) {
	if quantity <= 0 {
		quantity = 1
	}
	if !isMultipartImagesContentType(contentType) {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
			return nil, errors.New("failed to parse image request body")
		}
		payload["n"] = json.RawMessage(strconv.Itoa(quantity))
		return json.Marshal(payload)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || strings.TrimSpace(params["boundary"]) == "" {
		return nil, errors.New("invalid multipart image request")
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var rewritten bytes.Buffer
	writer := multipart.NewWriter(&rewritten)
	if err := writer.SetBoundary(params["boundary"]); err != nil {
		return nil, errors.New("invalid multipart image boundary")
	}
	sawQuantity := false
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, errors.New("failed to parse multipart image request")
		}
		destination, createErr := writer.CreatePart(part.Header)
		if createErr != nil {
			_ = part.Close()
			return nil, errors.New("failed to rebuild multipart image request")
		}
		if part.FileName() == "" && part.FormName() == "n" {
			sawQuantity = true
			_, err = io.WriteString(destination, strconv.Itoa(quantity))
		} else {
			_, err = io.Copy(destination, part)
		}
		_ = part.Close()
		if err != nil {
			return nil, errors.New("failed to rebuild multipart image request")
		}
	}
	if !sawQuantity {
		if err := writer.WriteField("n", strconv.Itoa(quantity)); err != nil {
			return nil, errors.New("failed to add image quantity")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, errors.New("failed to finalize multipart image request")
	}
	return rewritten.Bytes(), nil
}
func (h *AsyncImageHandler) validateRequest(c *gin.Context, platform string, body []byte) error {
	if platform == service.PlatformGemini {
		_, _, err := buildGeminiStudioImageRequest(c.Request.URL.Path, c.GetHeader("Content-Type"), body)
		return err
	}
	if h.openAI == nil || h.openAI.gatewayService == nil {
		return nil
	}
	if platform == service.PlatformGrok {
		parsed := service.ParseGrokMediaRequest(c.GetHeader("Content-Type"), body)
		if strings.TrimSpace(parsed.Model) == "" {
			return errors.New("model is required")
		}
		return nil
	}
	parsed, err := h.openAI.gatewayService.ParseOpenAIImagesRequest(c, body)
	if err != nil {
		return err
	}
	if parsed.Stream {
		return errors.New("streaming image requests cannot be submitted as asynchronous tasks")
	}
	return nil
}

func (h *AsyncImageHandler) executeWithGateway(platform string, c *gin.Context) {
	if h.openAI == nil {
		imageTaskJSONError(c, http.StatusServiceUnavailable, "api_error", "image gateway is unavailable")
		return
	}
	if platform == service.PlatformGrok {
		h.openAI.GrokImages(c)
		return
	}
	if platform == service.PlatformGemini {
		h.executeGeminiStudioImage(c)
		return
	}
	h.openAI.Images(c)
}

type asyncImageCallResult struct {
	statusCode int
	body       json.RawMessage
	taskErr    json.RawMessage
}

func (h *AsyncImageHandler) run(
	taskID, platform string,
	baseContext *gin.Context,
	body []byte,
	quantity int,
	executionTimeout time.Duration,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().Error("image_task.execution_panicked", zap.String("task_id", taskID), zap.Any("panic", recovered))
			h.failTask(taskID, http.StatusInternalServerError, imageTaskErrorPayload("api_error", "image generation task panicked"))
		}
	}()
	if quantity <= 0 {
		quantity = 1
	}

	results := make([]asyncImageCallResult, quantity)
	taskContexts := make([]*gin.Context, quantity)
	recorders := make([]*httptest.ResponseRecorder, quantity)
	cancels := make([]context.CancelFunc, quantity)
	for index := 0; index < quantity; index++ {
		taskContexts[index], recorders[index], cancels[index] = newAsyncImageContext(baseContext, body, executionTimeout)
		childID := taskID + "-" + strconv.Itoa(index+1)
		childContext := context.WithValue(taskContexts[index].Request.Context(), ctxkey.RequestID, childID)
		childContext = context.WithValue(childContext, ctxkey.ClientRequestID, childID)
		taskContexts[index].Request = taskContexts[index].Request.WithContext(childContext)
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(quantity)
	for index := 0; index < quantity; index++ {
		go func(index int) {
			defer waitGroup.Done()
			results[index] = h.executeAsyncImageCall(taskID, platform, taskContexts[index], recorders[index], cancels[index])
		}(index)
	}
	waitGroup.Wait()

	completedBodies := make([]json.RawMessage, 0, quantity)
	statusCode := http.StatusOK
	for _, result := range results {
		if len(result.taskErr) > 0 {
			h.failTask(taskID, result.statusCode, result.taskErr)
			return
		}
		statusCode = result.statusCode
		completedBodies = append(completedBodies, result.body)
	}
	mergedBody, err := mergeAsyncImageTaskResults(completedBodies)
	if err != nil {
		h.failTask(taskID, http.StatusBadGateway, imageTaskErrorPayload("api_error", err.Error()))
		return
	}
	if err := h.tasks.Complete(context.Background(), taskID, statusCode, mergedBody); err != nil {
		logger.L().Error("image_task.complete_store_failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

func (h *AsyncImageHandler) executeAsyncImageCall(
	taskID, platform string,
	taskCtx *gin.Context,
	recorder *httptest.ResponseRecorder,
	cancel context.CancelFunc,
) (result asyncImageCallResult) {
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().Error("image_task.execution_panicked", zap.String("task_id", taskID), zap.Any("panic", recovered))
			result = asyncImageCallResult{
				statusCode: http.StatusInternalServerError,
				taskErr:    imageTaskErrorPayload("api_error", "image generation task panicked"),
			}
		}
	}()

	h.execute(platform, taskCtx)
	responseBody := bytes.TrimSpace(recorder.Body.Bytes())
	if err := taskCtx.Request.Context().Err(); err != nil && len(responseBody) == 0 {
		return asyncImageCallResult{
			statusCode: http.StatusGatewayTimeout,
			taskErr:    imageTaskErrorPayload("timeout_error", "image generation task timed out"),
		}
	}
	statusCode := recorder.Code
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		if len(responseBody) == 0 || !json.Valid(responseBody) {
			return asyncImageCallResult{
				statusCode: http.StatusBadGateway,
				taskErr:    imageTaskErrorPayload("api_error", "upstream returned an invalid image response"),
			}
		}
		normalizedBody, err := normalizeGeminiImageTaskBody(platform, responseBody)
		if err != nil {
			return asyncImageCallResult{
				statusCode: http.StatusBadGateway,
				taskErr:    imageTaskErrorPayload("api_error", err.Error()),
			}
		}
		return asyncImageCallResult{statusCode: statusCode, body: json.RawMessage(normalizedBody)}
	}
	return asyncImageCallResult{statusCode: statusCode, taskErr: extractImageTaskError(responseBody)}
}

func mergeAsyncImageTaskResults(results []json.RawMessage) (json.RawMessage, error) {
	if len(results) == 0 {
		return nil, errors.New("image generation completed without results")
	}
	if len(results) == 1 {
		return results[0], nil
	}

	var merged map[string]json.RawMessage
	items := make([]json.RawMessage, 0, len(results))
	for _, result := range results {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(result, &envelope); err != nil {
			return nil, errors.New("upstream returned an invalid image response")
		}
		if merged == nil {
			merged = envelope
		}
		var resultItems []json.RawMessage
		if rawData := envelope["data"]; len(rawData) > 0 {
			if err := json.Unmarshal(rawData, &resultItems); err != nil {
				return nil, errors.New("upstream returned invalid image data")
			}
		}
		if len(resultItems) == 0 {
			var imageURL string
			if rawURL := envelope["image_url"]; len(rawURL) > 0 {
				_ = json.Unmarshal(rawURL, &imageURL)
			}
			if strings.TrimSpace(imageURL) != "" {
				item, _ := json.Marshal(map[string]string{"url": strings.TrimSpace(imageURL)})
				resultItems = append(resultItems, item)
			}
		}
		if len(resultItems) == 0 {
			return nil, errors.New("upstream completed without returning an image")
		}
		items = append(items, resultItems...)
	}
	encodedItems, err := json.Marshal(items)
	if err != nil {
		return nil, errors.New("failed to combine generated images")
	}
	merged["data"] = encodedItems
	delete(merged, "image_url")
	combined, err := json.Marshal(merged)
	if err != nil {
		return nil, errors.New("failed to combine image response")
	}
	return combined, nil
}
func (h *AsyncImageHandler) failTask(taskID string, statusCode int, taskErr json.RawMessage) {
	if err := h.tasks.Fail(context.Background(), taskID, statusCode, taskErr); err != nil {
		logger.L().Error("image_task.failure_store_failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

func newAsyncImageContext(c *gin.Context, body []byte, timeoutDuration time.Duration) (*gin.Context, *httptest.ResponseRecorder, context.CancelFunc) {
	base := context.WithoutCancel(c.Request.Context())
	executionCtx, cancel := context.WithTimeout(base, timeoutDuration)
	request := c.Request.Clone(executionCtx)
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	request.ContentLength = int64(len(body))
	request.URL.Path = strings.TrimSuffix(request.URL.Path, "/async")

	taskCtx := c.Copy()
	recorder := httptest.NewRecorder()
	recorderCtx, _ := gin.CreateTestContext(recorder)
	taskCtx.Writer = recorderCtx.Writer
	taskCtx.Request = request
	return taskCtx, recorder, cancel
}

func asyncImageRequestStreams(contentType string, body []byte) bool {
	if isMultipartImagesContentType(contentType) {
		return false
	}
	var envelope struct {
		Stream bool `json:"stream"`
	}
	return json.Unmarshal(body, &envelope) == nil && envelope.Stream
}

func imageTaskPollURL(submitPath, taskID string) string {
	if strings.HasPrefix(submitPath, "/v1/") {
		return "/v1/images/tasks/" + taskID
	}
	return "/images/tasks/" + taskID
}

func extractImageTaskError(body []byte) json.RawMessage {
	if json.Valid(body) {
		var envelope struct {
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 && json.Valid(envelope.Error) {
			return envelope.Error
		}
		return json.RawMessage(body)
	}
	return imageTaskErrorPayload("api_error", "image generation failed")
}

func imageTaskErrorPayload(errorType, message string) json.RawMessage {
	data, _ := json.Marshal(gin.H{"type": errorType, "message": message})
	return data
}

func imageTaskError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := infraerrors.Reason(err)
	message := infraerrors.Message(err)
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if strings.TrimSpace(code) == "" {
		code = "IMAGE_TASK_ERROR"
	}
	imageTaskJSONError(c, status, code, message)
}

func imageTaskJSONError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": gin.H{"type": code, "code": code, "message": message}})
}
