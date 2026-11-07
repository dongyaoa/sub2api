package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestSub2APITodayRetriesTransientFailureWithoutReadingHistory(t *testing.T) {
	svc := NewUpstreamFinanceService(nil, financeTestCipher{}, nil, nil, nil)
	target := &UpstreamFinanceTarget{ID: 1, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "test-only"}
	calls := 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "1", req.URL.Query().Get("days"))
		require.Equal(t, req.URL.Query().Get("start_date"), req.URL.Query().Get("end_date"))
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		require.Greater(t, time.Until(deadline), 18*time.Second)
		calls++
		if calls == 1 {
			return newAPICacheResponse(req, 503, "unavailable"), nil
		}
		return newAPICacheResponse(req, 200, `{"balance":9,"usage":{"today":{"actual_cost":0}}}`), nil
	})
	snapshot := svc.fetchSub2APIBalance(context.Background(), target)
	require.Equal(t, "ok", snapshot.Status)
	require.Empty(t, snapshot.Error)
	require.Equal(t, 0.0, *snapshot.DayUsed)
	require.Equal(t, 2, calls)
}

func TestSub2APITodayCrossingMidnightDoesNotRelabelYesterday(t *testing.T) {
	now := time.Date(2026, 11, 3, 23, 59, 59, 0, timezone.Location())
	svc := NewUpstreamFinanceService(nil, financeTestCipher{}, nil, nil, nil)
	svc.now = func() time.Time { return now }
	target := &UpstreamFinanceTarget{ID: 1, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "test-only"}
	calls := 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		now = now.Add(2 * time.Second)
		return newAPICacheResponse(req, 200, `{"balance":9,"usage":{"today":{"actual_cost":100}}}`), nil
	})
	snapshot := completeUpstreamBalanceSnapshot(svc.fetchSub2APIBalance(context.Background(), target), target, now)
	require.Equal(t, "ok", snapshot.Status)
	require.Equal(t, 9.0, *snapshot.Balance)
	require.Nil(t, snapshot.TodayUsed)
	require.Nil(t, snapshot.DayUsed)
	require.Equal(t, 1, calls)
}
