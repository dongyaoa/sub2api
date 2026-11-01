//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokWSHTTPBridgeContentPolicyHTTPRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		text   string
	}{
		{"safety check", http.StatusForbidden, `{"error":{"code":"permission-denied","message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"}}`, "I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"},
		{"image moderation", http.StatusBadRequest, `{"error":{"message":"Generated image rejected by content moderation","param":"max_output_tokens"}}`, "Generated image rejected by content moderation"},
		{"plain text", http.StatusForbidden, "I can’t help with this request. https://x.ai/example", "I can’t help with this request. https://x.ai/example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tc.status,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"reused-header"}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo}
			account := &Account{ID: 9161, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi","max_output_tokens":100}`)
			var writes [][]byte
			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "token", payload, len(payload),
				"grok-4.5", "", "", "", "cache-id", 1, func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				})
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.True(t, result.OpenAIWSMode)
			require.NotEmpty(t, result.RequestID)
			require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
			require.False(t, result.SucceededForScheduling())
			require.Len(t, upstream.requests, 1)
			require.Len(t, writes, 1)
			require.Equal(t, "error", gjson.GetBytes(writes[0], "type").String())
			require.Equal(t, int64(tc.status), gjson.GetBytes(writes[0], "status").Int())
			require.Equal(t, tc.text, gjson.GetBytes(writes[0], "error.message").String())
			require.Empty(t, recorder.Body.String(), "never write an HTTP body after the WebSocket upgrade")
			require.Zero(t, repo.tempUnschedCalls)
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestGrokWSHTTPBridgeContentPolicyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name       string
		sse        string
		responseID string
		wantError  bool
	}{
		{
			name: "split refusal in successful response", responseID: "resp-refusal",
			sse: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"I'm sorry, I can\"}\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"’t help with that request.\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-refusal\",\"status\":\"completed\",\"usage\":{\"input_tokens\":100,\"output_tokens\":20}}}\n\n",
		},
		{
			name: "failed response with retryable code", responseID: "resp-failed-refusal",
			sse: "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp-failed-refusal\",\"status\":\"failed\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"I can't help with this request.\"}}}\n\n",
		},
		{
			name: "bare error without response id", wantError: true,
			sse: "data: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"I can't help with this request.\"}}\n\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(tc.sse)),
			}}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo}
			account := &Account{ID: 9162, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi"}`)
			var writes [][]byte
			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "token", payload, len(payload),
				"grok-4.5", "", "", "", "cache-id", 1, func(message []byte) error {
					writes = append(writes, append([]byte(nil), message...))
					return nil
				})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.True(t, result.OpenAIWSMode)
			require.NotEmpty(t, result.RequestID)
			if tc.responseID != "" {
				require.Equal(t, tc.responseID, result.RequestID)
			}
			require.False(t, result.SucceededForScheduling())
			require.Len(t, upstream.requests, 1)
			require.NotEmpty(t, writes)
			if strings.Contains(tc.sse, "server_is_overloaded") {
				require.Contains(t, string(writes[len(writes)-1]), "server_is_overloaded", "keep the upstream refusal unchanged")
				require.Contains(t, string(writes[len(writes)-1]), "I can't help with this request.")
			}
			require.Zero(t, repo.tempUnschedCalls)
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestGrokWSHTTPBridgeContentPolicyBillsSeparateTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "shared-ws-connection")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil).WithContext(ctx)
	account := &Account{ID: 9163, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1}
	upstream := &httpUpstreamRecorder{}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
		&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.httpUpstream = upstream
	payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi"}`)
	var requestIDs []string
	for turn := 1; turn <= 2; turn++ {
		upstream.resp = &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"reused-upstream-header"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"I can't help with that request."}}`)),
		}
		result, err := svc.proxyOpenAIWSHTTPBridgeTurn(ctx, c, account, "token", payload, len(payload),
			"grok-4.5", "", "", "", "cache-id", turn, func([]byte) error { return nil })
		require.Error(t, err)
		require.NoError(t, svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
			Result: result, Account: account, APIKey: &APIKey{ID: 2}, User: &User{ID: 1},
		}))
		require.Equal(t, 0.1, billingRepo.lastCmd.BalanceCost)
		require.Equal(t, result.RequestID, billingRepo.lastCmd.RequestID)
		requestIDs = append(requestIDs, billingRepo.lastCmd.RequestID)
		firstFingerprint := billingRepo.lastCmd.RequestFingerprint
		require.NoError(t, svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
			Result: result, Account: account, APIKey: &APIKey{ID: 2}, User: &User{ID: 1},
		}))
		require.Equal(t, requestIDs[turn-1], billingRepo.lastCmd.RequestID)
		require.Equal(t, firstFingerprint, billingRepo.lastCmd.RequestFingerprint)
	}
	require.NotEqual(t, requestIDs[0], requestIDs[1], "the shared connection ID must not merge different turns")
}
