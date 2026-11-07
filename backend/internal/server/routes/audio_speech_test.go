package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAudioSpeechRoutesReachHandlerForSupportedGroups(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformOpenAI} {
		for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
			t.Run(platform+path, func(t *testing.T) {
				router := newGatewayRoutesTestRouter(platform)
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gemini-3.8-flash-tts","input":"hello"}`))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				// The fixture deliberately lacks an authenticated subject. This
				// proves routing passed platform checks and reached the handler.
				require.Equal(t, http.StatusInternalServerError, response.Code)
				require.Contains(t, response.Body.String(), "User context not found")
			})
		}
	}
}

func TestAudioSpeechRoutesEnforceModelAllowlist(t *testing.T) {
	for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
		router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformGemini, true, "gemini-3.8-flash-tts"))
		for _, body := range []string{
			`{"model":"blocked-model","input":"hello"}`,
			`{"model":"gemini-3.8-flash-tts","model":"blocked-model","input":"hello"}`,
		} {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusNotFound, response.Code)
			require.Contains(t, response.Body.String(), "blocked-model")
		}
	}
}

func TestAudioSpeechRoutesRejectUnsupportedPlatformAndOversizeBody(t *testing.T) {
	for _, platform := range []string{service.PlatformGrok, service.PlatformAnthropic} {
		router := newGatewayRoutesTestRouter(platform)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"tts","input":"hello"}`)))
		require.Equal(t, http.StatusNotFound, response.Code)
		require.Contains(t, response.Body.String(), "Audio Speech API")
	}
	for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
		router := newGatewayRoutesTestRouterWithConfig(&config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024, TextMaxBodySize: 32}}, service.PlatformGemini)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gemini-3.8-flash-tts","input":"`+strings.Repeat("a", 60)+`"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	}
}

func TestAudioSpeechRoutesKeepTTSWithoutVoiceCreation(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformOpenAI} {
		router := newGatewayRoutesTestRouter(platform)
		registered := map[string]bool{}
		for _, route := range router.Routes() {
			registered[route.Method+" "+route.Path] = true
		}
		for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
			require.True(t, registered[http.MethodPost+" "+path])
		}
		for _, path := range []string{"/v1/audio/voices", "/audio/voices"} {
			require.False(t, registered[http.MethodPost+" "+path])
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gemini-3.8-flash-tts"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusNotFound, response.Code)
		}
		// Existing Grok voice management is separate from this removed feature.
		require.True(t, registered[http.MethodPost+" /v1/custom-voices"])
		require.True(t, registered[http.MethodPost+" /custom-voices"])
	}
}
