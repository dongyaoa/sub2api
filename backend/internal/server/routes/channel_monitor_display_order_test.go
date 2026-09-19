package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelMonitorDisplayOrderRouteStore struct {
	value string
}

func (s *channelMonitorDisplayOrderRouteStore) GetValue(_ context.Context, key string) (string, error) {
	if key != service.SettingKeyChannelMonitorDisplayOrder {
		panic("unexpected setting key")
	}
	return s.value, nil
}

func (s *channelMonitorDisplayOrderRouteStore) Set(_ context.Context, key, value string) error {
	if key != service.SettingKeyChannelMonitorDisplayOrder {
		panic("unexpected setting key")
	}
	s.value = value
	return nil
}

type channelMonitorDisplayOrderRouteRepo struct {
	service.ChannelMonitorRepository
	validatedIDs []int64
}

func (r *channelMonitorDisplayOrderRouteRepo) ExistingIDs(_ context.Context, ids []int64) ([]int64, error) {
	r.validatedIDs = append([]int64{}, ids...)
	return []int64{2, 8}, nil
}

// Exercise the real route registration with the feature enabled: guard-only
// tests cannot detect /display-order accidentally falling through to /:id.
func TestChannelMonitorDisplayOrderMatchesStaticRouteAndPersists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &channelMonitorDisplayOrderRouteStore{}
	repo := &channelMonitorDisplayOrderRouteRepo{}
	monitorService := service.NewChannelMonitorService(repo, nil)
	monitorService.SetDisplayOrderStore(store)
	router := gin.New()
	var matchedPath string
	router.Use(func(c *gin.Context) {
		matchedPath = c.FullPath()
		c.Next()
	})
	registerChannelMonitorRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{
		ChannelMonitor:         admin.NewChannelMonitorHandler(monitorService),
		ChannelMonitorTemplate: &admin.ChannelMonitorRequestTemplateHandler{},
	}}, newChannelMonitorRouteSettings(true))

	const path = "/api/v1/admin/channel-monitors/display-order"
	request := func(method, body string) service.ChannelMonitorDisplayOrder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		require.Equal(t, path, matchedPath)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var response struct {
			Data service.ChannelMonitorDisplayOrder `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		return response.Data
	}

	require.Equal(t, service.DefaultChannelMonitorDisplayOrder(), request(http.MethodGet, ""))
	want := service.ChannelMonitorDisplayOrder{
		GroupOrder:   []string{"other", "anthropic", "openai"},
		MonitorOrder: []int64{8, 2},
	}
	require.Equal(t, want, request(http.MethodPut, `{"group_order":["other","anthropic","openai"],"monitor_order":[8,2]}`))
	require.Equal(t, []int64{8, 2}, repo.validatedIDs)
	require.NotEmpty(t, store.value)
	require.Equal(t, want, request(http.MethodGet, ""))
}

func TestChannelMonitorDisplayOrderRequiresAdminAndEnabledFeature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, anonymous := range []bool{true, false} {
		router := gin.New()
		group := router.Group("/api/v1/admin")
		if anonymous {
			group.Use(gin.HandlerFunc(middleware.NewAdminAuthMiddleware(nil, nil, nil, nil)))
		}
		registerChannelMonitorRoutes(group, &handler.Handlers{Admin: &handler.AdminHandlers{
			ChannelMonitor:         admin.NewChannelMonitorHandler(nil),
			ChannelMonitorTemplate: &admin.ChannelMonitorRequestTemplateHandler{},
		}}, newChannelMonitorRouteSettings(false))
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/api/v1/admin/channel-monitors/display-order", strings.NewReader(`{"group_order":["openai","anthropic","other"],"monitor_order":[]}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			if anonymous {
				require.Equal(t, http.StatusUnauthorized, rec.Code)
			} else {
				require.Equal(t, http.StatusForbidden, rec.Code)
				require.Contains(t, rec.Body.String(), "CHANNEL_MONITOR_DISABLED")
			}
		}
	}
}
