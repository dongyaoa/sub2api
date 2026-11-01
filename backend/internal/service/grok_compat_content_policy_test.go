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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokCompatContentPolicyWithoutUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{"chat", "messages"} {
		for _, stream := range []bool{false, true} {
			for _, shape := range []string{"completed", "delta_only", "failed_overload", "failed_cyber", "bare_error"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", protocol, stream, shape), func(t *testing.T) {
					message := "I'm sorry, I can’t help with that request. https://x.ai/original"
					var sse string
					wantError := false
					switch shape {
					case "completed":
						sse = fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"refused\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":%q}]}]}}\n\n", message)
					case "delta_only":
						sse = "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"I'm sorry, I can\"}\n\n" +
							"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"’t help with that request. https://x.ai/original\"}\n\n" +
							"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"refused\",\"status\":\"completed\",\"output\":[]}}\n\n"
					case "failed_overload", "failed_cyber":
						code := "server_is_overloaded"
						if shape == "failed_cyber" {
							code = "cyber_policy"
						}
						sse = fmt.Sprintf("data: {\"type\":\"response.failed\",\"response\":{\"id\":\"refused\",\"status\":\"failed\",\"error\":{\"code\":%q,\"message\":%q}}}\n\n", code, message)
						wantError = true
					case "bare_error":
						sse = fmt.Sprintf("data: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":%q}}\n\n", message)
						wantError = true
					}
					body := []byte(fmt.Sprintf(`{"model":"grok","stream":%t,"messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"policy-session","max_tokens":256}`, stream))
					path := "/v1/chat/completions"
					if protocol == "messages" {
						path = "/v1/messages"
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					c.Set("api_key", &APIKey{ID: 9916})
					account := grokChatBridgeTestAccount(9917)
					repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
						accountsByID: map[int64]*Account{account.ID: account},
					}}
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"compat-refusal"}},
						Body:       io.NopCloser(iotest.OneByteReader(strings.NewReader(sse))),
					}}
					svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, nil)}
					var result *OpenAIForwardResult
					var err error
					if protocol == "messages" {
						result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
					} else {
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					}
					if wantError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					var failover *UpstreamFailoverError
					require.False(t, errors.As(err, &failover))
					require.NotNil(t, result)
					require.True(t, result.GrokContentPolicyRejection)
					require.Nil(t, GetOpsCyberPolicy(c), "Grok violation fees must not enter the separate cyber billing path")
					require.Len(t, upstream.requests, 1)
					require.Contains(t, upstream.lastReq.URL.Path, "responses")
					require.Equal(t, http.StatusOK, recorder.Code)
					if stream && shape == "delta_only" {
						require.Contains(t, recorder.Body.String(), "I'm sorry, I can")
						require.Contains(t, recorder.Body.String(), "’t help with that request. https://x.ai/original")
					} else {
						require.Contains(t, recorder.Body.String(), message)
					}
					if !stream && wantError {
						require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
					}
					require.Zero(t, repo.tempUnschedCalls)
					require.Zero(t, repo.rateLimitedCalls)
				})
			}
		}
	}
}
