package handler

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const audioVoiceHandlerPrivateKey = "voicekey_supplier_private_secret"

// Generated PCM silence is test data, not a person's recorded voice.
func audioVoiceHandlerTestWAV(seconds int) []byte {
	wav := make([]byte, 44+24000*seconds*2)
	copy(wav[:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 24000)
	binary.LittleEndian.PutUint32(wav[28:32], 48000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(wav)-44))
	return wav
}

func audioVoiceHandlerCreateBody(t *testing.T, includeConsent bool) string {
	t.Helper()
	payload := map[string]any{"model": audioSpeechHandlerModel, "display_name": "test voice",
		"source_audio": map[string]any{"mime_type": "audio/wav", "data": base64.StdEncoding.EncodeToString(audioVoiceHandlerTestWAV(10))}}
	if includeConsent {
		payload["consent_audio"] = map[string]any{"mime_type": "audio/wav", "data": base64.StdEncoding.EncodeToString(audioVoiceHandlerTestWAV(1))}
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(body)
}

func audioVoiceHandlerSpeechBody(t *testing.T, voice any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": audioSpeechHandlerModel, "input": "用我的克隆声音朗读这段话。", "voice": voice, "response_format": "wav", "instructions": "自然温暖"})
	require.NoError(t, err)
	return string(body)
}

func audioVoiceHandlerNativeAudioResponse(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"status": "completed", "steps": []any{
		map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "audio", "mime_type": "audio/wav", "data": base64.StdEncoding.EncodeToString(audioVoiceHandlerTestWAV(1))}}},
	}})
	require.NoError(t, err)
	return body
}

func newAudioVoiceHandlerFixture(t *testing.T, platform string) *audioSpeechHandlerFixture {
	t.Helper()
	f := newAudioSpeechHandlerFixture(t, platform)
	f.handler.cfg.JWT.Secret = "test-voice-envelope-secret-at-least-32-characters"
	response := audioVoiceHandlerNativeAudioResponse(t)
	f.upstream.respond = func(request *http.Request, _ int) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1beta/voices":
			return audioSpeechHandlerResponse(200, "application/json", []byte(`{"voice":{"type":"replicated","key":"`+audioVoiceHandlerPrivateKey+`"}}`)), nil
		case "/v1beta/interactions":
			return audioSpeechHandlerResponse(200, "application/json", response), nil
		default:
			return audioSpeechHandlerResponse(404, "application/json", []byte(`{"error":{"message":"unexpected endpoint"}}`)), nil
		}
	}
	return f
}

func audioVoiceHandlerRequest(f *audioSpeechHandlerFixture, ctx context.Context, path, body string) *httptest.ResponseRecorder {
	ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "same-client-request-id")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), f.key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.key.UserID, Concurrency: 2})
		c.Next()
	})
	router.Use(middleware.GroupModelAllowlist())
	router.POST("/v1/audio/voices", f.handler.AudioVoiceReplication)
	router.POST("/v1/audio/speech", f.handler.AudioSpeech)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func audioVoiceHandlerCreate(t *testing.T, f *audioSpeechHandlerFixture) string {
	t.Helper()
	response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/voices", audioVoiceHandlerCreateBody(t, true))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	id := gjson.Get(response.Body.String(), "id").String()
	require.True(t, strings.HasPrefix(id, service.AudioVoiceTokenPrefix))
	require.NotContains(t, response.Body.String(), audioVoiceHandlerPrivateKey)
	return id
}

