//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestWebAccessPublicSettingsAndInjectionExposeOnlyEffectiveSwitch(t *testing.T) {
	for _, test := range []struct {
		name, stored, forceDisabled string
		want                        bool
	}{
		{name: "missing defaults to disabled"},
		{name: "disabled", stored: "false"},
		{name: "enabled", stored: "true", want: true},
		{name: "emergency override", stored: "true", forceDisabled: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("WEB_ACCESS_FORCE_DISABLED", test.forceDisabled)
			repo := &settingPublicRepoStub{values: map[string]string{
				SettingKeyWebAccessCNBlockEnabled: test.stored,
				SettingKeyWebAccessIPAllowlist:    `{"invalid":"private allowlist fixture"}`,
				SettingKeyWebAccessBlockMessage:   "private message fixture",
			}}
			svc := NewSettingService(repo, &config.Config{})
			public, err := svc.GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, test.want, public.WebAccessCNBlockEnabled)
			injection, err := svc.GetPublicSettingsForInjection(context.Background())
			require.NoError(t, err)
			encoded, err := json.Marshal(injection)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			require.Equal(t, test.want, payload[SettingKeyWebAccessCNBlockEnabled])
			for _, privateKey := range []string{SettingKeyWebAccessIPAllowlist, SettingKeyWebAccessBlockMessage, "blocked", "country_code", "location_source"} {
				require.NotContains(t, payload, privateKey)
			}
			require.NotContains(t, string(encoded), "private allowlist fixture")
			require.NotContains(t, string(encoded), "private message fixture")
		})
	}
}

func TestWebAccessDisabledSkipsUnusedConfigurationAndDetection(t *testing.T) {
	t.Setenv("WEB_ACCESS_LOCAL_NETWORK_CHECK", "true")
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	for _, address := range []string{"127.0.0.1", "223.5.5.5"} {
		repo := &webAccessSettingsRepoStub{values: map[string]string{
			SettingKeyWebAccessCNBlockEnabled: "false",
			SettingKeyWebAccessIPAllowlist:    `{"not":"an allowlist"}`,
		}}
		svc := NewSettingService(repo, &config.Config{Server: config.ServerConfig{Host: "127.0.0.1"}})
		resolver := newWebAccessLocalNetworkResolver()
		resolver.loadProxyPolicy = func() (webAccessLocalProxyPolicy, error) {
			t.Fatal("a disabled feature must not read proxy settings or start a public IP probe")
			return webAccessLocalProxyPolicy{}, nil
		}
		svc.webAccessLocalNetwork.Store(resolver)
		decision, err := svc.GetWebAccessDecision(context.Background(), address)
		require.NoError(t, err, "disabled features must ignore unused allowlist parse failures")
		require.False(t, decision.Enabled)
		require.False(t, decision.Blocked)
		require.Empty(t, decision.CountryCode)
		require.Equal(t, WebAccessLocationClientIP, decision.LocationSource)
		require.Nil(t, resolver.state)
	}
}
