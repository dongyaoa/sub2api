//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const intelligenceAnthropicArtwork = "<!doctype html><html><style>svg{height:100%}</style><body><svg viewBox=\"0 0 10 10\"><circle r=\"2\"/></svg></body></html>"

func intelligenceAnthropicTestMessage(t *testing.T, stop string) string {
	t.Helper()
	message, err := json.Marshal(map[string]any{"type": "message", "role": "assistant", "stop_reason": stop, "content": []map[string]string{
		{"type": "thinking", "thinking": "unpublished private analysis <html>bad</html>"},
		{"type": "redacted_thinking", "data": "secret signature"},
		{"type": "text", "text": "```html\n" + intelligenceAnthropicArtwork + "\n```"},
	}})
	require.NoError(t, err)
	return string(message)
}

func TestIntelligenceAnthropicRequestUsesNativeModelAndHighEffort(t *testing.T) {
	for _, tc := range []struct{ model, endpoint string }{
		{"claude-opus-5-5", "https://8.8.8.8/prefix"},
		{"claude-sonnet-5-5", "https://8.8.8.8/prefix/v1/"},
		{"claude-fable-5-2", "https://8.8.8.8/prefix/v1/messages"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			svc := NewIntelligenceMonitorService(nil, nil, nil, nil, nil, nil, nil)
			prompt := "绘制我的鹈鹕 HTML 场景"
			svc.externalClient.Transport = upstreamModelsTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "/prefix/v1/messages", r.URL.Path)
				require.Equal(t, "secret-key", r.Header.Get("x-api-key"))
				require.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
				require.Empty(t, r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, tc.model, body["model"])
				require.Equal(t, true, body["stream"])
				require.Equal(t, float64(24000), body["max_tokens"])
				require.Equal(t, map[string]any{"effort": "high"}, body["output_config"])
				require.Equal(t, map[string]any{"type": "adaptive"}, body["thinking"])
				require.NotContains(t, body, "input")
				require.Equal(t, []any{map[string]any{"role": "user", "content": prompt}}, body["messages"])
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(intelligenceAnthropicTestMessage(t, "end_turn")))}, nil
			})
			status, text, issue := svc.generate(context.Background(), &IntelligenceMonitorRun{Model: tc.model, APIMode: IntelligenceMonitorAPIModeMessages, SourceType: "external", SourceEndpoint: tc.endpoint, Prompt: prompt}, "secret-key")
			require.Empty(t, issue)
			require.Equal(t, 200, *status)
			require.NotContains(t, text, "private analysis")
			require.NotContains(t, text, "signature")
			require.Equal(t, intelligenceAnthropicArtwork, extractIntelligenceHTML(text))
		})
	}
}

