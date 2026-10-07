package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelsImageStudioQueryIncludesNewAllowlistedImageModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 41, Platform: service.PlatformGemini, ModelAllowlist: service.GroupModelAllowlist{
		Enabled: true, Models: []string{"gemini-nano-banana-2.1", "gemini-2.5-pro"},
	}}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{group.ID: {
		{ID: 1, Platform: service.PlatformGemini},
	}}})
	for _, tt := range []struct {
		query string
		ids   []string
	}{
		{"?image_studio=1", []string{"gemini-nano-banana-2.1"}},
		{"", []string{"gemini-2.5-pro"}},
	} {
		t.Run(tt.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/models"+tt.query, nil)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: group})
			h.Models(c)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var response struct {
				Object string `json:"object"`
				Data   []struct {
					ID              string `json:"id"`
					ImageGeneration bool   `json:"image_generation"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			require.Equal(t, "list", response.Object)
			ids := make([]string, 0, len(response.Data))
			for _, model := range response.Data {
				ids = append(ids, model.ID)
				require.Equal(t, tt.query != "", model.ImageGeneration)
			}
			require.Equal(t, tt.ids, ids)
		})
	}
}
