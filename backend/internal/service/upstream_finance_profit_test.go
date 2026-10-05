package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestUpstreamDailyProfitKeepsLastSuccessfulAmountDuringRefresh(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, timezone.Location())
	start, end := timezone.StartOfDay(now), timezone.StartOfDay(now).AddDate(0, 0, 1)
	supplier := int64(7)
	synced := now.Add(-10 * time.Minute)
	cost := 1.0
	sources := &UpstreamProfitSources{
		Targets:  map[int64]*UpstreamFinanceTarget{11: {ID: 11, SupplierID: &supplier, Endpoint: "https://example.com", APIKeyFingerprint: "key", ProfitIdentitySince: now.Add(-time.Hour)}},
		Balances: map[int64]*UpstreamBalanceSnapshot{11: {Status: "error", Currency: "USD", DayUsed: &cost, DayStart: &start, DayEnd: &end, SyncedAt: &synced}},
	}
	summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamDailyProfit(summary, UpstreamFinanceQuery{From: start, To: end}, sources, now)
	require.Equal(t, cost, *summary.RemoteUsed)
	require.Equal(t, 1.0, *summary.Profit)
	require.Equal(t, synced, *summary.RemoteSyncedAt)
	require.True(t, summary.RemoteStale)
	// A successful wallet response can omit best-effort usage fields. The
	// repository supplies the last metric observation with its own timestamp.
	sources.Balances[11].Status = "ok"
	sources.Balances[11].SyncedAt = &now
	sources.Balances[11].LastAttemptAt = &now
	sources.Balances[11].DaySyncedAt = &synced
	partial := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamDailyProfit(partial, UpstreamFinanceQuery{From: start, To: end}, sources, now)
	require.Equal(t, cost, *partial.RemoteUsed)
	require.Equal(t, synced, *partial.RemoteSyncedAt)
	require.True(t, partial.RemoteStale)
	// The stale copy cannot become today's cost after local midnight.
	tomorrow := now.AddDate(0, 0, 1)
	next := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamDailyProfit(next, UpstreamFinanceQuery{From: end, To: end.AddDate(0, 0, 1)}, sources, tomorrow)
	require.Nil(t, next.Profit)
}

func TestUpstreamPeriodProfitUses30DayChargeAndDeduplicatesKeys(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, timezone.Location())
	start, end := timezone.StartOfDay(now).AddDate(0, 0, -29), timezone.StartOfDay(now).AddDate(0, 0, 1)
	supplier := int64(7)
	synced := now.Add(-time.Minute)
	sources := &UpstreamProfitSources{
		Targets: map[int64]*UpstreamFinanceTarget{
			11: {ID: 11, SupplierID: &supplier, Endpoint: "https://example.com", APIKeyFingerprint: "key"},
			12: {ID: 12, SupplierID: &supplier, Endpoint: "https://example.com/v1", APIKeyFingerprint: "key"},
		},
		Balances: map[int64]*UpstreamBalanceSnapshot{
			11: {Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(1), TotalUsed: financeFloat(999), PeriodStart: &start, PeriodEnd: &end, SyncedAt: &synced},
			12: {Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(1.5), TotalUsed: financeFloat(999), PeriodStart: &start, PeriodEnd: &end, SyncedAt: &now},
		},
	}
	q := UpstreamFinanceQuery{From: start, To: end}
	summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamPeriodProfit(summary, q, sources, now, true)
	require.Equal(t, 1.5, *summary.RemoteUsed, "the same upstream key is charged once using its latest snapshot")
	require.Equal(t, 0.5, *summary.Profit)
	// Archived keys do not block the remaining active scope in either period.
	yesterday := timezone.StartOfDay(now).Add(-time.Hour)
	sources.Targets[13] = &UpstreamFinanceTarget{ID: 13, SupplierID: &supplier, Endpoint: "https://archived.example.com", APIKeyFingerprint: "archived", ArchivedAt: &yesterday}
	dayStart := timezone.StartOfDay(now)
	sources.Balances[11].DayUsed, sources.Balances[11].DayStart, sources.Balances[11].DayEnd = financeFloat(1), &dayStart, &end
	daily := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamDailyProfit(daily, UpstreamFinanceQuery{From: dayStart, To: end}, sources, now)
	require.Equal(t, 1.0, *daily.Profit)
	archived := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamPeriodProfit(archived, q, sources, now, true)
	require.Equal(t, 0.5, *archived.Profit)
	require.Equal(t, 1, archived.ArchivedKeyCount)
	delete(sources.Targets, 13)
	// Missing history must not fall back to the all-time charge or to zero.
	sources.Balances[11].Last30DaysUsed = nil
	sources.Balances[12].Last30DaysUsed = nil
	missing := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
	applyUpstreamPeriodProfit(missing, q, sources, now, true)
	require.Nil(t, missing.RemoteUsed)
	require.Nil(t, missing.Profit)
}

