package service

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const recentRequestObserverKey = "account_recent_request_observer"

// The outer protocol adapter owns the observation. Nested adapters use the
// same Gin context and must not create additional green bars for one response.
type recentRequestObservation struct {
	accountID int64
	seen      map[*OpsUpstreamErrorEvent]bool
	stream    int
	turn      int
	events    []*OpsUpstreamErrorEvent
	failure   *RecentRequestRecord
	// summarized marks an observed response that forwarding will describe again
	// through its terminal error handler without making another upstream call.
	summarized map[*OpsUpstreamErrorEvent]bool
}

func recentRequestModel(body []byte) string {
	return gjson.GetBytes(body, "model").String()
}

func recentRequestParsedModel(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}
	return parsed.Model
}

func recentRequestEvents(c *gin.Context) []*OpsUpstreamErrorEvent {
	if value, exists := c.Get(OpsUpstreamErrorsKey); exists {
		if events, ok := value.([]*OpsUpstreamErrorEvent); ok {
			return events
		}
	}
	return nil
}

// Error events can be appended through a copied Gin context in an adapter.
// Keep them on the shared observation as well as in the existing Ops context.
func observeRecentRequestEvent(c *gin.Context, event *OpsUpstreamErrorEvent) {
	if value, ok := c.Get(recentRequestObserverKey); ok {
		if observation, ok := value.(*recentRequestObservation); ok && observation != nil {
			observation.events = append(observation.events, event)
			if len(observation.events) > 256 {
				observation.events = append([]*OpsUpstreamErrorEvent(nil), observation.events[len(observation.events)-256:]...)
			}
		}
	}
}

// markRecentRequestFailure observes an in-band protocol failure without
// changing forwarding, billing, or the client's existing response behavior.
func markRecentRequestFailure(c *gin.Context, statusCode int, message string) {
	if c == nil {
		return
	}
	if value, ok := c.Get(recentRequestObserverKey); ok {
		if observation, ok := value.(*recentRequestObservation); ok && observation != nil {
			observation.failure = &RecentRequestRecord{StatusCode: statusCode, ErrorMessage: RecentRequestErrorMessage(message)}
		}
	}
}

// reuseRecentRequestResponse lets the terminal error event replace an earlier
// diagnostic for the exact same response. The explicit call at the retry budget
// boundary avoids conflating genuine retries that have identical error text and
// no upstream request ID. Existing Ops events are left unchanged.
func reuseRecentRequestResponse(c *gin.Context) {
	if c == nil {
		return
	}
	if value, ok := c.Get(recentRequestObserverKey); ok {
		if observation, ok := value.(*recentRequestObservation); ok && observation != nil && len(observation.events) > 0 {
			if observation.summarized == nil {
				observation.summarized = make(map[*OpsUpstreamErrorEvent]bool)
			}
			observation.summarized[observation.events[len(observation.events)-1]] = true
		}
	}
}

func observeRecentResponseError(c *gin.Context, body []byte, statusCode int) {
	if value := gjson.GetBytes(body, "error"); value.Exists() && value.Type != gjson.Null {
		markRecentRequestFailure(c, statusCode, string(body))
	}
}

// beginAccountRecentRequest snapshots only attempt-local state. Ops events are
// available even when persistent operations logging is disabled. Reading the
// new events here preserves recovered failures without querying the log tables.
func beginAccountRecentRequest(ctx context.Context, c *gin.Context, cache GatewayCache, account *Account, model string) func(bool, error) {
	noop := func(bool, error) {}
	store, ok := cache.(RecentRequestStore)
	if !ok || store == nil || c == nil || account == nil || account.ID <= 0 || account.IsSyntheticUITest() {
		return noop
	}
	if value, exists := c.Get(recentRequestObserverKey); exists {
		if active, ok := value.(*recentRequestObservation); ok && active != nil {
			return noop
		}
	}
	observation := &recentRequestObservation{accountID: account.ID, seen: make(map[*OpsUpstreamErrorEvent]bool), stream: len(GetOpsStreamErrors(c))}
	for _, event := range recentRequestEvents(c) {
		observation.seen[event] = true
	}
	c.Set(recentRequestObserverKey, observation)
	// Build the route now: subsequent scheduling/hydration must not rewrite
	// the historical proxy identity stored for this forwarding attempt.
	base := BuildRecentRequestRecord(account, model, false, 0, nil)
	return func(success bool, forwardErr error) {
		c.Set(recentRequestObserverKey, (*recentRequestObservation)(nil))
		records := collectAccountRecentRequests(c, observation, base, success, forwardErr)
		RecordAccountRecentRequests(ctx, store, records)
	}
}

