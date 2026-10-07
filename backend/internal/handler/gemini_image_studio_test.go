package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestBuildGeminiStudioImageRequestAcceptsGeminiProImage(t *testing.T) {
	model, _, err := buildGeminiStudioImageRequest(
		"/v1/images/generations",
		"application/json",
		[]byte(`{"model":"gemini-3-pro-image","prompt":"draw a banana","n":1,"resolution":"1K"}`),
	)
	require.NoError(t, err)
	require.Equal(t, gemini3ProImageLegacyModel, model)
}

func TestBuildGeminiStudioImageRequestAcceptsConfiguredAliases(t *testing.T) {
	for _, modelID := range []string{"gemini-nano-banana-2.1", "CustomBanana2", "gemini-2.5-flash-image"} {
		t.Run(modelID, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"model": modelID, "prompt": "draw a banana", "n": 1})
			require.NoError(t, err)
			model, _, err := buildGeminiStudioImageRequest("/v1/images/generations", "application/json", body)
			require.NoError(t, err)
			require.Equal(t, modelID, model)
		})
	}
}

func TestPrepareGeminiStudioImageContextPreservesConfiguredAlias(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"CustomBanana2","prompt":"draw a banana","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	require.NoError(t, prepareGeminiStudioImageContext(c))
	require.Equal(t, "/v1beta/models/CustomBanana2:generateContent", c.Request.URL.Path)
	require.Equal(t, "CustomBanana2:generateContent", c.Param("modelAction"))
	convertedBody, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	require.Contains(t, string(convertedBody), `"responseModalities":["TEXT","IMAGE"]`)
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
	for _, modelID := range []string{"../gemini-nano-banana-2.1", "gemini/image", "banana:generateContent", "banana?key=test"} {
		body, err := json.Marshal(map[string]any{"model": modelID, "prompt": "draw", "n": 1})
		require.NoError(t, err)
		_, _, err = buildGeminiStudioImageRequest("/v1/images/generations", "application/json", body)
		require.ErrorContains(t, err, "invalid model")
	}
	_, _, err := buildGeminiStudioImageRequest("/v1/images/generations", "application/json", []byte(`{"prompt":"draw","n":1}`))
	require.ErrorContains(t, err, "model is required")
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
