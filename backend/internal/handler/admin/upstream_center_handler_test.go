package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamTodaySummaryHandlerRepo struct {
	service.UpstreamFinanceRepository
	queries []service.UpstreamFinanceQuery
}

func (r *upstreamTodaySummaryHandlerRepo) Summary(_ context.Context, q service.UpstreamFinanceQuery) (*service.UpstreamFinanceSummary, error) {
	r.queries = append(r.queries, q)
	return &service.UpstreamFinanceSummary{Revenue: 12, From: q.From, To: q.To, Currency: "USD"}, nil
}

func TestUpstreamFinanceSummaryReturnsOnlyToday(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &upstreamTodaySummaryHandlerRepo{}
	handler := NewUpstreamCenterHandler(nil, service.NewUpstreamFinanceService(repo, nil, nil, nil, nil))
	router := gin.New()
	router.GET("/finance/summary", handler.FinanceSummary)
	out := httptest.NewRecorder()
	before := timezone.StartOfDay(time.Now())
	router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/finance/summary?supplier_id=3&target_id=7", nil))
	require.Equal(t, http.StatusOK, out.Code)
	require.Len(t, repo.queries, 1, "the summary endpoint must never perform a second historical aggregation")
	query := repo.queries[0]
	require.Equal(t, int64(3), *query.SupplierID)
	require.Equal(t, int64(7), *query.TargetID)
	require.True(t, query.From.Equal(before) || query.From.Equal(timezone.StartOfDay(time.Now())))
	require.True(t, query.To.Equal(query.From.AddDate(0, 0, 1)))
	var response struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &response))
	require.JSONEq(t, `12`, string(response.Data["revenue"]))
	require.Contains(t, response.Data, "from")
	require.NotContains(t, response.Data, "today", "there is no longer a multi-period response wrapper")
	require.NotContains(t, response.Data, "last_30_days")
	require.NotContains(t, response.Data, "period_cost_disabled")
}

func TestUpstreamHandlerRejectsInvalidRangesBeforeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUpstreamCenterHandler(nil, nil)
	router := gin.New()
	router.GET("/targets/:id/history", handler.History)
	router.GET("/finance", handler.Finance)
	for _, path := range []string{"/targets/no/history", "/targets/1/history?from=not-a-date", "/targets/1/history?from=2026-09-23T00:00:00Z&to=2026-09-22T00:00:00Z", "/finance?supplier_id=-1", "/finance?from=2026-09-23T00:00:00Z&to=2026-09-23T00:00:00Z"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestUpstreamHandlerMalformedJSONDoesNotEchoAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUpstreamCenterHandler(nil, nil)
	router := gin.New()
	router.POST("/targets", handler.CreateTarget)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/targets", strings.NewReader(`{"api_key":"sensitive-test-value","enabled":"invalid"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.NotContains(t, response.Body.String(), "sensitive-test-value")
}
