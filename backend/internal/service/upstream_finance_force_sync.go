package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type newAPIUsageForceKey struct{}

func newAPIUsageForceRequested(ctx context.Context) bool {
	force, _ := ctx.Value(newAPIUsageForceKey{}).(bool)
	return force
}

// ForceSyncNewAPIBalance advances usage immediately, bypassing completed usage
// cache entries. The normal lease, request budgets and resumable scans still apply.
func (s *UpstreamFinanceService) ForceSyncNewAPIBalance(ctx context.Context, targetID int64) (*UpstreamBalanceSnapshot, error) {
	return s.syncBalance(ctx, targetID, true)
}

func (s *UpstreamFinanceService) validateNewAPIForceTarget(ctx context.Context, target *UpstreamFinanceTarget) error {
	if target.NewAPIUserID > 0 && target.NewAPIAccessTokenEncrypted != "" {
		return nil
	}
	previous, err := s.repo.LatestBalance(ctx, target.ID, upstreamBalanceIdentity(target))
	if err != nil {
		return err
	}
	if previous != nil && (previous.CurrencySource == "newapi_status" || previous.Billing != nil && strings.HasPrefix(previous.Billing.Source, "newapi_")) {
		return nil
	}
	return infraerrors.BadRequest("UPSTREAM_FORCE_SYNC_REQUIRES_NEWAPI", "Force usage synchronization is only available for New API upstreams")
}

// Classify the current attempt before the repository merges prior snapshots.
// A fresh wallet or a retained old day amount is not a successful usage refresh.
func newAPIUsageSyncStatus(snapshot *UpstreamBalanceSnapshot, now time.Time) string {
	if snapshot == nil {
		return "failed"
	}
	dayStart := timezone.StartOfDay(now)
	observed := snapshot.DaySyncedAt
	if observed == nil {
		observed = snapshot.SyncedAt
	}
	if snapshot.Status == "ok" && snapshot.Currency == "USD" && newAPIQuota(snapshot.DayUsed, false) &&
		snapshot.DayStart != nil && snapshot.DayStart.Equal(dayStart) &&
		snapshot.DayEnd != nil && snapshot.DayEnd.Equal(dayStart.AddDate(0, 0, 1)) &&
		observed != nil && !observed.Before(dayStart) && !observed.After(now) && now.Sub(*observed) <= upstreamProfitSnapshotMaxAge {
		return "synced"
	}
	if snapshot.Error == "" || snapshot.Error == "newapi_usage_incomplete" || snapshot.Error == "newapi_rate_limited" {
		return "pending"
	}
	return "failed"
}