func TestUpstreamPeriodProfitRetainsKnownKeyCostsAndConvertsEachSupplier(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, timezone.Location())
	start, end := timezone.StartOfDay(now).AddDate(0, 0, -29), timezone.StartOfDay(now).AddDate(0, 0, 1)
	one, two := int64(1), int64(2)
	sources := &UpstreamProfitSources{
		Targets: map[int64]*UpstreamFinanceTarget{
			1: {ID: 1, SupplierID: &one, Endpoint: "https://example.com", APIKeyFingerprint: "one", RechargeRatio: financeFloat(10)},
			2: {ID: 2, SupplierID: &one, Endpoint: "https://example.com", APIKeyFingerprint: "two", RechargeRatio: financeFloat(10)},
			3: {ID: 3, SupplierID: &two, Endpoint: "https://example.org", APIKeyFingerprint: "three", RechargeRatio: financeFloat(2)},
		},
		Balances: map[int64]*UpstreamBalanceSnapshot{
			1: {Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(10), PeriodStart: &start, PeriodEnd: &end, PeriodSyncedAt: &now},
			3: {Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(6), PeriodStart: &start, PeriodEnd: &end, PeriodSyncedAt: &now},
		},
	}
	q := UpstreamFinanceQuery{From: start, To: end}
	partial := &UpstreamFinanceSummary{Revenue: 8}
	applyUpstreamPeriodProfit(partial, q, sources, now, true)
	require.Equal(t, 4.0, *partial.RemoteUsed)
	require.Equal(t, 16.0, *partial.RemoteRawUsed)
	require.True(t, partial.CostPartial)
	require.Equal(t, 2, partial.KnownKeyCount)
	require.Equal(t, 1, partial.MissingKeyCount)
	require.True(t, partial.ConversionApplied)
	require.Nil(t, partial.Profit)
	// Deleting the missing key removes its revenue in the repository and must
	// no longer make the remaining supplier/key statistics unavailable.
	sources.Targets[2].ArchivedAt = &now
	remaining := &UpstreamFinanceSummary{Revenue: 7}
	applyUpstreamPeriodProfit(remaining, q, sources, now, true)
	require.False(t, remaining.CostPartial)
	require.Equal(t, 4.0, *remaining.RemoteUsed)
	require.Equal(t, 3.0, *remaining.Profit)
	require.Equal(t, 1, remaining.ArchivedKeyCount)
	// Two distinct keys under the same supplier are summed before displaying.
	sources.Targets[2].ArchivedAt = nil
	sources.Balances[2] = &UpstreamBalanceSnapshot{Status: "ok", Currency: "USD", Last30DaysUsed: financeFloat(20), PeriodStart: &start, PeriodEnd: &end, PeriodSyncedAt: &now}
	supplier := &UpstreamFinanceSummary{Revenue: 5}
	q.SupplierID = &one
	applyUpstreamPeriodProfit(supplier, q, sources, now, true)
	require.Equal(t, 30.0, *supplier.RemoteRawUsed)
	require.Equal(t, 3.0, *supplier.RemoteUsed)
	require.Equal(t, 2.0, *supplier.Profit)
	require.Equal(t, 2, supplier.KnownKeyCount)
	sources.Targets[1].RechargeRatio, sources.Targets[2].RechargeRatio = financeFloat(1), financeFloat(1)
	unitRatio := &UpstreamFinanceSummary{Revenue: 40}
	applyUpstreamPeriodProfit(unitRatio, q, sources, now, true)
	require.True(t, unitRatio.ConversionApplied, "an explicitly configured 1:1 ratio still enables conversion")
	require.Equal(t, 30.0, *unitRatio.RemoteUsed)
	require.Equal(t, 10.0, *unitRatio.Profit)
}

type financePeriodsTestRepo struct {
	UpstreamFinanceRepository
	queries []UpstreamFinanceQuery
}

func (r *financePeriodsTestRepo) Summary(_ context.Context, q UpstreamFinanceQuery) (*UpstreamFinanceSummary, error) {
	r.queries = append(r.queries, q)
	return &UpstreamFinanceSummary{From: q.From, To: q.To, Revenue: 2, Currency: "USD", CostSource: "unknown"}, nil
}

func TestUpstreamFinancePeriodSummariesIncludeTodayWithoutDetails(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, timezone.Location())
	repo := &financePeriodsTestRepo{}
	svc := NewUpstreamFinanceService(repo, nil, nil, nil, nil)
	svc.now = func() time.Time { return now }
	result, err := svc.PeriodSummaries(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, repo.queries, 2)
	require.Equal(t, timezone.StartOfDay(now), result.Today.From)
	require.Equal(t, timezone.StartOfDay(now).AddDate(0, 0, -29), result.Last30Days.From)
	require.Equal(t, timezone.StartOfDay(now).AddDate(0, 0, 1), result.Last30Days.To)
	require.Nil(t, result.Last30Days.Profit)
}

