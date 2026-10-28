package service

import (
	"maps"
	"strings"
)

const legacyOpenAICodexTicketExtraKeyPrefix = "codex_turn_ticket:"

// IsLegacyOpenAICodexTicketPrivateExtraKey identifies retired ticket state that
// may remain on older account records. Keep filtering it from account DTOs and
// exports after ticket collection has been removed.
func IsLegacyOpenAICodexTicketPrivateExtraKey(key string) bool {
	return strings.HasPrefix(key, legacyOpenAICodexTicketExtraKeyPrefix) || key == "codex_harvest_proxy_url"
}

func RedactLegacyOpenAICodexTicketExtra(extra map[string]any) map[string]any {
	redacted := maps.Clone(extra)
	for key := range redacted {
		if IsLegacyOpenAICodexTicketPrivateExtraKey(key) {
			delete(redacted, key)
		}
	}
	return redacted
}
