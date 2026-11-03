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
	require.Nil(t, snapshot.Last30DaysUsed)
	require.Equal(t, 18.0, *snapshot.TotalUsed, "cumulative Key consumption is not today's cost")
	require.True(t, day.Equal(*snapshot.DayStart))
	require.Nil(t, snapshot.PeriodStart)
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
		{"known complete lifetime", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}}, 10, true, false, ""},
		{"zero lifetime", []newAPIUsageLog{}, 0, true, false, ""},
		{"deleted records", []newAPIUsageLog{}, 10, false, false, "newapi_usage_account_auth_required"},
		{"day boundary same second", []newAPIUsageLog{{42, 13, day.Unix(), 2, financeFloat(10)}}, 20, false, false, "newapi_usage_account_auth_required"},
		{"day covered without historical coverage", []newAPIUsageLog{{42, 13, day.Add(time.Hour).Unix(), 2, financeFloat(10)}, {42, 13, day.Add(-time.Second).Unix(), 2, financeFloat(20)}}, 40, true, false, ""},
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
		from, _ := strconv.ParseInt(q.Get("start_timestamp"), 10, 64)
		require.GreaterOrEqual(t, from, day.Unix(), "New API must never fetch prior days")
		if from > day.Unix() {
			total = 0
			require.Equal(t, strconv.FormatInt(now.Unix()-1, 10), q.Get("end_timestamp"), "catch up after completing the old fixed window")
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
	require.Nil(t, snapshot.Last30DaysUsed)
	require.True(t, snapshot.SyncedAt.Equal(*now), "finish the resumed range, then catch up to the new cutoff in the same round")
	require.LessOrEqual(t, requests.Load(), int64(210), "continue checked pages instead of restarting all 20,001 records")
	calls := requests.Load()
	other := &UpstreamBalanceSnapshot{}
	code = svc.fetchNewAPIAccountUsage(context.Background(), "https://example.com/prefix", "console-key", target, 14, other, day)
	require.Empty(t, code)
	require.Equal(t, 10000000.0, *other.DayUsed)
	require.Nil(t, other.Last30DaysUsed)
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
	require.Empty(t, code, "today is complete and no prior-day request is necessary")
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

func TestNewAPIUsageCountCapSplitsRangesAndResumesWithoutDoubleCounting(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day, until := timezone.StartOfDay(*now), *now
	middle := day.Add(until.Sub(day) / 2).Unix()
	var failed atomic.Bool
	var calls atomic.Int64
	var expected, expectedTail float64
	logs := make([]newAPIUsageLog, 350)
	for i := range logs {
		created := day.Unix() + int64(i)*(until.Unix()-day.Unix()-1)/int64(len(logs)-1)
		quota := float64(i + 1)
		logs[i] = newAPIUsageLog{42, int64(13 + i%2), created, 2, &quota}
		if logs[i].TokenID == 13 {
			expected += quota
			if created >= until.Add(-30*time.Second).Unix() {
				expectedTail += quota
			}
		}
	}
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		end, _ := strconv.ParseInt(req.URL.Query().Get("end_timestamp"), 10, 64)
		page, _ := strconv.Atoi(req.URL.Query().Get("p"))
		require.Less(t, end, until.Unix(), "split ranges retain the original frozen upper bound")
		if from >= middle && failed.CompareAndSwap(false, true) {
			return newAPICacheResponse(req, 429, "limited"), nil
		}
		matching := []newAPIUsageLog{}
		for i := len(logs) - 1; i >= 0; i-- {
			if logs[i].CreatedAt >= from && logs[i].CreatedAt <= end {
				matching = append(matching, logs[i])
			}
		}
		start := min((page-1)*100, len(matching))
		finish := min(page*100, len(matching))
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, int64(min(100, len(matching))), matching[start:finish])), nil
	})
	got, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, until, false)
	require.NotEmpty(t, code)
	require.Nil(t, got, "completed left ranges are not published while the right range remains incomplete")
	*now = now.Add(3 * time.Minute)
	got, synced, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, expected, got[13])
	require.True(t, synced.Equal(until))
	require.Less(t, calls.Load(), int64(25), "capped counts must converge by narrowing the query, not repeatedly restarting it")
	for _, entry := range svc.newAPICache.usage {
		require.Equal(t, expectedTail, entry.baseline.tail[13], "split windows share the root overlap boundary")
	}
}

