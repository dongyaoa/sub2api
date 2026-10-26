//go:build embed

package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webAccessStub struct {
	decision service.WebAccessDecision
	err      error
	ips      []string
}

func (s *webAccessStub) GetWebAccessDecision(_ context.Context, ip string) (service.WebAccessDecision, error) {
	s.ips = append(s.ips, ip)
	return s.decision, s.err
}

func webAccessTestServer(t *testing.T, access *webAccessStub) (*gin.Engine, *FrontendServer) {
	t.Helper()
	files := fstest.MapFS{
		"index.html":        {Data: []byte("<html>normal application</html>")},
		"assets/a.js":       {Data: []byte("/* static application asset */")},
		"notice.html":       {Data: []byte("<html>static notice</html>")},
		"nested/index.html": {Data: []byte("<html>nested application</html>")},
	}
	cache := NewHTMLCache()
	cache.Set(files["index.html"].Data, []byte("{}"))
	server := &FrontendServer{distFS: files, fileServer: http.FileServer(http.FS(files)), cache: cache, webAccess: access}
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(server.Middleware())
	router.NoRoute(func(c *gin.Context) {
		c.Header("X-API-Handler", "reached")
		c.Data(http.StatusAccepted, "application/json", []byte(`{"api":"unchanged"}`))
	})
	return router, server
}

func TestWebAccessBlocksOnlyWebPagesBeforeCachedHTML(t *testing.T) {
	for _, path := range []string{"/", "/index.html", "/login", "/register", "/dashboard", "/admin/settings", "/custom/deep/link", "/notice.html", "/nested", "/nested/"} {
		t.Run(path, func(t *testing.T) {
			access := &webAccessStub{decision: service.WebAccessDecision{Enabled: true, Blocked: true, CountryCode: "CN", Message: "本網站暫不向中國大陸地區提供網頁訪問服務。"}}
			router, server := webAccessTestServer(t, access)
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.RemoteAddr = "203.0.113.7:1234"
			request.Header.Set("If-None-Match", server.cache.Get().ETag)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			require.Contains(t, recorder.Body.String(), "本站僅面向海外用戶提供服務")
			require.Contains(t, recorder.Body.String(), "<title>Exclusively for overseas users</title>")
			require.NotContains(t, recorder.Body.String(), "重新檢查")
			require.NotContains(t, recorder.Body.String(), "<button")
			require.NotContains(t, recorder.Body.String(), "normal application")
			require.Equal(t, []string{"203.0.113.7"}, access.ips)
		})
	}
}

func TestWebAccessNeverEvaluatesAPIRoutes(t *testing.T) {
	paths := []string{
		"/api/v1/settings/web-access", "/api/v1/auth/login", "/api/v1/payment/webhook/test",
		"/v1/messages", "/v1/responses", "/v1beta/models", "/backend-api/codex/responses",
		"/antigravity/v1/messages", "/health", "/setup/status", "/models", "/models/example",
		"/responses", "/responses/compact", "/chat/completions", "/embeddings", "/messages/count_tokens",
		"/alpha/search", "/web_search", "/x_search", "/tts", "/stt", "/realtime",
		"/custom-voices", "/custom-voices/example/audio", "/contents/generations/tasks",
		"/v3/contents/generations/tasks/example", "/images/generations", "/images/tasks/example",
		"/videos", "/videos/example/content",
	}
	for _, path := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
			t.Run(method+path, func(t *testing.T) {
				access := &webAccessStub{decision: service.WebAccessDecision{Enabled: true, Blocked: true}}
				router, _ := webAccessTestServer(t, access)
				request := httptest.NewRequest(method, path, nil)
				request.Header.Set("Accept", "text/html, text/event-stream")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusAccepted, recorder.Code)
				require.Equal(t, "reached", recorder.Header().Get("X-API-Handler"))
				require.JSONEq(t, `{"api":"unchanged"}`, recorder.Body.String())
				require.Empty(t, access.ips, "API must not perform a region/config lookup")
			})
		}
	}
}

func TestWebAccessAllowedHTMLIsPrivateAndAssetsRemainAvailable(t *testing.T) {
	access := &webAccessStub{decision: service.WebAccessDecision{Enabled: true}}
	router, _ := webAccessTestServer(t, access)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Body.String(), "normal application")
	access.decision.Blocked = true
	access.ips = nil
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/a.js", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, access.ips)
}

func TestWebAccessUsesTrustedProxyChainAndEscapesMessage(t *testing.T) {
	access := &webAccessStub{decision: service.WebAccessDecision{Enabled: true, Blocked: true, Message: "<strong>plain text</strong>"}}
	router, _ := webAccessTestServer(t, access)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.7:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.9")
	request.Header.Set("CF-Connecting-IP", "198.51.100.10")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, []string{"203.0.113.7"}, access.ips)
	require.Contains(t, recorder.Body.String(), "&lt;strong&gt;plain text&lt;/strong&gt;")
	require.NotContains(t, recorder.Body.String(), "<strong>plain text</strong>")
}

func TestWebAccessConfigFailureDoesNotServeCachedApplication(t *testing.T) {
	access := &webAccessStub{err: errors.New("settings unavailable")}
	router, server := webAccessTestServer(t, access)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("If-None-Match", server.cache.Get().ETag)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Body.String(), "暫時無法檢查訪問狀態")
	require.Contains(t, recorder.Body.String(), "Unable to check access")
	require.NotContains(t, recorder.Body.String(), "normal application")
}
