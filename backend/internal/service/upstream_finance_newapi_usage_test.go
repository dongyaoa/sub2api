package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestNewAPIRecentUsageUsesCompleteKeyLogsAndQuotaUnit(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	day := timezone.StartOfDay(now)
	fixtures := newAPIFixtures()
	fixtures["/api/log/token"] = fmt.Sprintf(`{"success":true,"data":[{"user_id":42,"token_id":13,"created_at":%d,"type":2,"quota":500000},{"user_id":42,"token_id":13,"created_at":%d,"type":2,"quota":1000000},{"user_id":42,"token_id":13,"created_at":%d,"type":2,"quota":7500000}]}`, day.Add(time.Hour).Unix(), day.AddDate(0, 0, -5).Unix(), day.AddDate(0, 0, -31).Unix())
	fixtures["/api/usage/token/"] = strings.Replace(fixtures["/api/usage/token/"], `"total_used":250000`, `"total_used":9000000`, 1)
	svc, target, paths := newAPIFinanceFixture(t, fixtures, false)
	svc.now = func() time.Time { return now }
	snapshot := svc.fetchBalance(context.Background(), target)
	require.Empty(t, snapshot.Error)
	require.Equal(t, 1.0, *snapshot.DayUsed)
	require.Equal(t, 1.0, *snapshot.TodayUsed, "daily alias must not divide quota twice")
	require.Equal(t, 3.0, *snapshot.Last30DaysUsed)
	require.Equal(t, 18.0, *snapshot.TotalUsed, "cumulative Key consumption is not today's cost")
	require.True(t, day.Equal(*snapshot.DayStart))
	require.True(t, day.AddDate(0, 0, -29).Equal(*snapshot.PeriodStart))
	require.Contains(t, *paths, "/api/log/token")
	require.NotContains(t, *paths, "/api/log/self/stat")
	supplierID := int64(1)
	target.SupplierID, target.APIKeyFingerprint, target.RechargeRatio = &supplierID, "newapi-key13", financeFloat(10)
	summary := &UpstreamFinanceSummary{Revenue: 2, RequestCount: 1, Currency: "USD"}
	query := UpstreamFinanceQuery{From: day, To: day.AddDate(0, 0, 1)}
	sources := &UpstreamProfitSources{Targets: map[int64]*UpstreamFinanceTarget{target.ID: target}, Balances: map[int64]*UpstreamBalanceSnapshot{target.ID: snapshot}, LastBusinessAt: map[int64]time.Time{target.ID: now}}
	applyUpstreamDailyProfit(summary, query, sources, now)
	require.InDelta(t, .1, *summary.RemoteUsed, 1e-10, "divide raw quota by quota_per_unit first, then apply 1:10 recharge conversion")
	require.InDelta(t, 1.9, *summary.Profit, 1e-10)
}

func TestNewAPIRecentUsageNeverTreatsTruncatedListAsComplete(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	day := timezone.StartOfDay(now)
	for _, tc := range []struct {
		name               string
		logs               []newAPIUsageLog
		total              float64
		wantDay, wantMonth bool
		code               string
	}{
		{"truncated recent rows", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}}, 20, false, false, "newapi_usage_account_auth_required"},
		{"known complete lifetime", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}}, 10, true, true, ""},
		{"zero lifetime", []newAPIUsageLog{}, 0, true, true, ""},
		{"deleted records", []newAPIUsageLog{}, 10, false, false, "newapi_usage_account_auth_required"},
		{"day boundary same second", []newAPIUsageLog{{42, 13, day.Unix(), 2, financeFloat(10)}}, 20, false, false, "newapi_usage_account_auth_required"},
		{"day covered month truncated", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}, {42, 13, day.Add(-time.Second).Unix(), 2, financeFloat(20)}}, 40, true, false, "newapi_usage_account_auth_required"},
		{"foreign Key mixed", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}, {42, 14, day.Add(-time.Second).Unix(), 2, financeFloat(20)}}, 30, false, false, "newapi_usage_incomplete"},
		{"unordered rows", []newAPIUsageLog{{42, 13, day.Unix(), 2, financeFloat(10)}, {42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(20)}}, 30, false, false, "newapi_usage_incomplete"},
		{"negative charge", []newAPIUsageLog{{42, 13, day.Unix(), 2, financeFloat(-1)}}, 0, false, false, "newapi_usage_incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"success": true, "data": tc.logs})
			require.NoError(t, err)
			fixtures := newAPIFixtures()
			fixtures["/api/log/token"] = string(body)
			svc, _, _ := newAPIFinanceFixture(t, fixtures, false)
			snapshot := &UpstreamBalanceSnapshot{TotalUsed: &tc.total}
			code := svc.fetchNewAPIRecentUsage(context.Background(), "https://8.8.8.8/prefix", "sk-inference-secret", snapshot, day)
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.wantDay, snapshot.DayUsed != nil)
			require.Equal(t, tc.wantMonth, snapshot.Last30DaysUsed != nil)
		})
	}
}

