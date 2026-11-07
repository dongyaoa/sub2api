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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseAudioSpeechRequestPreservesProviderParameters(t *testing.T) {
	body := []byte(`{"model":" gemini-3.8-flash-tts ","text":"  你好，世界。  ","style_prompt":"warm tone","voice":{"id":"custom_voice_123"},"speakers":[{"speaker":"Alice","voice":"Kore"}],"provider_option":{"large_id":9007199254740993},"response_format":"wav"}`)
	normalized, model, input, err := ParseAudioSpeechRequest(body)
	require.NoError(t, err)
	require.Equal(t, "gemini-3.8-flash-tts", model)
	require.Equal(t, "  你好，世界。  ", input)
	require.Equal(t, input, gjson.GetBytes(normalized, "input").String())
	require.Equal(t, "warm tone", gjson.GetBytes(normalized, "instructions").String())
	require.Equal(t, "custom_voice_123", gjson.GetBytes(normalized, "voice.id").String())
	require.Equal(t, "Alice", gjson.GetBytes(normalized, "speakers.0.speaker").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "provider_option.large_id").Raw)
	require.False(t, gjson.GetBytes(normalized, "voice_clone").Exists())
}

func TestParseAudioSpeechRequestAliasesAndExplicitInstructions(t *testing.T) {
	for _, alias := range []string{"input", "text", "prompt"} {
		t.Run(alias, func(t *testing.T) {
			body := []byte(`{"model":"custom-model","` + alias + `":"hello","instructions":"keep this","style_prompt":"ignore this","voice":"custom-provider-voice"}`)
			normalized, model, input, err := ParseAudioSpeechRequest(body)
			require.NoError(t, err)
			require.Equal(t, "custom-model", model)
			require.Equal(t, "hello", input)
			require.Equal(t, "hello", gjson.GetBytes(normalized, "input").String())
			require.Equal(t, "keep this", gjson.GetBytes(normalized, "instructions").String())
			require.Equal(t, "custom-provider-voice", gjson.GetBytes(normalized, "voice").String())
		})
	}
}

