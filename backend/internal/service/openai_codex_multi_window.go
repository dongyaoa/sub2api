package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type codexMultiWindowIdentity struct {
	codexClient bool
	compact     bool
	cacheKey    string
	requestID   string
}

type codexMultiWindowSession struct {
	session string
	thread  string
	window  string
}

const codexMultiWindowSessionContextKey = "codex_multi_window_ws_session"

func isCodexMultiWindowAccount(account *Account) bool {
	if account == nil || account.GetCodexFingerprintMode() != codexFingerprintMultiWindow {
		return false
	}
	_, valid := codexFingerprintSeed(account.Extra)
	return valid
}

// Length-delimited inputs keep caller-controlled identifiers in separate namespaces.
func codexMultiWindowPseudonym(seed string, apiKeyID int64, kind string, values ...string) string {
	mac := hmac.New(sha256.New, []byte(seed))
	for _, value := range append([]string{"sub2api:codex-multi-window:v1", strconv.FormatInt(apiKeyID, 10), kind}, values...) {
		fmt.Fprintf(mac, "%d:%s", len(value), value)
	}
	var id uuid.UUID
	copy(id[:], mac.Sum(nil))
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func codexMultiWindowString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func codexMultiWindowEmbedded(values map[string]any) map[string]any {
	var embedded map[string]any
	if raw, ok := values[openAIWSTurnMetadataHeader].(string); ok {
		_ = json.Unmarshal([]byte(raw), &embedded)
	}
	return embedded
}

func codexMultiWindowMetadata(body map[string]any) map[string]any {
	switch value := body["client_metadata"].(type) {
	case map[string]any:
		return value
	case map[string]string:
		converted := make(map[string]any, len(value))
		for key, item := range value {
			converted[key] = item
		}
		return converted
	default:
		return nil
	}
}

func resolveCodexMultiWindowIDs(c *gin.Context, account *Account, body map[string]any) *codexFingerprintIDs {
	source := codexAccountIdentitySource(c, account)
	if !isCodexMultiWindowAccount(source) || account == nil {
		return nil
	}
	seed, ok := codexFingerprintSeed(source.Extra)
	if !ok {
		return nil
	}
	var headers http.Header
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	identity := &codexMultiWindowIdentity{
		codexClient: openai.IsCodexOfficialClientRequestStrict(headers.Get("User-Agent")) || openai.IsCodexOfficialClientOriginator(headers.Get("originator")),
		compact:     c != nil && isOpenAIResponsesCompactPath(c),
	}
	ids := &codexFingerprintIDs{accountID: account.ID, mode: codexFingerprintMultiWindow, multiWindow: identity}
	metadata := codexMultiWindowMetadata(body)
	embedded := codexMultiWindowEmbedded(metadata)
	var headerMetadata map[string]any
	_ = json.Unmarshal([]byte(headers.Get(openAIWSTurnMetadataHeader)), &headerMetadata)
	read := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(headers.Get(key)); value != "" {
				return value
			}
		}
		return firstNonEmptyString(codexMultiWindowString(metadata, keys...), codexMultiWindowString(embedded, keys...), codexMultiWindowString(headerMetadata, keys...))
	}
	rawSession := read("session-id", "session_id")
	rawThread := read("thread-id", "thread_id")
	rawWindow := read("x-codex-window-id", "window_id")
	websocketSession := identity.codexClient && c != nil && c.Request != nil && c.IsWebsocket()
	if websocketSession {
		// Later response.create frames may omit the identity sent before the
		// handshake. Retain raw IDs so account failover can still remap them.
		if stored, exists := c.Get(codexMultiWindowSessionContextKey); exists {
			if previous, ok := stored.(codexMultiWindowSession); ok && (rawSession == "" || rawSession == previous.session) {
				rawSession = firstNonEmptyString(rawSession, previous.session)
				rawThread = firstNonEmptyString(rawThread, previous.thread)
				rawWindow = firstNonEmptyString(rawWindow, previous.window)
			}
		}
	}
	rawCacheKey := codexMultiWindowString(body, "prompt_cache_key")
	if rawCacheKey != "" {
		kind := "prompt-cache"
		if rawCacheKey == rawSession {
			kind = "session"
		}
		identity.cacheKey = codexMultiWindowPseudonym(seed, apiKeyID, kind, rawCacheKey)
	}
	// Non-Codex clients only receive cache-key pseudonymization.
	if !identity.codexClient {
		return ids
	}
	ids.installationID = resolveConvergedInstallationID(source, seed)
	if rawSession == "" {
		rawSession = firstNonEmptyString(rawThread, rawWindow, rawCacheKey)
	}
	if rawSession == "" {
		// Missing conversation identity must never collapse unrelated requests.
		// Keep the fallback within this downstream request, including failover.
		const fallbackKey = "codex_multi_window_request_seed"
		if c != nil {
			rawSession = c.GetString(fallbackKey)
		}
		if rawSession == "" {
			rawSession = uuid.NewString()
			if c != nil {
				c.Set(fallbackKey, rawSession)
			}
		}
	}
	ids.sessionID = codexMultiWindowPseudonym(seed, apiKeyID, "session", rawSession)
	if rawThread == "" {
		rawThread = rawSession
	}
	ids.threadID = codexMultiWindowPseudonym(seed, apiKeyID, "thread", rawSession, rawThread)
	if rawWindow == "" {
		rawWindow = rawThread
	}
	if websocketSession {
		c.Set(codexMultiWindowSessionContextKey, codexMultiWindowSession{session: rawSession, thread: rawThread, window: rawWindow})
	}
	ids.windowID = codexMultiWindowPseudonym(seed, apiKeyID, "window", rawSession, rawWindow) + ":0"
	if rawTurn := read("turn-id", "turn_id"); rawTurn != "" {
		ids.turnID = codexMultiWindowPseudonym(seed, apiKeyID, "turn", rawSession, rawTurn)
	}
	if rawRequest := strings.TrimSpace(headers.Get("x-client-request-id")); rawRequest != "" {
		identity.requestID = codexMultiWindowPseudonym(seed, apiKeyID, "request", rawRequest)
	}
	return ids
}

