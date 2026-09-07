package service

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintAccountEligibility(t *testing.T) {
	for _, tc := range []struct {
		platform string
		kind     string
		want     bool
	}{
		{PlatformOpenAI, AccountTypeOAuth, true},
		{PlatformOpenAI, AccountTypeSetupToken, true},
		{PlatformOpenAI, AccountTypeAPIKey, false},
		{PlatformAnthropic, AccountTypeOAuth, true},
		{PlatformAnthropic, AccountTypeSetupToken, true},
		{PlatformAnthropic, AccountTypeAPIKey, false},
		{PlatformGemini, AccountTypeOAuth, false},
	} {
		t.Run(tc.platform+"/"+tc.kind, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.kind}
			require.False(t, account.IsTLSFingerprintEnabled())
			account.Extra = map[string]any{"enable_tls_fingerprint": true}
			require.Equal(t, tc.want, account.IsTLSFingerprintEnabled())
		})
	}
	require.False(t, (*Account)(nil).IsTLSFingerprintEnabled())
}

type openAITLSRecordingUpstream struct {
	profile    *tlsfingerprint.Profile
	plainCalls int
}

func (u *openAITLSRecordingUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.plainCalls++
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[]}`))}, nil
}

func (u *openAITLSRecordingUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.profile = profile
	return u.Do(req, proxy, id, concurrency)
}

func TestOpenAITLSFingerprintOverridesPluginForForwardAndAccountTest(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "plugin unavailable"})
	account := &Account{ID: 71, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		"enable_tls_fingerprint": true, "tls_fingerprint_profile_id": 9,
	}}
	profiles := &TLSFingerprintProfileService{localCache: map[int64]*model.TLSFingerprintProfile{
		9: {ID: 9, Name: "custom", Curves: []uint16{29, 23}},
	}}
	require.False(t, manager.ShouldRouteOpenAIOAuth(account))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com/responses", nil)
	require.NoError(t, err)
	upstream := &openAITLSRecordingUpstream{}
	gateway := &OpenAIGatewayService{httpUpstream: upstream, pluginManager: manager, tlsFPProfileService: profiles}
	resp, err := gateway.doOpenAIUpstream(req, "", account)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, []uint16{29, 23}, upstream.profile.Curves)
	testService := &AccountTestService{httpUpstream: upstream, pluginManager: manager, tlsFPProfileService: profiles}
	upstream.profile = nil
	resp, err = testService.doOpenAIAccountTestUpstream(req, "", account, false)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "custom", upstream.profile.Name)
	account.Extra["enable_tls_fingerprint"] = false
	require.True(t, manager.ShouldRouteOpenAIOAuth(account))
	_, err = gateway.doOpenAIUpstream(req, "", account)
	require.ErrorContains(t, err, "plugin unavailable")
}

type openAITLSContextDialer struct {
	profiles []string
}

func (d *openAITLSContextDialer) Dial(ctx context.Context, _ string, _ http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	profile, _ := ctx.Value(openAIWSTLSProfileContextKey{}).(*tlsfingerprint.Profile)
	d.profiles = append(d.profiles, profile.CacheKey())
	return &openAIWSFakeConn{}, 0, nil, nil
}

func TestOpenAITLSFingerprintPoolRotatesWhenTemplateChangesOrDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &openAITLSContextDialer{}
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	defer pool.ClearAccount(account.ID)
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/responses"}
	first := &tlsfingerprint.Profile{Curves: []uint16{29}}
	changed := &tlsfingerprint.Profile{Curves: []uint16{29, 23}}
	lastID := ""
	for _, profile := range []*tlsfingerprint.Profile{first, changed, nil} {
		req.TLSProfile = profile
		lease, err := pool.Acquire(t.Context(), req)
		require.NoError(t, err)
		require.False(t, lease.Reused())
		require.NotEqual(t, lastID, lease.ConnID())
		lastID = lease.ConnID()
		lease.Release()
		again, err := pool.Acquire(t.Context(), req)
		require.NoError(t, err)
		require.True(t, again.Reused())
		again.Release()
	}
	require.Equal(t, []string{first.CacheKey(), changed.CacheKey(), ""}, dialer.profiles)
}

func TestOpenAITLSFingerprintWSClientUsesSeparateProfileTransports(t *testing.T) {
	dialer := &coderOpenAIWSClientDialer{}
	first := &tlsfingerprint.Profile{Curves: []uint16{29}}
	changed := &tlsfingerprint.Profile{Curves: []uint16{29, 23}}
	for _, proxy := range []string{"", "http://proxy.example:8080", "https://proxy.example:8443", "socks5://proxy.example:1080"} {
		a, err := dialer.upstreamHTTPClient(proxy, first)
		require.NoError(t, err)
		transport := a.Transport.(*http.Transport)
		require.NotNil(t, transport.DialTLSContext)
		require.False(t, transport.ForceAttemptHTTP2)
		b, err := dialer.upstreamHTTPClient(proxy, changed)
		require.NoError(t, err)
		require.NotSame(t, a, b)
		again, err := dialer.upstreamHTTPClient(proxy, changed)
		require.NoError(t, err)
		require.Same(t, b, again)
	}
}

func TestOpenAITLSFingerprintWebSocketEmitsSelectedClientHello(t *testing.T) {
	hellos := make(chan []uint16, 1)
	server := httptest.NewUnstartedServer(nil)
	server.TLS = &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		hellos <- append([]uint16(nil), info.CipherSuites...)
		return nil, errors.New("capture complete")
	}}
	server.StartTLS()
	defer server.Close()
	profile := &tlsfingerprint.Profile{CipherSuites: []uint16{0x1301, 0x1302}}
	dialer := newDefaultOpenAIWSClientDialer()
	_, _, _, err := dialer.Dial(withOpenAIWSTLSProfile(t.Context(), profile), "wss"+strings.TrimPrefix(server.URL, "https"), nil, "")
	require.Error(t, err)
	select {
	case suites := <-hellos:
		require.Equal(t, profile.CipherSuites, suites)
	default:
		t.Fatal("WebSocket transport did not emit a ClientHello")
	}
}

func TestOpenAITLSFingerprintModelsManifestUsesSelectedProfile(t *testing.T) {
	upstream := &openAITLSRecordingUpstream{}
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		"enable_tls_fingerprint": true,
	}}
	gateway := &OpenAIGatewayService{httpUpstream: upstream}
	manifest, err := gateway.fetchCodexModelsManifestUpstream(t.Context(), openAIModelsRequest{
		url: "https://example.com/models", headers: make(http.Header), credentialAccount: account, accountID: account.ID,
	}, "")
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.NotNil(t, upstream.profile)
}
