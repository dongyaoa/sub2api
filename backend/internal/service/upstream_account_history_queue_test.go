//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestUpstreamHistoryQueueProfitWaitsOnlyForOverlappingActiveHistory(t *testing.T) {
	now := time.Now()
	start := timezone.StartOfDay(now)
	end := start.AddDate(0, 0, 1)
	supplier := int64(1)
	sources := &UpstreamProfitSources{
		Targets:        map[int64]*UpstreamFinanceTarget{1: {ID: 1, SupplierID: &supplier, Provider: "openai", Endpoint: "https://example.com", APIKeyFingerprint: "one"}},
		Balances:       map[int64]*UpstreamBalanceSnapshot{1: {Status: "ok", Currency: "USD", DayUsed: financeFloat(1), DayStart: &start, DayEnd: &end, DaySyncedAt: &now}},
		LastBusinessAt: map[int64]time.Time{1: now},
		PendingHistory: []UpstreamAccountHistoryWindow{{TargetID: 1, From: start.AddDate(0, 0, -29), Until: now}},
	}
	summary := &UpstreamFinanceSummary{Revenue: 2}
	q := UpstreamFinanceQuery{From: start, To: end}
	applyUpstreamDailyProfit(summary, q, sources, now)
	require.Equal(t, 1, summary.HistorySyncPending)
	require.Nil(t, summary.Profit, "incomplete local revenue must never produce a false negative profit")
	olderCursor := start.Add(-time.Microsecond)
	sources.PendingHistory[0].CursorAt = &olderCursor
	applyUpstreamDailyProfit(summary, q, sources, now)
	require.Zero(t, summary.HistorySyncPending)
	require.Equal(t, 1.0, *summary.Profit, "older history can continue without hiding today's complete profit")
	sources.PendingHistory[0].CursorAt = &now
	sources.PendingHistory[0].Until = start
	applyUpstreamDailyProfit(summary, q, sources, now)
	require.Zero(t, summary.HistorySyncPending, "an interval ending at midnight does not overlap today")
	sources.PendingHistory[0].Until = now
	sources.Targets[1].ArchivedAt = &now
	applyUpstreamDailyProfit(summary, q, sources, now)
	require.Zero(t, summary.HistorySyncPending, "archived imports do not hold active totals pending")
}
