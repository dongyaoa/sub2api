package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxAudioVoiceReplicationRecordingBytes = 10 << 20
	maxAudioVoiceReplicationResponseBytes  = 1 << 20
	audioVoiceReplicationMaxLifetime       = 7 * 24 * time.Hour
	audioVoiceReplicationUpstreamEndpoint  = "/v1beta/voices"
)

var ErrAudioVoiceReplicationNotSupported = errors.New("selected upstream does not support voice replication")

type AudioVoiceReplicationAudio struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

type AudioVoiceReplicationRequest struct {
	Model        string                     `json:"model"`
	SourceAudio  AudioVoiceReplicationAudio `json:"source_audio"`
	ConsentAudio AudioVoiceReplicationAudio `json:"consent_audio"`
	DisplayName  string                     `json:"display_name,omitempty"`
}

type AudioVoiceReplicationResult struct {
	VoiceKey      string
	ExpireTime    time.Time
	ForwardResult *OpenAIForwardResult
}

// ParseAudioVoiceReplicationRequest requires two independently supplied WAV
// recordings. Consent is never synthesized or substituted from the reference.
func ParseAudioVoiceReplicationRequest(body []byte) (*AudioVoiceReplicationRequest, error) {
	var request *AudioVoiceReplicationRequest
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return nil, errors.New("voice replication request must be a JSON object with string fields")
	}
	if err := validateAudioVoiceReplicationRequest(request); err != nil {
		return nil, err
	}
	return request, nil
}

func validateAudioVoiceReplicationRequest(request *AudioVoiceReplicationRequest) error {
	if request == nil || strings.TrimSpace(request.Model) == "" {
		return errors.New("model must be a non-empty string")
	}
	request.Model = strings.TrimSpace(request.Model)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if utf8.RuneCountInString(request.DisplayName) > 128 {
		return errors.New("display_name must not exceed 128 characters")
	}
	if err := validateAudioVoiceReplicationRecording(&request.SourceAudio, "source_audio", 10, 30); err != nil {
		return err
	}
	return validateAudioVoiceReplicationRecording(&request.ConsentAudio, "consent_audio", 0, 60)
}

func validateAudioVoiceReplicationRecording(audio *AudioVoiceReplicationAudio, field string, minSeconds, maxSeconds uint64) error {
	if audio == nil || strings.TrimSpace(audio.Data) == "" {
		return fmt.Errorf("%s recording is required", field)
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(audio.MIMEType))
	if err != nil || (mediaType != "audio/wav" && mediaType != "audio/x-wav" && mediaType != "audio/wave") {
		return fmt.Errorf("%s must use audio/wav", field)
	}
	encoded := strings.TrimSpace(audio.Data)
	if len(encoded) > base64.StdEncoding.EncodedLen(maxAudioVoiceReplicationRecordingBytes) {
		return fmt.Errorf("%s must not exceed 10 MiB", field)
	}
	wav, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("%s data must be valid base64", field)
	}
	if len(wav) > maxAudioVoiceReplicationRecordingBytes {
		return fmt.Errorf("%s must not exceed 10 MiB", field)
	}
	samples, sampleRate, err := audioVoiceReplicationWAVSamples(wav)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if samples == 0 || samples < minSeconds*sampleRate || samples > maxSeconds*sampleRate {
		if minSeconds > 0 {
			return fmt.Errorf("%s must contain 10 to 30 seconds of audio", field)
		}
		return fmt.Errorf("%s must contain more than 0 and at most 60 seconds of audio", field)
	}
	audio.MIMEType = "audio/wav"
	audio.Data = base64.StdEncoding.EncodeToString(wav)
	return nil
}