func collectAccountRecentRequests(c *gin.Context, observation *recentRequestObservation, base RecentRequestRecord, success bool, forwardErr error) []RecentRequestRecord {
	records := make([]RecentRequestRecord, 0, 4)
	seenRequestIDs := make(map[string]bool)
	seenEvents := make(map[*OpsUpstreamErrorEvent]bool)
	var lastKind string
	events := append(append([]*OpsUpstreamErrorEvent(nil), recentRequestEvents(c)...), observation.events...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i] == nil {
			return false
		}
		if events[j] == nil {
			return true
		}
		return events[i].AtUnixMs < events[j].AtUnixMs
	})
	for _, event := range events {
		if event == nil || observation.seen[event] || observation.summarized[event] || event.AccountID != observation.accountID {
			continue
		}
		if seenEvents[event] {
			continue
		}
		seenEvents[event] = true
		// Some error handlers append both a transport error and a failover
		// summary for the same upstream response. A real upstream request ID
		// distinguishes these from separate attempts returning the same error.
		if event.UpstreamRequestID != "" {
			if seenRequestIDs[event.UpstreamRequestID] {
				continue
			}
			seenRequestIDs[event.UpstreamRequestID] = true
		}
		record := base
		record.StatusCode = event.UpstreamStatusCode
		record.ProxyID = 0
		if event.ProxyID != nil {
			record.ProxyID = *event.ProxyID
		}
		record.ProxyName = event.ProxyName
		if event.AtUnixMs > 0 {
			record.CreatedAt = time.UnixMilli(event.AtUnixMs).UTC()
		}
		record.ErrorMessage = event.Message
		if record.ErrorMessage == "" {
			record.ErrorMessage = event.Detail
		}
		if record.ErrorMessage == "" {
			record.ErrorMessage = event.UpstreamResponseBody
		}
		record.ErrorMessage = RecentRequestErrorMessage(record.ErrorMessage)
		records = append(records, record)
		lastKind = event.Kind
	}
	streamErrors := GetOpsStreamErrors(c)
	if len(streamErrors) > observation.stream {
		last := streamErrors[len(streamErrors)-1]
		if (observation.turn == 0 || last.Turn == observation.turn) && (last.AccountID == 0 || last.AccountID == base.AccountID) {
			success = false
			if forwardErr == nil {
				forwardErr = errors.New(last.Message)
			}
			// Keep actual upstream HTTP status separate from a logical SSE
			// status; do not invent an HTTP response for a transport failure.
			base.StatusCode = last.UpstreamStatus
			if last.UpstreamMessage != "" {
				base.ErrorMessage = last.UpstreamMessage
			}
		}
	}
	base.CreatedAt = time.Now().UTC()
	if observation.failure != nil {
		success = false
		base.StatusCode = observation.failure.StatusCode
		base.ErrorMessage = observation.failure.ErrorMessage
	}
	base.Success = success && forwardErr == nil
	if base.Success {
		base.StatusCode = http.StatusOK
		base.ErrorMessage = ""
		return append(records, base)
	}
	if forwardErr != nil {
		var failover *UpstreamFailoverError
		if observation.failure == nil && errors.As(forwardErr, &failover) && failover != nil {
			base.StatusCode = failover.StatusCode
			base.ErrorMessage = string(failover.ResponseBody)
		}
		if strings.TrimSpace(base.ErrorMessage) == "" {
			base.ErrorMessage = forwardErr.Error()
		}
	}
	base.ErrorMessage = RecentRequestErrorMessage(base.ErrorMessage)
	if len(records) == 0 {
		if base.ErrorMessage == "" {
			base.ErrorMessage = "Upstream response did not complete successfully"
		}
		return append(records, base)
	}
	// A prior retry failure cannot stand in for a subsequent stream/transport
	// failure. They are separate actual attempts even when no usage was emitted.
	last := &records[len(records)-1]
	// Claude's stream_error Ops event carries a semantic 403/529 for scheduler
	// policy even though its HTTP response was 200. The explicit in-band marker
	// describes that same terminal response, so replace its status rather than
	// creating another attempt. Earlier retry records remain untouched.
	if lastKind == "stream_error" && observation.failure != nil {
		last.StatusCode = base.StatusCode
		last.ErrorMessage = base.ErrorMessage
		return records
	}
	retrying := strings.Contains(lastKind, "retry") && !strings.Contains(lastKind, "exhaust") && !strings.Contains(lastKind, "failover")
	if retrying || (observation.failure != nil && (last.StatusCode != base.StatusCode || last.ErrorMessage != base.ErrorMessage)) {
		return append(records, base)
	}
	// The terminal failed response has already been captured in the attempt
	// events. Enrich it instead of adding a second bar for the returned error.
	if last.ErrorMessage == "" {
		last.ErrorMessage = base.ErrorMessage
	}
	return records
}

