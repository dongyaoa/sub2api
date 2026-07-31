package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

const (
	VideoTaskStatusProcessing  = "processing"
	VideoTaskStatusCompleted   = "completed"
	VideoTaskStatusFailed      = "failed"
	VideoPlaybackFormatVersion = 1

	defaultVideoTaskTTL = 7 * 24 * time.Hour
)

var (
	ErrVideoTaskNotFound    = infraerrors.New(http.StatusNotFound, "VIDEO_TASK_NOT_FOUND", "video task not found")
	ErrVideoTaskUnavailable = infraerrors.New(http.StatusServiceUnavailable, "VIDEO_TASK_UNAVAILABLE", "video task storage is unavailable")
)

type VideoTaskMetadata struct {
	Operation  string `json:"operation,omitempty"`
	Model      string `json:"model,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Duration   int    `json:"duration,omitempty"`
}

// VideoTaskRecord is the private Redis representation. AccountID is retained
// so status/content lookups keep reaching the account that created the task.
type VideoTaskRecord struct {
	ID                    string            `json:"id"`
	UserID                int64             `json:"user_id"`
	APIKeyID              int64             `json:"api_key_id"`
	GroupID               int64             `json:"group_id,omitempty"`
	AccountID             int64             `json:"account_id"`
	Status                string            `json:"status"`
	HTTPStatus            int               `json:"http_status,omitempty"`
	Error                 json.RawMessage   `json:"error,omitempty"`
	CreatedAt             int64             `json:"created_at"`
	CompletedAt           *int64            `json:"completed_at,omitempty"`
	ExpiresAt             int64             `json:"expires_at"`
	Metadata              VideoTaskMetadata `json:"metadata,omitempty"`
	VideoURL              string            `json:"video_url,omitempty"`
	ContentType           string            `json:"content_type,omitempty"`
	ByteSize              int64             `json:"byte_size,omitempty"`
	BrowserPlayable       bool              `json:"browser_playable,omitempty"`
	PlaybackFormatVersion int               `json:"playback_format_version,omitempty"`
}

type VideoTask struct {
	ID               string            `json:"id"`
	RequestID        string            `json:"request_id"`
	Object           string            `json:"object"`
	Status           string            `json:"status"`
	HTTPStatus       int               `json:"http_status,omitempty"`
	Error            json.RawMessage   `json:"error,omitempty"`
	CreatedAt        int64             `json:"created_at"`
	CompletedAt      *int64            `json:"completed_at,omitempty"`
	ExpiresAt        int64             `json:"expires_at"`
	Metadata         VideoTaskMetadata `json:"metadata,omitempty"`
	ContentAvailable bool              `json:"content_available"`
	ContentType      string            `json:"content_type,omitempty"`
	ByteSize         int64             `json:"byte_size,omitempty"`
	BrowserPlayable  bool              `json:"browser_playable"`
}

type VideoTaskOwner struct {
	UserID   int64
	APIKeyID int64
}

func (r *VideoTaskRecord) NeedsBrowserPlaybackUpgrade() bool {
	return r == nil || !r.BrowserPlayable || r.PlaybackFormatVersion < VideoPlaybackFormatVersion
}

type VideoTaskStore interface {
	Save(ctx context.Context, task *VideoTaskRecord, ttl time.Duration) error
	Get(ctx context.Context, id string) (*VideoTaskRecord, error)
	List(ctx context.Context, owner VideoTaskOwner, limit int) ([]*VideoTaskRecord, error)
	Clear(ctx context.Context, owner VideoTaskOwner) error
}

type VideoTaskService struct {
	store       VideoTaskStore
	resolve     ImageStorageResolver
	ttl         time.Duration
	ttlResolver func() time.Duration
	httpClient  *http.Client
}

func NewVideoTaskService(store VideoTaskStore, resolve ImageStorageResolver) *VideoTaskService {
	return &VideoTaskService{
		store:      store,
		resolve:    resolve,
		ttl:        defaultVideoTaskTTL,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (s *VideoTaskService) WithTTLResolver(resolve func() time.Duration) *VideoTaskService {
	if s != nil {
		s.ttlResolver = resolve
	}
	return s
}

func (s *VideoTaskService) taskTTL() time.Duration {
	if s != nil && s.ttlResolver != nil {
		if ttl := s.ttlResolver(); ttl > 0 {
			return ttl
		}
	}
	if s == nil || s.ttl <= 0 {
		return defaultVideoTaskTTL
	}
	return s.ttl
}

func (s *VideoTaskService) RetentionDays() int {
	ttl := s.taskTTL()
	return max(1, int((ttl+24*time.Hour-1)/(24*time.Hour)))
}

func (s *VideoTaskService) RecordSubmission(
	ctx context.Context,
	owner VideoTaskOwner,
	groupID, accountID int64,
	requestID string,
	metadata VideoTaskMetadata,
	responseBody []byte,
) error {
	if s == nil || s.store == nil {
		return ErrVideoTaskUnavailable
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ErrVideoTaskNotFound
	}
	now := time.Now().UTC()
	ttl := s.taskTTL()
	status, taskErr := videoTaskState(responseBody)
	record := &VideoTaskRecord{
		ID:        requestID,
		UserID:    owner.UserID,
		APIKeyID:  owner.APIKeyID,
		GroupID:   groupID,
		AccountID: accountID,
		Status:    status,
		Error:     taskErr,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Metadata:  metadata,
	}
	if status != VideoTaskStatusProcessing {
		completedAt := now.Unix()
		record.CompletedAt = &completedAt
	}
	if err := s.store.Save(ctx, record, ttl); err != nil {
		return ErrVideoTaskUnavailable.WithCause(err)
	}
	return nil
}

func (s *VideoTaskService) GetRecord(ctx context.Context, owner VideoTaskOwner, id string) (*VideoTaskRecord, error) {
	if s == nil || s.store == nil {
		return nil, ErrVideoTaskUnavailable
	}
	record, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrVideoTaskNotFound) {
			return nil, ErrVideoTaskNotFound
		}
		return nil, ErrVideoTaskUnavailable.WithCause(err)
	}
	if record.UserID != owner.UserID || record.APIKeyID != owner.APIKeyID {
		return nil, ErrVideoTaskNotFound
	}
	return record, nil
}

func (s *VideoTaskService) List(ctx context.Context, owner VideoTaskOwner, limit int) ([]*VideoTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrVideoTaskUnavailable
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	records, err := s.store.List(ctx, owner, limit)
	if err != nil {
		return nil, ErrVideoTaskUnavailable.WithCause(err)
	}
	tasks := make([]*VideoTask, 0, len(records))
	for _, record := range records {
		if record != nil && record.UserID == owner.UserID && record.APIKeyID == owner.APIKeyID {
			tasks = append(tasks, videoTaskToPublic(record))
		}
	}
	return tasks, nil
}

func (s *VideoTaskService) Clear(ctx context.Context, owner VideoTaskOwner) error {
	if s == nil || s.store == nil {
		return ErrVideoTaskUnavailable
	}
	if err := s.store.Clear(ctx, owner); err != nil {
		return ErrVideoTaskUnavailable.WithCause(err)
	}
	return nil
}

func (s *VideoTaskService) UpdateStatus(ctx context.Context, owner VideoTaskOwner, id string, statusCode int, body []byte) error {
	record, err := s.GetRecord(ctx, owner, id)
	if err != nil {
		return err
	}
	status, taskErr := videoTaskState(body)
	record.Status = status
	record.HTTPStatus = statusCode
	record.Error = taskErr
	if status != VideoTaskStatusProcessing && record.CompletedAt == nil {
		completedAt := time.Now().UTC().Unix()
		record.CompletedAt = &completedAt
	}
	return s.save(ctx, record, time.Now().UTC())
}

func (s *VideoTaskService) StoreContent(ctx context.Context, owner VideoTaskOwner, id, contentType string, data []byte) error {
	record, err := s.GetRecord(ctx, owner, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(record.VideoURL) != "" && !record.NeedsBrowserPlaybackUpgrade() {
		return nil
	}
	normalizedType, _, normalizeErr := normalizeGeneratedVideoContent(contentType, data)
	if normalizeErr != nil || normalizedType != "video/mp4" || !isBrowserCompatibleMP4(data) {
		return errors.New("video content is not a browser-compatible H.264 MP4")
	}
	if s.resolve == nil {
		return ErrVideoTaskUnavailable
	}
	uploader, enabled := s.resolve()
	if !enabled || uploader == nil {
		return ErrVideoTaskUnavailable
	}
	url, storedType, err := uploader.StoreVideo(ctx, id, normalizedType, data)
	if err != nil {
		return err
	}
	record.VideoURL = url
	record.ContentType = storedType
	record.ByteSize = int64(len(data))
	record.BrowserPlayable = true
	record.PlaybackFormatVersion = VideoPlaybackFormatVersion
	record.Status = VideoTaskStatusCompleted
	record.HTTPStatus = http.StatusOK
	if record.CompletedAt == nil {
		completedAt := time.Now().UTC().Unix()
		record.CompletedAt = &completedAt
	}
	return s.save(ctx, record, time.Now().UTC())
}

func (s *VideoTaskService) UpgradeStoredContent(
	ctx context.Context,
	owner VideoTaskOwner,
	id, contentType string,
	data []byte,
) ([]byte, string, error) {
	prepared, preparedType, err := ensureBrowserPlayableVideo(ctx, contentType, data)
	if err != nil {
		return nil, "", err
	}
	if err := s.StoreContent(ctx, owner, id, preparedType, prepared); err != nil {
		return prepared, preparedType, err
	}
	return prepared, preparedType, nil
}

func (s *VideoTaskService) OpenStoredContent(ctx context.Context, record *VideoTaskRecord, rangeHeader string) (*http.Response, error) {
	if s == nil || record == nil || strings.TrimSpace(record.VideoURL) == "" {
		return nil, ErrVideoTaskNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, record.VideoURL, nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader = strings.TrimSpace(rangeHeader); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = resp.Body.Close()
		return nil, errors.New("stored video returned an unsuccessful response")
	}
	return resp, nil
}

func (s *VideoTaskService) save(ctx context.Context, record *VideoTaskRecord, now time.Time) error {
	ttl := s.taskTTL()
	record.ExpiresAt = now.Add(ttl).Unix()
	if err := s.store.Save(ctx, record, ttl); err != nil {
		return ErrVideoTaskUnavailable.WithCause(err)
	}
	return nil
}

func videoTaskToPublic(record *VideoTaskRecord) *VideoTask {
	return &VideoTask{
		ID:               record.ID,
		RequestID:        record.ID,
		Object:           "video.generation.task",
		Status:           record.Status,
		HTTPStatus:       record.HTTPStatus,
		Error:            record.Error,
		CreatedAt:        record.CreatedAt,
		CompletedAt:      record.CompletedAt,
		ExpiresAt:        record.ExpiresAt,
		Metadata:         record.Metadata,
		ContentAvailable: strings.TrimSpace(record.VideoURL) != "",
		ContentType:      record.ContentType,
		ByteSize:         record.ByteSize,
		BrowserPlayable:  !record.NeedsBrowserPlaybackUpgrade(),
	}
}

func videoTaskState(body []byte) (string, json.RawMessage) {
	status := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		gjson.GetBytes(body, "status").String(),
		gjson.GetBytes(body, "data.status").String(),
	)))
	switch status {
	case "completed", "succeeded", "success", "done":
		return VideoTaskStatusCompleted, nil
	case "failed", "cancelled", "canceled", "expired", "error":
		raw := gjson.GetBytes(body, "error").Raw
		if raw == "" {
			raw = gjson.GetBytes(body, "data.error").Raw
		}
		if raw == "" {
			raw = `{"message":"video generation failed"}`
		}
		return VideoTaskStatusFailed, json.RawMessage(raw)
	default:
		return VideoTaskStatusProcessing, nil
	}
}
