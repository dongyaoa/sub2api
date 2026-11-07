package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Synthetic zero samples exercise WAV validation; no person's voice is used.
func voiceReplicationTestWAV(samples, sampleRate uint32) []byte {
	data := make([]byte, 44+int(samples)*2)
	copy(data[:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], sampleRate)
	binary.LittleEndian.PutUint32(data[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], samples*2)
	return data
}

func voiceReplicationTestAudio(wav []byte) AudioVoiceReplicationAudio {
	return AudioVoiceReplicationAudio{MIMEType: "audio/wav", Data: base64.StdEncoding.EncodeToString(wav)}
}

func voiceReplicationTestRequest() *AudioVoiceReplicationRequest {
	return &AudioVoiceReplicationRequest{
		Model: "voice-alias", SourceAudio: voiceReplicationTestAudio(voiceReplicationTestWAV(24000*10, 24000)),
		ConsentAudio: voiceReplicationTestAudio(voiceReplicationTestWAV(24000, 24000)), DisplayName: "My test voice",
	}
}

func TestParseAudioVoiceReplicationRequestRequiresBothIndependentRecordings(t *testing.T) {
	request := voiceReplicationTestRequest()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	parsed, err := ParseAudioVoiceReplicationRequest(body)
	require.NoError(t, err)
	require.Equal(t, request, parsed)
	require.NotEqual(t, parsed.SourceAudio.Data, parsed.ConsentAudio.Data)

	request.ConsentAudio = AudioVoiceReplicationAudio{}
	body, _ = json.Marshal(request)
	_, err = ParseAudioVoiceReplicationRequest(body)
	require.ErrorContains(t, err, "consent_audio")
	request = voiceReplicationTestRequest()
	request.SourceAudio = AudioVoiceReplicationAudio{}
	body, _ = json.Marshal(request)
	_, err = ParseAudioVoiceReplicationRequest(body)
	require.ErrorContains(t, err, "source_audio")
	for _, invalid := range []string{`null`, `[]`, `"audio"`, `{}`, `{"model":42}`, `{"model":"m","source_audio":"base64"}`, `{"model":"m","display_name":true}`} {
		_, err = ParseAudioVoiceReplicationRequest([]byte(invalid))
		require.Error(t, err, invalid)
	}
}

func TestAudioVoiceReplicationRecordingDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		samples   uint32
		min, max  uint64
		wantError bool
	}{
		{"reference minimum", 24000 * 10, 10, 30, false},
		{"reference maximum", 24000 * 30, 10, 30, false},
		{"reference too short", 24000*10 - 1, 10, 30, true},
		{"reference too long", 24000*30 + 1, 10, 30, true},
		{"consent positive", 1, 0, 60, false},
		{"consent maximum", 24000 * 60, 0, 60, false},
		{"consent empty", 0, 0, 60, true},
		{"consent too long", 24000*60 + 1, 0, 60, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			audio := voiceReplicationTestAudio(voiceReplicationTestWAV(tc.samples, 24000))
			err := validateAudioVoiceReplicationRecording(&audio, "test_audio", tc.min, tc.max)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAudioVoiceReplicationRejectsInvalidWAVStructures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"non-WAV", func(b []byte) []byte { copy(b[:4], "OggS"); return b }},
		{"truncated RIFF", func(b []byte) []byte { return b[:len(b)-1] }},
		{"invalid RIFF length", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[4:8], 1); return b }},
		{"truncated fmt", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[16:20], 15); return b }},
		{"float WAV", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[20:22], 3); return b }},
		{"stereo", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[22:24], 2); return b }},
		{"24 bit", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[34:36], 24); return b }},
		{"unsupported rate", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[24:28], 12345); return b }},
		{"wrong byte rate", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[28:32], 1); return b }},
		{"wrong block alignment", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[32:34], 4); return b }},
		{"missing data", func(b []byte) []byte { copy(b[36:40], "JUNK"); return b }},
		{"missing format", func(b []byte) []byte { copy(b[12:16], "JUNK"); return b }},
		{"chunk overflow", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[40:44], ^uint32(0)); return b }},
		{"incomplete sample", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[40:44], uint32(len(b)-45)); return b }},
		{"duplicate data", func(b []byte) []byte {
			b = append(b, []byte("data\x02\x00\x00\x00\x00\x00")...)
			binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := audioVoiceReplicationWAVSamples(tc.mutate(voiceReplicationTestWAV(24000, 24000)))
			require.Error(t, err)
		})
	}
}

