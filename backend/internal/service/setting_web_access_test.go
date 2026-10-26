//go:build unit

package service

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type webAccessSettingsRepoStub struct {
	SettingRepository
	values map[string]string
	reads  int
	err    error
}

func (r *webAccessSettingsRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.reads++
	if r.err != nil {
		return nil, r.err
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *webAccessSettingsRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return r.err
}

func (r *webAccessSettingsRepoStub) GetAll(context.Context) (map[string]string, error) {
	return r.values, r.err
}

func TestWebAccessDecisionCountryScope(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	for _, test := range []struct {
		name    string
		address string
		blocked bool
	}{
		{"mainland IPv4", "223.5.5.5", true},
		{"mainland IPv6", "2400:3200::1", true},
		{"mapped mainland IPv4", "::ffff:223.5.5.5", true},
		{"Hong Kong", "1.36.0.1", false},
		{"Macau", "202.175.0.1", false},
		{"Taiwan", "168.95.1.1", false},
		{"other IPv4", "8.8.8.8", false},
		{"other IPv6", "2001:4860:4860::8888", false},
		{"private IPv4", "192.168.1.1", false},
		{"private IPv6", "fd00::1", false},
		{"loopback", "127.0.0.1", false},
		{"mapped loopback", "::ffff:127.0.0.1", false},
		{"IPv6 loopback", "::1", false},
		{"link local", "fe80::1", false},
		{"scoped IPv6", "2400:3200::1%eth0", false},
		{"unknown", "unknown", false},
		{"raw forwarded header", "223.5.5.5, 8.8.8.8", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &webAccessSettingsRepoStub{values: map[string]string{SettingKeyWebAccessCNBlockEnabled: "true"}}
			svc := NewSettingService(repo, nil)
			decision, err := svc.GetWebAccessDecision(context.Background(), test.address)
			require.NoError(t, err)
			require.True(t, decision.Enabled)
			require.Equal(t, test.blocked, decision.Blocked)
			require.Equal(t, DefaultWebAccessBlockMessage, decision.Message)
			if test.blocked {
				require.Equal(t, "CN", decision.CountryCode)
			}
		})
	}
}

func TestWebAccessDecisionDefaultsAndEmergencyDisable(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	repo := &webAccessSettingsRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	decision, err := svc.GetWebAccessDecision(context.Background(), "223.5.5.5")
	require.NoError(t, err)
	require.False(t, decision.Enabled)
	require.False(t, decision.Blocked)

	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "true")
	repo.values[SettingKeyWebAccessCNBlockEnabled] = "true"
	repo.err = errors.New("database unavailable")
	svc.invalidateWebAccessSettings()
	decision, err = svc.GetWebAccessDecision(context.Background(), "223.5.5.5")
	require.NoError(t, err, "emergency recovery must not depend on the database")
	require.False(t, decision.Enabled)
	require.False(t, decision.Blocked)
	require.Equal(t, 1, repo.reads)
}

func TestWebAccessDecisionIPAllowlist(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	repo := &webAccessSettingsRepoStub{values: map[string]string{
		SettingKeyWebAccessCNBlockEnabled: "true",
		SettingKeyWebAccessBlockMessage:   "  自定义提示  ",
		SettingKeyWebAccessIPAllowlist:    `["223.5.5.5", "114.114.114.123/24", "2400:3200::/32"]`,
	}}
	svc := NewSettingService(repo, nil)
	for _, address := range []string{"223.5.5.5", "::ffff:223.5.5.5", "114.114.114.114", "2400:3200::1"} {
		decision, err := svc.GetWebAccessDecision(context.Background(), address)
		require.NoError(t, err)
		require.True(t, decision.Enabled)
		require.False(t, decision.Blocked, address)
		require.Equal(t, "自定义提示", decision.Message)
	}
	decision, err := svc.GetWebAccessDecision(context.Background(), "223.6.6.6")
	require.NoError(t, err)
	require.True(t, decision.Blocked, "allowlist must not exempt other mainland addresses")
	require.Equal(t, 1, repo.reads, "visitor decisions should share only the configuration cache")
}