// audioVoiceReplicationWAVSamples validates the complete RIFF container before
// calculating duration from PCM data. File size alone is not an audio duration.
func audioVoiceReplicationWAVSamples(wav []byte) (samples, sampleRate uint64, err error) {
	invalid := func() (uint64, uint64, error) { return 0, 0, errors.New("recording must be a complete PCM WAV file") }
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(wav[4:8]))+8 != uint64(len(wav)) {
		return invalid()
	}
	var haveFormat, haveData bool
	var dataBytes uint64
	for offset := uint64(12); offset < uint64(len(wav)); {
		if uint64(len(wav))-offset < 8 {
			return invalid()
		}
		kind := string(wav[offset : offset+4])
		length := uint64(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		start, end := offset+8, offset+8+length
		next := end + length%2
		if end > uint64(len(wav)) || next > uint64(len(wav)) {
			return invalid()
		}
		switch kind {
		case "fmt ":
			if haveFormat || length < 16 {
				return invalid()
			}
			format := wav[start:end]
			if binary.LittleEndian.Uint16(format[0:2]) != 1 || binary.LittleEndian.Uint16(format[2:4]) != 1 ||
				binary.LittleEndian.Uint16(format[14:16]) != 16 {
				return 0, 0, errors.New("recording must use mono 16-bit PCM WAV")
			}
			sampleRate = uint64(binary.LittleEndian.Uint32(format[4:8]))
			switch sampleRate {
			case 8000, 11025, 16000, 22050, 24000, 32000, 44100, 48000:
			default:
				return 0, 0, errors.New("recording uses an unsupported sample rate; 24000 Hz is recommended")
			}
			if binary.LittleEndian.Uint16(format[12:14]) != 2 || uint64(binary.LittleEndian.Uint32(format[8:12])) != sampleRate*2 {
				return invalid()
			}
			if length > 16 && (length < 18 || uint64(binary.LittleEndian.Uint16(format[16:18]))+18 != length) {
				return invalid()
			}
			haveFormat = true
		case "data":
			if !haveFormat || haveData || length%2 != 0 {
				return invalid()
			}
			haveData, dataBytes = true, length
		}
		offset = next
	}
	if !haveFormat || !haveData {
		return invalid()
	}
	return dataBytes / 2, sampleRate, nil
}

