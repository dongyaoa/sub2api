package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	newAPIUsagePageSize   = 100 // common/page_info.go caps larger requests to 100.
	newAPIUsageMaxPages   = 512
	newAPIUsageMaxTokens  = 8192
	newAPIUsageCacheLimit = 128
	newAPIUsageCacheTTL   = 30 * time.Second
	newAPIUsageRetryDelay = 30 * time.Second
)

// Official contracts (QuantumNous/new-api v0.10.8 and v1.0.0-rc.40):
// controller/log.go, model/log.go, common/page_info.go. /log/token is a
// truncated recent list, not a paginated API. /log/self is time-filtered and
// paginated but cannot filter by token_id. /log/self/stat filters by mutable,
// non-unique token_name, so it must never be used as a Key cost aggregate.
type newAPIUsageLog struct {
	UserID    int64    `json:"user_id"`
	TokenID   int64    `json:"token_id"`
	CreatedAt int64    `json:"created_at"`
	Type      int      `json:"type"`
	Quota     *float64 `json:"quota"`
}

type newAPIUsagePage struct {
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    *int64           `json:"total"`
	Items    []newAPIUsageLog `json:"items"`
}

func setNewAPIUsageWindow(snapshot *UpstreamBalanceSnapshot, start time.Time, amount float64) {
	end := start.AddDate(0, 0, 1)
	snapshot.DayUsed, snapshot.DayStart, snapshot.DayEnd = &amount, &start, &end
}

func (s *UpstreamFinanceService) fetchNewAPIRecentUsage(ctx context.Context, base, key string, snapshot *UpstreamBalanceSnapshot, dayStart time.Time) string {
	raw, code := s.newAPIGet(ctx, base, "/api/log/token", key, 0, nil)
	data, ok := newAPIData(raw)
	var logs []newAPIUsageLog
	if code != "" || !ok || !decodeNewAPI(data, &logs) || logs == nil {
		return "newapi_usage_unavailable"
	}
	end := dayStart.AddDate(0, 0, 1).Unix()
	var day, total float64
	var owner, token, oldest int64
	for i, log := range logs {
		if log.UserID <= 0 || log.TokenID <= 0 || log.CreatedAt <= 0 || log.CreatedAt >= end ||
			(i > 0 && (log.UserID != owner || log.TokenID != token || log.CreatedAt > oldest)) {
			return "newapi_usage_incomplete"
		}
		owner, token, oldest = log.UserID, log.TokenID, log.CreatedAt
		// Match New API's consumption statistics: type=2 includes posted usage;
		// topups, management/error messages and type=6 refund notices are not
		// themselves consumption rows. Reject malformed or negative charges.
		if log.Type != 2 {
			continue
		}
		if !newAPIQuota(log.Quota, false) {
			return "newapi_usage_incomplete"
		}
		total += *log.Quota
		if log.CreatedAt >= dayStart.Unix() {
			day += *log.Quota
		}
	}
	if !newAPIQuota(&total, false) {
		return "newapi_usage_incomplete"
	}
	// A short list alone proves nothing: MaxRecentItems is site configurable.
	// Coverage requires a strictly earlier record, or equality with the Key's
	// cumulative raw quota (which proves no positive charge is omitted).
	all := snapshot.TotalUsed != nil && total == *snapshot.TotalUsed
	if all || (oldest > 0 && oldest < dayStart.Unix()) {
		setNewAPIUsageWindow(snapshot, dayStart, day)
	}
	if snapshot.DayUsed == nil {
		return "newapi_usage_account_auth_required"
	}
	return ""
}

// Cache only aggregates, never response bodies, keys or per-request records.
// The identity includes a credential digest and the precise calendar window.
// In-flight sharing prevents three Keys on one account from scanning the same
// account history three times. Failed scans get a short backoff as well.
type newAPIUsageCacheEntry struct {
	ready       chan struct{}
	expiresAt   time.Time
	retainUntil time.Time
	used        map[int64]float64
	syncedAt    time.Time
	errorCode   string
	baseline    *newAPIUsageBaseline
	scan        *newAPIUsageScan
}