// A WebSocket session contains many requests. Observe AfterTurn instead of
// treating a successfully opened/closed socket as one inference response.
func beginAccountRecentWebSocket(ctx context.Context, c *gin.Context, cache GatewayCache, account *Account, model string, hooks *OpenAIWSIngressHooks) (*OpenAIWSIngressHooks, func(error)) {
	store, ok := cache.(RecentRequestStore)
	if !ok || store == nil || c == nil || account == nil || account.IsSyntheticUITest() {
		return hooks, func(error) {}
	}
	observation := &recentRequestObservation{accountID: account.ID, seen: make(map[*OpsUpstreamErrorEvent]bool)}
	for _, event := range recentRequestEvents(c) {
		observation.seen[event] = true
	}
	c.Set(recentRequestObserverKey, observation)
	base := BuildRecentRequestRecord(account, model, false, 0, nil)
	if base.ProxyID == 0 {
		base.ProxyName = opsProxyNameUnknown
	}
	wrapped := &OpenAIWSIngressHooks{}
	if hooks != nil {
		*wrapped = *hooks
	}
	afterTurn := wrapped.AfterTurn
	completed := false
	wrapped.AfterTurn = func(turn int, result *OpenAIForwardResult, err error) {
		if afterTurn != nil {
			afterTurn(turn, result, err)
		}
		observation.turn = turn
		current := base
		if result != nil && result.Model != "" {
			current.Model = result.Model
		}
		records := collectAccountRecentRequests(c, observation, current, result != nil && result.SucceededForScheduling(), err)
		RecordAccountRecentRequests(ctx, store, records)
		completed = true
		observation.events = nil
		observation.failure = nil
		observation.summarized = nil
		observation.seen = make(map[*OpsUpstreamErrorEvent]bool)
		for _, event := range recentRequestEvents(c) {
			observation.seen[event] = true
		}
	}
	return wrapped, func(err error) {
		c.Set(recentRequestObserverKey, (*recentRequestObservation)(nil))
		// Handshake/auth failures may occur before AfterTurn is called. A
		// normal session close is not an additional request or a failed bar.
		if !completed && err != nil {
			RecordAccountRecentRequests(ctx, store, collectAccountRecentRequests(c, observation, base, false, err))
		}
	}
}
