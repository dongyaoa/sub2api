package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type UpstreamFinanceOverviewTarget struct {
	ID         int64
	SupplierID *int64
}

type UpstreamFinanceOverviewData struct {
	Summary        *UpstreamFinanceSummary
	Suppliers      map[int64]*UpstreamFinanceSummary
	Targets        map[int64]*UpstreamFinanceSummary
	Balances       map[int64]*UpstreamBalanceSnapshot
	BalanceTargets map[int64]*UpstreamFinanceTarget
}

// Optional capability keeps single-item APIs and repository decorators intact.
type UpstreamFinanceOverviewRepository interface {
	LoadOverviewFinance(context.Context, UpstreamFinanceQuery, []int64, []UpstreamFinanceOverviewTarget) (*UpstreamFinanceOverviewData, error)
}

func (s *UpstreamFinanceService) OverviewFinance(ctx context.Context, suppliers []*UpstreamSupplier, targets []*UpstreamTarget, from, to time.Time) (*UpstreamFinanceOverviewData, error) {
	q, err := normalizeUpstreamFinanceQuery(UpstreamFinanceQuery{From: from, To: to}, s.now())
	if err != nil {
		return nil, err
	}
	if repo, ok := s.repo.(UpstreamFinanceOverviewRepository); ok {
		supplierIDs := make([]int64, 0, len(suppliers))
		for _, supplier := range suppliers {
			supplierIDs = append(supplierIDs, supplier.ID)
		}
		scopes := make([]UpstreamFinanceOverviewTarget, 0, len(targets))
		for _, target := range targets {
			scopes = append(scopes, UpstreamFinanceOverviewTarget{ID: target.ID, SupplierID: target.SupplierID})
		}
		data, err := repo.LoadOverviewFinance(ctx, q, supplierIDs, scopes)
		if err != nil {
			return nil, err
		}
		now := s.now()
		for id, target := range data.BalanceTargets {
			data.Balances[id] = completeUpstreamBalanceSnapshot(data.Balances[id], target, now)
		}
		sources, err := s.profitSources(ctx, timezone.StartOfDay(now))
		if err != nil {
			return nil, err
		}
		applyUpstreamDailyProfit(data.Summary, q, sources, now)
		for id, summary := range data.Suppliers {
			supplierID := id
			applyUpstreamDailyProfit(summary, UpstreamFinanceQuery{SupplierID: &supplierID, From: q.From, To: q.To}, sources, now)
		}
		for id, summary := range data.Targets {
			targetID := id
			applyUpstreamDailyProfit(summary, UpstreamFinanceQuery{TargetID: &targetID, From: q.From, To: q.To}, sources, now)
		}
		return data, nil
	}
	// In-memory stores and decorators can continue using the original interface.
	data := &UpstreamFinanceOverviewData{Suppliers: map[int64]*UpstreamFinanceSummary{}, Targets: map[int64]*UpstreamFinanceSummary{}, Balances: map[int64]*UpstreamBalanceSnapshot{}}
	data.Summary, err = s.Summary(ctx, nil, nil, q.From, q.To)
	if err != nil {
		return nil, err
	}
	for _, supplier := range suppliers {
		data.Suppliers[supplier.ID], err = s.Summary(ctx, &supplier.ID, nil, q.From, q.To)
		if err != nil {
			return nil, err
		}
	}
	for _, target := range targets {
		data.Targets[target.ID], err = s.Summary(ctx, target.SupplierID, &target.ID, q.From, q.To)
		if err != nil {
			return nil, err
		}
		data.Balances[target.ID], err = s.LatestBalance(ctx, target.ID)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func completeUpstreamBalanceSnapshot(snapshot *UpstreamBalanceSnapshot, target *UpstreamFinanceTarget, now time.Time) *UpstreamBalanceSnapshot {
	if snapshot == nil {
		snapshot = &UpstreamBalanceSnapshot{TargetID: target.ID, WalletRef: target.WalletRef, Kind: "unknown", Status: "pending"}
	}
	if snapshot.Billing == nil {
		snapshot.Billing = pendingUpstreamRemoteBilling()
	}
	// Usage fallback can outlive midnight even though the wallet remains valid.
	// Never label a prior day's retained amount as today's key spending.
	today := timezone.StartOfDay(now)
	if snapshot.DayStart != nil && !snapshot.DayStart.Equal(today) ||
		snapshot.DaySyncedAt != nil && snapshot.DaySyncedAt.Before(today) ||
		snapshot.DayStart == nil && (snapshot.SyncedAt == nil || snapshot.SyncedAt.Before(today)) {
		snapshot.TodayUsed = nil
	}
	markUpstreamRemoteBillingStale(snapshot.Billing, now)
	return snapshot
}