func newAPIUsagePageBody(page, size int, total int64, logs []newAPIUsageLog) string {
	body, _ := json.Marshal(map[string]any{"success": true, "data": newAPIUsagePage{Page: page, PageSize: size, Total: &total, Items: logs}})
	return string(body)
}

func TestNewAPIAccountUsagePaginatesBusyAccountAndSeparatesKeys(t *testing.T) {
	svc, target, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day := timezone.StartOfDay(*now)
	firstUntil := *now
	var requests atomic.Int64
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		require.Equal(t, "/prefix/api/log/self", req.URL.Path)
		require.Equal(t, "Bearer console-key", req.Header.Get("Authorization"))
		require.Equal(t, "42", req.Header.Get("New-Api-User"))
		q := req.URL.Query()
		require.Equal(t, "2", q.Get("type"))
		require.Equal(t, "100", q.Get("page_size"))
		require.Len(t, q, 5, "never aggregate by a mutable/non-unique token_name")
		page, _ := strconv.Atoi(q.Get("p"))
		total := int64(20001)
		created := day.Add(time.Hour).Unix()
		if q.Get("start_timestamp") == strconv.FormatInt(day.AddDate(0, 0, -29).Unix(), 10) {
			total, created = 3, day.AddDate(0, 0, -1).Unix()
			require.Equal(t, strconv.FormatInt(day.Unix()-1, 10), q.Get("end_timestamp"))
		} else {
			require.Equal(t, strconv.FormatInt(firstUntil.Unix()-1, 10), q.Get("end_timestamp"), "resume must keep its original cutoff")
		}
		logs := []newAPIUsageLog{}
		for i := (page - 1) * 100; i < min(page*100, int(total)); i++ {
			logs = append(logs, newAPIUsageLog{42, int64(13 + i%2), created, 2, financeFloat(1000)})
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, total, logs)), nil
	})
	snapshot := &UpstreamBalanceSnapshot{}
	code := svc.fetchNewAPIAccountUsage(context.Background(), "https://example.com/prefix", "console-key", target, 13, snapshot, day)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Nil(t, snapshot.DayUsed, "never publish partial charges while the initial scan is paused")
	require.LessOrEqual(t, requests.Load(), int64(120), "reserve API limit headroom for wallet reads")
	*now = now.Add(3 * time.Minute)
	code = svc.fetchNewAPIAccountUsage(context.Background(), "https://example.com/prefix", "console-key", target, 13, snapshot, day)
	require.Empty(t, code)
	require.Equal(t, 10001000.0, *snapshot.DayUsed)
	require.Equal(t, 10003000.0, *snapshot.Last30DaysUsed)
	require.True(t, snapshot.SyncedAt.Equal(firstUntil), "a resumed result must retain its real data cutoff")
	require.LessOrEqual(t, requests.Load(), int64(210), "continue checked pages instead of restarting all 20,001 records")
	calls := requests.Load()
	other := &UpstreamBalanceSnapshot{}
	code = svc.fetchNewAPIAccountUsage(context.Background(), "https://example.com/prefix", "console-key", target, 14, other, day)
	require.Empty(t, code)
	require.Equal(t, 10000000.0, *other.DayUsed)
	require.Equal(t, 10001000.0, *other.Last30DaysUsed)
	require.Equal(t, calls, requests.Load(), "same account and window must share a scan across Keys")
}

func TestNewAPIAccountUsageRejectsIncompletePagesAndKeepsToday(t *testing.T) {
	for _, problem := range []string{"missing quota", "wrong user", "wrong type", "out of window", "wrong page", "short page", "total changed", "count capped", "too many pages"} {
		t.Run(problem, func(t *testing.T) {
			svc, _, now := newAPICacheFixture(t)
			*now = now.Add(12 * time.Hour)
			day := timezone.StartOfDay(*now)
			svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
				page, _ := strconv.Atoi(req.URL.Query().Get("p"))
				total := int64(101)
				if problem == "count capped" {
					total = 100
				}
				if problem == "too many pages" {
					total = newAPIUsageMaxPages*100 + 1
				}
				logs := []newAPIUsageLog{}
				for i := (page - 1) * 100; i < min(page*100, 101); i++ {
					log := newAPIUsageLog{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(100)}
					switch problem {
					case "missing quota":
						log.Quota = nil
					case "wrong user":
						log.UserID = 43
					case "wrong type":
						log.Type = 1
					case "out of window":
						log.CreatedAt = day.Unix() - 1
					}
					logs = append(logs, log)
				}
				if problem == "wrong page" {
					page++
				}
				if problem == "short page" {
					logs = []newAPIUsageLog{}
				}
				if problem == "total changed" && page > 1 {
					total++
				}
				return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, total, logs)), nil
			})
			totals, code := svc.scanNewAPIAccountLogs(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now)
			require.Nil(t, totals)
			require.NotEmpty(t, code)
		})
	}
	svc, target, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day := timezone.StartOfDay(*now)
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("start_timestamp") != strconv.FormatInt(day.Unix(), 10) {
			return newAPICacheResponse(req, 429, `secret must not be exposed`), nil
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 1, []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(123)}})), nil
	})
	snapshot := &UpstreamBalanceSnapshot{}
	code := svc.fetchNewAPIAccountUsage(context.Background(), "https://example.com/prefix", "console-key", target, 13, snapshot, day)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Equal(t, 123.0, *snapshot.DayUsed)
	require.Nil(t, snapshot.Last30DaysUsed)
}

