//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webAccessUnavailableRepo struct {
	*settingHandlerPublicRepoStub
}

func (s *webAccessUnavailableRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, service.ErrWebAccessLocalNetworkUnavailable
}

func TestGetWebAccessReportsExplicitLocalNetworkFailure(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "false")
	repo := &webAccessUnavailableRepo{&settingHandlerPublicRepoStub{values: map[string]string{}}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test")
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/api/v1/settings/web-access", handler.GetWebAccess)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/settings/web-access", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	var payload struct {
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "WEB_ACCESS_LOCAL_NETWORK_UNAVAILABLE", payload.Reason)
	require.Equal(t, "local_network_unavailable", payload.Metadata["location_source"])
}

func TestGetWebAccessUsesVisitorIPWithoutSharingDecision(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "false")
	repo := &settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyWebAccessCNBlockEnabled: "true",
		service.SettingKeyWebAccessIPAllowlist:    `["223.5.5.6"]`,
	}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test")
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/api/v1/settings/web-access", handler.GetWebAccess)
	for _, test := range []struct {
		ip      string
		blocked bool
	}{
		{"223.5.5.5:1234", true},
		{"8.8.8.8:1234", false},
		{"223.5.5.6:1234", false},
		{"[240e::1]:1234", true},
		{"127.0.0.1:1234", false},
	} {
		t.Run(test.ip, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/settings/web-access", nil)
			request.RemoteAddr = test.ip
			request.Header.Set("X-Forwarded-For", "8.8.8.8")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, "decision endpoint stays available for blocked visitors")
			require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			var payload struct {
				Code int                       `json:"code"`
				Data service.WebAccessDecision `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
			require.Zero(t, payload.Code)
			require.True(t, payload.Data.Enabled)
			require.Equal(t, test.blocked, payload.Data.Blocked)
			require.NotContains(t, recorder.Body.String(), "allowlist")
			require.NotContains(t, recorder.Body.String(), "223.5.5.6")
		})
	}
}
