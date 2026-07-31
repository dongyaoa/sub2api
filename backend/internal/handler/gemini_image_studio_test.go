package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGeminiStudioImageRequestGeneration(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-flash-image","prompt":"draw a city","n":1,"resolution":"4k","aspect_ratio":"16:9"}`)
	model, nativeBody, err := buildGeminiStudioImageRequest("/v1/images/generations", "application/json", body)
	require.NoError(t, err)
	require.Equal(t, gemini31FlashImageModel, model)

	var got map[string]any
	require.NoError(t, json.Unmarshal(nativeBody, &got))
	config, ok := got["generationConfig"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"TEXT", "IMAGE"}, config["responseModalities"])
	require.Equal(t, map[string]any{"aspectRatio": "16:9", "imageSize": "4K"}, config["imageConfig"])
	contents, ok := got["contents"].([]any)
	require.True(t, ok)
	content, ok := contents[0].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	firstPart, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "draw a city", firstPart["text"])
}

func TestBuildGeminiStudioImageRequestEdit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", gemini3ProImageModel))
	require.NoError(t, writer.WriteField("prompt", "replace the background"))
	require.NoError(t, writer.WriteField("resolution", "2k"))
	require.NoError(t, writer.WriteField("aspect_ratio", "3:2"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	sourceData := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	_, err = part.Write(sourceData)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	_, nativeBody, err := buildGeminiStudioImageRequest("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(nativeBody, &got))
	contents, ok := got["contents"].([]any)
	require.True(t, ok)
	content, ok := contents[0].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	firstPart, ok := parts[0].(map[string]any)
	require.True(t, ok)
	inline, ok := firstPart["inlineData"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image/png", inline["mimeType"])
	require.Equal(t, base64.StdEncoding.EncodeToString(sourceData), inline["data"])
	secondPart, ok := parts[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "replace the background", secondPart["text"])
}

func TestBuildGeminiStudioImageRequestRejectsUnsupportedInputs(t *testing.T) {
	_, _, err := buildGeminiStudioImageRequest("/v1/images/generations", "application/json", []byte(`{"model":"gemini-2.5-flash-image","prompt":"draw","n":1}`))
	require.ErrorContains(t, err, "model must be")
	_, _, err = buildGeminiStudioImageRequest("/v1/images/generations", "application/json", []byte(`{"model":"gemini-3.1-flash-image","prompt":"draw","n":2}`))
	require.ErrorContains(t, err, "one image per request")
}

func TestNormalizeGeminiStudioImageResponse(t *testing.T) {
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"done"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}}`)
	got, err := normalizeGeminiStudioImageResponse(body)
	require.NoError(t, err)
	var result struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got, &result))
	require.Len(t, result.Data, 1)
	require.Equal(t, "aW1hZ2U=", result.Data[0].B64JSON)
	require.Equal(t, "done", result.Data[0].RevisedPrompt)
}

func TestNormalizeGeminiStudioImageResponseRequiresImage(t *testing.T) {
	_, err := normalizeGeminiStudioImageResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"no image"}]}}]}`))
	require.ErrorContains(t, err, "without returning an image")
}