func applyCodexMultiWindowRequest(c *gin.Context, account *Account, body map[string]any) bool {
	ids := resolveCodexMultiWindowIDs(c, account, body)
	stageCodexFingerprintIDs(c, ids)
	return applyCodexFingerprintClientMetadata(body, ids)
}

func applyCodexMultiWindowRequestRaw(c *gin.Context, account *Account, body []byte) ([]byte, bool, error) {
	if !isCodexMultiWindowAccount(codexAccountIdentitySource(c, account)) {
		return body, false, nil
	}
	if eventType := gjson.GetBytes(body, "type").String(); eventType != "" && eventType != "response.create" {
		return body, false, nil
	}
	identityBody, err := codexMultiWindowIdentityBody(body)
	if err != nil {
		return body, false, err
	}
	ids := resolveCodexMultiWindowIDs(c, account, identityBody)
	stageCodexFingerprintIDs(c, ids)
	return applyCodexFingerprintClientMetadataRaw(body, ids)
}

// Decode only the small identity subobject; input/tools may be many megabytes.
func codexMultiWindowIdentityBody(body []byte) (map[string]any, error) {
	if !gjson.ParseBytes(body).IsObject() {
		return nil, nil
	}
	result := make(map[string]any, 2)
	if metadata := gjson.GetBytes(body, "client_metadata"); metadata.IsObject() {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(metadata.Raw), &decoded); err != nil {
			return nil, fmt.Errorf("decode multi-window metadata: %w", err)
		}
		result["client_metadata"] = decoded
	}
	if key := gjson.GetBytes(body, "prompt_cache_key"); key.Type == gjson.String {
		result["prompt_cache_key"] = key.String()
	}
	return result, nil
}

func codexMultiWindowFields(ids *codexFingerprintIDs) map[string]any {
	fields := map[string]any{
		"installation_id": ids.installationID,
		"session_id":      ids.sessionID,
		"thread_id":       ids.threadID,
		"window_id":       ids.windowID,
	}
	if ids.turnID != "" {
		fields["turn_id"] = ids.turnID
	}
	return fields
}

