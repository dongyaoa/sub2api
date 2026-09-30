package admin

import (
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func parsePromotionOrderFilter(c *gin.Context) (service.PromotionOrderFilter, error) {
	page, size := response.ParsePagination(c)
	if size > 100 {
		size = 100
	}
	filter := service.PromotionOrderFilter{Page: page, PageSize: size, Status: strings.TrimSpace(c.Query("status")), Keyword: strings.TrimSpace(c.Query("keyword"))}
	for _, field := range []struct {
		name   string
		target **time.Time
	}{{"start_time", &filter.StartTime}, {"end_time", &filter.EndTime}} {
		if raw := strings.TrimSpace(c.Query(field.name)); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return filter, infraerrors.BadRequest("INVALID_PROMOTION_FILTER", field.name+" must be an RFC3339 timestamp")
			}
			*field.target = &parsed
		}
	}
	if raw := c.Query("user_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return filter, infraerrors.BadRequest("INVALID_PROMOTION_FILTER", "user_id must be a positive integer")
		}
		filter.UserID = id
	}
	return filter, service.ValidatePromotionOrderFilter(filter)
}

func (h *PaymentHandler) GetPromotionSummary(c *gin.Context) {
	filter, err := parsePromotionOrderFilter(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	summary, err := h.paymentService.GetPromotionSummary(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

func (h *PaymentHandler) ListPromotionOrders(c *gin.Context) {
	filter, err := parsePromotionOrderFilter(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	orders, total, err := h.paymentService.ListPromotionOrders(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	type promotionOrderResult struct {
		*AdminPaymentOrderResult
		UserUsername string `json:"user_username"`
	}
	items := make([]promotionOrderResult, 0, len(orders))
	for _, order := range orders {
		items = append(items, promotionOrderResult{AdminPaymentOrderResult: sanitizeAdminPaymentOrderForResponse(order), UserUsername: order.UserName})
	}
	response.Paginated(c, items, int64(total), filter.Page, filter.PageSize)
}
