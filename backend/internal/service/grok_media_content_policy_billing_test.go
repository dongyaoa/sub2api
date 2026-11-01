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

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardGrokMediaContentPolicyBilling(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		endpoint GrokMediaEndpoint
		status   int
		body     string
	}{
		{"image 400", GrokMediaEndpointImagesGenerations, http.StatusBadRequest, `{"error":{"message":"Generated image rejected by content moderation"}}`},
		{"video 400", GrokMediaEndpointVideosGenerations, http.StatusBadRequest, `{"error":{"message":"Generated video rejected by content moderation"}}`},
		{"image edit 403", GrokMediaEndpointImagesEdits, http.StatusForbidden, `{"error":{"message":"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE_IMAGE"}}`},
		{"video edit 403", GrokMediaEndpointVideosEdits, http.StatusForbidden, `{"error":{"message":"content violates usage guidelines"}}`},
		{"image 200 refusal", GrokMediaEndpointImagesGenerations, http.StatusOK, `{"output_text":"I’m sorry, I cannot help with that request."}`},
		{"video 200 refusal", GrokMediaEndpointVideosGenerations, http.StatusOK, `{"output_text":"I can't help with this request."}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"grok-imagine-image","prompt":"draw something","image":{"url":"https://images.test/input.png"},"video":{"url":"https://videos.test/input.mp4"}}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tc.status,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Request-Id": []string{"policy-media-request"},
				},
				Body: io.NopCloser(strings.NewReader(tc.body)),
			}}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo}
			result, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaPolicyAccount(), tc.endpoint, "", body, "application/json")
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "a refused generation must not be retried")
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.Zero(t, result.ImageCount)
			require.Zero(t, result.VideoCount)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.body, recorder.Body.String(), "return the upstream refusal verbatim")
			require.Zero(t, repo.tempUnschedCalls)
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestForwardGrokVideoContentPolicyUsesTaskBillingIdentity(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []GrokMediaEndpoint{GrokMediaEndpointVideoStatus, GrokMediaEndpointVideoContent} {
		for _, upstreamID := range []string{"first-poll", "second-poll"} {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-123", nil)
			body := `{"error":{"message":"Generated video rejected by content moderation"}}`
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{upstreamID}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			result, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaPolicyAccount(), endpoint, "task-123", nil, "application/json")
			require.Error(t, err)
			require.NotNil(t, result)
			require.True(t, result.GrokContentPolicyRejection)
			require.Equal(t, "grok-video:content-policy:task-123", result.RequestID)
			require.True(t, isForcedUsageBillingRequestID(result.RequestID))
			require.NotEqual(t, StableGrokVideoBillingRequestID("task-123"), result.RequestID)
			require.NotEmpty(t, result.Model)
			require.Equal(t, body, recorder.Body.String())
		}
	}
}

func TestForwardGrokVoiceContentPolicyProducesBillableResultWithoutAudio(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusForbidden, http.StatusOK} {
		body := `{"output_text":"I can't help with that request.","error":{"message":"I can't help with that request."}}`
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/tts", nil)
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"voice-refusal"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		result, err := svc.ForwardGrokVoice(context.Background(), c, grokMediaPolicyAccount(), "tts", []byte(`{"text":"hello"}`), "application/json")
		require.Error(t, err)
		require.NotNil(t, result)
		require.True(t, result.GrokContentPolicyRejection)
		require.Nil(t, result.AudioUsage)
		require.Equal(t, status, recorder.Code)
		require.Equal(t, body, recorder.Body.String())
	}
}

func grokMediaPolicyAccount() *Account {
	return &Account{
		ID: 4721, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://xai.test/v1"},
	}
}
