package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeVoiceTestBinding(account *Account) *AudioVoiceBinding {
	return &AudioVoiceBinding{AccountID: account.ID, UpstreamModel: "gemini-3.8-flash-tts", VoiceKey: "voicekey_native_private", ExpiresAt: time.Now().Add(time.Hour).Unix()}
}

func nativeVoiceTestResponse(audio []byte, mimeType string) string {
	item := map[string]any{"type": "audio", "data": base64.StdEncoding.EncodeToString(audio)}
	if mimeType != "" {
		item["mime_type"] = mimeType
	}
	body, _ := json.Marshal(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{item}}}})
	return string(body)
}

func TestForwardAudioVoiceSpeechUsesNativeInteractionsAndLastAudio(t *testing.T) {
	for _, platform := range []string{PlatformGemini, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			account := audioSpeechTestAccount(platform)
			binding := nativeVoiceTestBinding(account)
			ctx := WithAudioVoiceBinding(context.Background(), binding)
			body := []byte(`{"model":"voice-alias","input":"你好，测试克隆声音","voice":"s2voice_local_token","instructions":"warm and clear","response_format":"wav"}`)
			audio := voiceReplicationTestWAV(24000, 24000)
			// The native API can return multiple generated versions. Select the
			// last audio item, never concatenate independent WAV containers.
			responseBody := `{"steps":[{"type":"model_output","content":[{"type":"audio","data":"invalid-earlier-version"}]},` +
				`{"type":"tool_output","content":[{"type":"audio","data":"invalid-tool-output"}]},` +
				strings.TrimSuffix(strings.TrimPrefix(nativeVoiceTestResponse(audio, "audio/wav"), `{"status":"completed","steps":[`), `]}`) + `]}`
			upstream := &httpUpstreamRecorder{resp: voiceReplicationTestResponse(200, "application/json", responseBody)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			rec := httptest.NewRecorder()
			c := audioSpeechTestContext(rec, body)
			result, err := svc.ForwardAudioSpeech(ctx, c, account, body, "")
			require.NoError(t, err)
			require.Equal(t, audio, rec.Body.Bytes())
			require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
			require.Equal(t, "https://speech.example/v1beta/interactions", upstream.lastReq.URL.String())
			require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
			require.Equal(t, "/v1beta/interactions", GetActualOpenAIUpstreamEndpoint(c))
			require.Equal(t, "/v1beta/interactions", result.UpstreamEndpoint)
			require.True(t, result.AudioSpeech)
			require.Equal(t, "gemini-3.8-flash-tts", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "user_input", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
			require.Equal(t, "你好，测试克隆声音", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
			require.Equal(t, "speech_metadata", gjson.GetBytes(upstream.lastBody, "input.0.content.0.annotations.0.type").String())
			require.Equal(t, "warm and clear", gjson.GetBytes(upstream.lastBody, "input.0.content.0.annotations.0.style").String())
			require.Equal(t, binding.VoiceKey, gjson.GetBytes(upstream.lastBody, "generation_config.speech_config.0.voice").String())
			require.Equal(t, "audio/wav", gjson.GetBytes(upstream.lastBody, "response_format.mime_type").String())
			require.NotContains(t, string(upstream.lastBody), "s2voice_local_token")
			if platform == PlatformGemini {
				require.Equal(t, "sk-test-audio", upstream.lastReq.Header.Get("x-goog-api-key"))
				require.Empty(t, upstream.lastReq.Header.Get("Authorization"))
			} else {
				require.Equal(t, "Bearer sk-test-audio", upstream.lastReq.Header.Get("Authorization"))
				require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))
			}
		})
	}
}

