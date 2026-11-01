//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokComposerImageBridgeRefusalStopsAndBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		priorSuccess bool
		stream       bool
	}{
		{name: "400", status: 400, body: `{"error":{"message":"Generated image rejected by content moderation"}}`},
		{name: "403", status: 403, body: `{"error":{"message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE_IMAGE"}}`},
		{name: "200", status: 200, body: `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I cannot help with this request."}]}],"usage":{"input_tokens":14,"output_tokens":8}}`},
		{name: "second image", status: 200, priorSuccess: true, stream: true, body: `{"output_text":"Sorry, I can't help with your request."}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := "false"
			if tc.stream {
				stream = "true"
			}
			body := []byte(`{"model":"grok-composer-2.5-fast","stream":` + stream + `,"messages":[{"role":"user","content":[{"type":"text","text":"Describe both images"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}},{"type":"image_url","image_url":{"url":"data:image/png;base64,REVG"}}]}]}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Set("api_key", &APIKey{ID: 5501})
			account := healthyGrokOAuthGatewayTestAccount(55, "access-token")
			repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
				accountsByID: map[int64]*Account{55: account},
			}}
			newResponse := func(status int, data string) *http.Response {
				return &http.Response{
					StatusCode: status,
					Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"vision-refusal"}},
					Body:       io.NopCloser(strings.NewReader(data)),
				}
			}
			upstream := &httpUpstreamRecorder{resp: newResponse(200, `{"output_text":"unexpected continuation"}`)}
			if tc.priorSuccess {
				upstream.responses = append(upstream.responses, newResponse(200, `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"A diagram with ABC letters."}]}],"usage":{"input_tokens":20,"output_tokens":10}}`))
			}
			upstream.responses = append(upstream.responses, newResponse(tc.status, tc.body))
			svc := &OpenAIGatewayService{
				cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream,
				grokTokenProvider: NewGrokTokenProvider(repo, nil), accountRepo: repo,
			}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.Equal(t, "grok-composer-2.5-fast", result.Model)
			require.Equal(t, grokComposerImageBridgeVisionModel, result.UpstreamModel)
			require.Equal(t, openAIResponsesUpstreamEndpoint, result.UpstreamEndpoint)
			require.Equal(t, tc.stream, result.Stream)
			require.Equal(t, OpenAIUsage{}, result.Usage, "refusal fee must not include earlier image-description tokens")
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.body, recorder.Body.String())
			wantCalls := 1
			if tc.priorSuccess {
				wantCalls++
			}
			require.Len(t, upstream.requests, wantCalls, "do not describe more images or submit Composer after a refusal")
			for _, sent := range upstream.bodies {
				require.Equal(t, grokComposerImageBridgeVisionModel, gjson.GetBytes(sent, "model").String())
			}
			require.Zero(t, repo.tempUnschedCalls)
			require.Zero(t, repo.rateLimitedCalls)
		})
	}
}
