package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeGeminiImageRequestBody(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{name: "gemini 3.1 flash image", model: "gemini-3.1-flash-image"},
		{name: "gemini 3 pro image", model: "gemini-3-pro-image"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"draw a cat"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"],"responseFormat":{"image":{"aspectRatio":"16:9","imageSize":"4k"}}}}`)

			normalized, changed, err := normalizeGeminiImageRequestBody(body)
			require.NoError(t, err)
			require.True(t, changed)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(normalized, &payload))
			generationConfig := payload["generationConfig"].(map[string]any)
			require.NotContains(t, generationConfig, "responseFormat")
			imageConfig := generationConfig["imageConfig"].(map[string]any)
			require.Equal(t, "16:9", imageConfig["aspectRatio"])
			require.Equal(t, "4K", imageConfig["imageSize"])
		})
	}
}

func TestNormalizeGeminiImageRequestBodyPreservesStandardConfig(t *testing.T) {
	body := []byte(`{"generationConfig":{"imageConfig":{"aspectRatio":"1:1","imageSize":"2K"},"responseFormat":{"image":{"aspectRatio":"16:9","imageSize":"4K"}}}}`)

	normalized, changed, err := normalizeGeminiImageRequestBody(body)
	require.NoError(t, err)
	require.True(t, changed)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(normalized, &payload))
	generationConfig := payload["generationConfig"].(map[string]any)
	imageConfig := generationConfig["imageConfig"].(map[string]any)
	require.Equal(t, "1:1", imageConfig["aspectRatio"])
	require.Equal(t, "2K", imageConfig["imageSize"])
}

func TestNormalizeGeminiImageRequestBodyNoop(t *testing.T) {
	body := []byte(`{"generationConfig":{"imageConfig":{"aspectRatio":"16:9","imageSize":"4K"}}}`)

	normalized, changed, err := normalizeGeminiImageRequestBody(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(normalized))
}