// CreateAudioVoiceUpstream implements the native, non-persistent voice creation
// protocol. It never writes a success response or exposes the provider voice key
// in errors; the handler owns sealing the returned key into its local token.
func (s *OpenAIGatewayService) CreateAudioVoiceUpstream(ctx context.Context, c *gin.Context, account *Account, request *AudioVoiceReplicationRequest, defaultMappedModel string) (result *AudioVoiceReplicationResult, resultErr error) {
	if s == nil || c == nil || c.Request == nil || !SupportsAudioSpeechAccount(account) {
		return nil, errors.New("voice replication requires a supported API key account")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateAudioVoiceReplicationRequest(request); err != nil {
		writeAudioVoiceReplicationError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	started := time.Now()
	finishRecentRequest := beginAccountRecentRequest(ctx, c, s.cache, account, request.Model)
	defer func() { finishRecentRequest(result != nil, resultErr) }()
	billingModel := resolveOpenAIForwardModel(account, request.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	SetActualOpenAIUpstreamEndpoint(c, audioVoiceReplicationUpstreamEndpoint)
	// Build a closed provider payload: client fields cannot enable storage or
	// replace the required independently supplied consent recording.
	body, err := json.Marshal(map[string]any{
		"store": false,
		"voice": map[string]any{
			"model": upstreamModel, "type": "replicated",
			"replicated": map[string]any{"source_audio": request.SourceAudio, "consent_audio": request.ConsentAudio},
		},
	})
	if err != nil {
		return nil, errors.New("failed to encode voice replication request")
	}
	base := strings.TrimSpace(account.GetCredential("base_url"))
	if account.Platform == PlatformOpenAI {
		base = account.GetOpenAIFormatBaseURL()
		if base == "" {
			base = "https://api.openai.com"
		}
	}
	validatedBase, err := s.validateUpstreamBaseURL(base)
	if err != nil {
		return nil, errors.New("invalid voice replication upstream base URL")
	}
	target, err := buildAudioVoiceReplicationURL(validatedBase)
	if err != nil {
		return nil, err
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	upstreamRequest, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to construct voice replication request")
	}
	upstreamRequest = upstreamRequest.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(upstreamRequest.Context(), HTTPUpstreamProfileOpenAI)))
	account.ApplyHeaderOverrides(upstreamRequest.Header)
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("Accept", "application/json")
	upstreamRequest.Header.Del("Authorization")
	upstreamRequest.Header.Del("x-goog-api-key")
	key := strings.TrimSpace(account.GetCredential("api_key"))
	if account.Platform == PlatformGemini {
		upstreamRequest.Header.Set("x-goog-api-key", key)
	} else {
		upstreamRequest.Header.Set("Authorization", "Bearer "+key)
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	response, err := s.doOpenAIUpstream(upstreamRequest, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		// Transport errors can contain provider-controlled text. Keep recording
		// data and voice credentials out of logs and failover responses.
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, 0, upstreamModel, "Voice replication upstream transport failed", true)
	}
	if response == nil || response.Body == nil {
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, 0, upstreamModel, "Voice replication upstream returned no response", false)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		message := "This upstream does not support voice replication"
		recordAudioVoiceReplicationError(c, account, response.StatusCode, message)
		writeAudioVoiceReplicationError(c, http.StatusNotImplemented, "voice_replication_not_supported", message)
		return nil, ErrAudioVoiceReplicationNotSupported
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := "Upstream rejected the voice replication request; check the model and both recordings"
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			message = "Upstream authentication or permission failed for voice replication"
		} else if response.StatusCode == http.StatusTooManyRequests {
			message = "Upstream voice replication rate limit exceeded"
		} else if response.StatusCode >= http.StatusInternalServerError {
			message = "Upstream voice replication service is unavailable"
		}
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, response.StatusCode, upstreamModel, message, response.StatusCode != http.StatusForbidden && s.shouldFailoverUpstreamError(response.StatusCode))
	}
	contentType, _, contentTypeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentTypeErr != nil || (contentType != "application/json" && !strings.HasSuffix(contentType, "+json")) {
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, response.StatusCode, upstreamModel, "Voice replication upstream returned a non-JSON response", false)
	}
	limit := resolveUpstreamResponseReadLimit(s.cfg)
	if limit > maxAudioVoiceReplicationResponseBytes {
		limit = maxAudioVoiceReplicationResponseBytes
	}
	responseBody, err := readUpstreamResponseBodyLimited(response.Body, limit)
	if err != nil || (response.ContentLength > 0 && int64(len(responseBody)) != response.ContentLength) {
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, response.StatusCode, upstreamModel, "Failed to read a complete voice replication response", false)
	}
	voiceKey, expires, err := parseAudioVoiceReplicationResponse(responseBody, time.Now().UTC())
	if err != nil {
		return nil, s.audioVoiceReplicationFailure(ctx, c, account, response.StatusCode, upstreamModel, err.Error(), false)
	}
	result = &AudioVoiceReplicationResult{
		VoiceKey: voiceKey, ExpireTime: expires,
		ForwardResult: &OpenAIForwardResult{
			RequestID: "voice_clone:" + uuid.NewString(), Model: request.Model,
			BillingModel: billingModel, UpstreamModel: upstreamModel,
			UpstreamEndpoint: audioVoiceReplicationUpstreamEndpoint, AudioSpeech: true,
			Duration: time.Since(started),
		},
	}
	if err := ctx.Err(); err != nil {
		result.ForwardResult.ClientDisconnect = true
		return result, err
	}
	return result, nil
}

func buildAudioVoiceReplicationURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid voice replication upstream base URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{"/v1/audio/speech", "/v1/audio/voices", "/v1beta/voices", "/v1beta/models", "/v1/models", "/v1beta", "/v1"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	parsed.Path, parsed.RawPath = path+audioVoiceReplicationUpstreamEndpoint, ""
	return parsed.String(), nil
}

func parseAudioVoiceReplicationResponse(body []byte, now time.Time) (string, time.Time, error) {
	invalid := func() (string, time.Time, error) {
		return "", time.Time{}, errors.New("upstream did not return a usable replicated voice key")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil || audioVoiceReplicationHasError(root) {
		return invalid()
	}
	voice := root
	if _, exists := root["key"]; !exists {
		for _, wrapper := range []string{"voice", "replicated_voice"} {
			if raw, exists := root[wrapper]; exists {
				var inner map[string]json.RawMessage
				if err := json.Unmarshal(raw, &inner); err != nil || inner == nil {
					return invalid()
				}
				voice = inner
				break
			}
		}
	}
	if audioVoiceReplicationHasError(voice) {
		return invalid()
	}
	var voiceKey string
	if err := json.Unmarshal(voice["key"], &voiceKey); err != nil || !strings.HasPrefix(voiceKey, "voicekey_") || len(voiceKey) <= len("voicekey_") || len(voiceKey) > audioVoiceMaxUpstreamKeyBytes {
		return invalid()
	}
	for _, character := range voiceKey {
		if character < 33 || character > 126 {
			return invalid()
		}
	}
	if raw, exists := voice["type"]; exists {
		var voiceType string
		if err := json.Unmarshal(raw, &voiceType); err != nil || voiceType != "replicated" {
			return invalid()
		}
	}
	expires := now.Add(audioVoiceReplicationMaxLifetime)
	for _, object := range []map[string]json.RawMessage{root, voice} {
		for _, field := range []string{"expire_time", "expireTime"} {
			if raw, exists := object[field]; exists {
				var text string
				if err := json.Unmarshal(raw, &text); err != nil {
					return invalid()
				}
				value, err := time.Parse(time.RFC3339Nano, text)
				if err != nil || !value.After(now) {
					return "", time.Time{}, errors.New("upstream returned an expired or invalid voice expiration")
				}
				if value.Before(expires) {
					expires = value.UTC()
				}
			}
		}
	}
	return voiceKey, expires, nil
}

func audioVoiceReplicationHasError(object map[string]json.RawMessage) bool {
	raw, exists := object["error"]
	return exists && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func (s *OpenAIGatewayService) audioVoiceReplicationFailure(ctx context.Context, c *gin.Context, account *Account, upstreamStatus int, model, message string, failover bool) error {
	recordAudioVoiceReplicationError(c, account, upstreamStatus, message)
	status := upstreamStatus
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": "upstream_error", "code": "voice_replication_upstream_error", "message": message}})
	if failover && upstreamStatus != http.StatusForbidden {
		// Only the safe replacement body reaches account health handlers.
		disabled := s.handleOpenAIAccountUpstreamError(ctx, account, status, http.Header{}, body, model)
		return &UpstreamFailoverError{StatusCode: status, ResponseBody: body, RetryableOnSameAccount: !disabled && account.IsPoolMode() && account.IsPoolModeRetryableStatus(status)}
	}
	writeAudioVoiceReplicationError(c, status, "voice_replication_upstream_error", message)
	return errors.New(message)
}

func recordAudioVoiceReplicationError(c *gin.Context, account *Account, status int, message string) {
	setOpsUpstreamError(c, status, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: status, Kind: "voice_replication_error", Message: message,
	})
}

func writeAudioVoiceReplicationError(c *gin.Context, status int, code, message string) {
	if c != nil && !c.Writer.Written() {
		kind := "upstream_error"
		if code == "invalid_request_error" {
			kind = code
		}
		c.JSON(status, gin.H{"error": gin.H{"type": kind, "code": code, "message": message}})
	}
}
