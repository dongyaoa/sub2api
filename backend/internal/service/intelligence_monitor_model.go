package service

import (
	"regexp"
	"strings"
)

const (
	IntelligenceMonitorAstra62Model    = "gpt-6.2-astra"
	IntelligenceMonitorSol62Model      = "gpt-6.2-sol"
	IntelligenceMonitorClaudeDefault   = "claude-opus-5-5"
	IntelligenceMonitorAPIModeMessages = "messages"
)

var intelligenceClaudeModelID = regexp.MustCompile(`^claude-[a-z0-9]+([.-][a-z0-9]+)*$`)

// NormalizeIntelligenceReasoningEffort defaults legacy plans and snapshots to high.
func NormalizeIntelligenceReasoningEffort(raw string) (string, error) {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "":
		return IntelligenceMonitorReasoning, nil
	case "low", "medium", "high", "xhigh", "max":
		return value, nil
	default:
		return "", ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": "reasoning_effort", "detail": "choose low, medium, high, xhigh or max"})
	}
}

// Omitted models retain the original comparison model for legacy plans/runs.
func intelligenceMonitorModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return IntelligenceMonitorModel
	}
	return model
}

func intelligenceMonitorModelProvider(model string) string {
	switch model {
	case IntelligenceMonitorModel, IntelligenceMonitorSolModel, IntelligenceMonitorAstra62Model, IntelligenceMonitorSol62Model:
		return MonitorProviderOpenAI
	}
	// Keep the original Claude wire ID, including dated or provider-specific variants.
	if len(model) <= 100 && intelligenceClaudeModelID.MatchString(model) {
		return MonitorProviderAnthropic
	}
	return ""
}

func validateIntelligenceMonitorModel(model string) error {
	if intelligenceMonitorModelProvider(model) == "" {
		return ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": "model", "detail": "choose a supported GPT monitoring model or a valid Claude model ID"})
	}
	return nil
}

// ValidateIntelligenceMonitorProtocol checks the model and route before saving or dispatching a plan.
func ValidateIntelligenceMonitorProtocol(model, mode, source string) error {
	switch intelligenceMonitorModelProvider(model) {
	case MonitorProviderAnthropic:
		if mode == IntelligenceMonitorAPIModeMessages && (source == "external" || source == "upstream") {
			return nil
		}
	case MonitorProviderOpenAI:
		if mode == MonitorAPIModeResponses || (mode == MonitorAPIModeChatCompletions && source != "openai_oauth") {
			return nil
		}
	default:
		return validateIntelligenceMonitorModel(model)
	}
	return ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": "api_mode", "detail": "the selected model, protocol and source must be compatible"})
}