type newAPIUsageBaseline struct {
	used, tail        map[int64]float64
	until, fullScanAt time.Time
}

type newAPIUsageScan struct {
	from, until time.Time
	tailFrom    time.Time
	total       int64
	nextPage    int
	firstHash   [32]byte
	used, tail  map[int64]float64
	baseline    *newAPIUsageBaseline
	// Oversized or count-capped ranges are divided into disjoint time windows.
	// Only fully verified children are merged; incomplete children retain their
	// own pagination cursor. Completed children are released immediately.
	parts     []*newAPIUsageScan
	partIndex int
}

type newAPILogRequestBudget struct {
	until    time.Time
	requests int
}

// Official common/init.go defaults to 180 API reads / 180 seconds per IP.
// Leave headroom for balances and other clients.
func (s *UpstreamFinanceService) allowNewAPILogRead(base string) bool {
	identity := newAPIOriginIdentity(base)
	if identity == "" {
		return false
	}
	now := s.now()
	cache := &s.newAPICache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.logSites == nil {
		cache.logSites = make(map[string]newAPILogRequestBudget)
	}
	entry, exists := cache.logSites[identity]
	if !exists && len(cache.logSites) >= newAPIUsageCacheLimit {
		for id, budget := range cache.logSites {
			if !now.Before(budget.until) {
				delete(cache.logSites, id)
			}
		}
		if len(cache.logSites) >= newAPIUsageCacheLimit {
			return false
		}
	}
	if !now.Before(entry.until) {
		entry = newAPILogRequestBudget{until: now.Add(3 * time.Minute)}
	}
	if entry.requests >= 120 {
		return false
	}
	entry.requests++
	cache.logSites[identity] = entry
	return true
}

type newAPIUsageLogTotals struct {
	used map[int64]float64
	tail map[int64]float64
}

func (s *UpstreamFinanceService) newAPIAccountLogTotals(ctx context.Context, base, key string, userID int64, from, until time.Time) (map[int64]float64, time.Time, string) {
	// At exactly local midnight the second-granularity cutoff equals from.
	// Never retain this empty window as a pending scan that cannot advance.
	if !until.After(from) {
		return nil, time.Time{}, "newapi_usage_incomplete"
	}
	identityInput := fmt.Sprintf("%s\x00%s\x00%d\x00%d", base, key, userID, from.Unix())
	digest := sha256.Sum256([]byte(identityInput))
	identity := hex.EncodeToString(digest[:])
	cache := &s.newAPICache
	now := s.now()
	cache.mu.Lock()
	previous := cache.usage[identity]
	if entry := previous; entry != nil {
		select {
		case <-entry.ready:
			if !newAPIUsageForceRequested(ctx) && now.Before(entry.expiresAt) {
				cache.mu.Unlock()
				return entry.used, entry.syncedAt, entry.errorCode
			}
		default:
			// An in-flight scan must not be replaced after its TTL expires:
			// its mutable cursor belongs to exactly one worker.
			cache.mu.Unlock()
			select {
			case <-entry.ready:
				return entry.used, entry.syncedAt, entry.errorCode
			case <-ctx.Done():
				return nil, time.Time{}, "newapi_usage_incomplete"
			}
		}
	}
	if previous == nil || !now.Before(previous.retainUntil) {
		previous = nil
	}
	if cache.usage == nil {
		cache.usage = make(map[string]*newAPIUsageCacheEntry)
	}
	for id, entry := range cache.usage {
		if !now.Before(entry.retainUntil) {
			select {
			case <-entry.ready:
				delete(cache.usage, id)
			default:
			}
		}
	}
	if _, exists := cache.usage[identity]; !exists && len(cache.usage) >= newAPIUsageCacheLimit {
		cache.mu.Unlock()
		return nil, time.Time{}, "newapi_usage_incomplete"
	}
	entry := &newAPIUsageCacheEntry{ready: make(chan struct{}), expiresAt: now.Add(time.Minute), retainUntil: now.Add(2 * time.Hour)}
	if previous != nil {
		entry.baseline, entry.scan = previous.baseline, previous.scan
	}
	cache.usage[identity] = entry
	cache.mu.Unlock()
	if entry.scan == nil {
		scan := &newAPIUsageScan{from: from, until: until, nextPage: 1}
		prior := entry.baseline
		if prior != nil && prior.until.Before(until) && now.Sub(prior.fullScanAt) < 15*time.Minute {
			// Replace the overlap; never add already counted records twice.
			scan.from, scan.baseline = prior.until.Add(-30*time.Second), prior
			if scan.from.Before(from) {
				scan.from = from
			}
		}
		entry.scan = scan
	}
	scan := entry.scan
	totals, code := s.resumeNewAPIAccountLogs(ctx, base, key, userID, scan)
	if code == "" && scan.baseline != nil {
		combined := make(map[int64]float64, len(totals.used))
		for tokenID, amount := range totals.used {
			combined[tokenID] = amount
		}
		for tokenID, amount := range scan.baseline.used {
			combined[tokenID] += amount - scan.baseline.tail[tokenID]
			if !newAPIQuota(ptrNewAPIQuota(combined[tokenID]), false) {
				code = "newapi_usage_incomplete"
			}
		}
		if len(combined) > newAPIUsageMaxTokens {
			code = "newapi_usage_incomplete"
		}
		if code == "" {
			totals.used = combined
		} else {
			entry.scan = nil
		}
	}
	cache.mu.Lock()
	if code == "" {
		entry.used = totals.used
		fullScanAt := s.now()
		if scan.baseline != nil {
			fullScanAt = scan.baseline.fullScanAt
		}
		entry.baseline = &newAPIUsageBaseline{used: totals.used, tail: totals.tail, until: scan.until, fullScanAt: fullScanAt}
		entry.scan = nil
	}
	entry.syncedAt, entry.errorCode = scan.until, code
	ttl := newAPIUsageCacheTTL
	if code != "" {
		ttl = newAPIUsageRetryDelay
	}
	entry.expiresAt = s.now().Add(ttl)
	if code == "" && scan.until.Add(ttl).Before(entry.expiresAt) {
		// Completing a long resumed scan must not impose another cache wait
		// before its old cutoff can be caught up to current consumption.
		entry.expiresAt = scan.until.Add(ttl)
	}
	close(entry.ready)
	cache.mu.Unlock()
	return entry.used, entry.syncedAt, code
}

