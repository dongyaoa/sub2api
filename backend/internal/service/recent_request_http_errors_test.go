package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRecentRequestHTTPErrorFromAnthropicProtocolAdapters(t *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		t.Run(protocol, func(t *testing.T) {
			c, store, account := newRecentObserverTest(t)
			proxyID := int64(21)
			account.Platform, account.Type = PlatformAnthropic, AccountTypeAPIKey
			account.Credentials = map[string]any{"api_key": "test-key"}
			account.ProxyID = &proxyID
			account.Proxy = &Proxy{ID: proxyID, Name: "selected node", Protocol: "http", Host: "proxy.example", Port: 8080}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"X-Request-Id": []string{"actual-400"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"level minimal not supported"}}`)),
			}}
			svc := &GatewayService{cache: store, httpUpstream: upstream, cfg: &config.Config{}}
			var result *ForwardResult
			var err error
			if protocol == "responses" {
				result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-6","input":"hi"}`), nil)
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`), nil)
			}
			require.Nil(t, result)
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			record := takeRecentObserverRecord(t, store)
			require.Equal(t, http.StatusBadRequest, record.StatusCode)
			require.False(t, record.Success)
			require.Equal(t, "level minimal not supported", record.ErrorMessage)
			require.Equal(t, proxyID, record.ProxyID)
			require.Equal(t, "selected node", record.ProxyName)
			require.Len(t, recentRequestEvents(c), 1)
			require.Empty(t, store.records)
		})
	}
}

func TestRecentRequestAntigravityHTTPErrorHasNoSuccessfulBar(t *testing.T) {
	c, store, account := newRecentObserverTest(t)
	account.Platform, account.Type = PlatformAntigravity, AccountTypeAPIKey
	account.Credentials = map[string]any{"base_url": "https://upstream.example", "api_key": "test-key"}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"unsupported reasoning level"}}`)),
	}}
	svc := &AntigravityGatewayService{cache: store, httpUpstream: upstream}
	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`))
	// Preserve the existing passthrough result contract, while independently
	// recording its actual HTTP error instead of a green bar.
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusBadRequest, c.Writer.Status())
	record := takeRecentObserverRecord(t, store)
	require.Equal(t, http.StatusBadRequest, record.StatusCode)
	require.False(t, record.Success)
	require.Equal(t, "unsupported reasoning level", record.ErrorMessage)
	require.Len(t, recentRequestEvents(c), 1)
	require.Empty(t, store.records)
}

func TestRecentRequestOpenAICyberHTTPErrorKeepsActualStatus(t *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		t.Run(protocol, func(t *testing.T) {
			_, store, _ := newRecentObserverTest(t)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			account := compatCyberOAuthAccount()
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"cyber_policy","message":"flagged for cyber policy"}}`)),
			}}
			svc := &OpenAIGatewayService{cache: store, httpUpstream: upstream}
			var err error
			if protocol == "responses" {
				_, err = svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","input":"hi","stream":false}`))
			} else {
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"stream":false}`), "", "gpt-5.5")
			}
			require.Error(t, err)
			record := takeRecentObserverRecord(t, store)
			require.False(t, record.Success)
			require.Equal(t, http.StatusBadRequest, record.StatusCode)
			require.Contains(t, record.ErrorMessage, "cyber policy")
			require.Len(t, recentRequestEvents(c), 1)
			require.Empty(t, store.records)
		})
	}
}