func TestParseAudioSpeechRequestRejectsInvalidRequiredFields(t *testing.T) {
	for _, body := range []string{
		``, `null`, `[]`, `"hello"`, `{} {}`, `{"model":42,"input":"hi"}`,
		`{"model":null,"input":"hi"}`, `{"model":" ","input":"hi"}`,
		`{"model":"tts"}`, `{"model":"tts","input":null}`, `{"model":"tts","input":["hi"]}`,
		`{"model":"tts","text":true}`, `{"model":"tts","prompt":{}}`, `{"model":"tts","input":" \n"}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, _, _, err := ParseAudioSpeechRequest([]byte(body))
			require.Error(t, err)
		})
	}
}

func TestBuildAudioSpeechURL(t *testing.T) {
	for _, base := range []string{
		"https://speech.example", "https://speech.example/v1", "https://speech.example/v1/",
		"https://speech.example/v1/audio/speech", "https://speech.example/v1beta", "https://speech.example/v1beta/models/",
	} {
		require.Equal(t, "https://speech.example/v1/audio/speech", buildAudioSpeechURL(base))
	}
	require.Equal(t, "https://speech.example/proxy/v1/audio/speech", buildAudioSpeechURL("https://speech.example/proxy/v1beta"))
}

func audioSpeechTestAccount(platform string) *Account {
	return &Account{
		ID: 42, Platform: platform, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test-audio", "base_url": "https://speech.example/v1",
			"model_mapping": map[string]any{"voice-alias": "gemini-3.8-flash-tts"},
		},
	}
}

func audioSpeechTestContext(writer http.ResponseWriter, body []byte) *gin.Context {
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(body))
	c.Request.Header.Set("X-Client-Request-Id", "reused-client-id")
	return c
}

func audioSpeechTestResponse(status int, contentType string, data []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"reused-upstream-id"}},
		Body:       io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)),
	}
}

func TestForwardAudioSpeechPreservesBinaryAndParametersOnBothPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	data := append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), []byte{0, 255, 128, 1, 2, 13, 10}...)
	body := []byte(`{"model":"voice-alias","prompt":"你好","style_prompt":"warm","voice":{"id":"custom"},"speakers":[{"speaker":"A","voice":"Kore"}],"response_format":"wav","custom_number":9007199254740993}`)
	var previousRequestID string
	for _, platform := range []string{PlatformOpenAI, PlatformGemini} {
		t.Run(platform, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := audioSpeechTestContext(rec, body)
			upstream := &httpUpstreamRecorder{resp: audioSpeechTestResponse(http.StatusOK, "audio/wav", data)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.ForwardAudioSpeech(context.Background(), c, audioSpeechTestAccount(platform), body, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.AudioSpeech)
			require.Nil(t, result.AudioUsage)
			require.Equal(t, "voice-alias", result.Model)
			require.Equal(t, "gemini-3.8-flash-tts", result.UpstreamModel)
			require.Equal(t, "gemini-3.8-flash-tts", result.BillingModel)
			require.Equal(t, "/v1/audio/speech", result.UpstreamEndpoint)
			require.True(t, strings.HasPrefix(result.RequestID, "audio_speech:"))
			require.NotEqual(t, previousRequestID, result.RequestID)
			previousRequestID = result.RequestID
			require.Equal(t, "reused-upstream-id", result.UpstreamHeaders.Get("x-request-id"))
			require.Equal(t, data, rec.Body.Bytes())
			require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "https://speech.example/v1/audio/speech", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer sk-test-audio", upstream.lastReq.Header.Get("Authorization"))
			require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))
			require.Equal(t, "gemini-3.8-flash-tts", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "你好", gjson.GetBytes(upstream.lastBody, "input").String())
			require.Equal(t, "warm", gjson.GetBytes(upstream.lastBody, "instructions").String())
			require.Equal(t, "custom", gjson.GetBytes(upstream.lastBody, "voice.id").String())
			require.Equal(t, "Kore", gjson.GetBytes(upstream.lastBody, "speakers.0.voice").String())
			require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "custom_number").Raw)
		})
	}
}

func TestForwardAudioSpeechAcceptsRawPCM(t *testing.T) {
	data := []byte{0, 0, 255, 127, 0, 128, 0, 0}
	body := []byte(`{"model":"voice-alias","input":"hello","response_format":"pcm"}`)
	rec := httptest.NewRecorder()
	upstream := &httpUpstreamRecorder{resp: audioSpeechTestResponse(http.StatusOK, "application/octet-stream", data)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(rec, body), audioSpeechTestAccount(PlatformGemini), body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, data, rec.Body.Bytes())
}

func TestForwardAudioSpeechRejectsNonAudioSuccessWithoutBilling(t *testing.T) {
	for _, test := range []struct{ name, contentType, data string }{
		{"html", "text/html", "<!doctype html><html>login</html>"},
		{"json", "application/json", `{"error":"failed"}`},
		{"mislabeled html", "audio/wav", "<!doctype html><html>login</html>"},
		{"mislabeled json", "application/octet-stream", `{"error":"failed"}`},
		{"empty", "audio/wav", ""},
		{"missing content type", "", "RIFFwave"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"voice-alias","input":"hello"}`)
			rec := httptest.NewRecorder()
			upstream := &httpUpstreamRecorder{resp: audioSpeechTestResponse(http.StatusOK, test.contentType, []byte(test.data))}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(rec, body), audioSpeechTestAccount(PlatformOpenAI), body, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.True(t, gjson.Valid(rec.Body.String()))
		})
	}
}

func TestForwardAudioSpeechUpstreamErrorsDoNotBill(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := []byte(`{"model":"voice-alias","input":"hello"}`)
			errorBody := []byte(`{"error":{"message":"invalid voice"}}`)
			if status == http.StatusForbidden {
				errorBody = []byte(`{"error":{"code":"deactivated_workspace","message":"Workspace is deactivated"}}`)
			}
			rec := httptest.NewRecorder()
			c := audioSpeechTestContext(rec, body)
			upstream := &httpUpstreamRecorder{resp: audioSpeechTestResponse(status, "application/json", errorBody)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.ForwardAudioSpeech(context.Background(), c, audioSpeechTestAccount(PlatformOpenAI), body, "")
			require.Error(t, err)
			require.Nil(t, result)
			if status == http.StatusBadRequest {
				require.Equal(t, status, rec.Code)
				require.Equal(t, errorBody, rec.Body.Bytes())
			} else {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, status, failover.StatusCode)
				require.False(t, c.Writer.Written())
			}
		})
	}
}