func (s *UpstreamFinanceService) scanNewAPIAccountLogs(ctx context.Context, base, key string, userID int64, from, until time.Time) (*newAPIUsageLogTotals, string) {
	return s.resumeNewAPIAccountLogs(ctx, base, key, userID, &newAPIUsageScan{from: from, until: until, nextPage: 1})
}

func (s *UpstreamFinanceService) resumeNewAPIAccountLogs(ctx context.Context, base, key string, userID int64, scan *newAPIUsageScan) (*newAPIUsageLogTotals, string) {
	if scan.tailFrom.IsZero() {
		scan.tailFrom = scan.until.Add(-30 * time.Second)
	}
	if scan.parts == nil {
		totals, code, split := s.resumeNewAPIAccountLogWindow(ctx, base, key, userID, scan)
		if !split {
			return totals, code
		}
		// New API deployments may cap total, or this account can outgrow our
		// page bound. Retrying the same full range forever cannot make progress.
		// Split on whole seconds because New API's bounds have second precision.
		seconds := scan.until.Unix() - scan.from.Unix()
		if seconds <= 1 {
			return nil, "newapi_usage_incomplete"
		}
		middle := time.Unix(scan.from.Unix()+seconds/2, 0).In(scan.from.Location())
		scan.parts = []*newAPIUsageScan{
			{from: scan.from, until: middle, tailFrom: scan.tailFrom, nextPage: 1},
			{from: middle, until: scan.until, tailFrom: scan.tailFrom, nextPage: 1},
		}
		scan.used, scan.tail = make(map[int64]float64), make(map[int64]float64)
	}
	for scan.partIndex < len(scan.parts) {
		part := scan.parts[scan.partIndex]
		totals, code := s.resumeNewAPIAccountLogs(ctx, base, key, userID, part)
		if code != "" {
			return nil, code
		}
		for tokenID, amount := range totals.used {
			value := scan.used[tokenID] + amount
			if !newAPIQuota(&value, false) {
				return nil, "newapi_usage_incomplete"
			}
		}
		// Validate before mutating the parent's totals so a failed merge can
		// never add a verified child twice on the following synchronization.
		for tokenID, amount := range totals.used {
			scan.used[tokenID] += amount
			scan.tail[tokenID] += totals.tail[tokenID]
		}
		scan.parts[scan.partIndex] = nil
		scan.partIndex++
		if len(scan.used) > newAPIUsageMaxTokens {
			return nil, "newapi_usage_incomplete"
		}
	}
	if len(scan.used) > newAPIUsageMaxTokens {
		return nil, "newapi_usage_incomplete"
	}
	return &newAPIUsageLogTotals{used: scan.used, tail: scan.tail}, ""
}