func TestAudioVoiceReplicationValidatesDataDurationRatherThanFileSize(t *testing.T) {
	wav := voiceReplicationTestWAV(24000, 24000)
	padding := make([]byte, 8+24000*30*2)
	copy(padding[:4], "JUNK")
	binary.LittleEndian.PutUint32(padding[4:8], uint32(len(padding)-8))
	wav = append(wav, padding...)
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	audio := voiceReplicationTestAudio(wav)
	require.ErrorContains(t, validateAudioVoiceReplicationRecording(&audio, "source_audio", 10, 30), "10 to 30 seconds")

	for _, rate := range []uint32{8000, 11025, 16000, 22050, 24000, 32000, 44100, 48000} {
		samples, actualRate, err := audioVoiceReplicationWAVSamples(voiceReplicationTestWAV(rate, rate))
		require.NoError(t, err)
		require.Equal(t, uint64(rate), samples)
		require.Equal(t, uint64(rate), actualRate)
	}
}

func TestAudioVoiceReplicationRejectsWrongMimeMalformedBase64AndOversize(t *testing.T) {
	audio := voiceReplicationTestAudio(voiceReplicationTestWAV(24000*10, 24000))
	audio.MIMEType = "audio/mp3"
	require.ErrorContains(t, validateAudioVoiceReplicationRecording(&audio, "source_audio", 10, 30), "audio/wav")
	audio.MIMEType, audio.Data = "audio/wav", "not valid base64"
	require.ErrorContains(t, validateAudioVoiceReplicationRecording(&audio, "source_audio", 10, 30), "base64")
	audio.Data = strings.Repeat("A", base64.StdEncoding.EncodedLen(maxAudioVoiceReplicationRecordingBytes)+1)
	require.ErrorContains(t, validateAudioVoiceReplicationRecording(&audio, "source_audio", 10, 30), "10 MiB")

	audio = voiceReplicationTestAudio(voiceReplicationTestWAV(24000*10, 24000))
	audio.MIMEType = "audio/x-wav"
	require.NoError(t, validateAudioVoiceReplicationRecording(&audio, "source_audio", 10, 30))
	require.Equal(t, "audio/wav", audio.MIMEType)
}

