package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// RecentRequestRecord is a compact per-account request attempt summary kept in Redis.
type RecentRequestRecord struct {
	AccountID    int64     `json:"account_id"`
	AccountName  string    `json:"account_name,omitempty"`
	Platform     string    `json:"platform,omitempty"`
	Model        string    `json:"model,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	Success      bool      `json:"success"`
	StatusCode   int       `json:"status_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ProxyID      int64     `json:"proxy_id,omitempty"`
	ProxyName    string    `json:"proxy_name,omitempty"`
}

// RecentRequestStore stores and retrieves recent account request summaries.
// Its failures must never affect gateway traffic.
type RecentRequestStore interface {
	RecordRecentRequest(context.Context, int64, RecentRequestRecord) error
	GetRecentRequests(context.Context, []int64, int) (map[int64][]RecentRequestRecord, error)
}

var (
	recentRequestSecretHeaders = regexp.MustCompile(`(?i)\b(cookie|set-cookie|authorization|proxy-authorization)[ \t]*:[ \t]*[^\r\n]+`)
	recentRequestURLs          = regexp.MustCompile(`(?i)\b(?:https?|socks5h?)://[^\s<>"']+`)
	recentRequestAuth          = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`)
	recentRequestTokens        = regexp.MustCompile(`\b(?:sk-[a-zA-Z0-9_-]{8,}|eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+)\b`)
)

// BuildRecentRequestRecord snapshots the account route without retaining proxy
// addresses or credentials. A missing HTTP status remains zero rather than
// presenting a synthetic status as an upstream response.
func BuildRecentRequestRecord(account *Account, model string, success bool, statusCode int, err error) RecentRequestRecord {
	record := RecentRequestRecord{Model: strings.TrimSpace(model), CreatedAt: time.Now().UTC(), Success: success, StatusCode: statusCode}
	if account != nil {
		record.AccountID = account.ID
		record.AccountName = account.Name
		record.Platform = account.Platform
		proxyID, proxyName := opsUpstreamProxyAttribution(account)
		if proxyID != nil {
			record.ProxyID = *proxyID
		}
		record.ProxyName = proxyName
	}
	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
		var failoverErr *UpstreamFailoverError
		if errors.As(err, &failoverErr) && failoverErr != nil {
			if failoverErr.StatusCode > 0 {
				record.StatusCode = failoverErr.StatusCode
			}
			if len(failoverErr.ResponseBody) > 0 {
				record.ErrorMessage = string(failoverErr.ResponseBody)
			}
		}
	}
	record.ErrorMessage = RecentRequestErrorMessage(record.ErrorMessage)
	return record
}

type recentRequestWriteTask struct {
	store   RecentRequestStore
	records []RecentRequestRecord
}

var recentRequestWriter = struct {
	once  sync.Once
	queue chan recentRequestWriteTask
}{queue: make(chan recentRequestWriteTask, 512)}

// RecordAccountRecentRequests queues a small immutable batch. A fixed worker
// count and bounded queue keep Redis failure or request bursts off the forwarding
// path. Losing a recent-history sample on overflow never changes billing or the
// client's result.
func RecordAccountRecentRequests(_ context.Context, store RecentRequestStore, records []RecentRequestRecord) {
	if store == nil || len(records) == 0 {
		return
	}
	if len(records) > 20 {
		records = records[len(records)-20:]
	}
	snapshot := make([]RecentRequestRecord, 0, len(records))
	for _, record := range records {
		if record.AccountID <= 0 {
			continue
		}
		record = NormalizeRecentRequestRecord(record)
		snapshot = append(snapshot, record)
	}
	if len(snapshot) == 0 {
		return
	}
	sort.SliceStable(snapshot, func(i, j int) bool { return snapshot[i].CreatedAt.Before(snapshot[j].CreatedAt) })
	recentRequestWriter.once.Do(func() {
		for i := 0; i < 4; i++ {
			go func() {
				for task := range recentRequestWriter.queue {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					for _, record := range task.records {
						if ctx.Err() != nil {
							break
						}
						_ = task.store.RecordRecentRequest(ctx, record.AccountID, record)
					}
					cancel()
				}
			}()
		}
	})
	select {
	case recentRequestWriter.queue <- recentRequestWriteTask{store: store, records: snapshot}:
	default:
	}
}

func RecordAccountRecentRequest(ctx context.Context, store RecentRequestStore, record RecentRequestRecord) {
	RecordAccountRecentRequests(ctx, store, []RecentRequestRecord{record})
}

// RecentRequestErrorMessage extracts only a useful upstream message and strips
// common authentication details before anything is persisted for the account UI.
func RecentRequestErrorMessage(raw string) string {
	if json.Valid([]byte(raw)) {
		if message := ExtractUpstreamErrorMessage([]byte(raw)); strings.TrimSpace(message) != "" {
			raw = message
		} else {
			var body struct {
				Error string `json:"error"`
			}
			if json.Unmarshal([]byte(raw), &body) == nil && strings.TrimSpace(body.Error) != "" {
				raw = body.Error
			} else {
				raw = "Upstream returned an error without a message"
			}
		}
	}
	// A header can contain multiple whitespace-separated secrets (for example,
	// Cookie: session=...; csrf=...). Redact its entire value, not just one token.
	raw = recentRequestSecretHeaders.ReplaceAllString(raw, "$1: [REDACTED]")
	raw = recentRequestAuth.ReplaceAllString(raw, "[REDACTED]")
	raw = recentRequestTokens.ReplaceAllString(raw, "[REDACTED]")
	raw = logredact.RedactText(raw, "authorization", "api_key", "x-api-key", "cookie", "proxy_authorization", "token")
	raw = recentRequestURLs.ReplaceAllString(raw, "[URL]")
	runes := []rune(raw)
	if len(runes) > 1000 {
		raw = string(runes[:1000]) + "…"
	}
	return raw
}

// NormalizeRecentRequestRecord keeps every queued and persisted string bounded.
func NormalizeRecentRequestRecord(record RecentRequestRecord) RecentRequestRecord {
	trim := func(value string, limit int) string {
		runes := []rune(strings.TrimSpace(value))
		if len(runes) > limit {
			runes = runes[:limit]
		}
		return string(runes)
	}
	record.Model = trim(record.Model, 256)
	record.AccountName = trim(record.AccountName, 256)
	record.ProxyName = trim(record.ProxyName, 256)
	record.Platform = trim(record.Platform, 64)
	record.ErrorMessage = RecentRequestErrorMessage(record.ErrorMessage)
	return record
}
