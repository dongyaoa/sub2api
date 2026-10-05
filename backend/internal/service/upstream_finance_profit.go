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
	Targets  map[int64]*UpstreamFinanceTarget
	Balances map[int64]*UpstreamBalanceSnapshot
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
	// The repository uses this same active target/supplier scope for revenue.
	// Archived records remain in the ledger but never poison current totals.
	// Missing usage is explicit: retain known costs without claiming full profit.
	type keyObservation struct {
		amounts              []*UpstreamBalanceSnapshot
		ratio                float64
		invalidRatio         bool
		conversionConfigured bool
	}
	keys := make(map[string]*keyObservation)
	missing, archived := 0, 0
	for id, target := range sources.Targets {
		if target == nil {
			continue
		}
		inScope := (q.TargetID == nil || id == *q.TargetID) &&
			(q.SupplierID == nil && (q.TargetID != nil || target.SupplierID != nil) ||
				q.SupplierID != nil && target.SupplierID != nil && *target.SupplierID == *q.SupplierID)
		if !inScope {
			continue
		}
		if target.ArchivedAt != nil {
			archived++
			continue
		}
		endpoint, err := upstreamUsageURL(target.Endpoint)
		if err != nil || target.APIKeyFingerprint == "" {
			missing++
			continue
		}
		key := endpoint + "\x00" + target.APIKeyFingerprint
		ratio := 1.0
		if target.RechargeRatio != nil {
			ratio = *target.RechargeRatio
		}
		observation := keys[key]
		if observation == nil {
			observation = &keyObservation{ratio: ratio}
			keys[key] = observation
		}
		// One remote key cannot be converted using conflicting supplier prices.
		observation.invalidRatio = observation.invalidRatio || ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio != observation.ratio
		observation.conversionConfigured = observation.conversionConfigured || target.RechargeRatio != nil
		observation.amounts = append(observation.amounts, sources.Balances[id])
	}
	var used, rawUsed float64
	var oldest time.Time
	stale, converted := false, false
	known := 0
	for _, observation := range keys {
		if observation.invalidRatio {
			missing++
			continue
		}
		var latest *UpstreamBalanceSnapshot
		var amount *float64
		var syncedAt *time.Time
		for _, balance := range observation.amounts {
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
			missing++
			continue
		}
		nextUsed, nextRaw := used+*amount/observation.ratio, rawUsed+*amount
		if math.IsNaN(nextUsed) || math.IsInf(nextUsed, 0) || nextUsed >= 1e14 || math.IsInf(nextRaw, 0) || nextRaw >= 1e14 {
			missing++
			continue
		}
		used, rawUsed = nextUsed, nextRaw
		known++
		converted = converted || observation.conversionConfigured
		stale = stale || latest.Status != "ok" || now.Sub(*syncedAt) > upstreamProfitSnapshotMaxAge ||
			latest.LastAttemptAt != nil && latest.LastAttemptAt.After(*syncedAt)
		if oldest.IsZero() || syncedAt.Before(oldest) {
			oldest = *syncedAt
		}
	}
	summary.CostPartial = missing > 0
	summary.KnownKeyCount, summary.MissingKeyCount = known, missing
	summary.ArchivedKeyCount = archived
	summary.ConversionApplied = converted
	summary.BusinessCost, summary.RemoteUsed, summary.RemoteRawUsed = nil, nil, nil
	summary.RemoteSyncedAt, summary.Profit = nil, nil
	summary.RemoteStale = stale
	summary.CostSource = "unknown"
	if known == 0 && missing > 0 {
		return
	}
	summary.BusinessCost = &used
	summary.RemoteUsed = &used
	summary.RemoteRawUsed = &rawUsed
	if !oldest.IsZero() {
		summary.RemoteSyncedAt = &oldest
	}
	if !summary.CostPartial {
		profit := summary.Revenue - used
		summary.Profit = &profit
	}
	summary.CostSource = "reported"
}
