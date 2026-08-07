package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterPublicChannelMonitorRoutes exposes the read-only, sanitized channel
// monitor view without authentication. The handler response deliberately omits
// endpoints, API keys, request overrides, and all other monitor configuration.
func RegisterPublicChannelMonitorRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	monitors := v1.Group("/public/channel-monitors")
	monitors.Use(panelRateLimiter.PublicIP())
	{
		monitors.GET("", h.ChannelMonitor.List)
		monitors.GET("/:id/status", h.ChannelMonitor.GetStatus)
	}
}
