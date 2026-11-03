package service

import (
	"context"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type sub2APIHistoryAttempt struct {
	next      time.Time
	errorCode string
}

// This stores only scheduling metadata. Old period amounts must never be
// copied into a new daily snapshot, which would give them a false fresh date.
type sub2APIHistorySchedule struct {
	mu       sync.Mutex
	attempts map[string]sub2APIHistoryAttempt
}

func (schedule *sub2APIHistorySchedule) claim(identity string, now time.Time) (bool, string) {
	schedule.mu.Lock()
	defer schedule.mu.Unlock()
	entry, exists := schedule.attempts[identity]
	if exists && now.Before(entry.next) {
		return false, entry.errorCode
	}
	if schedule.attempts == nil {
		schedule.attempts = make(map[string]sub2APIHistoryAttempt)
	}
	if len(schedule.attempts) >= newAPIFinanceCacheLimit {
		for key, item := range schedule.attempts {
			if !now.Before(item.next) {
				delete(schedule.attempts, key)
			}
		}
		if len(schedule.attempts) >= newAPIFinanceCacheLimit && !exists {
			return false, "sub2api_history_sync_failed"
		}
	}
	// Repeated history failures should not put a heavy 30-day query in every
	// wallet refresh. Today still refreshes on its independent normal cadence.
	schedule.attempts[identity] = sub2APIHistoryAttempt{next: now.Add(2 * time.Minute), errorCode: entry.errorCode}
	return true, ""
}

func (schedule *sub2APIHistorySchedule) finish(identity string, code string) {
	schedule.mu.Lock()
	defer schedule.mu.Unlock()
	entry := schedule.attempts[identity]
	entry.errorCode = code
	schedule.attempts[identity] = entry
}

func (s *UpstreamFinanceService) fetchSub2APIHistory(ctx context.Context, target *UpstreamFinanceTarget, key string, todayURL *url.URL, snapshot *UpstreamBalanceSnapshot, dayStart time.Time) string {
	// Snapshots are persisted per target, so duplicate credential entries must
	// not suppress one another's historical observation.
	identity := strconv.FormatInt(target.ID, 10) + ":" + upstreamBalanceIdentity(target) + ":" + dayStart.Format("2006-01-02")
	claimed, previousError := s.sub2APIHistory.claim(identity, s.now())
	if !claimed {
		return previousError
	}
	request := *todayURL
	query := request.Query()
	query.Set("days", "30")
	query.Set("start_date", dayStart.AddDate(0, 0, -29).Format("2006-01-02"))
	query.Set("end_date", dayStart.Format("2006-01-02"))
	request.RawQuery = query.Encode()
	historyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	body, code := s.fetchUpstreamFinanceJSON(historyCtx, request.String(), key, upstreamBalanceMaxBody, 20*time.Second, 1)
	cancel()
	if code == "" {
		period := parseUpstreamPeriodUsed(body)
		parsed, err := parseUpstreamUsage(body)
		if period == nil || err != nil || parsed.Currency != snapshot.Currency {
			code = "usage_response_unsupported"
		} else if !timezone.StartOfDay(s.now()).Equal(dayStart) {
			code = "usage_response_unsupported"
		} else {
			periodStart, periodEnd := dayStart.AddDate(0, 0, -29), dayStart.AddDate(0, 0, 1)
			snapshot.Last30DaysUsed, snapshot.PeriodStart, snapshot.PeriodEnd = period, &periodStart, &periodEnd
			// Keep the earlier daily observation time: persistence has one time
			// column, and history must never make today's older data look newer.
			snapshot.PeriodSyncedAt = snapshot.SyncedAt
		}
	}
	if code != "" {
		code = "sub2api_history_sync_failed: " + code
	}
	s.sub2APIHistory.finish(identity, code)
	return code
}