func rewriteCodexMultiWindowAliases(metadata map[string]any, ids *codexFingerprintIDs) {
	for key, value := range map[string]string{
		"x-codex-installation-id": ids.installationID, "session-id": ids.sessionID,
		"thread-id": ids.threadID, "turn-id": ids.turnID, "x-codex-window-id": ids.windowID,
		"x-client-request-id": ids.multiWindow.requestID,
	} {
		if _, exists := metadata[key]; exists && value != "" {
			metadata[key] = value
		}
	}
}

func rewriteCodexMultiWindowEmbedded(raw string, ids *codexFingerprintIDs) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any)
	}
	for key, value := range codexMultiWindowFields(ids) {
		metadata[key] = value
	}
	rewriteCodexMultiWindowAliases(metadata, ids)
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return string(rebuilt)
}

func applyCodexMultiWindowHeaders(headers http.Header, ids *codexFingerprintIDs) {
	if ids.multiWindow == nil || !ids.multiWindow.codexClient {
		return
	}
	headers.Set("x-codex-installation-id", ids.installationID)
	headers.Set("x-codex-window-id", ids.windowID)
	headers.Set("session-id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
	for _, obsolete := range []string{"session_id", "conversation_id", "thread_id"} {
		headers.Del(obsolete)
	}
	if ids.multiWindow.requestID != "" {
		headers.Set("x-client-request-id", ids.multiWindow.requestID)
	}
	if ids.turnID != "" {
		for _, key := range []string{"turn-id", "turn_id"} {
			if headers.Get(key) != "" {
				headers.Set(key, ids.turnID)
			}
		}
	}
	if raw := headers.Get(openAIWSTurnMetadataHeader); raw != "" {
		headers.Set(openAIWSTurnMetadataHeader, rewriteCodexMultiWindowEmbedded(raw, ids))
	}
}

func applyCodexMultiWindowBody(body map[string]any, ids *codexFingerprintIDs) bool {
	if body == nil || ids.multiWindow == nil {
		return false
	}
	changed := false
	if ids.multiWindow.cacheKey != "" {
		if existing, ok := body["prompt_cache_key"].(string); ok && existing != "" && existing != ids.multiWindow.cacheKey {
			body["prompt_cache_key"] = ids.multiWindow.cacheKey
			changed = true
		}
	}
	if !ids.multiWindow.codexClient {
		return changed
	}
	metadata := codexMultiWindowMetadata(body)
	if metadata == nil {
		if ids.multiWindow.compact {
			return changed
		}
		metadata = make(map[string]any)
	}
	fields := codexMultiWindowFields(ids)
	for key, value := range fields {
		if _, exists := metadata[key]; ids.multiWindow.compact && !exists {
			continue
		}
		metadata[key] = value
	}
	if !ids.multiWindow.compact {
		metadata["x-codex-installation-id"] = ids.installationID
		metadata["x-codex-window-id"] = ids.windowID
	}
	rewriteCodexMultiWindowAliases(metadata, ids)
	if raw, ok := metadata[openAIWSTurnMetadataHeader].(string); ok {
		metadata[openAIWSTurnMetadataHeader] = rewriteCodexMultiWindowEmbedded(raw, ids)
	}
	body["client_metadata"] = metadata
	return true
}

func applyCodexMultiWindowBodyRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	identityBody, err := codexMultiWindowIdentityBody(body)
	if err != nil || identityBody == nil {
		return body, false, err
	}
	if !applyCodexMultiWindowBody(identityBody, ids) {
		return body, false, nil
	}
	next := body
	if _, exists := identityBody["client_metadata"]; ids.multiWindow.codexClient && exists {
		metadata, err := json.Marshal(identityBody["client_metadata"])
		if err != nil {
			return body, false, err
		}
		next, err = sjson.SetRawBytes(next, "client_metadata", metadata)
		if err != nil {
			return body, false, err
		}
	}
	if ids.multiWindow.cacheKey != "" {
		next, err = sjson.SetBytes(next, "prompt_cache_key", ids.multiWindow.cacheKey)
		if err != nil {
			return body, false, err
		}
	}
	return next, true, nil
}
