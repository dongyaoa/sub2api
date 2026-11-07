package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAudioSpeechEndpointLogging(t *testing.T) {
	for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
		require.Equal(t, EndpointAudioSpeech, NormalizeInboundEndpoint(path))
		for _, platform := range []string{service.PlatformGemini, service.PlatformOpenAI} {
			require.Equal(t, EndpointAudioSpeech, DeriveUpstreamEndpoint(NormalizeInboundEndpoint(path), path, platform))
		}
	}
	// Speech dispatch must not change the existing Gemini text path.
	require.Equal(t, EndpointGeminiModels, DeriveUpstreamEndpoint(EndpointChatCompletions, "/v1/chat/completions", service.PlatformGemini))
	for _, path := range []string{"/v1/audio/voices", "/audio/voices"} {
		require.Equal(t, EndpointAudioVoices, NormalizeInboundEndpoint(path))
		for _, platform := range []string{service.PlatformGemini, service.PlatformOpenAI} {
			require.Equal(t, EndpointGeminiVoices, DeriveUpstreamEndpoint(NormalizeInboundEndpoint(path), path, platform))
		}
	}
}