func TestParseAudioVoiceSpeechResponsePCMAndStrictFailures(t *testing.T) {
	pcm := []byte{0, 0, 255, 127, 0, 128}
	data, contentType, err := parseAudioVoiceSpeechResponse([]byte(nativeVoiceTestResponse(pcm, "")), "audio/l16", 100)
	require.NoError(t, err)
	require.Equal(t, pcm, data)
	require.Equal(t, "audio/l16", contentType)
	validWAV := voiceReplicationTestWAV(24, 24000)
	for _, tc := range []struct {
		name, body, mime string
		limit            int64
	}{
		{"HTML", `<!doctype html><html>bad</html>`, "audio/wav", 1000},
		{"JSON error", `{"error":{"message":"failed"}}`, "audio/wav", 1000},
		{"empty", `{}`, "audio/wav", 1000},
		{"nonterminal", `{"status":"in_progress","steps":[]}`, "audio/wav", 1000},
		{"missing data", `{"steps":[{"type":"model_output","content":[{"type":"audio"}]}]}`, "audio/wav", 1000},
		{"invalid base64", `{"steps":[{"type":"model_output","content":[{"type":"audio","data":"!!!"}]}]}`, "audio/wav", 1000},
		{"truncated WAV", nativeVoiceTestResponse(validWAV[:len(validWAV)-1], "audio/wav"), "audio/wav", 1000},
		{"wrong MIME", nativeVoiceTestResponse(validWAV, "audio/mp3"), "audio/wav", 1000},
		{"empty audio", nativeVoiceTestResponse(nil, "audio/wav"), "audio/wav", 1000},
		{"odd PCM", nativeVoiceTestResponse([]byte{1, 2, 3}, "audio/l16"), "audio/l16", 1000},
		{"wrong PCM rate", nativeVoiceTestResponse(pcm, "audio/l16;rate=8000"), "audio/l16", 1000},
		{"oversize", nativeVoiceTestResponse(validWAV, "audio/wav"), "audio/wav", 8},
		{"JSON disguised as PCM", nativeVoiceTestResponse([]byte(`{"error":true}`), "audio/l16"), "audio/l16", 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _, err := parseAudioVoiceSpeechResponse([]byte(tc.body), tc.mime, tc.limit)
			require.Error(t, err)
			require.Nil(t, data)
		})
	}
}

func TestForwardAudioVoiceSpeechRejectsUnsupportedFormatsAndBindingsBeforeHTTP(t *testing.T) {
	for _, scenario := range []string{"mp3", "wrong account", "wrong model", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			account := audioSpeechTestAccount(PlatformGemini)
			binding := nativeVoiceTestBinding(account)
			format := "wav"
			switch scenario {
			case "mp3":
				format = "mp3"
			case "wrong account":
				binding.AccountID++
			case "wrong model":
				binding.UpstreamModel = "different-model"
			case "expired":
				binding.ExpiresAt = time.Now().Add(-time.Minute).Unix()
			}
			body := []byte(`{"model":"voice-alias","input":"hello","response_format":"` + format + `"}`)
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.ForwardAudioSpeech(WithAudioVoiceBinding(context.Background(), binding), audioSpeechTestContext(httptest.NewRecorder(), body), account, body, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, upstream.requests)
		})
	}
}

func TestForwardAudioVoiceSpeechErrorsNeverExposeVoiceKeyOrFallBack(t *testing.T) {
	for _, status := range []int{307, 400, 404, 405, 429, 500, 501} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			account := audioSpeechTestAccount(PlatformGemini)
			binding := nativeVoiceTestBinding(account)
			body := []byte(`{"model":"voice-alias","input":"hello"}`)
			response := voiceReplicationTestResponse(status, "application/json", `{"error":{"message":"`+binding.VoiceKey+`"}}`)
			response.Header.Set("x-request-id", binding.VoiceKey)
			upstream := &httpUpstreamRecorder{resp: response}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			rec := httptest.NewRecorder()
			c := audioSpeechTestContext(rec, body)
			result, err := svc.ForwardAudioSpeech(WithAudioVoiceBinding(context.Background(), binding), c, account, body, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "/v1beta/interactions", upstream.lastReq.URL.Path)
			keys, marshalErr := json.Marshal(c.Keys)
			require.NoError(t, marshalErr)
			for _, text := range []string{rec.Body.String(), err.Error(), string(keys), rec.Header().Get("x-request-id")} {
				require.NotContains(t, text, binding.VoiceKey)
			}
			if status == 404 || status == 405 || status == 501 {
				require.Equal(t, 501, rec.Code)
				require.Equal(t, "voice_synthesis_not_supported", gjson.Get(rec.Body.String(), "error.code").String())
			}
		})
	}
}

