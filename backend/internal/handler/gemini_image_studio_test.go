package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestBuildGeminiStudioImageRequestEditMultipleReferences(t *testing.T) {
	for _, count := range []int{10, 14, 15} {
		t.Run(fmt.Sprintf("%d images", count), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("model", gemini3ProImageModel))
			require.NoError(t, writer.WriteField("prompt", "combine the references"))
			for i := 0; i < count; i++ {
				part, err := writer.CreateFormFile("image", fmt.Sprintf("reference-%d.png", i))
				require.NoError(t, err)
				_, err = part.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, byte(i)})
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			_, nativeBody, err := buildGeminiStudioImageRequest("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
			if count > 14 {
				require.ErrorContains(t, err, "maximum of 14 source images")
				require.Nil(t, nativeBody)
				return
			}
			require.NoError(t, err)
			var request geminiStudioRequest
			require.NoError(t, json.Unmarshal(nativeBody, &request))
			require.Len(t, request.Contents[0].Parts, count+1)
			for i := 0; i < count; i++ {
				inline := request.Contents[0].Parts[i].InlineData
				require.NotNil(t, inline)
				require.Equal(t, "image/png", inline.MimeType)
				data, err := base64.StdEncoding.DecodeString(inline.Data)
				require.NoError(t, err)
				require.Equal(t, byte(i), data[len(data)-1], "reference order must be preserved")
			}
			require.Equal(t, "combine the references", request.Contents[0].Parts[count].Text)
		})
	}
}

func TestBuildGeminiStudioImageRequestEditValidatesEveryReference(t *testing.T) {
	for _, invalidData := range [][]byte{nil, []byte("not an image")} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", gemini3ProImageModel))
		require.NoError(t, writer.WriteField("prompt", "edit both"))
		for i, data := range [][]byte{{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, invalidData} {
			part, err := writer.CreateFormFile("image", fmt.Sprintf("source-%d.png", i))
			require.NoError(t, err)
			_, err = part.Write(data)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		_, nativeBody, err := buildGeminiStudioImageRequest("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
		require.Error(t, err, "an invalid later reference must not be ignored")
		require.Nil(t, nativeBody)
	}
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
