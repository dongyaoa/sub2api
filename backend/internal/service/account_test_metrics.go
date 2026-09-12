package service

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

type accountTestMetricsContextKey struct{}

// A multi-endpoint test reports the first upstream response and first actual
// text output from its first dispatch, plus the duration of the whole test.
// Preparation/log events and media-only results never manufacture a text TTFT.
type accountTestMetrics struct {
	mu           sync.Mutex
	now          func() time.Time
	startedAt    time.Time
	dispatchedAt time.Time
	latencyMs    *int64
	firstTokenMs *int64
	emitted      bool
}

func newAccountTestMetrics(now func() time.Time) *accountTestMetrics {
	return &accountTestMetrics{now: now, startedAt: now()}
}

func accountTestMetricsFromContext(ctx context.Context) *accountTestMetrics {
	if ctx == nil {
		return nil
	}
	m, _ := ctx.Value(accountTestMetricsContextKey{}).(*accountTestMetrics)
	return m
}

func accountTestMetricsFromGin(c *gin.Context) *accountTestMetrics {
	if c == nil || c.Request == nil {
		return nil
	}
	return accountTestMetricsFromContext(c.Request.Context())
}

func (m *accountTestMetrics) beginRequest() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dispatchedAt.IsZero() {
		m.dispatchedAt = m.now()
	}
}

func (m *accountTestMetrics) receivedResponse() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.latencyMs == nil && !m.dispatchedAt.IsZero() {
		ms := m.now().Sub(m.dispatchedAt).Milliseconds()
		m.latencyMs = &ms
	}
}

func (m *accountTestMetrics) receivedText() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firstTokenMs == nil && !m.dispatchedAt.IsZero() {
		ms := m.now().Sub(m.dispatchedAt).Milliseconds()
		m.firstTokenMs = &ms
	}
}

func (s *AccountTestService) sendAccountTestContent(c *gin.Context, text string) {
	if text != "" {
		accountTestMetricsFromGin(c).receivedText()
	}
	s.sendEvent(c, TestEvent{Type: "content", Text: text})
}

func (s *AccountTestService) sendAccountTestMetrics(c *gin.Context) {
	m := accountTestMetricsFromGin(c)
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.emitted {
		m.mu.Unlock()
		return
	}
	m.emitted = true
	duration := m.now().Sub(m.startedAt).Milliseconds()
	event := TestEvent{Type: "test_metrics", LatencyMs: m.latencyMs, FirstTokenMs: m.firstTokenMs, DurationMs: &duration}
	m.mu.Unlock()
	s.sendEvent(c, event)
}

func beginAccountTestHTTPRequest(request *http.Request) (*http.Request, func(*http.Response)) {
	m := accountTestMetricsFromContext(request.Context())
	if m == nil {
		return request, func(*http.Response) {}
	}
	m.beginRequest()
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotFirstResponseByte: m.receivedResponse,
	}))
	return request, func(response *http.Response) {
		// Custom transports/plugins need not implement net/http tracing. Their
		// response return still establishes that response headers arrived.
		if response != nil {
			m.receivedResponse()
		}
	}
}

type accountTestMeasuredUpstream struct{ HTTPUpstream }

func accountTestUpstream(upstream HTTPUpstream) HTTPUpstream {
	if upstream == nil {
		return nil
	}
	return accountTestMeasuredUpstream{upstream}
}

func (u accountTestMeasuredUpstream) Do(request *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	request, received := beginAccountTestHTTPRequest(request)
	response, err := u.HTTPUpstream.Do(request, proxyURL, accountID, concurrency)
	received(response)
	return response, err
}

func (u accountTestMeasuredUpstream) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	request, received := beginAccountTestHTTPRequest(request)
	response, err := u.HTTPUpstream.DoWithTLS(request, proxyURL, accountID, concurrency, profile)
	received(response)
	return response, err
}

// Antigravity's test buffers the body before extracting text. Observe complete
// SSE data lines while reading so TTFT excludes the remaining response time.
func readAccountTestGeminiBody(ctx context.Context, body io.Reader, status int) ([]byte, error) {
	m := accountTestMetricsFromContext(ctx)
	if m == nil || status >= 400 {
		return io.ReadAll(body)
	}
	reader := bufio.NewReader(body)
	var output bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		_, _ = output.WriteString(line)
		if extractTextFromSSEResponse([]byte(line)) != "" {
			m.receivedText()
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return output.Bytes(), err
		}
	}
}
