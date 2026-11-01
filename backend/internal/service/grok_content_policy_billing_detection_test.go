//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokContentPolicyRefusalText(t *testing.T) {
	for _, phrase := range []string{
		"sorry, i can't help with", "sorry, i cannot help with",
		"i'm sorry, i can't help with", "i'm sorry, i cannot help with",
		"i can't help with", "i cannot help with",
		"sorry, i c***t help with", "sorry, i ca*** help with",
		"i'm sorry, i c***t help with", "i'm sorry, i ca*** help with",
		"i c***t help with", "i ca*** help with",
	} {
		for _, suffix := range []string{"that", "this", "you", "your", "the request", "the prompt", "it", "thatSuffix", "items"} {
			t.Run(phrase+" "+suffix, func(t *testing.T) {
				text := strings.ReplaceAll(strings.ToUpper(phrase), "'", "’") + " \n" + suffix
				require.True(t, grokContentPolicyRefusalText(text))
			})
		}
	}
	for _, text := range []string{
		"I can help with that", "I can't help with", "I can't help with a request",
		"I cannot help with those", "I cannot help with the image", "I can't help with: that",
	} {
		require.False(t, grokContentPolicyRefusalText(text), text)
	}
	require.True(t, grokContentPolicyRefusalText("I can't help with a request, but I can't help with this either"))
}

func TestGrokBillableContentPolicyStatusRules(t *testing.T) {
	for _, phrase := range []string{
		"content violates usage guidelines", "SAFETY_CHECK_TYPE_HARMFUL", "content moderation", "content policy",
		"image is sensitive", "text is sensitive", "prohibited content", "forbidden content",
		"content policy violation", "content policy rejection", "content policy rejected",
		"content moderation rejection", "content moderation rejected", "content moderation blocked",
		"request blocked by content moderation", "request rejected by content moderation",
		"request blocked by policy", "request rejected by policy", "request violates policy",
		"prompt violates content policy", "prompt violates policy", "input violates content policy", "input violates policy",
		"I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE",
	} {
		t.Run(phrase, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"error": map[string]string{"message": strings.ToUpper(phrase)}})
			require.NoError(t, err)
			require.True(t, isGrokBillableContentPolicyRejection(http.StatusForbidden, body))
			require.True(t, isGrokContentPolicyRejection(http.StatusForbidden, body))
			require.False(t, (&OpenAIGatewayService{}).shouldFailoverGrokUpstreamError(http.StatusForbidden, body))
			for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusBadGateway} {
				require.False(t, isGrokBillableContentPolicyRejection(status, body), "status=%d", status)
			}
		})
	}
	for _, phrase := range []string{"generated image rejected by content moderation", "generated video rejected by content moderation"} {
		body := []byte(fmt.Sprintf(`{"error":{"message":%q}}`, strings.ToUpper(phrase)))
		require.True(t, isGrokBillableContentPolicyRejection(http.StatusBadRequest, body))
		require.True(t, isGrokContentPolicyRejection(http.StatusBadRequest, body))
		require.False(t, (&OpenAIGatewayService{}).shouldFailoverGrokUpstreamError(http.StatusBadRequest, body))
	}
	for status := 200; status < 300; status++ {
		require.True(t, isGrokBillableContentPolicyRejection(status, []byte("I can’t help with that request.")))
		require.False(t, isGrokBillableContentPolicyRejection(status, []byte("content moderation")))
	}
	require.True(t, isGrokBillableContentPolicyRejection(http.StatusForbidden, []byte(`{"response":{"error":{"message":"I can\u2019t help with this"}}}`)))
}

