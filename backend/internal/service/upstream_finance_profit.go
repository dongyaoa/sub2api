package service

import (
	"context"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// UpstreamProfitSources contains current ownership and credential-matched
// observations. All active targets participate in duplicate-key detection.
type UpstreamProfitSources struct {
	Targets      map[int64]*UpstreamFinanceTarget
	Balances     map[int64]*UpstreamBalanceSnapshot
	LedgerOwners []UpstreamProfitLedgerOwner
}

type UpstreamProfitLedgerOwner struct {
	TargetID       int64
	SupplierID     *int64
	LastRecordedAt time.Time
	LastUsedAt     time.Time
}

type UpstreamProfitSourcesRepository interface {
	LoadProfitSources(context.Context, time.Time) (*UpstreamProfitSources, error)
}

const upstreamProfitSnapshotMaxAge = 3 * time.Minute

func (s *UpstreamFinanceService) profitSources(ctx context.Context, dayStart time.Time) (*UpstreamProfitSources, error) {
	repo, ok := s.repo.(UpstreamProfitSourcesRepository)
	if !ok {
		return nil, nil
	}
	return repo.LoadProfitSources(ctx, dayStart)
}

func applyUpstreamDailyProfit(summary *UpstreamFinanceSummary, q UpstreamFinanceQuery, sources *UpstreamProfitSources, now time.Time) {
	applyUpstreamPeriodProfit(summary, q, sources, now, false)
}

func applyUpstreamPeriodProfit(summary *UpstreamFinanceSummary, q UpstreamFinanceQuery, sources *UpstreamProfitSources, now time.Time, last30Days bool) {
	if summary == nil || sources == nil {
		return
	}
	dayStart := timezone.StartOfDay(now)
	dayEnd := dayStart.AddDate(0, 0, 1)
	periodStart := dayStart
	if last30Days {
		periodStart = dayStart.AddDate(0, 0, -29)
	}
	if !q.From.Equal(periodStart) || !q.To.Equal(dayEnd) {
		return
	}
	// Report the latest upstream key totals, independently of the local ledger
	// ingestion clock. A newly recorded user request must not erase a valid
	// upstream observation while the next scheduled synchronization is pending.
	for _, owner := range sources.LedgerOwners {
		if !owner.LastUsedAt.IsZero() && owner.LastUsedAt.Before(periodStart) ||
			q.TargetID != nil && owner.TargetID != *q.TargetID ||
			q.SupplierID != nil && (owner.SupplierID == nil || *owner.SupplierID != *q.SupplierID) ||
			q.TargetID == nil && q.SupplierID == nil && owner.SupplierID == nil {
			continue
		}
		target := sources.Targets[owner.TargetID]
		if target == nil || !sameUpstreamSupplier(owner.SupplierID, target.SupplierID) {
			return
		}
	}
	keys := make(map[string][]*UpstreamBalanceSnapshot)
	for id, target := range sources.Targets {
		if target == nil {
			continue
		}
		inScope := (q.TargetID == nil || id == *q.TargetID) &&
			(q.SupplierID == nil && (q.TargetID != nil || target.SupplierID != nil) ||
				q.SupplierID != nil && target.SupplierID != nil && *target.SupplierID == *q.SupplierID)
		// A removed key can have real charges in this window. Without its
		// complete remote period total, do not report a partial sum as profit.
		if inScope && target.ArchivedAt != nil && !target.ArchivedAt.Before(periodStart) {
			return
		}
		if !inScope || target.ArchivedAt != nil {
			continue
		}
		endpoint, err := upstreamUsageURL(target.Endpoint)
		if err != nil || target.APIKeyFingerprint == "" {
			return
		}
		key := endpoint + "\x00" + target.APIKeyFingerprint
		keys[key] = append(keys[key], sources.Balances[id])
	}
	var used float64
	var oldest time.Time
	stale := false
	for _, snapshots := range keys {
		var latest *UpstreamBalanceSnapshot
		var amount *float64
		var syncedAt *time.Time
		for _, balance := range snapshots {
			if balance == nil || balance.Currency != "USD" {
				continue
			}
			value, from, to := balance.DayUsed, balance.DayStart, balance.DayEnd
			observedAt := balance.DaySyncedAt
			if last30Days {
				value, from, to = balance.Last30DaysUsed, balance.PeriodStart, balance.PeriodEnd
				observedAt = balance.PeriodSyncedAt
			}
			if observedAt == nil {
				observedAt = balance.SyncedAt
			}
			if value == nil || from == nil || to == nil || !from.Equal(periodStart) || !to.Equal(dayEnd) ||
				observedAt == nil || observedAt.Before(dayStart) || observedAt.After(now) ||
				*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
				continue
			}
			if latest == nil || observedAt.After(*syncedAt) ||
				(observedAt.Equal(*syncedAt) && balance.Status == "ok") {
				latest, amount, syncedAt = balance, value, observedAt
			}
		}
		if latest == nil {
			return
		}
		used += *amount
		if math.IsNaN(used) || math.IsInf(used, 0) || used >= 1e14 {
			return
		}
		stale = stale || latest.Status != "ok" || now.Sub(*syncedAt) > upstreamProfitSnapshotMaxAge ||
			latest.LastAttemptAt != nil && latest.LastAttemptAt.After(*syncedAt)
		if oldest.IsZero() || syncedAt.Before(oldest) {
			oldest = *syncedAt
		}
	}
	if len(keys) == 0 {
		return
	}
	summary.BusinessCost = &used
	summary.RemoteUsed = &used
	summary.RemoteSyncedAt = &oldest
	summary.RemoteStale = stale
	profit := summary.Revenue - used
	summary.Profit = &profit
	summary.CostSource = "reported"
}
