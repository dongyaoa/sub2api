package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recentObserverStore struct {
	GatewayCache
	records chan RecentRequestRecord
}

func (s *recentObserverStore) RecordRecentRequest(_ context.Context, _ int64, record RecentRequestRecord) error {
	s.records <- record
	return nil
}

func (s *recentObserverStore) GetRecentRequests(context.Context, []int64, int) (map[int64][]RecentRequestRecord, error) {
	return nil, nil
}

func newRecentObserverTest(t *testing.T) (*gin.Context, *recentObserverStore, *Account) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	store := &recentObserverStore{records: make(chan RecentRequestRecord, 30)}
	return c, store, &Account{ID: 19, Platform: PlatformOpenAI, Name: "account"}
}

func takeRecentObserverRecord(t *testing.T, store *recentObserverStore) RecentRequestRecord {
	t.Helper()
	select {
	case record := <-store.records:
		return record
	case <-time.After(2 * time.Second):
		t.Fatal("request history was not recorded")
		return RecentRequestRecord{}
	}
}

func TestRecentObserverRecoveredRetryAndCrossAccountFailover(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "model-a")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, UpstreamStatusCode: 400, Message: `level "minimal" not supported`, UpstreamRequestID: "attempt-a"})
	finish(false, &UpstreamFailoverError{StatusCode: 400, ResponseBody: []byte(`{"error":{"message":"level minimal not supported"}}`)})
	failed := takeRecentObserverRecord(t, store)
	require.Equal(t, account.ID, failed.AccountID)
	require.Equal(t, 400, failed.StatusCode)
	require.False(t, failed.Success)
	require.Contains(t, failed.ErrorMessage, "minimal")

	accountB := &Account{ID: 20, Name: "B"}
	finish = beginAccountRecentRequest(c.Request.Context(), c, store, accountB, "model-b")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: accountB.ID, UpstreamStatusCode: 429, Message: "rate limited", UpstreamRequestID: "attempt-b1"})
	finish(true, nil)
	retried := takeRecentObserverRecord(t, store)
	succeeded := takeRecentObserverRecord(t, store)
	require.Equal(t, accountB.ID, retried.AccountID)
	require.Equal(t, 429, retried.StatusCode)
	require.Equal(t, accountB.ID, succeeded.AccountID)
	require.True(t, succeeded.Success)
	require.Empty(t, store.records)
}

func TestRecentObserverNestedAdapterAndDuplicateSummary(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	finishOuter := beginAccountRecentRequest(c.Request.Context(), c, store, account, "client-model")
	copyContext := c.Copy()
	finishInner := beginAccountRecentRequest(c.Request.Context(), copyContext, store, account, "mapped-model")
	event := OpsUpstreamErrorEvent{AccountID: account.ID, UpstreamRequestID: "same-attempt", UpstreamStatusCode: 503, Message: "busy"}
	appendOpsUpstreamError(copyContext, event)
	event.Kind = "retry_exhausted"
	appendOpsUpstreamError(copyContext, event)
	finishInner(false, errors.New("busy"))
	finishOuter(false, errors.New("busy"))
	record := takeRecentObserverRecord(t, store)
	require.Equal(t, "client-model", record.Model)
	require.Equal(t, 503, record.StatusCode)
	require.Empty(t, store.records)
}

func TestRecentObserverHTTP200StreamFailureAndUnknownHTTPStatus(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "model")
	c.Writer.WriteHeaderNow()
	MarkOpsStreamError(c, "upstream_error", "stream interrupted", 502)
	finish(true, nil)
	record := takeRecentObserverRecord(t, store)
	require.False(t, record.Success)
	require.Zero(t, record.StatusCode, "logical SSE status must not be shown as an upstream HTTP response")
	require.Equal(t, "stream interrupted", record.ErrorMessage)

	c, store, account = newRecentObserverTest(t)
	finish = beginAccountRecentRequest(c.Request.Context(), c, store, account, "model")
	finish(false, errors.New("connection reset"))
	record = takeRecentObserverRecord(t, store)
	require.Zero(t, record.StatusCode)
	require.Equal(t, "connection reset", record.ErrorMessage)
}

func TestRecentObserverSnapshotsProxyAndSkipsOldEvents(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	id := int64(7)
	account.ProxyID = &id
	account.Proxy = &Proxy{ID: id, Name: "proxy-before"}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, UpstreamStatusCode: 401, Message: "old"})
	finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "model")
	account.Proxy.Name = "proxy-after"
	finish(true, nil)
	record := takeRecentObserverRecord(t, store)
	require.Equal(t, "proxy-before", record.ProxyName)
	require.Equal(t, int64(7), record.ProxyID)
	require.True(t, record.Success)
	require.Empty(t, store.records)
}