func TestGrokBillableContentPolicyExclusions(t *testing.T) {
	for _, message := range []string{
		"account suspended due to content policy", "account is banned due to content moderation",
		"subscription required: content policy", "insufficient entitlement: content policy", "not eligible: content moderation",
		"quota exceeded: content policy", "spending limit exceeded: content moderation", "included free usage exhausted: content policy",
		"checking your browser: content policy", "verify you are human: content moderation",
		"<html><title>Just a moment</title><div>Content policy</div></html>",
		`{"cloudflare_error":true,"code":1010,"message":"content policy"}`,
		`{"error":{"code":"account_suspended","message":"content policy"}}`,
		`{"error":{"code":"quota_exceeded","message":"content policy"}}`,
		"The moderation feature is not available for this request",
	} {
		require.False(t, isGrokBillableContentPolicyRejection(http.StatusForbidden, []byte(message)), message)
	}
	// Preserve legacy no-retry treatment without charging for bare policy codes.
	for _, body := range []string{
		`{"error":{"code":"new_sensitive"}}`, `{"error":{"code":"cyber_policy"}}`,
		`{"error":{"code":"content_filter"}}`, "The moderation feature is not available for this request",
	} {
		require.True(t, isGrokContentPolicyRejection(http.StatusForbidden, []byte(body)), body)
		require.False(t, isGrokBillableContentPolicyRejection(http.StatusForbidden, []byte(body)), body)
	}
	require.False(t, isGrokBillableContentPolicyRejection(http.StatusOK, []byte("I can't help with this because your account is suspended")))
	for _, body := range []string{
		`{"error":{"code":"account_suspended","message":"I can't help with that"}}`,
		`{"code":"quota_exceeded","error":"I can't help with that"}`,
		`{"type":"response.failed","response":{"error":{"code":"not_entitled","message":"I can't help with that"}}}`,
		`{"cloudflare_error":true,"code":1010,"error":"I can't help with that"}`,
	} {
		require.False(t, isGrokBillableContentPolicyRejection(http.StatusOK, []byte(body)), body)
	}
}

func TestGrokBillableContentPolicyOnlyInspectsModelOutput(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"role":"assistant","content":"I can\u2019t help with that"}}]}`,
		`{"choices":[{"delta":{"content":"I cannot help with this"}}]}`,
		`{"choices":[{"message":{"role":"assistant","refusal":"I can't help with the request"}}]}`,
		`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I can't help with your prompt"}]}]}`,
		`{"response":{"output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"I can't help with that"}]}]}}`,
		`{"role":"assistant","content":[{"type":"text","text":"I cannot help with it"}]}`,
		`{"type":"response.output_text.delta","delta":"I can't help with this"}`,
		`{"type":"response.refusal.delta","delta":"I cannot help with the prompt"}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"I cannot help with you"}}`,
		`{"type":"response.failed","response":{"error":{"message":"I can't help with that"}}}`,
		`{"input":[{"error":{"code":"account_suspended"}}],"metadata":{"error":{"code":"quota_exceeded"}},"output_text":"I can't help with that"}`,
	} {
		require.True(t, isGrokBillableContentPolicyRejection(http.StatusOK, []byte(body)), body)
	}
	for _, body := range []string{
		`{"input":[{"role":"user","content":"I can't help with that"}],"output_text":"Hello"}`,
		`{"metadata":{"hint":"I can't help with that"},"choices":[{"message":{"content":"Hello"}}]}`,
		`{"output":[{"type":"message","role":"user","content":[{"type":"output_text","text":"I can't help with that"}]}]}`,
		`{"choices":[{"message":{"role":"user","content":"I can't help with that"}}]}`,
		`{"output":[{"type":"function_call","arguments":"I can't help with that"}]}`,
		`{"choices":[{"message":{"reasoning_content":"I can't help with that","content":"Hello"}}]}`,
		`{"type":"response.function_call_arguments.delta","delta":"I can't help with this"}`,
		`{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"I cannot help with this"}}`,
		`{"choices":[{"message":{"content":"I cannot help with "}},{"message":{"content":"that"}}]}`,
		`{"output":[{"type":"message","content":[{"type":"output_text","text":"I cannot help with "}]},{"type":"message","content":[{"type":"output_text","text":"that"}]}]}`,
		`{"output":[{"type":"message","content":[{"type":"output_text","text":"I cannot help with "},{"type":"output_text","text":"that"}]}]}`,
		`{"input":"I cannot help with this`,
		`[{"metadata":"I cannot help with this`,
	} {
		require.False(t, isGrokBillableContentPolicyRejection(http.StatusOK, []byte(body)), body)
	}
}

func TestGrokContentPolicyOutputTextSpansStreamDeltas(t *testing.T) {
	body := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"I'm sorry, I can\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"’t help with th\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"at request.\"}\n\n" +
		"data: [DONE]\n\n"
	require.Equal(t, "I'm sorry, I can’t help with that request.", grokContentPolicyOutputText([]byte(body)))
	require.True(t, isGrokBillableContentPolicyRejection(http.StatusOK, []byte(body)))
}
