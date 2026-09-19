package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ChannelMonitorHandler) GetDisplayOrder(c *gin.Context) {
	order, err := h.monitorService.GetDisplayOrder(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, order)
}

func (h *ChannelMonitorHandler) UpdateDisplayOrder(c *gin.Context) {
	// A full order is small; reject oversized requests before JSON allocation.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var order service.ChannelMonitorDisplayOrder
	if err := c.ShouldBindJSON(&order); err != nil {
		response.BadRequest(c, "Invalid channel monitor display order")
		return
	}
	if err := h.monitorService.UpdateDisplayOrder(c.Request.Context(), order); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, order)
}
