package handler

import (
	"errors"
	"net/http"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ImageStudioPricing returns model-specific image prices for the current key.
// It does not depend on whether the public model plaza is enabled.
func (h *GatewayHandler) ImageStudioPricing(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		writeOpenAIModelsError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group == nil || apiKey.GroupID == nil {
		writeOpenAIModelsError(c, http.StatusForbidden, "permission_error", "API key is not assigned to a group")
		return
	}
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		writeOpenAIModelsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		writeOpenAIModelsError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	var quote *service.ImageStudioPricing
	var err error
	switch apiKey.Group.Platform {
	case service.PlatformOpenAI, service.PlatformGrok:
		quote, err = h.openAIGatewayService.ImageStudioPricing(c.Request.Context(), apiKey, model)
	case service.PlatformGemini:
		quote, err = h.gatewayService.ImageStudioPricing(c.Request.Context(), apiKey, model)
	default:
		writeOpenAIModelsError(c, http.StatusNotFound, "not_found_error", "Images API is not supported for this platform")
		return
	}
	if err != nil {
		if errors.Is(err, service.ErrImageStudioModelNotAllowed) {
			writeOpenAIModelsError(c, http.StatusNotFound, "permission_error", "Model is not available for this group")
			return
		}
		writeOpenAIModelsError(c, http.StatusServiceUnavailable, "api_error", "Image pricing is unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, quote)
}
