package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func multiWindowTestContext(apiKeyID int64, session, window string, codex bool) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: apiKeyID})
	if codex {
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	}
	if session != "" {
		c.Request.Header.Set("session-id", session)
	}
	if window != "" {
		c.Request.Header.Set("x-codex-window-id", window)
	}
	return c
}

func multiWindowTestAccount() *Account {
	return newTestOAuthAccount(5001, map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintMultiWindow)})
}

func TestCodexMultiWindowPreservesTenantAndWindowBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := multiWindowTestAccount()
	const count = 40
	results := make([]*codexFingerprintIDs, count)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := multiWindowTestContext(int64(i+1), "same-client-session", "same-client-window", true)
			results[i] = resolveCodexMultiWindowIDs(c, account, map[string]any{"prompt_cache_key": "same-client-session"})
		}(i)
	}
	wg.Wait()
	sessions, windows, caches := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, ids := range results {
		require.NotNil(t, ids)
		require.Equal(t, results[0].installationID, ids.installationID)
		require.NotEmpty(t, ids.installationID)
		require.False(t, sessions[ids.sessionID])
		require.False(t, windows[ids.windowID])
		require.False(t, caches[ids.multiWindow.cacheKey])
		sessions[ids.sessionID], windows[ids.windowID], caches[ids.multiWindow.cacheKey] = true, true, true
		again := resolveCodexMultiWindowIDs(multiWindowTestContext(int64(i+1), "same-client-session", "same-client-window", true), account, map[string]any{"prompt_cache_key": "same-client-session"})
		require.Equal(t, ids, again, "same account seed survives reconnects and process restarts")
	}
	a := resolveCodexMultiWindowIDs(multiWindowTestContext(1, "session", "window-a", true), account, nil)
	b := resolveCodexMultiWindowIDs(multiWindowTestContext(1, "session", "window-b", true), account, nil)
	require.Equal(t, a.sessionID, b.sessionID)
	require.NotEqual(t, a.windowID, b.windowID)
}