func TestAudioVoiceHandlerCreateThenSynthesizeUsesBoundAccountAndBillsBoth(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			f := newAudioVoiceHandlerFixture(t, platform)
			id := audioVoiceHandlerCreate(t, f)
			require.Len(t, f.billing.commands, 1)
			require.True(t, strings.HasPrefix(f.billing.commands[0].RequestID, "voice_clone:"))
			originalAccount := f.upstream.accountIDs[0]
			other := f.accounts.accounts[0]
			other.ID, other.Priority = 9999, -10
			other.AccountGroups = []service.AccountGroup{{AccountID: other.ID, GroupID: *f.key.GroupID}}
			f.accounts.accounts = append(f.accounts.accounts, other)
			response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", audioVoiceHandlerSpeechBody(t, map[string]any{"id": id}))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, audioVoiceHandlerTestWAV(1), response.Body.Bytes())
			require.Len(t, f.upstream.requests, 2)
			require.Equal(t, []int64{originalAccount, originalAccount}, f.upstream.accountIDs)
			require.Equal(t, "/v1beta/voices", f.upstream.requests[0].URL.Path)
			require.Equal(t, "/v1beta/interactions", f.upstream.requests[1].URL.Path)
			require.Equal(t, audioVoiceHandlerPrivateKey, gjson.GetBytes(f.upstream.bodies[1], "generation_config.speech_config.0.voice").String())
			require.Equal(t, "自然温暖", gjson.GetBytes(f.upstream.bodies[1], "input.0.content.0.annotations.0.style").String())
			require.Equal(t, gjson.False, gjson.GetBytes(f.upstream.bodies[0], "store").Type)
			require.NotContains(t, string(f.upstream.bodies[1]), service.AudioVoiceTokenPrefix)
			if platform == service.PlatformGemini {
				require.Equal(t, "test-speech-key", f.upstream.requests[1].Header.Get("x-goog-api-key"))
				require.Empty(t, f.upstream.requests[1].Header.Get("Authorization"))
			} else {
				require.Equal(t, "Bearer test-speech-key", f.upstream.requests[1].Header.Get("Authorization"))
			}
			require.Len(t, f.billing.commands, 2)
			for _, command := range f.billing.commands {
				require.InDelta(t, 0.1, command.BalanceCost, 1e-12)
			}
			require.InDelta(t, 0.2, f.cache.charged, 1e-12)
			createLog, speechLog := <-f.usage.created, <-f.usage.created
			require.Equal(t, "/v1beta/voices", *createLog.UpstreamEndpoint)
			require.Equal(t, "/v1beta/interactions", *speechLog.UpstreamEndpoint)
			logs, err := json.Marshal([]*service.UsageLog{createLog, speechLog})
			require.NoError(t, err)
			require.NotContains(t, string(logs), audioVoiceHandlerPrivateKey)
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioVoiceHandlerRejectsCrossOwnerTamperRotationAndUnavailableAccount(t *testing.T) {
	for _, scenario := range []string{"different key", "different group", "tampered", "credential rotation", "unavailable", "model mapping changed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
			id := audioVoiceHandlerCreate(t, f)
			other := f.accounts.accounts[0]
			other.ID = 9999
			other.AccountGroups = []service.AccountGroup{{AccountID: other.ID, GroupID: *f.key.GroupID}}
			f.accounts.accounts = append(f.accounts.accounts, other)
			switch scenario {
			case "different key":
				key := *f.key
				key.ID++
				f.key = &key
			case "different group":
				key, group := *f.key, *f.key.Group
				group.ID++
				key.GroupID, key.Group = &group.ID, &group
				f.key = &key
			case "tampered":
				position := len(service.AudioVoiceTokenPrefix) + 10
				replacement := byte('A')
				if id[position] == replacement {
					replacement = 'B'
				}
				id = id[:position] + string(replacement) + id[position+1:]
			case "credential rotation":
				f.accounts.accounts[0].Credentials["api_key"] = "rotated-key"
			case "unavailable":
				f.accounts.accounts[0].Schedulable = false
			case "model mapping changed":
				f.accounts.accounts[0].Credentials["model_mapping"] = map[string]any{audioSpeechHandlerModel: "different-model"}
			}
			response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", audioVoiceHandlerSpeechBody(t, id))
			require.GreaterOrEqual(t, response.Code, 400, response.Body.String())
			require.NotContains(t, response.Body.String(), audioVoiceHandlerPrivateKey)
			require.Len(t, f.upstream.requests, 1, "invalid bound voice must not use another available account")
			require.Len(t, f.billing.commands, 1)
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioVoiceHandlerExpiredVoiceDoesNotReachUpstream(t *testing.T) {
	f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
	expires := time.Now().Add(time.Second)
	id, err := f.handler.gatewayService.SealAudioVoice(f.key, &f.accounts.accounts[0], audioSpeechHandlerModel, audioVoiceHandlerPrivateKey, expires)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return time.Now().Unix() >= expires.Unix() }, 2*time.Second, 10*time.Millisecond)
	response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", audioVoiceHandlerSpeechBody(t, id))
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Empty(t, f.upstream.requests)
	require.Empty(t, f.billing.commands)
	f.assertReleased(t, 0)
}

func TestAudioVoiceHandlerBoundUpstreamFailureNeverFallsBack(t *testing.T) {
	f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
	id := audioVoiceHandlerCreate(t, f)
	other := f.accounts.accounts[0]
	other.ID = 9999
	f.accounts.accounts = append(f.accounts.accounts, other)
	f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
		return audioSpeechHandlerResponse(429, "application/json", []byte(`{"error":{"message":"`+audioVoiceHandlerPrivateKey+` rejected"}}`)), nil
	}
	response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", audioVoiceHandlerSpeechBody(t, id))
	require.GreaterOrEqual(t, response.Code, 400)
	require.NotContains(t, response.Body.String(), audioVoiceHandlerPrivateKey)
	require.Len(t, f.upstream.requests, 2)
	require.Equal(t, f.upstream.accountIDs[0], f.upstream.accountIDs[1])
	require.Len(t, f.billing.commands, 1)
	f.assertReleased(t, 0)
}

