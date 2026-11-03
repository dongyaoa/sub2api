//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestNewAPIForceSyncBypassesUsageCacheAndPublishesActualDay(t *testing.T) {
	svc, target, _ := newAPIFinanceFixture(t, newAPIFixtures(), true)
	repo := &upstreamScheduledBillingRepo{target: target}
	svc.repo = repo
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	dayStart := timezone.StartOfDay(now)
	dayCalls, quota := 0, 0
	metadata := svc.client.Transport
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/api/log/self") {
			return metadata.RoundTrip(req)
		}
		from, _ := strconv.ParseInt(req.URL.Query().Get("start_timestamp"), 10, 64)
		require.GreaterOrEqual(t, from, dayStart.Unix(), "force sync must never fetch historical New API consumption")
		items, total := "[]", 0
		if from >= dayStart.Unix() {
			dayCalls++
			if quota > 0 {
				total = 1
				items = fmt.Sprintf(`[{"user_id":42,"token_id":13,"type":2,"created_at":%d,"quota":%d}]`, now.Unix()-1, quota)
			}
		}
		return newAPICacheResponse(req, http.StatusOK, fmt.Sprintf(`{"success":true,"data":{"page":1,"page_size":100,"total":%d,"items":%s}}`, total, items)), nil
	})
	first, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, 0.0, *first.DayUsed)
	require.Empty(t, first.UsageSyncStatus)
	quota = 500000
	now = now.Add(10 * time.Second)
	cached, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, 0.0, *cached.DayUsed)
	require.Equal(t, 1, dayCalls)
	forced, err := svc.ForceSyncNewAPIBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "synced", forced.UsageSyncStatus)
	require.Equal(t, 1.0, *forced.DayUsed)
	require.Nil(t, forced.Last30DaysUsed)
	require.Equal(t, 2, dayCalls)
	require.True(t, forced.SyncedAt.Equal(now))
	require.Empty(t, repo.token, "force sync must release the regular lease")
}

func TestNewAPIForceSyncRejectsOtherUpstreamsAndRespectsLease(t *testing.T) {
	svc, target, paths := newAPIFinanceFixture(t, newAPIFixtures(), false)
	repo := &upstreamScheduledBillingRepo{target: target}
	svc.repo = repo
	_, err := svc.ForceSyncNewAPIBalance(context.Background(), target.ID)
	require.ErrorContains(t, err, "only available for New API")
	require.Empty(t, repo.token)
	require.Nil(t, repo.snapshot)
	require.Empty(t, *paths, "unrecognized targets must not make outbound requests")
	target.NewAPIUserID, target.NewAPIAccessTokenEncrypted = 42, "console-secret"
	repo.token = "background-worker"
	_, err = svc.ForceSyncNewAPIBalance(context.Background(), target.ID)
	require.ErrorIs(t, err, ErrUpstreamBalanceBusy)
	require.Equal(t, "background-worker", repo.token)
	require.Empty(t, *paths)
}

func TestNewAPIForceSyncStatusRequiresFreshConsumptionNotWallet(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	start, end := timezone.StartOfDay(now), timezone.StartOfDay(now).AddDate(0, 0, 1)
	old := now.Add(-10 * time.Hour)
	for _, tc := range []struct {
		name, want string
		change     func(*UpstreamBalanceSnapshot)
	}{
		{"complete zero", "synced", func(*UpstreamBalanceSnapshot) {}},
		{"recent cutoff with daily catchup pending", "pending", func(s *UpstreamBalanceSnapshot) { s.Error = "newapi_usage_incomplete" }},
		{"wallet only", "pending", func(s *UpstreamBalanceSnapshot) { s.DayUsed = nil }},
		{"old resumed scan", "pending", func(s *UpstreamBalanceSnapshot) { s.SyncedAt = &old }},
		{"independent old day", "pending", func(s *UpstreamBalanceSnapshot) { s.DaySyncedAt = &old }},
		{"rate limited", "pending", func(s *UpstreamBalanceSnapshot) { s.DayUsed = nil; s.Error = "newapi_rate_limited" }},
		{"authorization failed", "failed", func(s *UpstreamBalanceSnapshot) {
			s.DayUsed = nil
			s.Status = "error"
			s.Error = "newapi_account_auth_failed"
		}},
		{"unknown unit", "failed", func(s *UpstreamBalanceSnapshot) { s.Currency = "QUOTA"; s.Error = "newapi_quota_unit_unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &UpstreamBalanceSnapshot{Status: "ok", Currency: "USD", Balance: financeFloat(100), DayUsed: financeFloat(0), DayStart: &start, DayEnd: &end, SyncedAt: &now}
			tc.change(s)
			require.Equal(t, tc.want, newAPIUsageSyncStatus(s, now))
		})
	}
}