func TestIntelligenceAnthropicStreamCompletesAtMessageStopAndJoinsOnlyText(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	stream := strings.Join([]string{
		`{"type":"message_start","message":{"type":"message","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"<html>not the work</html>"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"secret signature"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":"<svg"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" viewBox=\"0 0 12 12\">"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"ping"}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"<circle r=\"2\"/></svg>"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	}, "\n\ndata: ")
	closeOnTimeout := time.AfterFunc(3*time.Second, func() { _ = reader.CloseWithError(context.DeadlineExceeded) })
	defer closeOnTimeout.Stop()
	go func() { _, _ = io.WriteString(writer, "data: "+stream+"\n\n") }()
	text, issue := readIntelligenceGenerationBody(reader, IntelligenceMonitorAPIModeMessages, "text/event-stream", "", false)
	require.Empty(t, issue)
	require.Equal(t, `<svg viewBox="0 0 12 12"><circle r="2"/></svg>`, text)
	require.Contains(t, extractIntelligenceHTML(text), "<body>"+text+"</body>")
}

func TestIntelligenceAnthropicArtworkPreservesAnimationOutsideDrawing(t *testing.T) {
	for _, fragment := range []string{
		`<style>@keyframes pedal{to{transform:rotate(360deg)}}</style><svg><circle/></svg><script>draw()</script>`,
		`<canvas id="scene"></canvas><script>animate()</script>`,
	} {
		raw := "\u212a: ```HTML\n" + fragment + "\n```"
		require.Contains(t, extractIntelligenceHTML(raw), "<body>"+fragment+"</body>")
	}
	document := "<HTML><title>\u212a</title><svg></svg></HTML>"
	require.Equal(t, document, extractIntelligenceHTML("\u212a: "+document))
}

func TestIntelligenceAnthropicMalformedOrInterruptedOutputIsNotPublished(t *testing.T) {
	const start = "data: {\"type\":\"message_start\"}\n\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"<html></html>\"}}\n\n"
	const blockStop = "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
	const reason = "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n"
	const stop = "data: {\"type\":\"message_stop\"}\n\n"
	for name, raw := range map[string]string{
		"missing terminal":    start + blockStop + reason,
		"open block":          start + reason + stop,
		"wrong index":         start + `data: {"type":"content_block_stop","index":1}` + "\n\n" + reason + stop,
		"missing stop reason": start + blockStop + stop,
		"transport sentinel":  start + blockStop + reason + "data: [DONE]\n\n",
		"max tokens":          start + blockStop + strings.Replace(reason, "end_turn", "max_tokens", 1) + stop,
		"upstream error":      start + `data: {"type":"error","error":{"type":"overloaded_error","message":"problem with sample-secret"}}` + "\n\n",
		"truncated json":      intelligenceAnthropicTestMessage(t, "max_tokens"),
		"tool request":        intelligenceAnthropicTestMessage(t, "tool_use"),
	} {
		t.Run(name, func(t *testing.T) {
			_, issue := readIntelligenceGenerationBody(strings.NewReader(raw), IntelligenceMonitorAPIModeMessages, "text/event-stream", "sample-secret", false)
			require.NotEmpty(t, issue, "even a closed HTML tag cannot complete an interrupted protocol response")
			require.NotContains(t, issue, "sample-secret")
		})
	}
}

func TestIntelligenceAnthropicKeyGroupUsesItsProxy(t *testing.T) {
	calls := 0
	// A .invalid hostname can only work if the HTTP request reaches the account proxy.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "anthropic-upstream.invalid", r.URL.Host)
		require.Equal(t, "/v1/messages", r.URL.Path)
		require.Equal(t, "test-key", r.Header.Get("x-api-key"))
		_, _ = io.WriteString(w, intelligenceAnthropicTestMessage(t, "end_turn"))
	}))
	defer proxy.Close()
	target := &UpstreamTarget{ID: 7, Provider: MonitorProviderAnthropic, Endpoint: "http://anthropic-upstream.invalid/v1", APIKeyEncrypted: "encrypted:test-key", AccountIDs: []int64{5}}
	route := financeTestProxy(t, proxy.URL, 1)
	accounts := &financeAccounts{accounts: map[int64]*Account{5: {ID: 5, Type: AccountTypeAPIKey, Platform: MonitorProviderAnthropic, Credentials: map[string]any{"base_url": target.Endpoint, "api_key": "test-key"}, ProxyID: &route.ID, Proxy: route}}}
	svc := NewIntelligenceMonitorService(nil, upstreamTestEncryptor{}, &upstreamTestRepo{target: target}, nil, nil, NewUpstreamFinanceService(nil, nil, nil, nil, accounts), nil)
	run := &IntelligenceMonitorRun{SourceType: "upstream", SourceEndpoint: target.Endpoint, SourceSnapshot: map[string]any{"upstream_target_id": int64(7)}, Model: "claude-opus-5-5", APIMode: IntelligenceMonitorAPIModeMessages}
	_, text, issue := svc.generate(context.Background(), run, "test-key")
	require.Empty(t, issue)
	require.Equal(t, intelligenceAnthropicArtwork, extractIntelligenceHTML(text))
	target.Provider = MonitorProviderOpenAI
	_, _, issue = svc.generate(context.Background(), run, "test-key")
	require.Equal(t, "upstream_proxy_account_mismatch", issue)
	require.Equal(t, 1, calls)
}