func TestNewAPIUsageOverDailyPageLimitMakesBoundedProgress(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day, until := timezone.StartOfDay(*now), *now
	const count = newAPIUsageMaxPages*newAPIUsagePageSize + 1
	var calls atomic.Int64
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		end, _ := strconv.ParseInt(req.URL.Query().Get("end_timestamp"), 10, 64)
		page, _ := strconv.Atoi(req.URL.Query().Get("p"))
		matching := []newAPIUsageLog{}
		for i := count - 1; i >= 0; i-- {
			created := day.Unix() + int64(i)*(until.Unix()-day.Unix()-1)/(count-1)
			if created >= from && created <= end {
				matching = append(matching, newAPIUsageLog{42, int64(13 + i%2), created, 2, financeFloat(1)})
			}
		}
		start := min((page-1)*100, len(matching))
		finish := min(page*100, len(matching))
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, int64(len(matching)), matching[start:finish])), nil
	})
	var got map[int64]float64
	var synced time.Time
	var code string
	for attempt := 0; attempt < 8; attempt++ {
		before := calls.Load()
		got, synced, code = svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
		require.LessOrEqual(t, calls.Load()-before, int64(120), "splitting must not bypass the site's request budget")
		if code == "" {
			break
		}
		require.Nil(t, got)
		*now = now.Add(3 * time.Minute)
	}
	require.Empty(t, code, "large daily windows must eventually finish across bounded synchronization rounds")
	require.Equal(t, float64(count/2+1), got[13])
	require.Equal(t, float64(count/2), got[14])
	require.True(t, synced.Equal(until), "completion time is not the data coverage time")
}

func TestNewAPIUsageCannotSplitAnOverfullSingleSecond(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	from := *now
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, newAPIUsageMaxPages*100+1, []newAPIUsageLog{})), nil
	})
	got, code := svc.scanNewAPIAccountLogs(context.Background(), "https://example.com/prefix", "console-key", 42, from, from.Add(time.Second))
	require.Nil(t, got)
	require.Equal(t, "newapi_usage_incomplete", code)
}

func TestNewAPIForceUsageBypassesOnlyCompletedCache(t *testing.T) {
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
	*now = now.Add(time.Second)
	_, _, code = svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 1, calls)
	ctx := context.WithValue(context.Background(), newAPIUsageForceKey{}, true)
	_, _, code = svc.newAPIAccountLogTotals(ctx, "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Empty(t, code)
	require.Equal(t, 2, calls)
	for svc.allowNewAPILogRead("https://example.com/prefix", false) {
	}
	_, _, code = svc.newAPIAccountLogTotals(ctx, "https://example.com/prefix", "console-key", 42, day, *now, false)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Equal(t, 2, calls, "manual refresh cannot bypass upstream throttling")
}

func TestNewAPIUsageExpiredInFlightEntryRemainsSingleFlight(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	*now = now.Add(12 * time.Hour)
	day, until := timezone.StartOfDay(*now), *now
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 0, []newAPIUsageLog{})), nil
	})
	done := make(chan string, 1)
	go func() {
		_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, until, false)
		done <- code
	}()
	<-entered
	svc.newAPICache.mu.Lock()
	for _, entry := range svc.newAPICache.usage {
		entry.expiresAt, entry.retainUntil = now.Add(-time.Hour), now.Add(-time.Hour)
	}
	svc.newAPICache.mu.Unlock()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), newAPIUsageForceKey{}, true))
	cancel()
	_, _, code := svc.newAPIAccountLogTotals(ctx, "https://example.com/prefix", "console-key", 42, day, until, false)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Equal(t, int64(1), calls.Load())
	close(release)
	require.Empty(t, <-done)
}

