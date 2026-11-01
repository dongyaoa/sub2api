package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// isGrokContentPolicyRejection identifies request-scoped safety refusals from
// xAI. These failures are caused by the prompt or media, so retrying another
// OAuth account cannot change the outcome and would incorrectly drain a pool.
// Keep this matcher deliberately narrow: account entitlement and suspension
// messages may mention policy but must retain the normal account failover path.
func isGrokContentPolicyRejection(statusCode int, responseBody []byte) bool {
	if isGrokBillableContentPolicyRejection(statusCode, responseBody) {
		return true
	}
	if statusCode != http.StatusForbidden || len(responseBody) == 0 {
		return false
	}
	if grokContentPolicyAccountOrBrowserFailure(responseBody) {
		return false
	}

	var payload any
	if json.Unmarshal(responseBody, &payload) == nil {
		if grokStructuredAccountAccessMarker(payload) {
			return false
		}
		if grokStructuredContentPolicyMarker(payload) {
			return true
		}
	}

	return grokContentPolicyMessage(string(responseBody))
}

// isGrokBillableContentPolicyRejection applies the fixed violation-fee rules.
// The legacy no-failover classifier above also recognizes some structured
// policy codes; those codes alone are deliberately insufficient to charge.
// For successful responses only assistant output is inspected, never echoed
// request input, tool arguments, reasoning, or response metadata.
func isGrokBillableContentPolicyRejection(statusCode int, responseBody []byte) bool {
	if len(responseBody) == 0 || (statusCode < 200 || statusCode >= 300) &&
		statusCode != http.StatusBadRequest && statusCode != http.StatusForbidden {
		return false
	}
	if statusCode >= 200 && statusCode < 300 {
		if grokContentPolicyResponseFailure(responseBody) {
			return false
		}
		output := grokContentPolicyOutputText(responseBody)
		return !grokContentPolicyAccountOrBrowserFailure([]byte(output)) && grokContentPolicyRefusalText(output)
	}
	if grokContentPolicyAccountOrBrowserFailure(responseBody) {
		return false
	}
	text := normalizeGrokRefusalText(string(responseBody))
	for _, candidate := range grokStructuredErrorMessageCandidates(responseBody) {
		text += "\n" + normalizeGrokRefusalText(candidate)
	}
	if statusCode == http.StatusBadRequest {
		return strings.Contains(text, "generated image rejected by content moderation") ||
			strings.Contains(text, "generated video rejected by content moderation")
	}
	if strings.Contains(text, "the moderation feature is not available") {
		return false
	}
	if grokContentPolicyRefusalText(text) {
		return true
	}
	for _, phrase := range []string{
		"content violates usage guidelines", "safety_check_type_", "content moderation", "content policy",
		"image is sensitive", "text is sensitive", "prohibited content", "forbidden content",
		"request blocked by policy", "request rejected by policy", "request violates policy",
		"prompt violates policy", "input violates policy",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// Inspect only explicit upstream errors, not account-like strings echoed in
// input or metadata on an otherwise successful response.
func grokContentPolicyResponseFailure(body []byte) bool {
	root := gjson.ParseBytes(body)
	for _, response := range []gjson.Result{root, root.Get("response")} {
		if !response.Get("error").Exists() && response.Get("type").String() != "error" && !response.Get("cloudflare_error").Bool() {
			continue
		}
		fields := make(map[string]json.RawMessage)
		for _, field := range []string{"error", "code", "error_code", "type", "category", "reason", "message", "detail", "cloudflare_error", "error_name"} {
			if value := response.Get(field); value.Exists() {
				fields[field] = json.RawMessage(value.Raw)
			}
		}
		errorBody, err := json.Marshal(fields)
		if err == nil && grokContentPolicyAccountOrBrowserFailure(errorBody) {
			return true
		}
	}
	return false
}

func normalizeGrokRefusalText(text string) string {
	return strings.ToLower(strings.NewReplacer("’", "'", "‘", "'", "ʼ", "'").Replace(text))
}

// grokContentPolicyRefusalText consumes model output (or an error message).
// Suffix matching intentionally has no word boundary: "that...", "your...",
// and "it..." all satisfy the published rule.
func grokContentPolicyRefusalText(text string) bool {
	text = normalizeGrokRefusalText(text)
	for _, phrase := range []string{"i can't help with", "i cannot help with", "i c***t help with", "i ca*** help with"} {
		rest := text
		for {
			index := strings.Index(rest, phrase)
			if index < 0 {
				break
			}
			rest = rest[index+len(phrase):]
			suffix := strings.TrimSpace(rest)
			for _, prefix := range []string{"that", "this", "you", "your", "the request", "the prompt", "it"} {
				if strings.HasPrefix(suffix, prefix) {
					return true
				}
			}
		}
	}
	return false
}

func grokContentPolicyAccountOrBrowserFailure(body []byte) bool {
	text := normalizeGrokRefusalText(string(body))
	for _, candidate := range grokStructuredErrorMessageCandidates(body) {
		text += "\n" + normalizeGrokRefusalText(candidate)
	}
	if grokAccountAccessMessage(text) || isGrokFreeUsageExhaustedText(text) || isGrokBillingQuotaText(text) || isCloudflareBotBlockResponse(body) {
		return true
	}
	var payload any
	if json.Unmarshal(body, &payload) == nil && grokStructuredAccountAccessMarker(payload) {
		return true
	}
	for _, phrase := range []string{
		"account is suspended", "account is disabled", "account banned", "account is banned", "account has been banned",
		"user disabled", "user is suspended", "user is disabled", "user is banned", "user has been banned",
		"insufficient entitlement", "insufficient permissions", "not eligible", "does not have access", "do not have access",
		"quota exceeded", "quota exhausted", "quota limit", "usage limit reached", "usage limit exceeded", "credit limit",
		"cf-chl-", "/cdn-cgi/challenge-platform", "checking your browser", "verify you are human", "browser verification",
		"browser_signature_banned", "enable javascript and cookies to continue",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return strings.Contains(text, "<html") && (strings.Contains(text, "cloudflare") || strings.Contains(text, "just a moment"))
}

// grokContentPolicyOutputText extracts only model-visible output from common
// Responses, Chat Completions, and Messages payloads, including SSE data events.
// Callers handling streamed deltas should concatenate these strings per output
// before matching so a refusal split across chunks can still be recognized.
// Independent JSON text parts are separated by NUL to prevent joining fragments
// from different choices, content blocks, or output items into a false refusal.
func grokContentPolicyOutputText(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") {
		var output strings.Builder
		for _, line := range strings.Split(trimmed, "\n") {
			if data, ok := strings.CutPrefix(strings.TrimSpace(line), "data:"); ok && strings.TrimSpace(data) != "[DONE]" {
				output.WriteString(grokContentPolicyOutputText([]byte(data)))
			}
		}
		return output.String()
	}
	if !json.Valid(body) {
		// A truncated JSON response may contain echoed input or metadata.
		// Never reinterpret that unfinished envelope as model output.
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return ""
		}
		return trimmed
	}
	root := gjson.ParseBytes(body)
	if root.Type == gjson.String {
		return root.String()
	}
	var output strings.Builder
	appendText := func(text string) {
		if text == "" {
			return
		}
		if output.Len() > 0 {
			output.WriteByte(0)
		}
		output.WriteString(text)
	}
	var appendContent func(gjson.Result)
	appendContent = func(content gjson.Result) {
		if content.Type == gjson.String {
			appendText(content.String())
		} else if content.IsArray() {
			content.ForEach(func(_, part gjson.Result) bool {
				appendContent(part)
				return true
			})
		} else if content.IsObject() {
			if role := content.Get("role").String(); role != "" && role != "assistant" {
				return
			}
			switch content.Get("type").String() {
			case "text", "output_text":
				appendText(content.Get("text").String())
			case "refusal":
				appendText(content.Get("refusal").String())
			}
		}
	}
	appendMessage := func(message gjson.Result) {
		if role := message.Get("role").String(); role != "" && role != "assistant" {
			return
		}
		appendContent(message.Get("content"))
		appendContent(message.Get("refusal"))
	}
	root.Get("choices").ForEach(func(_, choice gjson.Result) bool {
		appendMessage(choice.Get("message"))
		appendMessage(choice.Get("delta"))
		appendContent(choice.Get("text"))
		return true
	})
	for _, response := range []gjson.Result{root, root.Get("response")} {
		// Some relays keep HTTP 200 for terminal error / response.failed
		// events. Their explicit error message is an upstream refusal, too.
		if upstreamError := response.Get("error"); upstreamError.Type == gjson.String {
			appendContent(upstreamError)
		} else {
			appendContent(upstreamError.Get("message"))
			appendContent(upstreamError.Get("error"))
		}
		appendContent(response.Get("output_text"))
		response.Get("output").ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "message" {
				appendMessage(item)
			} else {
				appendContent(item)
			}
			return true
		})
		if response.Get("role").String() == "assistant" {
			appendMessage(response)
		}
	}
	switch root.Get("type").String() {
	case "response.output_text.delta", "response.refusal.delta":
		appendContent(root.Get("delta"))
	case "response.output_text.done":
		appendContent(root.Get("text"))
	case "response.refusal.done":
		appendContent(root.Get("refusal"))
	case "response.content_part.added", "response.content_part.done":
		appendContent(root.Get("part"))
	case "response.output_item.added", "response.output_item.done":
		appendMessage(root.Get("item"))
	case "content_block_delta":
		if root.Get("delta.type").String() == "text_delta" {
			appendContent(root.Get("delta.text"))
		}
	case "content_block_start":
		appendContent(root.Get("content_block"))
	}
	return output.String()
}

