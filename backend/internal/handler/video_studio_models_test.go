package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelsVideoStudioBypassesCodexManifest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 41, Platform: service.PlatformOpenAI}
	group.CodexModelsManifestConfig.Enabled = true
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{group.ID: {
		{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
			"base_url": "https://muse.example", "model_mapping": map[string]any{"muse-video": "muse-video"},
		}},
	}}})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?video_studio=1", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: group})
	h.Models(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"id":"muse-video"`)
	require.NotContains(t, rec.Body.String(), "gpt-")
}

func TestGatewayModelsVideoStudioRecognizesExistingGeminiKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 42, Platform: service.PlatformGemini}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{group.ID: {
		{ID: 2, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
			"base_url": "https://omni.example/v1beta", "model_mapping": map[string]any{"gemini-omni-1.2-flash": "gemini-omni-1.2-flash"},
		}},
	}}})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?video_studio=1", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: group})
	h.Models(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"id":"gemini-omni-1.2-flash"`)
}