func TestCodexMultiWindowHeaderBodyAndWebSocketParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := multiWindowTestAccount()
	c := multiWindowTestContext(7, "session-a", "window-a", true)
	c.Request.Header.Set("thread-id", "thread-a")
	c.Request.Header.Set("session_id", "old-session")
	c.Request.Header.Set("conversation_id", "old-conversation")
	c.Request.Header.Set("x-client-request-id", "request-a")
	c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"session_id":"session-a","installation_id":"raw-device","x-codex-installation-id":"raw-alias","turn_id":"turn-a","sandbox":"workspace-write"}`)
	body := map[string]any{
		"model": "gpt-5.4", "prompt_cache_key": "session-a",
		"client_metadata": map[string]any{"session_id": "session-a", "x-codex-turn-metadata": c.GetHeader(openAIWSTurnMetadataHeader)},
	}
	require.True(t, applyCodexMultiWindowRequest(c, account, body))
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{}
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, raw, "test-token", true, "session-a", true)
	require.NoError(t, err)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "test-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", c.GetHeader(openAIWSTurnMetadataHeader), "session-a", "", "")
	require.NoError(t, err)
	for _, header := range []string{"x-codex-installation-id", "x-codex-window-id", "session-id", "thread-id", "x-client-request-id", openAIWSTurnMetadataHeader} {
		require.NotEmpty(t, req.Header.Get(header), header)
		require.Equal(t, req.Header.Get(header), ws.Get(header), header)
	}
	for _, headers := range []http.Header{req.Header, ws} {
		require.Empty(t, headers.Get("session_id"))
		require.Empty(t, headers.Get("conversation_id"))
		require.Equal(t, body["prompt_cache_key"], headers.Get("session-id"))
		require.Equal(t, gjson.GetBytes(raw, "client_metadata.thread_id").String(), headers.Get("thread-id"))
		metadata := headers.Get(openAIWSTurnMetadataHeader)
		require.Equal(t, headers.Get("x-codex-installation-id"), gjson.Get(metadata, "installation_id").String())
		require.Equal(t, headers.Get("x-codex-installation-id"), gjson.Get(metadata, "x-codex-installation-id").String())
		require.Equal(t, "workspace-write", gjson.Get(metadata, "sandbox").String())
	}
	ids := stagedCodexFingerprintIDs(c, account)
	before, err := json.Marshal(body)
	require.NoError(t, err)
	applyCodexFingerprintClientMetadata(body, ids)
	after, err := json.Marshal(body)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "WS retries must not hash an already-mapped identity twice")
}

func TestCodexMultiWindowBodyOnlySessionAndMissingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := multiWindowTestAccount()
	a := resolveCodexMultiWindowIDs(multiWindowTestContext(1, "", "", true), account, map[string]any{"client_metadata": map[string]any{"session_id": "body-a"}})
	b := resolveCodexMultiWindowIDs(multiWindowTestContext(1, "", "", true), account, map[string]any{"client_metadata": map[string]any{"session_id": "body-b"}})
	require.NotEqual(t, a.sessionID, b.sessionID)
	require.NotEqual(t, a.threadID, b.threadID)
	c := multiWindowTestContext(1, "", "", true)
	missing := resolveCodexMultiWindowIDs(c, account, nil)
	require.Equal(t, missing.sessionID, resolveCodexMultiWindowIDs(c, account, nil).sessionID)
	require.NotEqual(t, missing.sessionID, resolveCodexMultiWindowIDs(multiWindowTestContext(1, "", "", true), account, nil).sessionID)
}

func TestCodexMultiWindowNonCodexOnlyPseudonymizesCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := multiWindowTestContext(9, "client-session", "", false)
	account := multiWindowTestAccount()
	original := []byte(`{"model":"gpt-5.4", "prompt_cache_key":"cache-a", "client_metadata":{"session_id":"keep-session","custom":true}, "input": [ {"role":"user", "content":"keep formatting"} ]}`)
	body, changed, err := applyCodexMultiWindowRequestRaw(c, account, original)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, "cache-a", gjson.GetBytes(body, "prompt_cache_key").String())
	require.Equal(t, gjson.GetBytes(original, "client_metadata").Raw, gjson.GetBytes(body, "client_metadata").Raw)
	require.Equal(t, gjson.GetBytes(original, "input").Raw, gjson.GetBytes(body, "input").Raw)
	headers := http.Header{"Session_id": []string{"existing-session"}}
	before := headers.Clone()
	applyStagedCodexFingerprintHeaders(c, account, headers)
	require.Equal(t, before, headers)
	noCache := []byte(`{"model":"gpt-5.4","input":[]}`)
	out, changed, err := applyCodexMultiWindowRequestRaw(c, account, noCache)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, noCache, out)
}

func TestCodexMultiWindowWebSocketRetainsOmittedFrameIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := multiWindowTestAccount()
	c := multiWindowTestContext(7, "", "", true)
	c.Request.Header.Set("Connection", "Upgrade")
	c.Request.Header.Set("Upgrade", "websocket")
	first := []byte(`{"type":"response.create","client_metadata":{"session_id":"body-session","thread_id":"body-thread","window_id":"body-window","turn_id":"turn-one"},"input":[]}`)
	_, changed, err := applyCodexMultiWindowRequestRaw(c, account, first)
	require.NoError(t, err)
	require.True(t, changed)
	initialIDs := stagedCodexFingerprintIDs(c, account)
	handshake := http.Header{}
	applyStagedCodexFingerprintHeaders(c, account, handshake)

	next := []byte(`{"type":"response.create","previous_response_id":"resp_one","input":[]}`)
	out, changed, err := applyCodexMultiWindowRequestRaw(c, account, next)
	require.NoError(t, err)
	require.True(t, changed)
	for field, header := range map[string]string{
		"installation_id": "x-codex-installation-id", "session_id": "session-id",
		"thread_id": "thread-id", "window_id": "x-codex-window-id",
	} {
		require.Equal(t, handshake.Get(header), gjson.GetBytes(out, "client_metadata."+field).String(), field)
	}
	require.False(t, gjson.GetBytes(out, "client_metadata.turn_id").Exists(), "turn identity must not carry over")
	require.False(t, gjson.GetBytes(out, "prompt_cache_key").Exists(), "absent cache keys must not be injected")

	third := []byte(`{"type":"response.create","prompt_cache_key":"body-session","client_metadata":{"turn_id":"turn-two"},"input":[]}`)
	out, _, err = applyCodexMultiWindowRequestRaw(c, account, third)
	require.NoError(t, err)
	require.Equal(t, initialIDs.sessionID, gjson.GetBytes(out, "prompt_cache_key").String())
	require.NotEqual(t, initialIDs.turnID, gjson.GetBytes(out, "client_metadata.turn_id").String())

	otherAccount := newTestOAuthAccount(5002, map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintMultiWindow)})
	_, _, err = applyCodexMultiWindowRequestRaw(c, otherAccount, next)
	require.NoError(t, err)
	remapped := stagedCodexFingerprintIDs(c, otherAccount)
	expected := resolveCodexMultiWindowIDs(multiWindowTestContext(7, "body-session", "body-window", true), otherAccount,
		map[string]any{"client_metadata": map[string]any{"thread_id": "body-thread"}})
	require.Equal(t, expected.sessionID, remapped.sessionID)
	require.Equal(t, expected.threadID, remapped.threadID)
	require.Equal(t, expected.windowID, remapped.windowID)
}

func TestCodexMultiWindowRawMapParityAndCompactShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprint(compact), func(t *testing.T) {
			account := multiWindowTestAccount()
			c := multiWindowTestContext(3, "session", "window", true)
			if compact {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
				c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
				c.Request.Header.Set("session-id", "session")
			}
			original := []byte(`{"prompt_cache_key":"session", "input": [ {"role":"user", "content":"hello"} ]}`)
			var body map[string]any
			require.NoError(t, json.Unmarshal(original, &body))
			require.True(t, applyCodexMultiWindowRequest(c, account, body))
			out, changed, err := applyCodexMultiWindowRequestRaw(c, account, original)
			require.NoError(t, err)
			require.True(t, changed)
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(out))
			require.Equal(t, !compact, gjson.GetBytes(out, "client_metadata").Exists())
			require.True(t, bytes.Contains(out, []byte(`[ {"role":"user", "content":"hello"} ]`)))
		})
	}
}

func TestCodexMultiWindowSeedLifecycleAndOptIn(t *testing.T) {
	require.Equal(t, codexFingerprintOff, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).GetCodexFingerprintMode())
	extra := prepareCodexFingerprintExtraForCreate(PlatformOpenAI, AccountTypeOAuth, map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintMultiWindow)})
	seed, valid := codexFingerprintSeed(extra)
	require.True(t, valid)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
	require.True(t, isCodexMultiWindowAccount(account))
	updated := prepareCodexFingerprintExtraForUpdate(account, map[string]any{codexFingerprintModeExtraKey: "off"})
	require.Equal(t, seed, updated[codexFingerprintSeedExtraKey])
	require.True(t, ShouldEnsureCodexFingerprintSeedForExtraUpdates(map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintMultiWindow)}))
	account.Extra = map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintMultiWindow)}
	require.False(t, isCodexMultiWindowAccount(account), "missing seed must preserve existing identity isolation")
}

func TestCodexMultiWindowPoolIgnoresPerRequestIDButSeparatesWindows(t *testing.T) {
	account := multiWindowTestAccount()
	first := http.Header{}
	first.Set("x-codex-installation-id", "device")
	first.Set("session-id", "session")
	first.Set("thread-id", "thread")
	first.Set("x-codex-window-id", "window-a")
	first.Set("x-client-request-id", "request-a")
	second := first.Clone()
	second.Set("x-client-request-id", "request-b")
	require.Equal(t, normalizeOpenAIWSHandshakeCompatibility(account, first), normalizeOpenAIWSHandshakeCompatibility(account, second))
	second.Set("x-codex-window-id", "window-b")
	require.NotEqual(t, normalizeOpenAIWSHandshakeCompatibility(account, first), normalizeOpenAIWSHandshakeCompatibility(account, second))
}

func TestCodexMultiWindowHTTPForwardingPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, codex := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/codex=%v", passthrough, codex), func(t *testing.T) {
				account := multiWindowTestAccount()
				account.Extra["openai_oauth_passthrough"] = passthrough
				account.Status, account.Schedulable, account.Concurrency = StatusActive, true, 4
				account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account", "openai_device_id": "configured-device"}
				c := multiWindowTestContext(7, "session", "window", codex)
				body := []byte(`{"model":"gpt-5.4","instructions":"test","stream":true,"prompt_cache_key":"session","input":[{"role":"user","content":"hi"}]}`)
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
				}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
				_, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, upstream.lastReq)
				cache := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
				require.NotEmpty(t, cache)
				require.NotEqual(t, "session", cache)
				if codex {
					require.Equal(t, cache, upstream.lastReq.Header.Get("session-id"))
					require.Equal(t, cache, gjson.GetBytes(upstream.lastBody, "client_metadata.session_id").String())
					require.Empty(t, upstream.lastReq.Header.Get("session_id"))
					require.Empty(t, upstream.lastReq.Header.Get("conversation_id"))
					require.NotEmpty(t, upstream.lastReq.Header.Get("x-codex-window-id"))
					require.Equal(t, upstream.lastReq.Header.Get("x-codex-window-id"), gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-window-id").String())
				} else {
					require.False(t, gjson.GetBytes(upstream.lastBody, "client_metadata").Exists())
				}
			})
		}
	}
}