func grokStructuredAccountAccessMarker(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalizedKey := normalizeGrokErrorMarker(key)
			switch normalizedKey {
			case "code", "error_code", "type", "category", "reason":
				if marker, ok := child.(string); ok && isGrokAccountAccessCode(marker) {
					return true
				}
			}
			if grokStructuredAccountAccessMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if grokStructuredAccountAccessMarker(child) {
				return true
			}
		}
	}
	return false
}

func grokStructuredContentPolicyMarker(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalizedKey := normalizeGrokErrorMarker(key)
			switch normalizedKey {
			case "code", "error_code", "type", "category", "reason":
				if marker, ok := child.(string); ok && isGrokContentPolicyCode(marker) {
					return true
				}
			}
			if grokStructuredContentPolicyMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if grokStructuredContentPolicyMarker(child) {
				return true
			}
		}
	}
	return false
}

func normalizeGrokErrorMarker(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func isGrokContentPolicyCode(value string) bool {
	switch normalizeGrokErrorMarker(value) {
	case "content_filter",
		"content_policy",
		"content_policy_violation",
		"content_moderation",
		"cyber_policy",
		"new_sensitive":
		return true
	default:
		return false
	}
}

func isGrokAccountAccessCode(value string) bool {
	switch normalizeGrokErrorMarker(value) {
	case "account_suspended",
		"account_disabled",
		"account_banned",
		"user_suspended",
		"user_disabled",
		"user_banned",
		"subscription_required",
		"entitlement_required",
		"not_entitled",
		"insufficient_entitlement",
		"insufficient_quota",
		"quota_exceeded",
		"quota_exhausted",
		"usage_limit_exceeded",
		"spending_limit_exceeded",
		"plan_required":
		// permission-denied is omitted: xAI reuses it for both entitlement
		// refusals and request-scoped safety blocks, so the message decides.
		return true
	default:
		return false
	}
}

func grokAccountAccessMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{
		"account suspended",
		"account has been suspended",
		"account disabled",
		"account has been disabled",
		"user suspended",
		"user has been suspended",
		"subscription required",
		"entitlement required",
		"not entitled",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func grokContentPolicyMessage(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}

	// xAI's media safety responses use these exact phrases. They are specific
	// enough not to classify a generic account-policy or entitlement message.
	for _, phrase := range []string{
		"the moderation feature is not available",
		"image is sensitive",
		"text is sensitive",
		"prohibited content",
		"forbidden content",
		"content policy violation",
		"content policy rejection",
		"content policy rejected",
		"content moderation rejection",
		"content moderation rejected",
		"content moderation blocked",
		"request blocked by content moderation",
		"request rejected by content moderation",
		"request blocked by policy",
		"request rejected by policy",
		"request violates policy",
		"prompt violates content policy",
		"prompt violates policy",
		"input violates content policy",
		"input violates policy",
		"violates usage guidelines",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	return false
}

func grokContentPolicyClientMessage(responseBody []byte) string {
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	if message == "" {
		return "Request blocked by upstream content policy"
	}
	return message
}

// shouldFailoverGrokUpstreamError is the body-aware counterpart of the
// status-only failover helper. Grok content refusals must stay on the current
// account and be returned to the caller instead of consuming the account pool.
// Free-usage / empty-output / billing bodies also failover even when the HTTP
// status alone would not (e.g. 400 with free-usage-exhausted).
func (s *OpenAIGatewayService) shouldFailoverGrokUpstreamError(statusCode int, responseBody []byte) bool {
	if isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
	}
	// A 422 emitted by xAI's ModelInput decoder is account/runtime compatibility,
	// not quota exhaustion. Another account may run a different upstream build,
	// so fail over without applying an account cooldown.
	if isGrokDecoderCompatibilityError(statusCode, responseBody) {
		return true
	}
	decision := classifyGrokUpstreamFailure(statusCode, responseBody, "")
	switch decision.Class {
	case GrokFailureFreeUsage, GrokFailureEmptyUpstream, GrokFailureBilling, GrokFailureModelCapacity, GrokFailureModelUnsupported, GrokFailureCompatibility:
		return decision.ShouldFailover
	}
	return s.shouldFailoverUpstreamError(statusCode)
}

func isGrokDecoderCompatibilityError(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusUnprocessableEntity || len(responseBody) == 0 {
		return false
	}
	for _, candidate := range grokStructuredErrorMessageCandidates(responseBody) {
		message := strings.ToLower(candidate)
		decoderSignal := strings.Contains(message, "untagged enum") ||
			strings.Contains(message, "decode") ||
			strings.Contains(message, "deserialize") ||
			strings.Contains(message, "deserializ") ||
			strings.Contains(message, "decoder")
		inputSignal := strings.Contains(message, "modelinput") ||
			strings.Contains(message, "model input") ||
			strings.Contains(message, "input[") ||
			strings.Contains(message, "input.")
		messageContentSignal := (strings.Contains(message, "messages[") ||
			strings.Contains(message, "messages.")) &&
			strings.Contains(message, "content") &&
			strings.Contains(message, "did not match any variant")
		if decoderSignal && (inputSignal || messageContentSignal) {
			return true
		}
	}
	return false
}

func grokStructuredErrorMessageCandidates(body []byte) []string {
	candidates := make([]string, 0, 6)
	appendCandidate := func(result gjson.Result) {
		if !result.Exists() {
			return
		}
		value := strings.TrimSpace(result.String())
		if value != "" {
			candidates = append(candidates, value)
		}
	}
	appendCandidate(gjson.GetBytes(body, "error.message"))
	appendCandidate(gjson.GetBytes(body, "error.error"))
	appendCandidate(gjson.GetBytes(body, "response.error.message"))
	errorNode := gjson.GetBytes(body, "error")
	if errorNode.Type == gjson.String {
		appendCandidate(errorNode)
	}
	appendCandidate(gjson.GetBytes(body, "message"))
	appendCandidate(gjson.GetBytes(body, "detail"))
	if !json.Valid(body) {
		if plaintext := strings.TrimSpace(string(body)); plaintext != "" {
			candidates = append(candidates, plaintext)
		}
	}
	return candidates
}

// applyGrokForbiddenPolicy applies an administrator's existing temporary
// unschedulable rules to a non-content 403. It reports true only when a rule
// matched; unmatched responses retain the legacy entitlement cooldown.
func (s *OpenAIGatewayService) applyGrokForbiddenPolicy(ctx context.Context, account *Account, responseBody []byte) bool {
	if account == nil || !account.IsTempUnschedulableEnabled() {
		return false
	}

	matches := matchTempUnschedulableRules(account, http.StatusForbidden, responseBody)
	if len(matches) == 0 {
		return false
	}

	match := matches[0]
	// Reuse the central policy implementation when it has a repository. This
	// preserves the existing reason/cache format and avoids duplicating writes.
	if s != nil && s.rateLimitService != nil && s.rateLimitService.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		handled := s.rateLimitService.tryTempUnschedulable(
			stateCtx,
			account,
			http.StatusForbidden,
			responseBody,
		)
		cancel()
		if handled {
			return true
		}
	}

	// A partially constructed service (for example a unit-test gateway) still
	// honors the configured duration instead of silently falling back to 30m.
	cooldown := time.Duration(match.rule.DurationMinutes) * time.Minute
	if cooldown > 0 {
		s.tempUnscheduleGrok(ctx, account, cooldown, "grok configured forbidden rule")
	}
	return true
}
