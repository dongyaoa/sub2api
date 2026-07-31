package handler

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) GrokVideoTasks(c *gin.Context) {
	owner, ok := grokVideoTaskOwner(c)
	if !ok || h == nil || h.videoTasks == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task history is unavailable")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	tasks, err := h.videoTasks.List(c.Request.Context(), owner, limit)
	if err != nil {
		grokVideoTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"object":         "list",
		"data":           tasks,
		"retention_days": h.videoTasks.RetentionDays(),
	})
}

func (h *OpenAIGatewayHandler) ClearGrokVideoTasks(c *gin.Context) {
	owner, ok := grokVideoTaskOwner(c)
	if !ok || h == nil || h.videoTasks == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task history is unavailable")
		return
	}
	if err := h.videoTasks.Clear(c.Request.Context(), owner); err != nil {
		grokVideoTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

func grokVideoTaskOwner(c *gin.Context) (service.VideoTaskOwner, bool) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		return service.VideoTaskOwner{}, false
	}
	return service.VideoTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID}, true
}

func grokVideoTaskError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	errType := "api_error"
	message := "Video task history is unavailable"
	if errors.Is(err, service.ErrVideoTaskNotFound) {
		status = http.StatusNotFound
		errType = "not_found_error"
		message = "Video request not found"
	}
	c.JSON(status, gin.H{"error": gin.H{"type": errType, "message": message}})
}

func (h *OpenAIGatewayHandler) serveStoredGrokVideoContent(c *gin.Context, record *service.VideoTaskRecord) bool {
	if h == nil || h.videoTasks == nil || record == nil || strings.TrimSpace(record.VideoURL) == "" {
		return false
	}
	resp, err := h.videoTasks.OpenStoredContent(c.Request.Context(), record, c.GetHeader("Range"))
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag", "Last-Modified"} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
		contentType = strings.TrimSpace(record.ContentType)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
		contentType = "video/mp4"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, max-age=3600")
	c.Status(resp.StatusCode)
	service.MarkResponseCommitted(c)
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

func writeStoredGrokVideoStatus(c *gin.Context, record *service.VideoTaskRecord) {
	payload := gin.H{
		"id":         record.ID,
		"request_id": record.ID,
		"object":     "video.generation.task",
		"status":     record.Status,
		"created_at": record.CreatedAt,
	}
	if record.Error != nil {
		payload["error"] = record.Error
	}
	if record.CompletedAt != nil {
		payload["completed_at"] = record.CompletedAt
	}
	if strings.TrimSpace(record.VideoURL) != "" {
		prefix := ""
		if c.Request != nil && c.Request.URL != nil && strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			prefix = "/v1"
		}
		payload["video"] = gin.H{"url": prefix + "/videos/" + url.PathEscape(record.ID) + "/content"}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, payload)
}