func TestWebAccessSettingsRefreshAfterSaveAndExpiration(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	repo := &webAccessSettingsRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	cacheInvalidations := 0
	svc.SetOnUpdateCallback(func() { cacheInvalidations++ })
	before, err := svc.GetWebAccessDecision(context.Background(), "223.5.5.5")
	require.NoError(t, err)
	require.False(t, before.Blocked)
	require.NoError(t, svc.UpdateSettings(context.Background(), &SystemSettings{WebAccessCNBlockEnabled: true}))
	require.Equal(t, 1, cacheInvalidations, "settings saves must retain the public HTML cache invalidation callback")
	after, err := svc.GetWebAccessDecision(context.Background(), "223.5.5.5")
	require.NoError(t, err)
	require.True(t, after.Blocked, "successful save must invalidate even a fresh cache")

	repo.values[SettingKeyWebAccessCNBlockEnabled] = "false"
	cached := *svc.webAccessSettingsCache.Load()
	cached.expiresAt = time.Now().Add(-time.Second)
	svc.webAccessSettingsCache.Store(&cached)
	after, err = svc.GetWebAccessDecision(context.Background(), "223.5.5.5")
	require.NoError(t, err)
	require.False(t, after.Blocked, "expired cache must notice writes by another instance")
}

func TestWebAccessDecisionLookupErrorsRemainUnblocked(t *testing.T) {
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	for _, repo := range []*webAccessSettingsRepoStub{
		{err: errors.New("database unavailable")},
		{values: map[string]string{SettingKeyWebAccessCNBlockEnabled: "true", SettingKeyWebAccessIPAllowlist: `{"invalid": true}`}},
	} {
		decision, err := NewSettingService(repo, nil).GetWebAccessDecision(context.Background(), "223.5.5.5")
		require.Error(t, err)
		require.False(t, decision.Blocked)
	}
}

func TestNormalizeWebAccessIPAllowlist(t *testing.T) {
	got, err := NormalizeWebAccessIPAllowlist([]string{
		" 223.5.5.5 ", "223.5.5.5/32", "::ffff:223.5.5.5", "114.114.114.123/24",
		"2400:3200::1/32", "::ffff:223.5.5.0/120",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"223.5.5.5", "114.114.114.0/24", "2400:3200::/32", "223.5.5.0/24"}, got)
	for _, invalid := range []string{"", "example.com", "223.5.5.5:80", "1.2.3.4/33", "fe80::1%eth0", "::ffff:223.5.5.5/80"} {
		_, err := NormalizeWebAccessIPAllowlist([]string{invalid})
		require.Error(t, err, invalid)
	}
	_, err = NormalizeWebAccessIPAllowlist(make([]string, webAccessAllowlistMaxEntries+1))
	require.Error(t, err)
}

func TestWebAccessMessageValidation(t *testing.T) {
	settings := &SystemSettings{WebAccessBlockMessage: strings.Repeat("字", 500)}
	require.NoError(t, normalizeWebAccessSettings(settings))
	settings.WebAccessBlockMessage += "字"
	require.Error(t, normalizeWebAccessSettings(settings))
	settings.WebAccessBlockMessage = "hello\x00world"
	require.Error(t, normalizeWebAccessSettings(settings))
	settings.WebAccessBlockMessage = "  "
	require.NoError(t, normalizeWebAccessSettings(settings))
	require.Equal(t, DefaultWebAccessBlockMessage, settings.WebAccessBlockMessage)
	require.NotNil(t, settings.WebAccessIPAllowlist)
	settings.WebAccessBlockMessage = "本网站暂不向中国大陆地区提供网页访问服务。"
	require.NoError(t, normalizeWebAccessSettings(settings))
	require.Equal(t, "本網站暫不向中國大陸地區提供網頁訪問服務。", settings.WebAccessBlockMessage)
}

func TestWebAccessCountryDataValidation(t *testing.T) {
	index, err := embeddedWebAccessCNNetworks()
	require.NoError(t, err, "all shipped country networks must parse")
	require.True(t, index.contains(netip.MustParseAddr("223.5.5.5")))
	require.True(t, index.contains(netip.MustParseAddr("2400:3200::1")))
	for _, test := range []struct{ ipv4, ipv6 string }{
		{"", "2400:3200::/32"},
		{"223.5.5.0/24", ""},
		{"223.5.5.0/24\ninvalid", "2400:3200::/32"},
		{"223.5.5.0/24", "223.5.5.0/24"},
	} {
		_, err := parseWebAccessCNNetworks(test.ipv4, test.ipv6)
		require.Error(t, err, "a partial or wrong-family dataset must not be silently accepted")
	}
}
