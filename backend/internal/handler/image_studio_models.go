package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) imageStudioModels(c *gin.Context, apiKey *service.APIKey, platform string) {
	if apiKey == nil || apiKey.Group == nil {
		imageTaskJSONError(c, http.StatusUnauthorized, "authentication_error", "API key group is required")
		return
	}
	if platform != service.PlatformOpenAI && platform != service.PlatformGemini && platform != service.PlatformGrok {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Images API is not supported for this platform")
		return
	}
	if h == nil || h.gatewayService == nil {
		imageTaskJSONError(c, http.StatusServiceUnavailable, "api_error", "Image model catalog is unavailable")
		return
	}
	models, err := h.gatewayService.ImageStudioModels(c.Request.Context(), apiKey.Group, platform, defaultModelIDsForPlatform(platform))
	if err != nil {
		imageTaskJSONError(c, http.StatusServiceUnavailable, "api_error", "Unable to list image models")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
}
