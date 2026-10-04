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
	if summary == nil || sources == nil {
		return
	}
	dayStart := timezone.StartOfDay(now)
	dayEnd := dayStart.AddDate(0, 0, 1)
	if !q.From.Equal(dayStart) || !q.To.Equal(dayEnd) {
		return
	}
	for _, owner := range sources.LedgerOwners {
		if q.TargetID != nil && owner.TargetID != *q.TargetID {
			continue
		}
		if q.SupplierID != nil {
			if owner.SupplierID == nil || *owner.SupplierID != *q.SupplierID {
				continue
			}
		} else if q.TargetID == nil && owner.SupplierID == nil {
			continue
		}
		target := sources.Targets[owner.TargetID]
		if target == nil || target.ArchivedAt != nil ||
			!sameUpstreamSupplier(owner.SupplierID, target.SupplierID) ||
			target.ProfitIdentitySince.After(dayStart) {
			return
		}
		balance := sources.Balances[owner.TargetID]
		if balance == nil || balance.SyncedAt == nil || owner.LastRecordedAt.After(*balance.SyncedAt) {
			return
		}
	}
	keyOwners := make(map[string]int)
	for id, target := range sources.Targets {
		if target == nil {
			continue
		}
		inScope := (q.TargetID == nil || id == *q.TargetID) &&
			(q.SupplierID == nil && (q.TargetID != nil || target.SupplierID != nil) ||
				q.SupplierID != nil && target.SupplierID != nil && *target.SupplierID == *q.SupplierID)
		// A deleted key may have incurred monitor or out-of-band charges even
		// when it has no local business ledger row.
		if target.ArchivedAt != nil && inScope {
			return
		}
		if target.APIKeyFingerprint == "" {
			continue
		}
		endpoint, err := upstreamUsageURL(target.Endpoint)
		if err == nil {
			keyOwners[endpoint+"\x00"+target.APIKeyFingerprint]++
		}
	}
	var used float64
	var oldest time.Time
	matched := 0
	for id, target := range sources.Targets {
		if target == nil || target.ArchivedAt != nil || (q.TargetID != nil && id != *q.TargetID) ||
			(q.SupplierID == nil && q.TargetID == nil && target.SupplierID == nil) ||
			(q.SupplierID != nil && (target.SupplierID == nil || *target.SupplierID != *q.SupplierID)) {
			continue
		}
		matched++
		if target.ProfitIdentitySince.After(dayStart) {
			return
		}
		endpoint, err := upstreamUsageURL(target.Endpoint)
		if err != nil || target.APIKeyFingerprint == "" || keyOwners[endpoint+"\x00"+target.APIKeyFingerprint] != 1 {
			return
		}
		balance := sources.Balances[id]
		if balance == nil || balance.Status != "ok" || balance.DayUsed == nil || balance.DayStart == nil ||
			balance.DayEnd == nil || balance.SyncedAt == nil || balance.Currency != "USD" ||
			!balance.DayStart.Equal(dayStart) || !balance.DayEnd.Equal(dayEnd) ||
			balance.SyncedAt.Before(dayStart) || balance.SyncedAt.After(now) ||
			now.Sub(*balance.SyncedAt) > upstreamProfitSnapshotMaxAge ||
			*balance.DayUsed < 0 || math.IsNaN(*balance.DayUsed) || math.IsInf(*balance.DayUsed, 0) {
			return
		}
		used += *balance.DayUsed
		if math.IsNaN(used) || math.IsInf(used, 0) || used >= 1e14 {
			return
		}
		if oldest.IsZero() || balance.SyncedAt.Before(oldest) {
			oldest = *balance.SyncedAt
		}
	}
	if matched == 0 {
		return
	}
	summary.BusinessCost = &used
	summary.RemoteUsed = &used
	summary.RemoteSyncedAt = &oldest
	profit := summary.Revenue - used
	summary.Profit = &profit
	summary.CostSource = "reported"
}
