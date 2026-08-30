package service

import (
	"encoding/json"
	"strings"
)

// normalizeGeminiImageRequestBody converts the responseFormat.image shape
// emitted by canvas.best into Gemini's native generationConfig.imageConfig
// shape. The conversion is intentionally body-driven so it also works when a
// channel maps a custom model alias to a Gemini image model.
func normalizeGeminiImageRequestBody(body []byte) ([]byte, bool, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false, nil
	}

	generationConfig, ok := root["generationConfig"].(map[string]any)
	if !ok {
		return body, false, nil
	}
	responseFormat, ok := firstObject(generationConfig, "responseFormat", "response_format")
	if !ok {
		return body, false, nil
	}
	imageConfig, ok := firstObject(responseFormat, "image")
	if !ok {
		return body, false, nil
	}

	standardImageConfig, ok := firstObject(generationConfig, "imageConfig")
	if !ok {
		if legacyImageConfig, legacyOK := firstObject(generationConfig, "image_config"); legacyOK {
			standardImageConfig = legacyImageConfig
			generationConfig["imageConfig"] = standardImageConfig
			delete(generationConfig, "image_config")
		} else {
			standardImageConfig = make(map[string]any)
			generationConfig["imageConfig"] = standardImageConfig
		}
	}
	copyGeminiImageConfigField(standardImageConfig, imageConfig, "aspectRatio", "aspect_ratio")
	copyGeminiImageConfigField(standardImageConfig, imageConfig, "imageSize", "image_size")

	delete(generationConfig, "responseFormat")
	delete(generationConfig, "response_format")
	if value, ok := standardImageConfig["imageSize"].(string); ok {
		standardImageConfig["imageSize"] = strings.ToUpper(strings.TrimSpace(value))
	}

	normalized, err := json.Marshal(root)
	if err != nil {
		return body, false, err
	}
	return normalized, true, nil
}

func firstObject(parent map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := parent[key].(map[string]any); ok {
			return value, true
		}
	}
	return nil, false
}

func copyGeminiImageConfigField(dst, src map[string]any, canonical string, aliases ...string) {
	if value, exists := dst[canonical]; exists && !isEmptyGeminiImageConfigValue(value) {
		return
	}
	for _, key := range append([]string{canonical}, aliases...) {
		if value, exists := src[key]; exists && !isEmptyGeminiImageConfigValue(value) {
			dst[canonical] = value
			return
		}
	}
}

func isEmptyGeminiImageConfigValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}
