package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestSub2APIHistoryFailureKeepsTodayAndRetriesIndependently(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, timezone.Location())
	svc := NewUpstreamFinanceService(nil, financeTestCipher{}, nil, nil, nil)
	svc.now = func() time.Time { return now }
	target := &UpstreamFinanceTarget{ID: 1, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "test-only"}
	requests := []string{}
	historyFails := true
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		days := req.URL.Query().Get("days")
		requests = append(requests, days)
		require.Equal(t, "2026-11-03", req.URL.Query().Get("end_date"))
		if days == "1" {
			require.Equal(t, "2026-11-03", req.URL.Query().Get("start_date"))
			deadline, ok := req.Context().Deadline()
			require.True(t, ok)
			require.Greater(t, time.Until(deadline), 18*time.Second, "the old client/context 12s deadline must not cap daily reads")
			return newAPICacheResponse(req, 200, `{"balance":458,"usage":{"today":{"actual_cost":205.85}}}`), nil
		}
		require.Equal(t, "2026-10-05", req.URL.Query().Get("start_date"))
		if historyFails {
			return nil, context.DeadlineExceeded
		}
		return newAPICacheResponse(req, 200, `{"balance":458,"usage":{"today":{"actual_cost":205.85}},"model_stats":[{"actual_cost":700}]}`), nil
	})
	first := svc.fetchSub2APIBalance(context.Background(), target)
	require.Equal(t, []string{"1", "30"}, requests)
	require.Equal(t, "ok", first.Status)
	require.Equal(t, 458.0, *first.Balance)
	require.Equal(t, 205.85, *first.DayUsed)
	require.Nil(t, first.Last30DaysUsed)
	require.Equal(t, "sub2api_history_sync_failed: upstream_timeout", first.Error)
	now = now.Add(time.Minute)
	second := svc.fetchSub2APIBalance(context.Background(), target)
	require.Equal(t, []string{"1", "30", "1"}, requests)
	require.Equal(t, 205.85, *second.DayUsed)
	require.Nil(t, second.Last30DaysUsed)
	historyFails = false
	now = now.Add(time.Minute)
	third := svc.fetchSub2APIBalance(context.Background(), target)
	require.Equal(t, []string{"1", "30", "1", "1", "30"}, requests)
	require.Empty(t, third.Error)
	require.Equal(t, 700.0, *third.Last30DaysUsed)
	now = now.Add(time.Minute)
	fourth := svc.fetchSub2APIBalance(context.Background(), target)
	require.Nil(t, fourth.Last30DaysUsed, "a skipped history fetch must not relabel its old value as current")
	require.Nil(t, fourth.PeriodSyncedAt)
	require.True(t, fourth.DaySyncedAt.After(*third.PeriodSyncedAt))
	require.Empty(t, fourth.Error)
}

func TestSub2APIHistoricalResponseCannotChangeCurrencyOrDay(t *testing.T) {
	for _, scenario := range []string{"currency", "midnight"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 11, 3, 23, 59, 59, 0, timezone.Location())
			svc := NewUpstreamFinanceService(nil, financeTestCipher{}, nil, nil, nil)
			svc.now = func() time.Time { return now }
			target := &UpstreamFinanceTarget{ID: 1, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "test-only"}
			svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("days") == "1" {
					return newAPICacheResponse(req, 200, `{"balance":9,"unit":"USD","usage":{"today":{"actual_cost":3}}}`), nil
				}
				if scenario == "midnight" {
					now = now.Add(2 * time.Second)
					return newAPICacheResponse(req, 200, `{"balance":9,"unit":"USD","daily_usage":[{"actual_cost":7}]}`), nil
				}
				return newAPICacheResponse(req, 200, `{"balance":9,"unit":"CNY","daily_usage":[{"actual_cost":7}]}`), nil
			})
			snapshot := svc.fetchSub2APIBalance(context.Background(), target)
			require.Equal(t, "ok", snapshot.Status)
			require.Equal(t, "USD", snapshot.Currency)
			require.Nil(t, snapshot.Last30DaysUsed)
			require.Contains(t, snapshot.Error, "sub2api_history_sync_failed")
			if scenario == "midnight" {
				require.Nil(t, completeUpstreamBalanceSnapshot(snapshot, target, now).TodayUsed)
			}
		})
	}
}

func TestSub2APITodayRetriesTransientFailureWithoutRepeatingHistory(t *testing.T) {
	svc := NewUpstreamFinanceService(nil, financeTestCipher{}, nil, nil, nil)
	target := &UpstreamFinanceTarget{ID: 1, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "test-only"}
	todayCalls, historyCalls := 0, 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("days") == "1" {
			todayCalls++
			if todayCalls == 1 {
				return newAPICacheResponse(req, 503, "unavailable"), nil
			}
		} else {
			historyCalls++
		}
		return newAPICacheResponse(req, 200, `{"balance":9,"usage":{"today":{"actual_cost":0}},"daily_usage":[]}`), nil
	})
	snapshot := svc.fetchSub2APIBalance(context.Background(), target)
	require.Equal(t, "ok", snapshot.Status)
	require.Empty(t, snapshot.Error)
	require.Equal(t, 0.0, *snapshot.DayUsed)
	require.Equal(t, 0.0, *snapshot.Last30DaysUsed)
	require.Equal(t, 2, todayCalls)
	require.Equal(t, 1, historyCalls)
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
		return newAPICacheResponse(req, 200, `{"balance":9,"usage":{"today":{"actual_cost":100}},"daily_usage":[{"actual_cost":100}]}`), nil
	})
	snapshot := completeUpstreamBalanceSnapshot(svc.fetchSub2APIBalance(context.Background(), target), target, now)
	require.Equal(t, "ok", snapshot.Status)
	require.Equal(t, 9.0, *snapshot.Balance)
	require.Nil(t, snapshot.TodayUsed)
	require.Nil(t, snapshot.DayUsed)
	require.Nil(t, snapshot.Last30DaysUsed)
	require.Equal(t, 1, calls)
}
