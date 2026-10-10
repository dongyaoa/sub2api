package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// This configuration travels only through the authenticated, one-use loopback
// permit. It is never accepted from headers or a public model request body.
type intelligenceChannelPromptConfig struct {
	fallback string
	accounts map[int64]string
}

func intelligenceArtworkPrompt(raw string) (string, bool) {
	prompt := strings.TrimSpace(raw)
	if prompt == "" {
		return IntelligenceMonitorPrompt, true
	}
	return prompt, utf8.ValidString(prompt) && !strings.ContainsRune(prompt, '\x00') && utf8.RuneCountInString(prompt) <= IntelligenceMonitorMaxPromptCharacters
}

func (t *intelligenceExecutionTrace) configurePrompts(run *IntelligenceMonitorRun, fallback string) error {
	if run.SourceType != "local_group" || (run.TestKind != "" && run.TestKind != IntelligenceMonitorTestPelican) {
		return nil
	}
	var channels []IntelligenceChannelPrompt
	if raw := run.SourceSnapshot["channel_prompts"]; raw != nil {
		// Queue snapshots can contain typed structs in memory or decoded JSON
		// after a different worker claims them from persistent storage.
		encoded, err := json.Marshal(raw)
		if err != nil || json.Unmarshal(encoded, &channels) != nil {
			return errors.New("invalid channel prompt snapshot")
		}
	}
	if len(channels) > IntelligenceMonitorMaxChannelPrompts {
		return errors.New("too many channel prompts")
	}
	config := &intelligenceChannelPromptConfig{fallback: fallback, accounts: make(map[int64]string, len(channels))}
	characters := 0
	if fallback != IntelligenceMonitorPrompt {
		characters = utf8.RuneCountInString(fallback)
	}
	for _, channel := range channels {
		prompt, valid := intelligenceArtworkPrompt(channel.Prompt)
		if channel.AccountID <= 0 || !valid {
			return errors.New("invalid channel prompt")
		}
		if _, exists := config.accounts[channel.AccountID]; exists {
			return errors.New("duplicate channel prompt")
		}
		if strings.TrimSpace(channel.Prompt) == "" {
			continue
		}
		characters += utf8.RuneCountInString(prompt)
		if characters > IntelligenceMonitorMaxTotalPromptCharacters {
			return errors.New("channel prompts exceed the total limit")
		}
		config.accounts[channel.AccountID] = prompt
	}
	t.mu.Lock()
	t.promptConfig = config
	t.mu.Unlock()
	return nil
}

func (t *intelligenceExecutionTrace) appliedPrompt() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.prompt
}

// ApplyIntelligenceChannelPrompt selects the current forwarding account's
// captured prompt. Each attempt starts from its own body; failover never inherits
// the previous account's override. All ordinary traffic is returned unchanged.
func ApplyIntelligenceChannelPrompt(ctx context.Context, account *Account, body []byte, mode string) ([]byte, error) {
	if ctx == nil || account == nil || ctx.Value(intelligenceGenerationContextKey{}) != true {
		return body, nil
	}
	trace, _ := ctx.Value(intelligenceExecutionTraceContextKey{}).(*intelligenceExecutionTrace)
	if trace == nil {
		return body, nil
	}
	trace.mu.Lock()
	config := trace.promptConfig
	trace.mu.Unlock()
	if config == nil {
		return body, nil
	}
	prompt := config.fallback
	if override := config.accounts[account.ID]; override != "" {
		prompt = override
	}
	if !gjson.ValidBytes(body) {
		return nil, errors.New("invalid local artwork request")
	}
	var updated []byte
	var err error
	switch mode {
	case MonitorAPIModeResponses:
		updated, err = sjson.SetBytes(body, "input", prompt)
	case MonitorAPIModeChatCompletions:
		updated, err = sjson.SetBytes(body, "messages", []map[string]string{{"role": "user", "content": prompt}})
	default:
		return nil, errors.New("unsupported local artwork request mode")
	}
	if err != nil {
		return nil, errors.New("failed to apply channel artwork prompt")
	}
	trace.mu.Lock()
	trace.prompt = prompt
	trace.mu.Unlock()
	return updated, nil
}