func TestAudioVoiceHandlerUnsupportedMissingConsentAndMissingPriceDoNotBill(t *testing.T) {
	for _, scenario := range []string{"unsupported", "missing consent", "missing price"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
			body := audioVoiceHandlerCreateBody(t, scenario != "missing consent")
			if scenario == "missing price" {
				f.key.Group.ModelPricing = nil
			}
			if scenario == "unsupported" {
				f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
					return audioSpeechHandlerResponse(404, "application/json", []byte(`{"error":{"message":"no voice endpoint"}}`)), nil
				}
			}
			response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/voices", body)
			if scenario == "unsupported" {
				require.Equal(t, 501, response.Code)
				require.Equal(t, "voice_replication_not_supported", gjson.Get(response.Body.String(), "error.code").String())
				require.Len(t, f.upstream.requests, 1)
			} else {
				require.Equal(t, 400, response.Code, response.Body.String())
				require.Empty(t, f.upstream.requests)
			}
			require.Empty(t, f.billing.commands)
			require.Zero(t, f.cache.charged)
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioVoiceHandlerCancellationKeepsSlotsAndBillsCompletedCreation(t *testing.T) {
	f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, finish := make(chan struct{}), make(chan struct{})
	var finishOnce sync.Once
	release := func() { finishOnce.Do(func() { close(finish) }) }
	defer release()
	f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
		close(started)
		<-finish
		return audioSpeechHandlerResponse(200, "application/json", []byte(`{"key":"`+audioVoiceHandlerPrivateKey+`","type":"replicated"}`)), nil
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	body := audioVoiceHandlerCreateBody(t, true)
	go func() { done <- audioVoiceHandlerRequest(f, ctx, "/v1/audio/voices", body) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("voice creation did not reach upstream")
	}
	cancel()
	require.Never(t, func() bool {
		return atomic.LoadInt32(&f.slots.releaseUserCalled) != 0 || atomic.LoadInt32(&f.slots.releaseAccountCalled) != 0 || f.cache.count() != 1
	}, 100*time.Millisecond, time.Millisecond)
	release()
	select {
	case response := <-done:
		require.Empty(t, response.Body.String())
	case <-time.After(3 * time.Second):
		t.Fatal("voice creation did not finish")
	}
	require.Len(t, f.billing.commands, 1, "completed creation is billable even when the client disconnected")
	require.InDelta(t, 0.1, f.cache.charged, 1e-12)
	f.assertReleased(t, 0)
}

func TestAudioVoiceHandlerRejectsRawProviderKeysAndPrivateSpeakerVoices(t *testing.T) {
	for _, voice := range []any{"voicekey_raw", "voice_raw", map[string]any{"key": "voicekey_raw"}, map[string]any{"id": "Kore", "voice_key": "voicekey_raw"}, []any{"Kore"}, 42} {
		f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
		response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", audioVoiceHandlerSpeechBody(t, voice))
		require.Equal(t, 400, response.Code, response.Body.String())
		require.Empty(t, f.upstream.requests)
		require.Empty(t, f.billing.commands)
	}
	for _, voice := range []string{"voicekey_raw", "voice_raw", "s2voice_private"} {
		f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
		body, err := json.Marshal(map[string]any{"model": audioSpeechHandlerModel, "input": "hello", "voice": "Kore", "speakers": []any{map[string]any{"speaker": "A", "voice": map[string]any{"id": voice}}}})
		require.NoError(t, err)
		response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", string(body))
		require.Equal(t, 400, response.Code, response.Body.String())
		require.Empty(t, f.upstream.requests)
		require.Empty(t, f.billing.commands)
	}
}

func TestAudioVoiceHandlerAllowsNonPrivateScalarVoiceExtensions(t *testing.T) {
	f := newAudioVoiceHandlerFixture(t, service.PlatformGemini)
	f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
		return audioSpeechHandlerResponse(200, "audio/wav", audioSpeechHandlerWAV), nil
	}
	body := `{"model":"gemini-3.8-flash-tts","input":"hello","voice":{"id":"Kore","speed":1,"enabled":true,"optional":null},"speakers":[{"voice":"Kore","enabled":true}]}`
	response := audioVoiceHandlerRequest(f, context.Background(), "/v1/audio/speech", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Len(t, f.upstream.requests, 1)
	require.Equal(t, "/v1/audio/speech", f.upstream.requests[0].URL.Path)
	f.assertReleased(t, 0)
}