func TestRecentObserverWebSocketTurnsAndSessionClose(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	hookCalls := 0
	hooks, finish := beginAccountRecentWebSocket(c.Request.Context(), c, store, account, "initial-model", &OpenAIWSIngressHooks{
		AfterTurn: func(int, *OpenAIForwardResult, error) { hookCalls++ },
	})
	BeginOpsStreamTurn(c, 1)
	hooks.AfterTurn(1, &OpenAIForwardResult{Model: "first", OpenAIWSMode: true, UpstreamTerminalEvent: "response.completed"}, nil)
	first := takeRecentObserverRecord(t, store)
	require.True(t, first.Success)
	require.Equal(t, "first", first.Model)
	require.Equal(t, opsProxyNameUnknown, first.ProxyName)
	BeginOpsStreamTurn(c, 2)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, UpstreamStatusCode: 429, Message: "rate limited"})
	hooks.AfterTurn(2, &OpenAIForwardResult{Model: "second", OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"}, nil)
	second := takeRecentObserverRecord(t, store)
	require.False(t, second.Success)
	require.Equal(t, 429, second.StatusCode)
	finish(nil)
	require.Equal(t, 2, hookCalls)
	require.Empty(t, store.records, "a closed socket is not another request")
}

func TestRecentObserverKeepsFailureAfterAnEarlierRetry(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "model")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, Kind: "retry", UpstreamStatusCode: 429, Message: "rate limited"})
	markRecentRequestFailure(c, 200, "stream interrupted")
	finish(false, io.ErrUnexpectedEOF)
	first := takeRecentObserverRecord(t, store)
	last := takeRecentObserverRecord(t, store)
	require.Equal(t, 429, first.StatusCode)
	require.Equal(t, 200, last.StatusCode)
	require.False(t, last.Success)
	require.Equal(t, "stream interrupted", last.ErrorMessage)
}

func TestRecentObserverGeminiHTTP200ErrorPayload(t *testing.T) {
	for _, mode := range []string{"native-stream", "oauth-stream", "claude-stream", "native-json", "claude-json"} {
		t.Run(mode, func(t *testing.T) {
			c, store, account := newRecentObserverTest(t)
			finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "gemini")
			body := `{"error":{"code":400,"message":"invalid generation settings","status":"INVALID_ARGUMENT"}}`
			if mode == "oauth-stream" {
				body = `{"response":` + body + `}`
			}
			if strings.Contains(mode, "stream") {
				body = "data: " + body + "\n\n"
			}
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			svc := &GeminiMessagesCompatService{}
			switch mode {
			case "native-stream", "oauth-stream":
				_, _ = svc.handleNativeStreamingResponse(c, resp, time.Now(), mode == "oauth-stream")
			case "claude-stream":
				_, _ = svc.handleStreamingResponse(c, resp, time.Now(), "gemini")
			case "native-json":
				_, _ = svc.handleNativeNonStreamingResponse(c, resp, false)
			case "claude-json":
				_, _ = svc.handleNonStreamingResponse(c, resp, "gemini")
			}
			finish(true, nil)
			record := takeRecentObserverRecord(t, store)
			require.False(t, record.Success)
			require.Equal(t, 200, record.StatusCode)
			require.Equal(t, "invalid generation settings", record.ErrorMessage)
		})
	}
}

func TestRecentObserverMergesSemanticStreamErrorWithHTTP200Failure(t *testing.T) {
	for _, semanticStatus := range []int{403, 529} {
		t.Run(strconv.Itoa(semanticStatus), func(t *testing.T) {
			c, store, account := newRecentObserverTest(t)
			finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "claude")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, Kind: "retry", UpstreamStatusCode: 429, Message: "earlier actual attempt"})
			markRecentRequestFailure(c, 200, `{"type":"error","error":{"message":"stream overloaded"}}`)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, Kind: "stream_error", UpstreamStatusCode: semanticStatus, Message: "stream overloaded"})
			finish(false, &UpstreamFailoverError{StatusCode: semanticStatus, ResponseBody: []byte(`{"error":{"message":"stream overloaded"}}`)})
			retry := takeRecentObserverRecord(t, store)
			stream := takeRecentObserverRecord(t, store)
			require.Equal(t, 429, retry.StatusCode)
			require.Equal(t, 200, stream.StatusCode)
			require.False(t, stream.Success)
			require.Equal(t, "stream overloaded", stream.ErrorMessage)
			require.Empty(t, store.records, "a semantic Ops status must not create another physical attempt")
			require.Equal(t, semanticStatus, recentRequestEvents(c)[1].UpstreamStatusCode, "Ops policy remains unchanged")
		})
	}
}

func TestRecentObserverReusesSignatureResponseOnlyWhenNoRetryWasSent(t *testing.T) {
	for _, reused := range []bool{true, false} {
		t.Run(map[bool]string{true: "retry budget exhausted", false: "separate identical attempts"}[reused], func(t *testing.T) {
			c, store, account := newRecentObserverTest(t)
			finish := beginAccountRecentRequest(c.Request.Context(), c, store, account, "claude")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, Kind: "signature_error", UpstreamStatusCode: 400, Message: "invalid signature"})
			if reused {
				// The retry budget expired after the upstream response arrived;
				// forwarding restores it and runs the ordinary terminal handler.
				reuseRecentRequestResponse(c)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: account.ID, Kind: "http_error", UpstreamStatusCode: 400, Message: "invalid signature"})
			finish(false, &UpstreamFailoverError{StatusCode: 400, ResponseBody: []byte(`{"error":{"message":"invalid signature"}}`)})
			record := takeRecentObserverRecord(t, store)
			require.Equal(t, 400, record.StatusCode)
			require.Equal(t, "invalid signature", record.ErrorMessage)
			if !reused {
				record = takeRecentObserverRecord(t, store)
				require.Equal(t, 400, record.StatusCode)
			}
			require.Empty(t, store.records)
			require.Len(t, recentRequestEvents(c), 2, "recent-history deduplication must preserve Ops diagnostics")
		})
	}
}