func TestNewAPIUsageCountCapBetweenPageBoundaries(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	from, until := *now, now.Add(250*time.Second)
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		begin, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		end, _ := strconv.ParseInt(req.URL.Query().Get("end_timestamp"), 10, 64)
		page, _ := strconv.Atoi(req.URL.Query().Get("p"))
		matching := []newAPIUsageLog{}
		for at := end; at >= begin; at-- {
			matching = append(matching, newAPIUsageLog{42, 13, at, 2, financeFloat(1)})
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, int64(min(125, len(matching))), matching[min((page-1)*100, len(matching)):min(page*100, len(matching))])), nil
	})
	got, code := svc.scanNewAPIAccountLogs(context.Background(), "https://example.com/prefix", "console-key", 42, from, until)
	require.Empty(t, code)
	require.Equal(t, 250.0, got.used[13])
	require.Equal(t, 30.0, got.tail[13], "inclusive upstream end timestamps must neither overlap nor omit a boundary second")
}

func TestNewAPIUsageRevalidatesFirstPageBeforePublishing(t *testing.T) {
	svc, _, now := newAPICacheFixture(t)
	from, until := *now, now.Add(time.Hour)
	var firstPages int
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		page, _ := strconv.Atoi(req.URL.Query().Get("p"))
		if page == 1 {
			firstPages++
		}
		logs := []newAPIUsageLog{}
		for i := (page - 1) * 100; i < min(page*100, 101); i++ {
			quota := float64(1)
			if page == 1 && firstPages >= 2 && i == 0 {
				quota = 2
			}
			logs = append(logs, newAPIUsageLog{42, 13, from.Unix(), 2, &quota})
		}
		return newAPICacheResponse(req, 200, newAPIUsagePageBody(page, 100, 101, logs)), nil
	})
	scan := &newAPIUsageScan{from: from, until: until, nextPage: 1}
	got, code := svc.resumeNewAPIAccountLogs(context.Background(), "https://example.com/prefix", "console-key", 42, scan)
	require.Nil(t, got)
	require.Equal(t, "newapi_usage_incomplete", code)
	require.Equal(t, 1, scan.nextPage)
	got, code = svc.resumeNewAPIAccountLogs(context.Background(), "https://example.com/prefix", "console-key", 42, scan)
	require.Empty(t, code)
	require.Equal(t, 102.0, got.used[13])
}

func TestNewAPIDailyCacheAndIncompleteRetryRefreshWithinThirtySeconds(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		t.Run(fmt.Sprint("initial_failure=", failFirst), func(t *testing.T) {
			svc, _, now := newAPICacheFixture(t)
			*now = now.Add(12 * time.Hour)
			day := timezone.StartOfDay(*now)
			calls := 0
			svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				if failFirst && calls == 1 {
					return newAPICacheResponse(req, 503, "unavailable"), nil
				}
				from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
				require.GreaterOrEqual(t, from, day.Unix())
				return newAPICacheResponse(req, 200, newAPIUsagePageBody(1, 100, 0, []newAPIUsageLog{})), nil
			})
			_, _, firstCode := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
			if failFirst {
				require.NotEmpty(t, firstCode)
			} else {
				require.Empty(t, firstCode)
			}
			*now = now.Add(29 * time.Second)
			_, _, code := svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
			require.Equal(t, firstCode, code)
			require.Equal(t, 1, calls)
			*now = now.Add(time.Second)
			_, _, code = svc.newAPIAccountLogTotals(context.Background(), "https://example.com/prefix", "console-key", 42, day, *now, false)
			require.Empty(t, code)
			require.Equal(t, 2, calls)
		})
	}
}
