//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingsRechargePromotionFeatureSwitch(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	assertResponse := func(rec *httptest.ResponseRecorder, want bool) {
		t.Helper()
		require.Equal(t, http.StatusOK, rec.Code)
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		require.Equal(t, want, payload.Data["recharge_promotion_enabled"])
	}
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
		h.GetSettings(c)
		return rec
	}

	assertResponse(get(), false)
	assertResponse(doUpdateSettings(t, h, map[string]any{"recharge_promotion_enabled": true}, nil), true)
	require.Equal(t, "true", repo.values[service.SettingKeyRechargePromotionEnabled])
	assertResponse(get(), true)
	assertResponse(doUpdateSettings(t, h, map[string]any{"site_name": "Promotion Site"}, nil), true)
	require.Equal(t, "true", repo.values[service.SettingKeyRechargePromotionEnabled])
	assertResponse(doUpdateSettings(t, h, map[string]any{"recharge_promotion_enabled": false}, nil), false)
	require.Equal(t, "false", repo.values[service.SettingKeyRechargePromotionEnabled])
}
