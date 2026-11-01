//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func grokContentPolicyTextForwardFixture(t *testing.T, protocol string, stream bool, status int, upstreamBody string, requestOverride string) (*OpenAIForwardResult, error, *httptest.ResponseRecorder, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	account := rawChatCompletionsTestAccount()
	account.Platform = PlatformGrok
	account.Credentials["base_url"] = "https://api.x.ai/v1"
	body := []byte(fmt.Sprintf(`{"model":"grok-4.5","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
	path := "/v1/chat/completions"
	if protocol == "responses" {
		path = "/v1/responses"
		body = []byte(fmt.Sprintf(`{"model":"grok-4.5","stream":%t,"input":"hi"}`, stream))
	}
	if requestOverride != "" {
		body = []byte(requestOverride)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	contentType := "application/json"
	if status == http.StatusOK && (stream || strings.HasPrefix(upstreamBody, "data:") || strings.HasPrefix(upstreamBody, "event:")) {
		contentType = "text/event-stream"
	}
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{contentType},
			"X-Request-Id": []string{"grok-refusal-forward"},
		},
		Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(upstreamBody))),
	}}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	var result *OpenAIForwardResult
	var err error
	if protocol == "responses" {
		result, err = svc.forwardGrokResponses(context.Background(), c, account, body, "grok-4.5", stream, time.Now())
	} else {
		result, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	}
	return result, err, recorder, upstream.callCount
}

func TestGrokContentPolicyTextForwardSuccess(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, refusal := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/refusal=%t", protocol, stream, refusal), func(t *testing.T) {
					output := "Hello there."
					if refusal {
						output = "I'm sorry, I can’t help with that request."
					}
					runes := []rune(output)
					first, second := string(runes[:len(runes)/2]), string(runes[len(runes)/2:])
					var body string
					if protocol == "chat" {
						if stream {
							// Deliberately split the refusal phrase and the curly quote
							// across network reads and multiple semantic SSE chunks.
							body = fmt.Sprintf("data: {\"id\":\"chat_1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", first) +
								fmt.Sprintf("data: {\"id\":\"chat_1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", second) +
								"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n"
						} else {
							body = fmt.Sprintf(`{"id":"chat_1","model":"grok-4.5","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`, output)
						}
					} else if stream {
						body = fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":%q}\n\n", first) +
							fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":%q}\n\n", second) +
							"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"grok-4.5\",\"status\":\"completed\",\"usage\":{\"input_tokens\":7,\"output_tokens\":3}}}\n\n"
					} else {
						body = fmt.Sprintf(`{"id":"resp_1","object":"response","model":"grok-4.5","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":7,"output_tokens":3}}`, output)
					}
					result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, protocol, stream, http.StatusOK, body, "")
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, refusal, result.GrokContentPolicyRejection)
					require.Equal(t, 1, calls)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, 7, result.Usage.InputTokens)
					require.Equal(t, 3, result.Usage.OutputTokens)
					require.Contains(t, recorder.Body.String(), second)
				})
			}
		}
	}
}

func TestGrokContentPolicyTextForwardHTTPErrorPassesThroughWithoutRetry(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, status := range []int{http.StatusBadRequest, http.StatusForbidden} {
			t.Run(fmt.Sprintf("%s/%d", protocol, status), func(t *testing.T) {
				message := "I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"
				if status == http.StatusBadRequest {
					message = "Generated image rejected by content moderation"
				}
				body := fmt.Sprintf("{\n  \"error\": {\"message\": %q, \"type\": \"upstream_original_type\"}\n}", message)
				result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, protocol, false, status, body, "")
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.NotNil(t, result)
				require.True(t, result.GrokContentPolicyRejection)
				require.Equal(t, "grok-refusal-forward", result.RequestID)
				require.Equal(t, 1, calls)
				require.Equal(t, status, recorder.Code)
				require.Equal(t, body, recorder.Body.String())
			})
		}
	}
}

func TestGrokContentPolicyTextForwardRefusalOverridesReplayRetry(t *testing.T) {
	body := `{"error":{"code":"invalid_encrypted_content","message":"Generated image rejected by content moderation"}}`
	request := `{"model":"grok-4.5","stream":false,"input":[{"type":"reasoning","encrypted_content":"stale","summary":[]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`
	result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, "responses", false, http.StatusBadRequest, body, request)
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.NotNil(t, result)
	require.True(t, result.GrokContentPolicyRejection)
	require.Equal(t, 1, calls)
	require.Equal(t, body, recorder.Body.String())
}

func TestGrokContentPolicyRawChatRefusalWithoutUsage(t *testing.T) {
	body := `{"id":"chat_refused","model":"grok-4.5","choices":[{"message":{"role":"assistant","content":"I cannot help with that request."},"finish_reason":"stop"}]}`
	result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, "chat", false, http.StatusOK, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.GrokContentPolicyRejection)
	require.Equal(t, 1, calls)
	require.Equal(t, body, recorder.Body.String())
}

func TestGrokContentPolicyBufferedResponsesSSEFailure(t *testing.T) {
	body := "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"I cannot help with that request.\"}}}\n\n"
	result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, "responses", false, http.StatusOK, body, "")
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.NotNil(t, result)
	require.True(t, result.GrokContentPolicyRejection)
	require.Equal(t, 1, calls)
	require.Equal(t, body, recorder.Body.String())
}

func TestGrokContentPolicyTextForwardEmbeddedRefusalStopsFailover(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			body := "data: {\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"I cannot help with that request.\"}}\n\n"
			if protocol == "responses" {
				body = "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_refused\",\"status\":\"failed\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"I cannot help with that request.\"}}}\n\n"
			}
			result, err, recorder, calls := grokContentPolicyTextForwardFixture(t, protocol, true, http.StatusOK, body, "")
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.Equal(t, 1, calls)
			require.Contains(t, recorder.Body.String(), "I cannot help with that request.")
			require.Contains(t, recorder.Body.String(), "server_is_overloaded")
		})
	}
}

func TestGrokContentPolicyObserverSeparatesOutputsAndExcludesAccountFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"split refusal", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"I can\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"’t help with that\"}}]}\n\n", true},
		{"separate choices", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"I cannot help with \"}}]}\n\ndata: {\"choices\":[{\"index\":1,\"delta\":{\"content\":\"that\"}}]}\n\n", false},
		{"separate output items", "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"I cannot help with \"}\n\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":1,\"delta\":\"that\"}\n\n", false},
		{"separate content blocks", "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"I cannot help with \"}}\n\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"that\"}}\n\n", false},
		{"named event without type", "event: response.output_text.delta\ndata: {\"delta\":\"I cannot help with that\"}\n\n", true},
		{"structured account failure", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"account_suspended\",\"message\":\"I cannot help with that\"}}}\n\n", false},
		{"quota error", "data: {\"type\":\"error\",\"error\":{\"code\":\"quota_exceeded\",\"message\":\"I cannot help with that\"}}\n\n", false},
		{"input account marker ignored", "data: {\"type\":\"response.completed\",\"response\":{\"input\":[{\"error\":{\"code\":\"account_suspended\"}}],\"output_text\":\"I cannot help with that\"}}\n\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &grokContentPolicyBody{ReadCloser: io.NopCloser(iotest.OneByteReader(strings.NewReader(test.body))), stream: true, limit: 1 << 20, tails: make(map[string]string)}
			got, err := io.ReadAll(body)
			require.NoError(t, err)
			require.Equal(t, test.body, string(got), "observation must not change forwarded bytes")
			require.Equal(t, test.want, body.rejected())
		})
	}
}
