package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestUpstreamNewAPITodayOnlyDisablesHistoricalCostWithoutChangingDailyProfit(t *testing.T) {
	now := time.Date(2026, 11, 3, 18, 0, 0, 0, timezone.Location())
	day, end := timezone.StartOfDay(now), timezone.StartOfDay(now).AddDate(0, 0, 1)
	month := day.AddDate(0, 0, -29)
	supplier := int64(3)
	target := &UpstreamFinanceTarget{ID: 1, SupplierID: &supplier, Endpoint: "https://example.com", APIKeyFingerprint: "key"}
	balance := &UpstreamBalanceSnapshot{Status: "ok", Currency: "USD", DayUsed: financeFloat(1), DayStart: &day, DayEnd: &end, DaySyncedAt: &now, Last30DaysUsed: financeFloat(99), PeriodStart: &month, PeriodEnd: &end, PeriodSyncedAt: &now}
	sources := &UpstreamProfitSources{Targets: map[int64]*UpstreamFinanceTarget{1: target}, Balances: map[int64]*UpstreamBalanceSnapshot{1: balance}, LastBusinessAt: map[int64]time.Time{1: now}}
	for _, detection := range []string{"authorization", "currency", "billing"} {
		t.Run(detection, func(t *testing.T) {
			target.NewAPIUserID, target.NewAPIAccessTokenEncrypted = 0, ""
			balance.CurrencySource, balance.Billing = "", nil
			switch detection {
			case "authorization":
				target.NewAPIUserID, target.NewAPIAccessTokenEncrypted = 42, "encrypted"
			case "currency":
				balance.CurrencySource = "newapi_status"
			case "billing":
				balance.Billing = &UpstreamRemoteBillingSnapshot{Source: "newapi_token"}
			}
			period := &UpstreamFinanceSummary{Revenue: 120}
			applyUpstreamPeriodProfit(period, UpstreamFinanceQuery{From: month, To: end}, sources, now, true)
			require.True(t, period.PeriodCostDisabled)
			require.Equal(t, 120.0, period.Revenue)
			require.Nil(t, period.RemoteUsed, "old month values must not survive stopping historical synchronization")
			require.Nil(t, period.Profit)
			require.False(t, period.CostPartial, "disabled history must not claim it is still synchronizing")
			daily := &UpstreamFinanceSummary{Revenue: 2}
			applyUpstreamDailyProfit(daily, UpstreamFinanceQuery{From: day, To: end}, sources, now)
			require.False(t, daily.PeriodCostDisabled)
			require.Equal(t, 1.0, *daily.Profit)
		})
	}
	// An unused New API key does not disable historical totals for used Sub2API keys.
	other := &UpstreamFinanceTarget{ID: 2, SupplierID: &supplier, Endpoint: "https://sub2.example.com", APIKeyFingerprint: "other"}
	sources.Targets[2] = other
	sources.Balances[2] = &UpstreamBalanceSnapshot{Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(4), PeriodStart: &month, PeriodEnd: &end, PeriodSyncedAt: &now}
	sources.LastBusinessAt = map[int64]time.Time{2: now}
	period := &UpstreamFinanceSummary{Revenue: 6}
	applyUpstreamPeriodProfit(period, UpstreamFinanceQuery{From: month, To: end}, sources, now, true)
	require.False(t, period.PeriodCostDisabled)
	require.Equal(t, 2.0, *period.Profit)
	sources.LastBusinessAt[1] = now
	applyUpstreamPeriodProfit(period, UpstreamFinanceQuery{From: month, To: end}, sources, now, true)
	require.True(t, period.PeriodCostDisabled)
	require.Nil(t, period.RemoteUsed)
	require.Nil(t, period.Profit)
}

func TestUpstreamNewAPIBalanceDoesNotReturnOldPeriodSnapshot(t *testing.T) {
	now := time.Now()
	start, end := timezone.StartOfDay(now).AddDate(0, 0, -29), timezone.StartOfDay(now).AddDate(0, 0, 1)
	for _, newAPI := range []bool{false, true} {
		target := &UpstreamFinanceTarget{ID: 1}
		if newAPI {
			target.NewAPIUserID, target.NewAPIAccessTokenEncrypted = 42, "encrypted"
		}
		balance := &UpstreamBalanceSnapshot{Status: "ok", Last30DaysUsed: financeFloat(99), PeriodStart: &start, PeriodEnd: &end, PeriodSyncedAt: &now}
		result := completeUpstreamBalanceSnapshot(balance, target, now)
		if newAPI {
			require.Nil(t, result.Last30DaysUsed)
			require.Nil(t, result.PeriodStart)
			require.Nil(t, result.PeriodEnd)
			require.Nil(t, result.PeriodSyncedAt)
		} else {
			require.Equal(t, 99.0, *result.Last30DaysUsed)
		}
	}
}
