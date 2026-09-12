package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountTestMetricsUpstream struct {
	call func(*http.Request, string) (*http.Response, error)
}

func (u accountTestMetricsUpstream) Do(r *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
	return u.call(r, proxy)
}

func (u accountTestMetricsUpstream) DoWithTLS(r *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, proxy, id, concurrency)
}

type accountTestMetricsRepo struct {
	accountTestProxyRepo
	beforeGet func(context.Context)
}

func (r *accountTestMetricsRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.beforeGet(ctx)
	return r.accountTestProxyRepo.GetByID(ctx, id)
}

type accountTestTimedChunk struct {
	delay time.Duration
	text  string
}

type accountTestTimedReader struct {
	now       *time.Time
	chunks    []accountTestTimedChunk
	remaining string
}

func (r *accountTestTimedReader) Read(p []byte) (int, error) {
	if r.remaining == "" {
		if len(r.chunks) == 0 {
			return 0, io.EOF
		}
		chunk := r.chunks[0]
		r.chunks = r.chunks[1:]
		*r.now = r.now.Add(chunk.delay)
		r.remaining = chunk.text
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}

func accountTestEvents(t *testing.T, output string) []TestEvent {
	t.Helper()
	var events []TestEvent
	for _, line := range strings.Split(output, "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var event TestEvent
			require.NoError(t, json.Unmarshal([]byte(data), &event))
			events = append(events, event)
		}
	}
	return events
}

func TestAccountTestMetricsMeasureDispatchAndActualText(t *testing.T) {
	for _, scenario := range []string{"text", "no text", "http failure", "transport failure", "stream failure after text"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
			account := accountTestProxyFixture()
			repo := &accountTestMetricsRepo{
				accountTestProxyRepo: accountTestProxyRepo{account: account},
				beforeGet: func(ctx context.Context) {
					m := accountTestMetricsFromContext(ctx)
					m.now = func() time.Time { return now }
					m.startedAt = now
					now = now.Add(500 * time.Millisecond)
				},
			}
			upstream := accountTestMetricsUpstream{call: func(r *http.Request, proxy string) (*http.Response, error) {
				require.Equal(t, account.ProxyPool[1].Proxy.URL(), proxy)
				now = now.Add(40 * time.Millisecond)
				if scenario == "transport failure" {
					return nil, errors.New("connection refused")
				}
				httptrace.ContextClientTrace(r.Context()).GotFirstResponseByte()
				now = now.Add(40 * time.Millisecond)
				if scenario == "http failure" {
					return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"unavailable"}}`))}, nil
				}
				chunks := []accountTestTimedChunk{
					{30 * time.Millisecond, "data: {\"type\":\"message_start\"}\n\n"},
					{20 * time.Millisecond, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"\"}}\n\n"},
				}
				if scenario != "no text" {
					chunks = append(chunks, accountTestTimedChunk{50 * time.Millisecond, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n"})
				}
				terminal := "data: {\"type\":\"message_stop\"}\n\n"
				if scenario == "stream failure after text" {
					terminal = "data: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n"
				}
				chunks = append(chunks, accountTestTimedChunk{20 * time.Millisecond, terminal})
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(&accountTestTimedReader{now: &now, chunks: chunks})}, nil
			}}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			err := svc.TestAccountConnection(c, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
			if strings.Contains(scenario, "failure") {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			events := accountTestEvents(t, recorder.Body.String())
			require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_metrics"`))
			metrics := events[len(events)-2]
			require.Equal(t, "test_metrics", metrics.Type)
			require.NotNil(t, metrics.DurationMs)
			if scenario == "transport failure" {
				require.Nil(t, metrics.LatencyMs)
				require.Nil(t, metrics.FirstTokenMs)
				require.Equal(t, int64(540), *metrics.DurationMs)
				return
			}
			require.Equal(t, int64(40), *metrics.LatencyMs)
			if scenario == "text" || scenario == "stream failure after text" {
				require.Equal(t, int64(180), *metrics.FirstTokenMs)
				require.Equal(t, int64(700), *metrics.DurationMs)
			} else {
				require.Nil(t, metrics.FirstTokenMs)
			}
		})
	}
}

