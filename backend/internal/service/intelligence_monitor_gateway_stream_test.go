//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const intelligenceGatewayCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_iq\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"<html>done</html>\"}]}],\"usage\":{\"input_tokens\":11,\"output_tokens\":23}}}\n\n"

func intelligenceStreamingGatewayContext(t *testing.T, trusted bool, timeout time.Duration) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	worker, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if trusted {
		key := &APIKey{ID: 72, Key: "monitor-secret"}
		request := newIntelligenceLocalPermitRequest(t, worker, key)
		bound, release := BindIntelligenceLocalRequest(request, key)
		t.Cleanup(release)
		c.Request = request.WithContext(bound)
	} else {
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(worker)
		// Publicly forgeable headers and a client deadline cannot bypass guards.
		c.Request.Header.Set("User-Agent", "Sub2API-IntelligenceMonitor/1")
		c.Request.Header.Set(intelligenceLocalRequestHeader, strings.Repeat("a", 64))
	}
	return c, recorder
}

func TestIntelligenceLocalStreamingGatewayGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"responses_first_output", "responses_idle", "chat_idle"} {
		for _, trusted := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "_ordinary", true: "_trusted"}[trusted], func(t *testing.T) {
				t.Parallel()
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
				if mode == "responses_first_output" {
					cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
				} else {
					cfg.Gateway.StreamDataIntervalTimeout = 1
				}
				svc := &OpenAIGatewayService{cfg: cfg}
				c, recorder := intelligenceStreamingGatewayContext(t, trusted, 5*time.Second)
				reader, writer := io.Pipe()
				defer reader.Close()
				defer writer.Close()
				stop := context.AfterFunc(c.Request.Context(), func() { _ = reader.CloseWithError(c.Request.Context().Err()) })
				defer stop()
				go func() {
					defer writer.Close()
					_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_iq\"}}\n\n")
					// The first tick may fall just short of a full idle interval
					// after response.created. Allow the second tick to fire.
					time.Sleep(2250 * time.Millisecond)
					_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<html>done</html>\"}\n\n"+intelligenceGatewayCompleted)
				}()
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
				var usage *OpenAIUsage
				var err error
				if mode == "chat_idle" {
					result, readErr := svc.handleChatStreamingResponse(resp, c, account, "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now(), 0)
					err = readErr
					if result != nil {
						usage = &result.Usage
					}
				} else {
					result, readErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
					err = readErr
					if result != nil {
						usage = result.usage
					}
				}
				if !trusted {
					require.Error(t, err)
					if mode == "responses_first_output" {
						var failover *UpstreamFailoverError
						require.ErrorAs(t, err, &failover)
						require.Contains(t, string(failover.ResponseBody), "first_output_timeout")
					} else {
						require.ErrorContains(t, err, "stream data interval timeout")
					}
					return
				}
				require.NoError(t, err)
				require.NoError(t, c.Request.Context().Err())
				require.NotNil(t, usage)
				require.Equal(t, 11, usage.InputTokens)
				require.Equal(t, 23, usage.OutputTokens)
				require.Contains(t, recorder.Body.String(), "done")
			})
		}
	}
}

type intelligenceGatewayHTTPUpstream struct {
	HTTPUpstream
	do func(*http.Request) (*http.Response, error)
}

func (u *intelligenceGatewayHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

func (u *intelligenceGatewayHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.do(req)
}

func TestIntelligenceLocalStreamingHeaderWaitUsesWorkerDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "trusted"}[trusted], func(t *testing.T) {
			t.Parallel()
			c, recorder := intelligenceStreamingGatewayContext(t, trusted, 5*time.Second)
			upstream := &intelligenceGatewayHTTPUpstream{do: func(req *http.Request) (*http.Response, error) {
				if trusted {
					deadline, ok := req.Context().Deadline()
					require.True(t, ok)
					workerDeadline, _ := c.Request.Context().Deadline()
					require.Equal(t, workerDeadline, deadline)
					require.Equal(t, 15*time.Minute, HTTPUpstreamResponseHeaderTimeoutFromContext(req.Context()))
				}
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(1250 * time.Millisecond):
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(intelligenceGatewayCompleted))}, nil
				}
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIFirstOutputTimeoutSeconds: 1, MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream}
			body := []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`)
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			account := &Account{ID: 1, Name: "api-key-test", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "test-key"}}
			result, err := svc.Forward(c.Request.Context(), c, account, body)
			if !trusted {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Contains(t, string(failover.ResponseBody), "first_output_timeout")
				require.Empty(t, recorder.Body.String())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 23, result.Usage.OutputTokens)
			require.Contains(t, recorder.Body.String(), "response.completed")
		})
	}
}

func TestIntelligenceLocalStreamingWorkerDeadlineStillCancelsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := intelligenceStreamingGatewayContext(t, true, 100*time.Millisecond)
	deadline, ok := c.Request.Context().Deadline()
	require.True(t, ok)
	called := false
	upstream := &intelligenceGatewayHTTPUpstream{do: func(req *http.Request) (*http.Response, error) {
		called = true
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIFirstOutputTimeoutSeconds: 1, MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err := svc.Forward(c.Request.Context(), c, account, body)
	require.True(t, called)
	require.Error(t, err)
	// The worker's AfterFunc may cancel the bound context before its own
	// deadline timer runs. Both paths must stop the upstream at the deadline.
	require.False(t, time.Now().Before(deadline))
	require.Error(t, c.Request.Context().Err())
}

func TestIntelligenceLocalFirstOutputOverrideDoesNotAffectDirectOAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx = context.WithValue(ctx, boundUpstreamLifecycleContextKey{}, true)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	require.Equal(t, 30*time.Second, intelligenceMonitorFirstOutputTimeout(c, 30*time.Second))
}