func TestForwardAudioSpeechIncompleteOrOversizedBodyDoesNotBill(t *testing.T) {
	for _, mode := range []string{"read error", "content length", "body limit"} {
		t.Run(mode, func(t *testing.T) {
			body := []byte(`{"model":"voice-alias","input":"hello"}`)
			rec := httptest.NewRecorder()
			resp := audioSpeechTestResponse(http.StatusOK, "audio/wav", []byte("RIFFwave"))
			cfg := &config.Config{}
			switch mode {
			case "read error":
				resp.Body = passthroughErrReadCloser{err: io.ErrUnexpectedEOF}
			case "content length":
				resp.ContentLength = 100
			case "body limit":
				cfg.Gateway.UpstreamResponseReadMaxBytes = 4
			}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{resp: resp}}
			result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(rec, body), audioSpeechTestAccount(PlatformOpenAI), body, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.NotContains(t, rec.Body.String(), "RIFF")
		})
	}
}

type audioSpeechFailingWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *audioSpeechFailingWriter) Write(body []byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestForwardAudioSpeechDeliveryFailurePreservesCompletedResult(t *testing.T) {
	body := []byte(`{"model":"voice-alias","input":"hello"}`)
	writer := &audioSpeechFailingWriter{ResponseRecorder: httptest.NewRecorder()}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: audioSpeechTestResponse(http.StatusOK, "audio/wav", []byte("RIFFwave"))}}
	result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(writer, body), audioSpeechTestAccount(PlatformOpenAI), body, "")
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.NotNil(t, result)
	require.True(t, result.AudioSpeech)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 1, writer.writes, "must not append a JSON error after audio delivery begins")
}

func TestForwardAudioSpeechRejectsInvalidRequestAndUnsupportedAccountBeforeSending(t *testing.T) {
	for _, test := range []struct{ name, body, accountType string }{
		{"invalid input", `{"model":"voice-alias","input":42}`, AccountTypeAPIKey},
		{"oauth account", `{"model":"voice-alias","input":"hello"}`, AccountTypeOAuth},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := audioSpeechTestAccount(PlatformOpenAI)
			account.Type = test.accountType
			body := []byte(test.body)
			result, err := svc.ForwardAudioSpeech(context.Background(), audioSpeechTestContext(httptest.NewRecorder(), body), account, body, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, upstream.requests)
		})
	}
}

func TestForwardAudioSpeechTransportErrorIsFailoverWithoutBillableResult(t *testing.T) {
	body := []byte(`{"model":"voice-alias","input":"hello"}`)
	rec := httptest.NewRecorder()
	c := audioSpeechTestContext(rec, body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{err: errors.New("connection reset by peer")}}
	result, err := svc.ForwardAudioSpeech(context.Background(), c, audioSpeechTestAccount(PlatformOpenAI), body, "")
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.False(t, c.Writer.Written())
}

type audioSpeechCancelReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r audioSpeechCancelReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}

func TestForwardAudioSpeechClientCancellationAfterSynthesisStillHasBillingResult(t *testing.T) {
	body := []byte(`{"model":"voice-alias","input":"hello"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := audioSpeechTestResponse(http.StatusOK, "audio/wav", []byte("RIFFwave"))
	resp.Body = io.NopCloser(audioSpeechCancelReader{Reader: bytes.NewReader([]byte("RIFFwave")), cancel: cancel})
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: resp}}
	rec := httptest.NewRecorder()
	c := audioSpeechTestContext(rec, body)
	result, err := svc.ForwardAudioSpeech(ctx, c, audioSpeechTestAccount(PlatformOpenAI), body, "")
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.True(t, result.AudioSpeech)
	require.True(t, result.ClientDisconnect)
	require.False(t, c.Writer.Written())
}

func TestForwardAudioSpeechAlreadyCanceledRequestDoesNotReachUpstream(t *testing.T) {
	body := []byte(`{"model":"voice-alias","input":"hello"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	result, err := svc.ForwardAudioSpeech(ctx, audioSpeechTestContext(httptest.NewRecorder(), body), audioSpeechTestAccount(PlatformOpenAI), body, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Empty(t, upstream.requests)
}
