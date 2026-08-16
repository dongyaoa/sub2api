package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type CheckinHandler struct {
	service *service.CheckinService
}

func NewCheckinHandler(checkinService *service.CheckinService) *CheckinHandler {
	return &CheckinHandler{service: checkinService}
}

func (h *CheckinHandler) GetConfig(c *gin.Context) {
	config, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *CheckinHandler) UpdateConfig(c *gin.Context) {
	var config service.CheckinConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.service.UpdateConfig(c.Request.Context(), config)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *CheckinHandler) Summary(c *gin.Context) {
	summary, err := h.service.GetAdminSummary(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

func (h *CheckinHandler) Daily(c *gin.Context) {
	start, end, ok := parseCheckinDateRange(c)
	if !ok {
		return
	}
	items, err := h.service.GetDailyStats(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *CheckinHandler) Records(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListAdminRecords(c.Request.Context(), page, pageSize, c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *CheckinHandler) Users(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListUserReports(
		c.Request.Context(),
		page,
		pageSize,
		c.Query("search"),
		c.Query("sort_by"),
		c.Query("sort_order"),
		c.Query("qualification"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func parseCheckinDateRange(c *gin.Context) (time.Time, time.Time, bool) {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	startText := strings.TrimSpace(c.Query("start_date"))
	endText := strings.TrimSpace(c.Query("end_date"))
	if startText == "" {
		startText = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	if endText == "" {
		endText = now.Format("2006-01-02")
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start, err := time.ParseInLocation("2006-01-02", startText, location)
	if err != nil {
		response.BadRequest(c, "start_date must use YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	endDate, err := time.ParseInLocation("2006-01-02", endText, location)
	if err != nil {
		response.BadRequest(c, "end_date must use YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	if endDate.Before(start) || endDate.Sub(start) > 366*24*time.Hour {
		response.BadRequest(c, "date range must be between 1 and 366 days")
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), endDate.AddDate(0, 0, 1).UTC(), true
}
