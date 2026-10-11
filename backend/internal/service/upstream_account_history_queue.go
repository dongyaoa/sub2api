package service

import (
	"context"
	"log/slog"
	"time"
)

type UpstreamAccountHistoryRepository interface {
	BackfillAccountHistoryBatch(context.Context) (bool, error)
	PendingAccountHistory(context.Context, time.Time, time.Time) ([]UpstreamAccountHistoryWindow, error)
}

type UpstreamAccountHistoryWindow struct {
	TargetID int64
	From     time.Time
	Until    time.Time  // Exclusive upper accounting boundary.
	CursorAt *time.Time // Inclusive: more rows can share the last cursor's timestamp.
}

func (s *UpstreamCenterService) accountHistoryLoop(repo UpstreamAccountHistoryRepository) {
	defer s.wg.Done()
	for s.ctx.Err() == nil {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		worked, err := repo.BackfillAccountHistoryBatch(ctx)
		cancel()
		delay := 5 * time.Second
		if err != nil && s.ctx.Err() == nil {
			slog.Warn("upstream account history batch will retry", "error", err)
		} else if worked {
			delay = 100 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
