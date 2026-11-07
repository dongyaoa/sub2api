package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const audioVoiceSpeechUpstreamEndpoint = "/v1beta/interactions"

var ErrAudioVoiceSynthesisNotSupported = errors.New("selected upstream does not support replicated voice synthesis")

// forwardAudioVoiceSpeech uses the native Interactions protocol for a voice
// bound by the gateway. The supplier's OpenAI speech endpoint is not assumed
// to support these keys, and no failure falls back to a preset voice.
func (s *OpenAIGatewayService) forwardAudioVoiceSpeech(ctx context.Context, c *gin.Context, account *Account, body []byte, defaultMappedModel string, binding *AudioVoiceBinding) (result *OpenAIForwardResult, resultErr error) {
	started := time.Now()
	normalized, originalModel, input, err := ParseAudioSpeechRequest(body)
	if err != nil {
		writeAudioSpeechError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if binding == nil || binding.AccountID != account.ID || binding.UpstreamModel != upstreamModel ||
		binding.ExpiresAt <= time.Now().Unix() || !strings.HasPrefix(binding.VoiceKey, "voicekey_") || len(binding.VoiceKey) <= len("voicekey_") {
		writeAudioSpeechError(c, http.StatusBadRequest, "invalid_voice", "Voice is not available for this account and model")
		return nil, ErrAudioVoiceInvalid
	}
	format := "wav"
	if value := gjson.GetBytes(normalized, "response_format"); value.Exists() {
		if value.Type != gjson.String {
			writeAudioSpeechError(c, http.StatusBadRequest, "invalid_request_error", "Cloned voices support wav or pcm response_format")
			return nil, errors.New("cloned voices require a wav or pcm response format")
		}
		format = strings.ToLower(strings.TrimSpace(value.String()))
	}
	requestedMIME := "audio/wav"
	switch format {
	case "wav":
	case "pcm":
		requestedMIME = "audio/l16"
	default:
		writeAudioSpeechError(c, http.StatusBadRequest, "invalid_request_error", "Cloned voices support wav or pcm response_format")
		return nil, errors.New("unsupported cloned voice response format")
	}
	text := map[string]any{"type": "text", "text": input}
	if style := gjson.GetBytes(normalized, "instructions"); style.Exists() {
		if style.Type != gjson.String {
			writeAudioSpeechError(c, http.StatusBadRequest, "invalid_request_error", "instructions must be a string")
			return nil, errors.New("cloned voice instructions must be a string")
		}
		if strings.TrimSpace(style.String()) != "" {
			text["annotations"] = []map[string]any{{"type": "speech_metadata", "style": style.String()}}
		}
	}
	upstreamBody, err := json.Marshal(map[string]any{
		"model":             upstreamModel,
		"input":             []map[string]any{{"type": "user_input", "content": []map[string]any{text}}},
		"response_format":   map[string]any{"type": "audio", "mime_type": requestedMIME},
		"generation_config": map[string]any{"speech_config": []map[string]any{{"voice": binding.VoiceKey}}},
	})
	if err != nil {
		return nil, errors.New("failed to encode cloned voice synthesis request")
	}
	SetOpsUpstreamModel(c, upstreamModel)
	SetActualOpenAIUpstreamEndpoint(c, audioVoiceSpeechUpstreamEndpoint)
	finishRecentRequest := beginAccountRecentRequest(ctx, c, s.cache, account, originalModel)
	defer func() { finishRecentRequest(result != nil, resultErr) }()
	base := strings.TrimSpace(account.GetCredential("base_url"))
	if account.Platform == PlatformOpenAI {
		base = account.GetOpenAIFormatBaseURL()
		if base == "" {
			base = "https://api.openai.com"
		}
	}
	validatedBase, err := s.validateUpstreamBaseURL(base)
	if err != nil {
		return nil, errors.New("invalid cloned voice upstream base URL")
	}
	voiceURL, err := buildAudioVoiceReplicationURL(validatedBase)
	if err != nil {
		return nil, errors.New("invalid cloned voice upstream base URL")
	}
	target := strings.TrimSuffix(voiceURL, audioVoiceReplicationUpstreamEndpoint) + audioVoiceSpeechUpstreamEndpoint
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	request, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, target, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, errors.New("failed to construct cloned voice synthesis request")
	}
	request = request.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI)))
	account.ApplyHeaderOverrides(request.Header)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Del("Authorization")
	request.Header.Del("x-goog-api-key")
	key := strings.TrimSpace(account.GetCredential("api_key"))
	if account.Platform == PlatformGemini {
		request.Header.Set("x-goog-api-key", key)
	} else {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStarted := time.Now()
	response, err := s.doOpenAIUpstream(request, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStarted).Milliseconds())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, 0, upstreamModel, "Cloned voice upstream transport failed", true)
	}
	if response == nil || response.Body == nil {
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, 0, upstreamModel, "Cloned voice upstream returned no response", false)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		message := "This upstream does not support cloned voice synthesis through Interactions"
		recordAudioVoiceSpeechError(c, account, response.StatusCode, message)
		writeAudioVoiceReplicationError(c, http.StatusNotImplemented, "voice_synthesis_not_supported", message)
		return nil, ErrAudioVoiceSynthesisNotSupported
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, response.StatusCode, upstreamModel, "Upstream rejected the cloned voice synthesis request", response.StatusCode != http.StatusForbidden && s.shouldFailoverUpstreamError(response.StatusCode))
	}
	mediaType, _, mimeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mimeErr != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, response.StatusCode, upstreamModel, "Cloned voice upstream returned a non-JSON response", false)
	}
	responseBody, err := readUpstreamResponseBodyLimited(response.Body, resolveUpstreamResponseReadLimit(s.cfg))
	if err != nil || (response.ContentLength > 0 && int64(len(responseBody)) != response.ContentLength) {
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, response.StatusCode, upstreamModel, "Failed to read complete cloned voice audio", false)
	}
	data, contentType, err := parseAudioVoiceSpeechResponse(responseBody, requestedMIME, resolveUpstreamResponseReadLimit(s.cfg))
	if err != nil {
		return nil, s.audioVoiceSpeechFailure(ctx, c, account, response.StatusCode, upstreamModel, err.Error(), false)
	}
	result = &OpenAIForwardResult{
		RequestID: "audio_speech:" + uuid.NewString(), Model: originalModel,
		BillingModel: billingModel, UpstreamModel: upstreamModel, UpstreamEndpoint: audioVoiceSpeechUpstreamEndpoint,
		AudioSpeech: true, Duration: time.Since(started),
	}
	if err := ctx.Err(); err != nil {
		result.ClientDisconnect = true
		return result, err
	}
	// Do not copy provider headers: a voice key can appear in debug/trace fields.
	binaryResponse := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}}
	if err := writeAudioSpeechResponse(c, binaryResponse, data, s.responseHeaderFilter); err != nil {
		result.ClientDisconnect = true
		return result, errors.New("failed to deliver cloned voice audio to client")
	}
	return result, nil
}