func TestForwardAudioVoiceSpeechDeliveryFailureStillReturnsBillableResult(t *testing.T) {
	for _, cancelClient := range []bool{false, true} {
		account := audioSpeechTestAccount(PlatformGemini)
		binding := nativeVoiceTestBinding(account)
		ctx, cancel := context.WithCancel(context.Background())
		response := voiceReplicationTestResponse(200, "application/json", nativeVoiceTestResponse(voiceReplicationTestWAV(24, 24000), "audio/wav"))
		if cancelClient {
			response.Body = io.NopCloser(audioSpeechCancelReader{Reader: strings.NewReader(nativeVoiceTestResponse(voiceReplicationTestWAV(24, 24000), "audio/wav")), cancel: cancel})
		}
		writer := &audioSpeechFailingWriter{ResponseRecorder: httptest.NewRecorder()}
		body := []byte(`{"model":"voice-alias","input":"hello"}`)
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: response}}
		result, err := svc.ForwardAudioSpeech(WithAudioVoiceBinding(ctx, binding), audioSpeechTestContext(writer, body), account, body, "")
		cancel()
		require.Error(t, err)
		require.NotNil(t, result)
		require.True(t, result.AudioSpeech)
		require.True(t, result.ClientDisconnect)
		if cancelClient {
			require.ErrorIs(t, err, context.Canceled)
			require.Zero(t, writer.writes)
		} else {
			require.Equal(t, 1, writer.writes)
		}
	}
}

func TestForwardAudioVoiceSpeechTransportErrorIsSanitized(t *testing.T) {
	account := audioSpeechTestAccount(PlatformGemini)
	binding := nativeVoiceTestBinding(account)
	body := []byte(`{"model":"voice-alias","input":"hello"}`)
	upstream := &httpUpstreamRecorder{err: errors.New("transport returned " + binding.VoiceKey)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c := audioSpeechTestContext(rec, body)
	result, err := svc.ForwardAudioSpeech(WithAudioVoiceBinding(context.Background(), binding), c, account, body, "")
	require.Error(t, err)
	require.Nil(t, result)
	keys, marshalErr := json.Marshal(c.Keys)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(keys), binding.VoiceKey)
	require.NotContains(t, err.Error(), binding.VoiceKey)
}

type audioVoiceFeatureDeniedRepo struct {
	AccountRepository
	stateChanges int
}

func (r *audioVoiceFeatureDeniedRepo) SetError(context.Context, int64, string) error {
	r.stateChanges++
	return nil
}

func (r *audioVoiceFeatureDeniedRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.stateChanges++
	return nil
}

func TestAudioVoiceFeatureForbiddenDoesNotDisableOrdinaryTTS(t *testing.T) {
	for _, create := range []bool{true, false} {
		account := audioSpeechTestAccount(PlatformOpenAI)
		accountRepo := &audioVoiceFeatureDeniedRepo{}
		cfg := &config.Config{}
		upstream := &httpUpstreamRecorder{responses: []*http.Response{
			voiceReplicationTestResponse(403, "application/json", `{"error":{"message":"voicekey_secret feature not enabled"}}`),
			audioSpeechTestResponse(200, "audio/wav", voiceReplicationTestWAV(24, 24000)),
		}}
		svc := &OpenAIGatewayService{cfg: cfg, accountRepo: accountRepo, httpUpstream: upstream, rateLimitService: NewRateLimitService(accountRepo, nil, cfg, nil, nil)}
		c, recorder := voiceReplicationTestContext()
		var err error
		if create {
			var result *AudioVoiceReplicationResult
			result, err = svc.CreateAudioVoiceUpstream(context.Background(), c, account, voiceReplicationTestRequest(), "")
			require.Nil(t, result)
		} else {
			var result *OpenAIForwardResult
			result, err = svc.ForwardAudioSpeech(WithAudioVoiceBinding(context.Background(), nativeVoiceTestBinding(account)), c, account, []byte(`{"model":"voice-alias","input":"hello"}`), "")
			require.Nil(t, result)
		}
		require.Error(t, err)
		var retry *UpstreamFailoverError
		require.False(t, errors.As(err, &retry), "feature-specific 403 must not retry another account")
		require.Equal(t, 403, recorder.Code)
		require.Zero(t, accountRepo.stateChanges)
		body := []byte(`{"model":"voice-alias","input":"hello","voice":"Kore"}`)
		rec := httptest.NewRecorder()
		result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(rec, body), account, body, "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 200, rec.Code)
		require.Zero(t, accountRepo.stateChanges)
	}
}
