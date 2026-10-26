package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetWebAccess returns the current visitor's website-only access decision.
// This endpoint remains reachable even when the visitor's webpage is blocked.
func (h *SettingHandler) GetWebAccess(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	decision, err := h.settingService.GetWebAccessDecision(ctx, clientip.GetTrustedClientIP(c))
	if err != nil {
		if errors.Is(err, service.ErrWebAccessLocalNetworkUnavailable) {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable,
				"Unable to determine the local network location. Please try again.",
				"WEB_ACCESS_LOCAL_NETWORK_UNAVAILABLE",
				map[string]string{"location_source": service.WebAccessLocationLocalNetworkUnavailable})
			return
		}
		response.Error(c, http.StatusServiceUnavailable, "Unable to check website access. Please try again.")
		return
	}
	response.Success(c, decision)
}