func TestBuildAudioVoiceReplicationURL(t *testing.T) {
	for _, suffix := range []string{"", "/", "/v1", "/v1/", "/v1beta", "/v1beta/models", "/v1/models", "/v1/audio/speech", "/v1/audio/voices", "/v1beta/voices"} {
		value, err := buildAudioVoiceReplicationURL("https://speech.example" + suffix)
		require.NoError(t, err)
		require.Equal(t, "https://speech.example/v1beta/voices", value)
	}
	value, err := buildAudioVoiceReplicationURL("https://speech.example/proxy/v1")
	require.NoError(t, err)
	require.Equal(t, "https://speech.example/proxy/v1beta/voices", value)
	for _, invalid := range []string{"file:///recordings", "https://key@voice.example/v1", "https://speech.example/v1?key=secret", "https://speech.example/#key"} {
		_, err := buildAudioVoiceReplicationURL(invalid)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestParseAudioVoiceReplicationResponseAcceptsOnlyRealVoiceKeysAndCapsExpiry(t *testing.T) {
	now := time.Date(2026, 11, 7, 4, 0, 0, 0, time.UTC)
	for _, body := range []string{
		`{"key":"voicekey_secret","type":"replicated"}`,
		`{"voice":{"key":"voicekey_secret","type":"replicated"}}`,
		`{"replicated_voice":{"key":"voicekey_secret"}}`,
		`{"voice":{"key":"voicekey_secret"},"expire_time":"2026-12-07T04:00:00Z"}`,
	} {
		key, expiry, err := parseAudioVoiceReplicationResponse([]byte(body), now)
		require.NoError(t, err)
		require.Equal(t, "voicekey_secret", key)
		require.Equal(t, now.Add(7*24*time.Hour), expiry)
	}
	for _, field := range []string{"expire_time", "expireTime"} {
		body := `{"voice":{"key":"voicekey_secret","` + field + `":"2026-11-08T12:00:00+08:00"}}`
		_, expiry, err := parseAudioVoiceReplicationResponse([]byte(body), now)
		require.NoError(t, err)
		require.Equal(t, now.Add(24*time.Hour), expiry)
	}
}

func TestParseAudioVoiceReplicationResponseRejectsPresetMissingExpiredAndError(t *testing.T) {
	now := time.Now().UTC()
	for _, body := range []string{
		`{}`, `null`, `[]`, `<!doctype html><html>login</html>`,
		`{"key":"Kore"}`, `{"key":"voicekey_"}`, `{"voice":{"name":"Kore"}}`,
		`{"key":"voicekey_secret","type":"prebuilt"}`, `{"voice":{"key":"voicekey_secret","type":"prebuilt"}}`,
		`{"key":"voicekey_secret","error":{"message":"failed"}}`,
		`{"voice":{"key":"voicekey_secret","error":{"message":"failed"}}}`,
		`{"key":"voicekey_secret","expire_time":"invalid"}`,
		`{"key":"voicekey_secret","expireTime":"2000-01-01T00:00:00Z"}`,
		`{"key":"voicekey_secret","expire_time":null}`,
		`{"key":"voicekey_secret\nleak"}`,
	} {
		key, expiry, err := parseAudioVoiceReplicationResponse([]byte(body), now)
		require.Error(t, err, body)
		require.Empty(t, key)
		require.True(t, expiry.IsZero())
		require.NotContains(t, err.Error(), "voicekey_secret")
	}
}

func voiceReplicationTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/voices", nil)
	return c, recorder
}

func voiceReplicationTestResponse(status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCreateAudioVoiceUpstreamUsesNativeProtocolForBothPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var previousID string
	for _, platform := range []string{PlatformGemini, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			c, recorder := voiceReplicationTestContext()
			upstream := &httpUpstreamRecorder{resp: voiceReplicationTestResponse(http.StatusOK, "application/json", `{"voice":{"key":"voicekey_test_real_replication","type":"replicated"}}`)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := audioSpeechTestAccount(platform)
			request := voiceReplicationTestRequest()
			result, err := svc.CreateAudioVoiceUpstream(context.Background(), c, account, request, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "voicekey_test_real_replication", result.VoiceKey)
			require.WithinDuration(t, time.Now().Add(7*24*time.Hour), result.ExpireTime, time.Second)
			require.False(t, c.Writer.Written(), "the handler seals the raw key before responding")
			require.Empty(t, recorder.Body.String())
			require.Equal(t, "https://speech.example/v1beta/voices", upstream.lastReq.URL.String())
			require.Equal(t, http.MethodPost, upstream.lastReq.Method)
			require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
			require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
			if platform == PlatformGemini {
				require.Equal(t, "sk-test-audio", upstream.lastReq.Header.Get("x-goog-api-key"))
				require.Empty(t, upstream.lastReq.Header.Get("Authorization"))
			} else {
				require.Equal(t, "Bearer sk-test-audio", upstream.lastReq.Header.Get("Authorization"))
				require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))
			}
			require.Equal(t, gjson.False, gjson.GetBytes(upstream.lastBody, "store").Type)
			require.Equal(t, "gemini-3.8-flash-tts", gjson.GetBytes(upstream.lastBody, "voice.model").String())
			require.Equal(t, "replicated", gjson.GetBytes(upstream.lastBody, "voice.type").String())
			require.Equal(t, request.SourceAudio.Data, gjson.GetBytes(upstream.lastBody, "voice.replicated.source_audio.data").String())
			require.Equal(t, request.ConsentAudio.Data, gjson.GetBytes(upstream.lastBody, "voice.replicated.consent_audio.data").String())
			require.Equal(t, "audio/wav", gjson.GetBytes(upstream.lastBody, "voice.replicated.consent_audio.mime_type").String())
			require.True(t, result.ForwardResult.AudioSpeech)
			require.Equal(t, "/v1beta/voices", result.ForwardResult.UpstreamEndpoint)
			require.Equal(t, "gemini-3.8-flash-tts", result.ForwardResult.UpstreamModel)
			require.True(t, strings.HasPrefix(result.ForwardResult.RequestID, "voice_clone:"))
			require.NotEqual(t, previousID, result.ForwardResult.RequestID)
			previousID = result.ForwardResult.RequestID
		})
	}
}

func TestCreateAudioVoiceUpstreamRejectsUnsupportedRoutesWithoutLeakingRecordings(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, recorder := voiceReplicationTestContext()
			request := voiceReplicationTestRequest()
			secretResponse := `{"error":{"message":"voicekey_secret ` + request.SourceAudio.Data + `"}}`
			upstream := &httpUpstreamRecorder{resp: voiceReplicationTestResponse(status, "application/json", secretResponse)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.CreateAudioVoiceUpstream(context.Background(), c, audioSpeechTestAccount(PlatformGemini), request, "")
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrAudioVoiceReplicationNotSupported)
			require.Equal(t, http.StatusNotImplemented, recorder.Code)
			require.Equal(t, "voice_replication_not_supported", gjson.Get(recorder.Body.String(), "error.code").String())
			assertVoiceReplicationNoSecrets(t, c, recorder, err, request)
		})
	}
}

