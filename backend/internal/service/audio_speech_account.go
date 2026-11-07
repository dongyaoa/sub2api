package service

import (
	"net/url"
	"strings"
)

// SupportsAudioSpeechAccount admits API-key accounts that can expose the
// OpenAI-compatible speech endpoint. Gemini accounts need a custom upstream:
// Google's native API does not expose /v1/audio/speech.
func SupportsAudioSpeechAccount(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey || strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return false
	}
	switch account.Platform {
	case PlatformOpenAI:
	case PlatformGemini:
		base := strings.TrimSpace(account.GetCredential("base_url"))
		parsed, err := url.Parse(base)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		if host == "googleapis.com" || strings.HasSuffix(host, ".googleapis.com") || host == "aistudio.google.com" {
			return false
		}
	default:
		return false
	}

	// Existing account forms encode general API access as chat_completions.
	// Keep those accounts usable while honoring an explicit speech opt-out.
	if raw, ok := account.Credentials[openAIEndpointCapabilitiesCredentialKey]; ok {
		switch configured := raw.(type) {
		case map[string]any:
			if enabled, exists := configured[string(OpenAIEndpointCapabilityAudioSpeech)]; exists {
				value, valid := enabled.(bool)
				return valid && value
			}
		case map[string]bool:
			if enabled, exists := configured[string(OpenAIEndpointCapabilityAudioSpeech)]; exists {
				return enabled
			}
		}
	}
	configured, found := account.openAIEndpointCapabilitySet()
	return !found || configured[string(OpenAIEndpointCapabilityAudioSpeech)] || configured[string(OpenAIEndpointCapabilityChatCompletions)]
}
