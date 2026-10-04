package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

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
		LedgerOwners: []UpstreamProfitLedgerOwner{{TargetID: targetID, SupplierID: &supplier, LastRecordedAt: synced.Add(-time.Second)}},
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
	supplier, otherSupplier := int64(7), int64(8)
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
			LedgerOwners: []UpstreamProfitLedgerOwner{{TargetID: 11, SupplierID: &supplier, LastRecordedAt: fresh.Add(-time.Second)}},
		}
	}
	tests := []struct {
		name   string
		change func(*UpstreamProfitSources)
	}{
		{"stale snapshot", func(s *UpstreamProfitSources) { s.Balances[11].SyncedAt = &stale }},
		{"failed refresh", func(s *UpstreamProfitSources) { s.Balances[11].Status = "error" }},
		{"missing day charge", func(s *UpstreamProfitSources) { s.Balances[11].DayUsed = nil }},
		{"wrong day", func(s *UpstreamProfitSources) { s.Balances[11].DayStart = &stale }},
		{"revenue booked after upstream sync", func(s *UpstreamProfitSources) { s.LedgerOwners[0].LastRecordedAt = now }},
		{"different currency", func(s *UpstreamProfitSources) { s.Balances[11].Currency = "CNY" }},
		{"changed credentials", func(s *UpstreamProfitSources) { s.Targets[11].ProfitIdentitySince = now.Add(-time.Hour) }},
		{"created today", func(s *UpstreamProfitSources) { s.Targets[11].ProfitIdentitySince = now.Add(-time.Minute) }},
		{"supplier moved", func(s *UpstreamProfitSources) { s.Targets[11].SupplierID = &otherSupplier }},
		{"orphan ledger", func(s *UpstreamProfitSources) {
			s.LedgerOwners = append(s.LedgerOwners, UpstreamProfitLedgerOwner{TargetID: 99, SupplierID: &supplier})
		}},
		{"archived target", func(s *UpstreamProfitSources) { s.Targets[11].ArchivedAt = &fresh }},
		{"duplicate active key", func(s *UpstreamProfitSources) {
			s.Targets[12] = &UpstreamFinanceTarget{ID: 12, Endpoint: "https://example.com", APIKeyFingerprint: "one"}
		}},
		{"duplicate archived key", func(s *UpstreamProfitSources) {
			s.Targets[12] = &UpstreamFinanceTarget{ID: 12, Endpoint: "https://example.com", APIKeyFingerprint: "one", ArchivedAt: &fresh}
		}},
		{"archived monitor-only key", func(s *UpstreamProfitSources) {
			s.Targets[12] = &UpstreamFinanceTarget{ID: 12, SupplierID: &supplier, Endpoint: "https://other.example.com", APIKeyFingerprint: "other", ArchivedAt: &fresh}
		}},
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
		require.Nil(t, summary.Profit)
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
