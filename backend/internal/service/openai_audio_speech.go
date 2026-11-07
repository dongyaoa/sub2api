package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ParseAudioSpeechRequest normalizes common text/style aliases without decoding
// provider-specific fields (for example an object-valued voice or speakers).
func ParseAudioSpeechRequest(body []byte) (normalized []byte, model string, input string, err error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return nil, "", "", errors.New("request body must be a JSON object")
	}
	if err := json.Unmarshal(fields["model"], &model); err != nil || strings.TrimSpace(model) == "" {
		return nil, "", "", errors.New("model must be a non-empty string")
	}
	model = strings.TrimSpace(model)
	fields["model"], _ = json.Marshal(model)
	for _, name := range []string{"input", "text", "prompt"} {
		raw, exists := fields[name]
		if !exists {
			continue
		}
		var candidate string
		if err := json.Unmarshal(raw, &candidate); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, "", "", fmt.Errorf("%s must be a string", name)
		}
		if strings.TrimSpace(candidate) != "" {
			input = candidate
			fields["input"] = raw
			break
		}
	}
	if strings.TrimSpace(input) == "" {
		return nil, "", "", errors.New("input, text, or prompt must contain non-empty text")
	}
	if _, exists := fields["instructions"]; !exists {
		if style, exists := fields["style_prompt"]; exists {
			fields["instructions"] = style
		}
	}
	normalized, err = json.Marshal(fields)
	return normalized, model, input, err
}

// ForwardAudioSpeech buffers a complete, bounded audio response before sending
// any bytes. A non-nil result means upstream synthesis completed successfully;
// callers must record that result even when delivery to the client fails.
func (s *OpenAIGatewayService) ForwardAudioSpeech(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (recentResult *OpenAIForwardResult, recentErr error) {
	if s == nil || c == nil || c.Request == nil || !SupportsAudioSpeechAccount(account) {
		return nil, errors.New("audio speech requires a supported API key account")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	finishRecentRequest := beginAccountRecentRequest(ctx, c, s.cache, account, recentRequestModel(body))
	defer func() { finishRecentRequest(recentResult != nil, recentErr) }()
	started := time.Now()
	normalized, originalModel, _, err := ParseAudioSpeechRequest(body)
	if err != nil {
		writeAudioSpeechError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	SetActualOpenAIUpstreamEndpoint(c, "/v1/audio/speech")
	if upstreamModel != originalModel {
		normalized = ReplaceModelInBody(normalized, upstreamModel)
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if account.Platform == PlatformOpenAI {
		baseURL = account.GetOpenAIFormatBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildAudioSpeechURL(validatedURL)
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(normalized))
	if err != nil {
		return nil, fmt.Errorf("build audio speech request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "audio/*, application/octet-stream")
	if userAgent := account.GetOpenAIUserAgent(); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStarted := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStarted).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	if resp == nil || resp.Body == nil {
		writeAudioSpeechError(c, http.StatusBadGateway, "upstream_error", "Upstream returned an empty response")
		return nil, errors.New("audio speech upstream response is nil")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, s.handleAudioSpeechUpstreamError(ctx, c, account, resp, upstreamModel)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, rejectAudioSpeechResponse(c, account, resp, "Upstream did not return a successful audio response")
	}
	data, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeAudioSpeechError(c, http.StatusBadGateway, "upstream_error", "Failed to read complete upstream audio")
		}
		return nil, fmt.Errorf("read upstream audio: %w", err)
	}
	if resp.ContentLength > 0 && int64(len(data)) != resp.ContentLength {
		return nil, rejectAudioSpeechResponse(c, account, resp, "Upstream returned incomplete audio")
	}
	if !validAudioSpeechResponse(resp.Header.Get("Content-Type"), data) {
		return nil, rejectAudioSpeechResponse(c, account, resp, "Upstream returned an empty or non-audio response")
	}
	result := &OpenAIForwardResult{
		// Never use a client-supplied or reused upstream id as a money-event id.
		RequestID:        "audio_speech:" + uuid.NewString(),
		UpstreamHeaders:  resp.Header.Clone(),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1/audio/speech",
		AudioSpeech:      true,
		Duration:         time.Since(started),
	}
	if err := ctx.Err(); err != nil {
		result.ClientDisconnect = true
		return result, err
	}
	if err := writeAudioSpeechResponse(c, resp, data, s.responseHeaderFilter); err != nil {
		result.ClientDisconnect = true
		return result, fmt.Errorf("deliver audio speech response: %w", err)
	}
	return result, nil
}

func (s *OpenAIGatewayService) handleAudioSpeechUpstreamError(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, model string) error {
	body := s.readUpstreamErrorBody(resp)
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	kind := "http_error"
	failover := s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, message, body)
	if failover {
		kind = "failover"
	}
	setOpsUpstreamError(c, resp.StatusCode, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
		Kind: kind, Message: message,
	})
	if failover {
		disabled := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, model)
		retrySame := !disabled && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
		if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, message, body) {
			return newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, body, message, retrySame)
		}
		return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: body, RetryableOnSameAccount: retrySame}
	}
	_ = writeAudioSpeechResponse(c, resp, body, s.responseHeaderFilter)
	return fmt.Errorf("audio speech upstream returned status %d", resp.StatusCode)
}

func validAudioSpeechResponse(contentType string, data []byte) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || len(data) == 0 || (!strings.HasPrefix(mediaType, "audio/") && mediaType != "application/octet-stream") {
		return false
	}
	// A mislabeled HTML login page or JSON error must never become a paid audio
	// result. Do not inspect codecs: raw PCM and provider-specific audio are valid.
	trimmed := bytes.TrimSpace(data)
	return !json.Valid(trimmed) && !strings.HasPrefix(http.DetectContentType(trimmed), "text/html")
}

func buildAudioSpeechURL(base string) string {
	// Existing Gemini accounts sometimes store the native API prefix in base_url.
	// Speech uses the supplier's OpenAI-compatible surface instead.
	if parsed, err := url.Parse(base); err == nil {
		path := strings.TrimRight(parsed.Path, "/")
		for _, suffix := range []string{"/v1beta/models", "/v1beta"} {
			if strings.HasSuffix(path, suffix) {
				parsed.Path = strings.TrimSuffix(path, suffix)
				parsed.RawPath = ""
				base = parsed.String()
				break
			}
		}
	}
	return buildOpenAIEndpointURL(base, "/v1/audio/speech")
}

func rejectAudioSpeechResponse(c *gin.Context, account *Account, resp *http.Response, message string) error {
	setOpsUpstreamError(c, resp.StatusCode, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
		Kind: "invalid_response", Message: message,
	})
	writeAudioSpeechError(c, http.StatusBadGateway, "upstream_error", message)
	return errors.New(message)
}

func writeAudioSpeechError(c *gin.Context, status int, kind, message string) {
	if c != nil && !c.Writer.Written() {
		c.JSON(status, gin.H{"error": gin.H{"type": kind, "message": message}})
	}
}

func writeAudioSpeechResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) error {
	if c.Writer.Written() {
		return errors.New("response already committed")
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, filter)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Header("Content-Length", strconv.Itoa(len(body)))
	c.Writer.WriteHeader(resp.StatusCode)
	n, err := c.Writer.Write(body)
	if err == nil && n != len(body) {
		return io.ErrShortWrite
	}
	return err
}
