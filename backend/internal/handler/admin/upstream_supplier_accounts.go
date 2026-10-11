package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *UpstreamCenterHandler) SupplierAccounts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	result, err := h.svc.SupplierAccounts(c.Request.Context(), id, service.UpstreamSupplierAccountsQuery{
		Search: c.Query("search"), Page: page, PageSize: pageSize,
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
