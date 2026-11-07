package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) videoStudioModels(c *gin.Context, apiKey *service.APIKey) {
	if apiKey == nil || apiKey.Group == nil {
		writeOpenAIModelsError(c, http.StatusUnauthorized, "authentication_error", "API key group is required")
		return
	}
	switch apiKey.Group.Platform {
	case service.PlatformOpenAI, service.PlatformGrok, service.PlatformGemini, service.PlatformComposite:
	default:
		writeOpenAIModelsError(c, http.StatusBadRequest, "invalid_request_error", "Videos API is not supported for this platform")
		return
	}
	if h == nil || h.gatewayService == nil {
		writeOpenAIModelsError(c, http.StatusServiceUnavailable, "api_error", "Video model catalog is unavailable")
		return
	}
	models, err := h.gatewayService.VideoStudioModels(c.Request.Context(), apiKey.Group)
	if err != nil {
		writeOpenAIModelsError(c, http.StatusServiceUnavailable, "api_error", "Unable to list video models")
		return
	}
	c.Header("Cache-Control", "no-store")
	writeModelsList(c, apiKey.Group.Platform, models)
}
