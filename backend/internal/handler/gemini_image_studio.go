package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	gemini31FlashImageModel = "gemini-3.1-flash-image"
	gemini3ProImageModel    = "gemini-3-pro-image-preview"
	maxGeminiSourceImage    = 20 << 20
)

var geminiStudioAspectRatios = map[string]struct{}{
	"1:1": {}, "16:9": {}, "9:16": {}, "4:3": {}, "3:4": {}, "3:2": {}, "2:3": {},
}

type geminiStudioInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiStudioPart struct {
	Text       string                  `json:"text,omitempty"`
	InlineData *geminiStudioInlineData `json:"inlineData,omitempty"`
}

type geminiStudioContent struct {
	Role  string             `json:"role"`
	Parts []geminiStudioPart `json:"parts"`
}

type geminiStudioRequest struct {
	Contents         []geminiStudioContent `json:"contents"`
	GenerationConfig struct {
		ResponseModalities []string `json:"responseModalities"`
		ImageConfig        struct {
			AspectRatio string `json:"aspectRatio"`
			ImageSize   string `json:"imageSize"`
		} `json:"imageConfig"`
	} `json:"generationConfig"`
}

func isGeminiStudioImageModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case gemini31FlashImageModel, gemini3ProImageModel:
		return true
	default:
		return false
	}
}

func buildGeminiStudioImageRequest(path, contentType string, body []byte) (string, []byte, error) {
	metadata := parseAsyncImageTaskMetadata(path, contentType, body)
	model := strings.ToLower(strings.TrimSpace(metadata.Model))
	if !isGeminiStudioImageModel(model) {
		return "", nil, fmt.Errorf("model must be %s or %s", gemini31FlashImageModel, gemini3ProImageModel)
	}
	if strings.TrimSpace(metadata.Prompt) == "" {
		return "", nil, errors.New("prompt is required")
	}
	if metadata.Quantity > 1 {
		return "", nil, errors.New("Gemini image generation supports one image per request")
	}

	resolution := strings.TrimSpace(metadata.Resolution)
	if resolution == "" {
		resolution = strings.TrimSpace(metadata.Size)
	}
	resolution = service.NormalizeImageBillingTierOrDefault(resolution)
	aspectRatio := strings.TrimSpace(metadata.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "1:1"
	}
	if _, ok := geminiStudioAspectRatios[aspectRatio]; !ok {
		return "", nil, fmt.Errorf("unsupported aspect_ratio %q", aspectRatio)
	}

	parts := make([]geminiStudioPart, 0, 2)
	if strings.Contains(path, "/edits") {
		inline, err := geminiStudioSourceImage(contentType, body)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, geminiStudioPart{InlineData: inline})
	}
	parts = append(parts, geminiStudioPart{Text: strings.TrimSpace(metadata.Prompt)})

	request := geminiStudioRequest{Contents: []geminiStudioContent{{Role: "user", Parts: parts}}}
	request.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
	request.GenerationConfig.ImageConfig.AspectRatio = aspectRatio
	request.GenerationConfig.ImageConfig.ImageSize = resolution
	nativeBody, err := json.Marshal(request)
	if err != nil {
		return "", nil, fmt.Errorf("encode Gemini image request: %w", err)
	}
	return model, nativeBody, nil
}

func geminiStudioSourceImage(contentType string, body []byte) (*geminiStudioInlineData, error) {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, errors.New("Gemini image editing requires multipart/form-data")
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, errors.New("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			return nil, errors.New("failed to parse source image")
		}
		if part.FormName() != "image" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(part, maxGeminiSourceImage+1))
		mimeType := strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Type")))
		_ = part.Close()
		if readErr != nil {
			return nil, errors.New("failed to read source image")
		}
		if len(data) == 0 {
			return nil, errors.New("source image is empty")
		}
		if len(data) > maxGeminiSourceImage {
			return nil, errors.New("source image exceeds 20 MB")
		}
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = strings.ToLower(http.DetectContentType(data))
		}
		switch mimeType {
		case "image/png", "image/jpeg", "image/webp":
		default:
			return nil, errors.New("source image must be PNG, JPEG, or WebP")
		}
		return &geminiStudioInlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(data)}, nil
	}
	return nil, errors.New("source image is required")
}

func (h *AsyncImageHandler) executeGeminiStudioImage(c *gin.Context) {
	if h.gemini == nil {
		imageTaskJSONError(c, http.StatusServiceUnavailable, "api_error", "Gemini image gateway is unavailable")
		return
	}
	if err := prepareGeminiStudioImageContext(c); err != nil {
		imageTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	h.gemini.GeminiV1BetaModels(c)
}

func prepareGeminiStudioImageContext(c *gin.Context) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return errors.New("failed to read image request")
	}
	model, nativeBody, err := buildGeminiStudioImageRequest(c.Request.URL.Path, c.GetHeader("Content-Type"), body)
	if err != nil {
		return err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(nativeBody))
	c.Request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(nativeBody)), nil
	}
	c.Request.ContentLength = int64(len(nativeBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.URL.Path = "/v1beta/models/" + model + ":generateContent"
	for index := range c.Params {
		if c.Params[index].Key == "modelAction" {
			c.Params[index].Value = model + ":generateContent"
			return nil
		}
	}
	c.Params = append(c.Params, gin.Param{Key: "modelAction", Value: model + ":generateContent"})
	return nil
}

func normalizeGeminiImageTaskBody(platform string, body []byte) ([]byte, error) {
	if platform != service.PlatformGemini {
		return body, nil
	}
	return normalizeGeminiStudioImageResponse(body)
}

func normalizeGeminiStudioImageResponse(body []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, errors.New("Gemini returned an invalid image response")
	}
	response := root
	if nested, ok := root["response"].(map[string]any); ok {
		response = nested
	}
	candidates, _ := response["candidates"].([]any)
	images := make([]map[string]any, 0, len(candidates))
	for _, candidateValue := range candidates {
		candidate, _ := candidateValue.(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		var responseText strings.Builder
		for _, partValue := range parts {
			part, _ := partValue.(map[string]any)
			if text, _ := part["text"].(string); text != "" {
				responseText.WriteString(text)
			}
			inline, _ := part["inlineData"].(map[string]any)
			if inline == nil {
				inline, _ = part["inline_data"].(map[string]any)
			}
			data, _ := inline["data"].(string)
			mimeType, _ := inline["mimeType"].(string)
			if mimeType == "" {
				mimeType, _ = inline["mime_type"].(string)
			}
			if strings.TrimSpace(data) == "" || !isGeminiStudioOutputMimeType(mimeType) {
				continue
			}
			if _, err := base64.StdEncoding.DecodeString(data); err != nil {
				continue
			}
			item := map[string]any{"b64_json": data}
			if revisedPrompt := strings.TrimSpace(responseText.String()); revisedPrompt != "" {
				item["revised_prompt"] = revisedPrompt
			}
			images = append(images, item)
		}
	}
	if len(images) == 0 {
		return nil, errors.New("Gemini completed without returning an image")
	}
	result := map[string]any{"created": time.Now().Unix(), "data": images}
	return json.Marshal(result)
}

func isGeminiStudioOutputMimeType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}