func TestUpstreamDailyProfitUsesReportedCharge(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, timezone.Location())
	dayStart := timezone.StartOfDay(now)
	dayEnd := dayStart.AddDate(0, 0, 1)
	supplier := int64(7)
	targetID := int64(11)
	cost := 1.0
	synced := now.Add(-time.Minute)
	sources := &UpstreamProfitSources{
		Targets: map[int64]*UpstreamFinanceTarget{targetID: {
			ID: targetID, SupplierID: &supplier, Endpoint: "https://example.com/v1",
			APIKeyFingerprint: "one", ProfitIdentitySince: dayStart.Add(-time.Hour),
		}},
		Balances: map[int64]*UpstreamBalanceSnapshot{targetID: {
			Status: "ok", Currency: "USD", DayUsed: &cost,
			DayStart: &dayStart, DayEnd: &dayEnd, SyncedAt: &synced,
		}},
	}
	for _, q := range []UpstreamFinanceQuery{
		{From: dayStart, To: dayEnd},
		{SupplierID: &supplier, From: dayStart, To: dayEnd},
		{TargetID: &targetID, SupplierID: &supplier, From: dayStart, To: dayEnd},
	} {
		summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
		applyUpstreamDailyProfit(summary, q, sources, now)
		require.Equal(t, 1.0, *summary.BusinessCost)
		require.Equal(t, 1.0, *summary.RemoteUsed)
		require.Equal(t, 1.0, *summary.Profit)
		require.Equal(t, synced, *summary.RemoteSyncedAt)
		require.Equal(t, "reported", summary.CostSource)
	}
}

func TestUpstreamDailyProfitRejectsUnreconciledCost(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, timezone.Location())
	dayStart := timezone.StartOfDay(now)
	dayEnd := dayStart.AddDate(0, 0, 1)
	supplier := int64(7)
	cost := 1.0
	fresh := now.Add(-time.Minute)
	stale := now.Add(-4 * time.Minute)
	makeSources := func() *UpstreamProfitSources {
		return &UpstreamProfitSources{
			Targets: map[int64]*UpstreamFinanceTarget{11: {
				ID: 11, SupplierID: &supplier, Endpoint: "https://example.com/v1",
				APIKeyFingerprint: "one", ProfitIdentitySince: dayStart.Add(-time.Hour),
			}},
			Balances: map[int64]*UpstreamBalanceSnapshot{11: {
				Status: "ok", Currency: "USD", DayUsed: &cost,
				DayStart: &dayStart, DayEnd: &dayEnd, SyncedAt: &fresh,
			}},
		}
	}
	tests := []struct {
		name   string
		change func(*UpstreamProfitSources)
	}{
		{"missing day charge", func(s *UpstreamProfitSources) { s.Balances[11].DayUsed = nil }},
		{"wrong day", func(s *UpstreamProfitSources) { s.Balances[11].DayStart = &stale }},
		{"different currency", func(s *UpstreamProfitSources) { s.Balances[11].Currency = "CNY" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sources := makeSources()
			tc.change(sources)
			summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
			applyUpstreamDailyProfit(summary, UpstreamFinanceQuery{SupplierID: &supplier, From: dayStart, To: dayEnd}, sources, now)
			require.Nil(t, summary.BusinessCost)
			require.Nil(t, summary.Profit)
			require.Equal(t, "unknown", summary.CostSource)
		})
	}
	monitorOnly := makeSources()
	monitorOnly.Targets[12] = &UpstreamFinanceTarget{ID: 12, SupplierID: &supplier, Endpoint: "https://other.example.com", APIKeyFingerprint: "other", ArchivedAt: &fresh}
	for _, q := range []UpstreamFinanceQuery{
		{From: dayStart, To: dayEnd},
		{SupplierID: &supplier, From: dayStart, To: dayEnd},
	} {
		summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
		applyUpstreamDailyProfit(summary, q, monitorOnly, now)
		require.Equal(t, 1.0, *summary.Profit)
		require.False(t, summary.CostPartial)
		require.Equal(t, 1, summary.ArchivedKeyCount)
	}
	for _, q := range []UpstreamFinanceQuery{
		{From: dayStart.Add(-time.Hour), To: dayEnd},
		{From: dayStart.AddDate(0, 0, -1), To: dayStart},
	} {
		summary := &UpstreamFinanceSummary{Revenue: 2, CostSource: "unknown"}
		applyUpstreamDailyProfit(summary, q, makeSources(), now)
		require.Nil(t, summary.Profit)
	}
}