func TestNewAPIAccountUsageRefreshReplacesOverlapWithoutDoubleCounting(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	firstUntil := *now
	day := timezone.StartOfDay(*now)
	var starts []int64
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		starts = append(starts, from)
		logs := []newAPIUsageLog{{42, 13, firstUntil.Add(-time.Hour).Unix(), 2, financeFloat(100)}, {42, 13, firstUntil.Add(-time.Second).Unix(), 2, financeFloat(20)}}
		if len(starts) == 2 {
			logs = []newAPIUsageLog{{42, 13, firstUntil.Add(-time.Second).Unix(), 2, financeFloat(25)}, {42, 13, now.Add(-time.Second).Unix(), 2, financeFloat(30)}}
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, int64(len(logs)), logs)), nil
	})
	first, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 120.0, first[13])
	*now = now.Add(2 * time.Minute)
	second, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 155.0, second[13], "replace the old overlap, include late posted charge and new usage once")
	require.Equal(t, []int64{day.Unix(), firstUntil.Add(-30 * time.Second).Unix()}, starts)
	require.Equal(t, 120.0, first[13], "cached results returned to prior callers are immutable")
}

func TestNewAPIAccountUsageConcurrentKeysShareOnlyMatchingCredentials(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day := timezone.StartOfDay(*now)
	var calls atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 0, []newAPIUsageLog{})), nil
	})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
			require.Empty(t, code)
		}()
	}
	<-entered
	close(release)
	wg.Wait()
	require.Equal(t, int64(1), calls.Load())
	_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "changed-console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, int64(2), calls.Load())
	for identity := range svc.newAPICache.usage {
		require.NotContains(t, identity, "console-key")
	}
}

func TestNewAPIUsageCacheAtCapacityStillRefreshesExistingWindow(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day := timezone.StartOfDay(*now)
	var calls int
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 0, []newAPIUsageLog{})), nil
	})
	_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	for i := 1; i < newAPIUsageCacheLimit; i++ {
		ready := make(chan struct{})
		close(ready)
		svc.newAPICache.usage[fmt.Sprint(i)] = &newAPIUsageCacheEntry{ready: ready, retainUntil: now.Add(time.Hour), expiresAt: now.Add(time.Hour)}
	}
	*now = now.Add(2 * time.Minute)
	_, _, code = svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 2, calls)
	require.Len(t, svc.newAPICache.usage, newAPIUsageCacheLimit)
}

func TestNewAPIUsageFailureRetainsBaselineAndFullCalibrationRepairsOlderLogs(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	firstUntil := *now
	day := timezone.StartOfDay(*now)
	starts := []int64{}
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		starts = append(starts, from)
		if len(starts) == 2 {
			return newAPICacheResponse(req, 429, "secret"), nil
		}
		log := newAPIUsageLog{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(100)}
		if len(starts) == 3 {
			log.CreatedAt, log.Quota = firstUntil.Add(time.Minute).Unix(), financeFloat(30)
		}
		if len(starts) == 4 {
			log.Quota = financeFloat(999)
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 1, []newAPIUsageLog{log})), nil
	})
	_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	*now = now.Add(2 * time.Minute)
	failed, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.NotEmpty(t, code)
	require.Nil(t, failed, "a failed attempt is not a newly synchronized amount")
	*now = now.Add(3 * time.Minute)
	got, synced, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 130.0, got[13], "a failed delta must not destroy the preceding verified baseline")
	require.True(t, synced.Equal(firstUntil.Add(2*time.Minute)))
	*now = now.Add(16 * time.Minute)
	got, _, code = svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 999.0, got[13], "periodic full scan repairs late changes outside the overlap")
	require.Equal(t, []int64{day.Unix(), firstUntil.Add(-30 * time.Second).Unix(), firstUntil.Add(-30 * time.Second).Unix(), day.Unix()}, starts)
}

func TestNewAPILogBudgetIsSharedBySiteAndKeepsRoomForToday(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	for range 80 {
		require.True(t, svc.allowNewAPILogRead("https://example.com/a", true))
	}
	require.False(t, svc.allowNewAPILogRead("https://EXAMPLE.com:443/b", true))
	for range 40 {
		require.True(t, svc.allowNewAPILogRead("https://example.com/b", false))
	}
	require.False(t, svc.allowNewAPILogRead("https://example.com/a", false))
	require.True(t, svc.allowNewAPILogRead("https://different.example.com", false))
	*now = now.Add(3 * time.Minute)
	require.True(t, svc.allowNewAPILogRead("https://example.com/a", true))
}

func TestNewAPIUsageMidnightEmptyWindowDoesNotBecomePermanentPendingScan(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	day := timezone.StartOfDay(*now)
	*now = day
	_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Empty(t, svc.newAPICache.usage)
	*now = now.Add(time.Second)
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 0, []newAPIUsageLog{})), nil
	})
	got, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Empty(t, got)
}