func assertVoiceReplicationNoSecrets(t *testing.T, c *gin.Context, recorder *httptest.ResponseRecorder, err error, request *AudioVoiceReplicationRequest) {
	t.Helper()
	keys, marshalErr := json.Marshal(c.Keys)
	require.NoError(t, marshalErr)
	for _, text := range []string{recorder.Body.String(), err.Error(), string(keys)} {
		require.NotContains(t, text, "voicekey_secret")
		require.NotContains(t, text, request.SourceAudio.Data)
		require.NotContains(t, text, request.ConsentAudio.Data)
	}
}

func TestCreateAudioVoiceUpstreamErrorsAndInvalidSuccessNeverProduceBillingResult(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            int
		contentType, body string
	}{
		{"bad request", 400, "application/json", `{"error":{"message":"voicekey_secret"}}`},
		{"redirect", 307, "application/json", `{"redirect":"voicekey_secret"}`},
		{"rate limit", 429, "application/json", `{"error":{"message":"voicekey_secret"}}`},
		{"server error", 500, "application/json", `{"error":{"message":"voicekey_secret"}}`},
		{"HTML", 200, "text/html", `<!doctype html><html>voicekey_secret</html>`},
		{"mislabeled HTML", 200, "application/json", `<!doctype html><html>voicekey_secret</html>`},
		{"JSON error", 200, "application/json", `{"error":{"message":"voicekey_secret"}}`},
		{"preset", 200, "application/json", `{"key":"Kore"}`},
		{"expired", 200, "application/json", `{"key":"voicekey_secret","expire_time":"2000-01-01T00:00:00Z"}`},
		{"empty", 200, "application/json", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := voiceReplicationTestContext()
			request := voiceReplicationTestRequest()
			upstream := &httpUpstreamRecorder{resp: voiceReplicationTestResponse(tc.status, tc.contentType, tc.body)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.CreateAudioVoiceUpstream(context.Background(), c, audioSpeechTestAccount(PlatformOpenAI), request, "")
			require.Error(t, err)
			require.Nil(t, result)
			assertVoiceReplicationNoSecrets(t, c, recorder, err, request)
			if tc.status == 429 || tc.status == 500 {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.NotContains(t, string(failover.ResponseBody), "voicekey_secret")
				require.False(t, c.Writer.Written())
			} else {
				require.GreaterOrEqual(t, recorder.Code, 400)
			}
		})
	}
}

func TestCreateAudioVoiceUpstreamValidatesBeforeSendingAndSanitizesTransportErrors(t *testing.T) {
	for _, invalid := range []bool{true, false} {
		c, recorder := voiceReplicationTestContext()
		request := voiceReplicationTestRequest()
		upstream := &httpUpstreamRecorder{err: errors.New("transport echoed voicekey_secret " + request.SourceAudio.Data)}
		if invalid {
			request.ConsentAudio.Data = ""
		}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		result, err := svc.CreateAudioVoiceUpstream(context.Background(), c, audioSpeechTestAccount(PlatformGemini), request, "")
		require.Error(t, err)
		require.Nil(t, result)
		require.NotContains(t, recorder.Body.String(), "voicekey_secret")
		require.NotContains(t, err.Error(), request.SourceAudio.Data)
		if invalid {
			require.Empty(t, upstream.requests)
		} else {
			assertVoiceReplicationNoSecrets(t, c, recorder, err, request)
		}
	}
}

func TestCreateAudioVoiceUpstreamRejectsOversizedOrTruncatedResponse(t *testing.T) {
	for _, mode := range []string{"oversized", "read failure", "content length"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := voiceReplicationTestContext()
			response := voiceReplicationTestResponse(http.StatusOK, "application/json", `{"key":"voicekey_test"}`)
			switch mode {
			case "oversized":
				response.Body = io.NopCloser(strings.NewReader(strings.Repeat(" ", maxAudioVoiceReplicationResponseBytes+1)))
			case "read failure":
				response.Body = passthroughErrReadCloser{err: io.ErrUnexpectedEOF}
			case "content length":
				response.ContentLength = 1000
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: response}}
			result, err := svc.CreateAudioVoiceUpstream(context.Background(), c, audioSpeechTestAccount(PlatformGemini), voiceReplicationTestRequest(), "")
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
