package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type intelligenceLocalChannelsHandlerRepo struct {
	service.IntelligenceMonitorRepository
	groupID int64
}

func (r *intelligenceLocalChannelsHandlerRepo) ListIntelligenceLocalChannels(_ context.Context, groupID int64) ([]service.IntelligenceLocalChannel, error) {
	r.groupID = groupID
	return []service.IntelligenceLocalChannel{{AccountID: 11, Name: "Upstream", Platform: "openai", Type: "apikey", Status: "disabled"}}, nil
}

type intelligenceLocalChannelsHandlerGroups struct{ service.GroupRepository }

func (intelligenceLocalChannelsHandlerGroups) GetByID(_ context.Context, groupID int64) (*service.Group, error) {
	return &service.Group{ID: groupID, Platform: service.PlatformOpenAI}, nil
}

func TestIntelligenceLocalChannelsHandlerValidatesGroupAndReturnsSafeMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &intelligenceLocalChannelsHandlerRepo{}
	handler := NewIntelligenceMonitorHandler(service.NewIntelligenceMonitorService(repo, nil, nil, intelligenceLocalChannelsHandlerGroups{}, nil, nil, nil))
	router := gin.New()
	router.GET("/local-channels", handler.ListLocalChannels)
	for _, query := range []string{"", "?group_id=0", "?group_id=-1", "?group_id=8.5", "?group_id=bad", "?group_id=8&group_id=9"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/local-channels"+query, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, query)
		require.Zero(t, repo.groupID)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/local-channels?group_id=8", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, int64(8), repo.groupID)
	require.Contains(t, w.Body.String(), `"account_id":11`)
	require.Contains(t, w.Body.String(), `"status":"disabled"`)
	require.NotContains(t, w.Body.String(), "credentials")
	require.NotContains(t, w.Body.String(), "api_key")
}
