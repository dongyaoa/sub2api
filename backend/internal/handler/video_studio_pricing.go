package handler

import (
	"errors"
	"net/http"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// VideoStudioPricing returns an authenticated quote independent of whether the
// public model square is enabled, using the same prices as video completion.
func (h *GatewayHandler) VideoStudioPricing(c *gin.Context) {
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
	switch apiKey.Group.Platform {
	case service.PlatformOpenAI, service.PlatformGrok, service.PlatformGemini, service.PlatformComposite:
	default:
		writeOpenAIModelsError(c, http.StatusNotFound, "not_found_error", "Videos API is not supported for this platform")
		return
	}
	quote, err := h.openAIGatewayService.VideoStudioPricing(c.Request.Context(), apiKey, model)
	if err != nil {
		if errors.Is(err, service.ErrVideoStudioModelNotAllowed) {
			writeOpenAIModelsError(c, http.StatusNotFound, "permission_error", "Model is not available for this group")
			return
		}
		if errors.Is(err, service.ErrVideoPricingNotConfigured) {
			writeOpenAIModelsError(c, http.StatusServiceUnavailable, "pricing_not_configured", "Configure a per-request or per-second price for this video model before generating")
			return
		}
		writeOpenAIModelsError(c, http.StatusServiceUnavailable, "api_error", "Video pricing is unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, quote)
}
