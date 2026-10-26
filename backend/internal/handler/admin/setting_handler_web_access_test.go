package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsWebAccessPreservesOmittedFields(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyWebAccessCNBlockEnabled: "true",
		service.SettingKeyWebAccessBlockMessage:   "当前地区不可访问网页",
		service.SettingKeyWebAccessIPAllowlist:    `["223.5.5.5"]`,
	})
	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Example"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyWebAccessCNBlockEnabled])
	require.Equal(t, "当前地区不可访问网页", repo.values[service.SettingKeyWebAccessBlockMessage])
	require.Equal(t, `["223.5.5.5"]`, repo.values[service.SettingKeyWebAccessIPAllowlist])
}

func TestUpdateSettingsWebAccessNormalizesAndRefreshesDecision(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	before, err := h.settingService.GetWebAccessDecision(context.Background(), "223.6.6.6")
	require.NoError(t, err)
	require.False(t, before.Blocked)
	rec := doUpdateSettings(t, h, map[string]any{
		"web_access_cn_block_enabled": true,
		"web_access_block_message":    "  当前地区不可访问网页  ",
		"web_access_ip_allowlist":     []string{"223.5.5.123/24", "223.5.5.0/24", "2400:3200::1/32"},
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyWebAccessCNBlockEnabled])
	require.Equal(t, "当前地区不可访问网页", repo.values[service.SettingKeyWebAccessBlockMessage])
	var allowlist []string
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyWebAccessIPAllowlist]), &allowlist))
	require.Equal(t, []string{"223.5.5.0/24", "2400:3200::/32"}, allowlist)
	after, err := h.settingService.GetWebAccessDecision(context.Background(), "223.6.6.6")
	require.NoError(t, err)
	require.True(t, after.Blocked)

	rec = doUpdateSettings(t, h, map[string]any{"web_access_cn_block_enabled": false, "web_access_ip_allowlist": []string{}}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "false", repo.values[service.SettingKeyWebAccessCNBlockEnabled])
	require.Equal(t, "[]", repo.values[service.SettingKeyWebAccessIPAllowlist])
	after, err = h.settingService.GetWebAccessDecision(context.Background(), "223.6.6.6")
	require.NoError(t, err)
	require.False(t, after.Blocked)
}

func TestUpdateSettingsWebAccessRejectsInvalidValues(t *testing.T) {
	for _, payload := range []map[string]any{
		{"web_access_ip_allowlist": []string{"example.com"}},
		{"web_access_ip_allowlist": []string{"1.2.3.4/33"}},
		{"web_access_block_message": strings.Repeat("字", 501)},
	} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyWebAccessCNBlockEnabled: "false"})
		payload["web_access_cn_block_enabled"] = true
		rec := doUpdateSettings(t, h, payload, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Equal(t, "false", repo.values[service.SettingKeyWebAccessCNBlockEnabled])
	}
}

func TestDiffSettingsIncludesWebAccessChanges(t *testing.T) {
	changed := diffSettings(&service.SystemSettings{}, &service.SystemSettings{
		WebAccessCNBlockEnabled: true,
		WebAccessBlockMessage:   "提示",
		WebAccessIPAllowlist:    []string{"223.5.5.5"},
	}, nil, nil, UpdateSettingsRequest{})
	require.Contains(t, changed, "web_access_cn_block_enabled")
	require.Contains(t, changed, "web_access_block_message")
	require.Contains(t, changed, "web_access_ip_allowlist")
}