func TestAccountTestMetricsRetryKeepsSelectedProxyAndNoMediaTTFT(t *testing.T) {
	account := accountTestProxyFixture()
	account.Platform, account.Type = PlatformGrok, AccountTypeAPIKey
	account.Credentials = map[string]any{"api_key": "test-key"}
	selected := *account.ProxyPool[1].Proxy
	selected.ID, selected.Host = 13, "chosen.example"
	account.ProxyPool = append(account.ProxyPool, AccountProxyPoolEntry{ProxyID: 13, Concurrency: 1, Proxy: &selected})
	calls := 0
	upstream := accountTestMetricsUpstream{call: func(r *http.Request, proxy string) (*http.Response, error) {
		calls++
		require.Equal(t, selected.URL(), proxy)
		if calls == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"url":"https://images.example/result.png","revised_prompt":"A prepared image"}]}`))}, nil
	}}
	svc := &AccountTestService{accountRepo: &accountTestProxyRepo{account: account}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
	err := svc.TestAccountConnection(c, account.ID, "grok-imagine-image", "test", AccountTestModeGrokImage, AccountTestOptions{ProxyID: &selected.ID})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	events := accountTestEvents(t, recorder.Body.String())
	require.Equal(t, selected.ID, *events[0].ProxyID)
	metrics := events[len(events)-2]
	require.Equal(t, "test_metrics", metrics.Type)
	require.NotNil(t, metrics.LatencyMs)
	require.NotNil(t, metrics.DurationMs)
	require.Nil(t, metrics.FirstTokenMs)
	require.Equal(t, int64(11), *account.ProxyID)
}

func TestAccountTestMetricsIgnoreDiagnosticContentAndWaitForAllEndpoints(t *testing.T) {
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	m := newAccountTestMetrics(func() time.Time { return now })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(context.WithValue(context.Background(), accountTestMetricsContextKey{}, m))
	svc := &AccountTestService{}
	m.beginRequest()
	now = now.Add(25 * time.Millisecond)
	m.receivedResponse()
	svc.sendEvent(c, TestEvent{Type: "content", Text: "realtime handshake ok"})
	svc.sendEvent(c, TestEvent{Type: "status", Text: "checking next endpoint"})
	svc.sendEvent(c, TestEvent{Type: "image", ImageURL: "https://example/image.png"})
	c.Set(accountTestSuppressCompletionContextKey, true)
	svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	require.NotContains(t, recorder.Body.String(), "test_metrics")
	now = now.Add(100 * time.Millisecond)
	m.beginRequest()
	m.receivedResponse()
	c.Set(accountTestSuppressCompletionContextKey, false)
	svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	events := accountTestEvents(t, recorder.Body.String())
	metrics := events[len(events)-2]
	require.Equal(t, int64(25), *metrics.LatencyMs)
	require.Equal(t, int64(125), *metrics.DurationMs)
	require.Nil(t, metrics.FirstTokenMs)
}

func TestAccountTestMetricsBedrockAndSTTOnlyMeasureRealText(t *testing.T) {
	for _, platform := range []string{"bedrock", "stt"} {
		for _, withText := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/text=%t", platform, withText), func(t *testing.T) {
				account := &Account{ID: 5, Platform: PlatformAnthropic, Type: AccountTypeBedrock, Credentials: map[string]any{"auth_mode": "apikey", "api_key": "test-key"}}
				body := `{"content":[]}`
				if withText {
					body = `{"content":[{"text":"actual answer"}]}`
				}
				if platform == "stt" {
					account.Platform, account.Type = PlatformGrok, AccountTypeAPIKey
					body = `{"text":""}`
					if withText {
						body = `{"text":"actual transcript"}`
					}
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
				svc := &AccountTestService{accountRepo: &accountTestProxyRepo{account: account}, httpUpstream: upstream}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
				mode := AccountTestModeDefault
				if platform == "stt" {
					mode = AccountTestModeGrokSTT
				}
				err := svc.TestAccountConnection(c, account.ID, "claude-sonnet-4-6", "", mode)
				require.NoError(t, err)
				events := accountTestEvents(t, recorder.Body.String())
				metrics := events[len(events)-2]
				require.Equal(t, "test_metrics", metrics.Type)
				require.Equal(t, withText, metrics.FirstTokenMs != nil)
				require.NotNil(t, metrics.LatencyMs)
			})
		}
	}
}

func TestAccountTestMetricsAntigravityObservesFirstTextWhileBuffering(t *testing.T) {
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	m := newAccountTestMetrics(func() time.Time { return now })
	m.beginRequest()
	now = now.Add(20 * time.Millisecond)
	m.receivedResponse()
	chunks := []accountTestTimedChunk{
		{30 * time.Millisecond, "data: {\"response\":{\"candidates\":[]}}\n\n"},
		{50 * time.Millisecond, "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}}\n\n"},
		{100 * time.Millisecond, "data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\"}]}}\n\n"},
	}
	var expected string
	for _, chunk := range chunks {
		expected += chunk.text
	}
	ctx := context.WithValue(context.Background(), accountTestMetricsContextKey{}, m)
	body, err := readAccountTestGeminiBody(ctx, &accountTestTimedReader{now: &now, chunks: chunks}, 200)
	require.NoError(t, err)
	require.Equal(t, expected, string(body))
	require.Equal(t, "hello", extractTextFromSSEResponse(body))
	require.Equal(t, int64(100), *m.firstTokenMs)
	require.Equal(t, int64(200), now.Sub(m.startedAt).Milliseconds())
}