func parseAudioVoiceSpeechResponse(body []byte, requestedMIME string, maxAudioBytes int64) ([]byte, string, error) {
	invalid := func() ([]byte, string, error) {
		return nil, "", errors.New("upstream did not return complete cloned voice audio")
	}
	if !gjson.ValidBytes(body) {
		return invalid()
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || (root.Get("error").Exists() && root.Get("error").Type != gjson.Null) || !root.Get("steps").IsArray() {
		return invalid()
	}
	if status := root.Get("status"); status.Exists() && (status.Type != gjson.String || status.String() != "completed") {
		return invalid()
	}
	var lastAudio gjson.Result
	for _, step := range root.Get("steps").Array() {
		if step.Get("type").String() != "model_output" {
			continue
		}
		if !step.Get("content").IsArray() {
			return invalid()
		}
		for _, item := range step.Get("content").Array() {
			if item.Get("type").String() == "audio" {
				lastAudio = item
			}
		}
	}
	encoded := lastAudio.Get("data")
	if (lastAudio.Get("error").Exists() && lastAudio.Get("error").Type != gjson.Null) || encoded.Type != gjson.String || encoded.String() == "" || maxAudioBytes <= 0 || int64(len(encoded.String())) > (maxAudioBytes+2)/3*4 {
		return invalid()
	}
	contentType := requestedMIME
	if supplied := lastAudio.Get("mime_type"); supplied.Exists() {
		if supplied.Type != gjson.String || strings.TrimSpace(supplied.String()) == "" {
			return invalid()
		}
		contentType = supplied.String()
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return invalid()
	}
	switch requestedMIME {
	case "audio/wav":
		if mediaType != "audio/wav" && mediaType != "audio/x-wav" && mediaType != "audio/wave" {
			return invalid()
		}
	case "audio/l16":
		if mediaType != "audio/l16" || (params["rate"] != "" && params["rate"] != "24000") || (params["channels"] != "" && params["channels"] != "1") {
			return invalid()
		}
	default:
		return invalid()
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded.String())
	if err != nil || len(data) == 0 || int64(len(data)) > maxAudioBytes || !validAudioSpeechResponse(contentType, data) {
		return invalid()
	}
	if requestedMIME == "audio/wav" {
		if samples, _, err := audioVoiceReplicationWAVSamples(data); err != nil || samples == 0 {
			return invalid()
		}
	} else if len(data)%2 != 0 {
		return invalid()
	}
	return data, contentType, nil
}

func (s *OpenAIGatewayService) audioVoiceSpeechFailure(ctx context.Context, c *gin.Context, account *Account, upstreamStatus int, model, message string, failover bool) error {
	recordAudioVoiceSpeechError(c, account, upstreamStatus, message)
	status := upstreamStatus
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": "upstream_error", "code": "voice_synthesis_upstream_error", "message": message}})
	if failover && upstreamStatus != http.StatusForbidden {
		disabled := s.handleOpenAIAccountUpstreamError(ctx, account, status, http.Header{}, body, model)
		return &UpstreamFailoverError{StatusCode: status, ResponseBody: body, RetryableOnSameAccount: !disabled && account.IsPoolMode() && account.IsPoolModeRetryableStatus(status)}
	}
	writeAudioVoiceReplicationError(c, status, "voice_synthesis_upstream_error", message)
	return errors.New(message)
}

func recordAudioVoiceSpeechError(c *gin.Context, account *Account, status int, message string) {
	setOpsUpstreamError(c, status, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: status, Kind: "voice_synthesis_error", Message: message,
	})
}