func (s *UpstreamFinanceService) resumeNewAPIAccountLogWindow(ctx context.Context, base, key string, userID int64, scan *newAPIUsageScan) (*newAPIUsageLogTotals, string, bool) {
	from, until := scan.from, scan.until
	if !until.After(from) {
		return nil, "newapi_usage_incomplete", false
	}
	load := func(requestCtx context.Context, page int) (*newAPIUsagePage, string) {
		if !s.allowNewAPILogRead(base) {
			return nil, "newapi_usage_incomplete"
		}
		query := url.Values{"type": {"2"}, "start_timestamp": {strconv.FormatInt(from.Unix(), 10)}, "end_timestamp": {strconv.FormatInt(until.Unix()-1, 10)}, "p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(newAPIUsagePageSize)}}
		raw, code := s.newAPIGet(requestCtx, base, "/api/log/self", key, userID, query)
		data, ok := newAPIData(raw)
		var result newAPIUsagePage
		if code != "" || !ok || !decodeNewAPI(data, &result) || result.Total == nil || *result.Total < 0 || result.Page != page || result.PageSize != newAPIUsagePageSize {
			return nil, "newapi_usage_unavailable"
		}
		return &result, ""
	}
	first, code := load(ctx, 1)
	if code != "" {
		return nil, code, false
	}
	total := *first.Total
	maxPages := newAPIUsageMaxPages
	if total > int64(newAPIUsagePageSize*maxPages) {
		return nil, "newapi_usage_incomplete", true
	}
	pages := (int(total) + newAPIUsagePageSize - 1) / newAPIUsagePageSize
	if pages < 1 {
		pages = 1
	}
	firstJSON, _ := json.Marshal(first.Items)
	firstHash := sha256.Sum256(firstJSON)
	if scan.nextPage <= 1 || scan.total != total || scan.firstHash != firstHash {
		// New/removed logs invalidate offsets. Restart safely within this same
		// fixed time window; never merge pages from incompatible snapshots.
		scan.nextPage, scan.total, scan.firstHash = 1, total, firstHash
		scan.used, scan.tail = make(map[int64]float64), make(map[int64]float64)
	}
	used, tail := scan.used, scan.tail
	tailFrom := scan.tailFrom.Unix()
	beyondReportedCount := func(page *newAPIUsagePage) bool {
		want := max(0, min(newAPIUsagePageSize, int(total)-(page.Page-1)*newAPIUsagePageSize))
		return *page.Total == total && len(page.Items) > want && len(page.Items) <= newAPIUsagePageSize
	}
	merge := func(page *newAPIUsagePage) bool {
		want := int(total) - (page.Page-1)*newAPIUsagePageSize
		if want > newAPIUsagePageSize {
			want = newAPIUsagePageSize
		}
		if *page.Total != total || len(page.Items) != want {
			return false
		}
		for _, log := range page.Items {
			if log.UserID != userID || log.TokenID <= 0 || log.Type != 2 || log.CreatedAt < from.Unix() || log.CreatedAt >= until.Unix() || !newAPIQuota(log.Quota, false) {
				return false
			}
			used[log.TokenID] += *log.Quota
			if log.CreatedAt >= tailFrom {
				tail[log.TokenID] += *log.Quota
			}
			if !newAPIQuota(ptrNewAPIQuota(used[log.TokenID]), false) {
				return false
			}
		}
		return len(used) <= newAPIUsageMaxTokens
	}
	if scan.nextPage == 1 {
		if beyondReportedCount(first) {
			return nil, "newapi_usage_incomplete", true
		}
		if !merge(first) {
			scan.nextPage = 1
			return nil, "newapi_usage_incomplete", false
		}
		scan.nextPage = 2
	}
	for start := scan.nextPage; start <= pages; start += 4 {
		count := min(4, pages-start+1)
		batch := make([]*newAPIUsagePage, count)
		group, groupCtx := errgroup.WithContext(ctx)
		for i := 0; i < count; i++ {
			i := i
			group.Go(func() error {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				var code string
				batch[i], code = load(groupCtx, start+i)
				if code != "" {
					return fmt.Errorf("%s", code)
				}
				return nil
			})
		}
		if group.Wait() != nil {
			return nil, "newapi_usage_incomplete", false
		}
		for _, page := range batch {
			if beyondReportedCount(page) {
				return nil, "newapi_usage_incomplete", true
			}
			if !merge(page) {
				scan.nextPage = 1
				return nil, "newapi_usage_incomplete", false
			}
		}
		scan.nextPage = start + count
	}
	// Some deployments cap the reported count. A full terminal page is not
	// proof of completion: explicitly check the next page before saving money.
	if total > 0 && total%newAPIUsagePageSize == 0 {
		tail, code := load(ctx, pages+1)
		if code != "" || *tail.Total != total || len(tail.Items) != 0 {
			if code == "" {
				scan.nextPage = 1
			}
			return nil, "newapi_usage_incomplete", code == "" && *tail.Total == total && len(tail.Items) > 0
		}
	}
	if pages > 1 {
		// Logs can be posted late into a fixed timestamp range. Do not publish
		// offsets collected before that insertion, even if it happened during
		// this last batch. A following attempt restarts only the affected range.
		latest, code := load(ctx, 1)
		if code != "" {
			return nil, code, false
		}
		latestJSON, _ := json.Marshal(latest.Items)
		if *latest.Total != total || sha256.Sum256(latestJSON) != firstHash {
			scan.nextPage = 1
			return nil, "newapi_usage_incomplete", false
		}
	}
	return &newAPIUsageLogTotals{used: used, tail: tail}, "", false
}

func ptrNewAPIQuota(value float64) *float64 { return &value }

func (s *UpstreamFinanceService) fetchNewAPIAccountUsage(ctx context.Context, base, key string, target *UpstreamFinanceTarget, tokenID int64, snapshot *UpstreamBalanceSnapshot, dayStart time.Time) string {
	// Freeze the time boundary before any page is requested. Calls posted after
	// it cannot shift offsets while a busy account is being paginated.
	until := s.now().UTC().Truncate(time.Second)
	dayCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	today, syncedAt, code := s.newAPIAccountLogTotals(dayCtx, base, key, target.NewAPIUserID, dayStart, until)
	if code != "" {
		return code
	}
	latestUntil := s.now().UTC().Truncate(time.Second)
	if dayCtx.Err() == nil && latestUntil.Sub(syncedAt) >= newAPIUsageCacheTTL && latestUntil.Before(dayStart.AddDate(0, 0, 1)) {
		// A resumed scan can finish with a much older fixed upper bound. Spend
		// the remaining daily budget on one delta rather than reading history
		// or delaying the catch-up until a later polling round.
		catchupCtx := context.WithValue(dayCtx, newAPIUsageForceKey{}, true)
		updated, updatedAt, updateCode := s.newAPIAccountLogTotals(catchupCtx, base, key, target.NewAPIUserID, dayStart, latestUntil)
		if updateCode == "" {
			today, syncedAt = updated, updatedAt
		} else {
			code = updateCode
		}
	}
	setNewAPIUsageWindow(snapshot, dayStart, today[tokenID])
	snapshot.SyncedAt, snapshot.DaySyncedAt = &syncedAt, &syncedAt
	return code
}
