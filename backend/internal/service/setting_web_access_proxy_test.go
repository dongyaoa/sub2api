//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func webAccessProxyTestRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	return req
}

func TestWebAccessProxyEnvironmentPrecedence(t *testing.T) {
	environmentValues := map[string]string{
		"HTTPS_PROXY": "http://upper.example:8080",
		"https_proxy": " http://lower.example:8081 ",
		"NO_PROXY":    "api.ipify.org",
	}
	environment := readWebAccessProxyEnvironment(func(key string) (string, bool) {
		value, exists := environmentValues[key]
		return value, exists
	})
	require.True(t, environment.HasProxy)
	var systemReads int
	policy, err := buildWebAccessLocalProxyPolicy(environment, func() (webAccessSystemProxySettings, error) {
		systemReads++
		return webAccessSystemProxySettings{}, errors.New("system settings unavailable")
	})
	require.NoError(t, err)
	proxy, err := policy.proxy(webAccessProxyTestRequest(t, "https://www.cloudflare.com/cdn-cgi/trace"))
	require.NoError(t, err)
	require.Equal(t, "http://lower.example:8081", proxy.String())
	proxy, err = policy.proxy(webAccessProxyTestRequest(t, "https://api.ipify.org?format=json"))
	require.NoError(t, err)
	require.Nil(t, proxy)
	require.Zero(t, systemReads, "explicit process policy must take precedence over Windows and PAC")
}

func TestWebAccessProxyExplicitEmptyEnvironmentOverridesSystem(t *testing.T) {
	environment := readWebAccessProxyEnvironment(func(key string) (string, bool) {
		return "", key == "HTTPS_PROXY"
	})
	require.True(t, environment.HasProxy)
	policy, err := buildWebAccessLocalProxyPolicy(environment, func() (webAccessSystemProxySettings, error) {
		return webAccessSystemProxySettings{}, errors.New("must not read system settings")
	})
	require.NoError(t, err)
	proxy, err := policy.proxy(webAccessProxyTestRequest(t, "https://www.cloudflare.com/cdn-cgi/trace"))
	require.NoError(t, err)
	require.Nil(t, proxy)
}

func TestWebAccessProxyNoProxyOverlaysWindowsPolicy(t *testing.T) {
	policy, err := buildWebAccessLocalProxyPolicy(webAccessProxyEnvironment{HasNoProxy: true, NoProxy: "api.ipify.org"}, func() (webAccessSystemProxySettings, error) {
		return webAccessSystemProxySettings{Enabled: true, Server: "127.0.0.1:8080"}, nil
	})
	require.NoError(t, err)
	proxy, err := policy.proxy(webAccessProxyTestRequest(t, "https://api.ipify.org?format=json"))
	require.NoError(t, err)
	require.Nil(t, proxy)
	proxy, err = policy.proxy(webAccessProxyTestRequest(t, "https://www.cloudflare.com/cdn-cgi/trace"))
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080", proxy.String())
}

func TestWebAccessProxyWindowsManualFormats(t *testing.T) {
	for _, test := range []struct {
		name, server, target, expected string
	}{
		{"shared HTTP proxy", "127.0.0.1:8080", "https://www.cloudflare.com/", "http://127.0.0.1:8080"},
		{"IPv6 proxy", "[::1]:8080", "https://www.cloudflare.com/", "http://[::1]:8080"},
		{"explicit HTTPS proxy", "https://proxy.example:8443", "https://www.cloudflare.com/", "https://proxy.example:8443"},
		{"protocol map", "http=proxy.example:8080;https=secure.example:8081", "https://www.cloudflare.com/", "http://secure.example:8081"},
		{"URL in protocol map", "https=http://secure.example:8081", "https://www.cloudflare.com/", "http://secure.example:8081"},
		{"HTTP map", "http=proxy.example:8080;https=secure.example:8081", "http://www.cloudflare.com/", "http://proxy.example:8080"},
		{"explicit protocol direct", "http=proxy.example:8080", "https://www.cloudflare.com/", ""},
		{"credential with equals", "http://fixture:pass=word@proxy.example:8080", "https://www.cloudflare.com/", "http://fixture:pass=word@proxy.example:8080"},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxyForRequest, err := buildWebAccessSystemProxy(webAccessSystemProxySettings{Enabled: true, Server: test.server})
			require.NoError(t, err)
			proxy, err := proxyForRequest(webAccessProxyTestRequest(t, test.target))
			require.NoError(t, err)
			if test.expected == "" {
				require.Nil(t, proxy)
			} else {
				require.Equal(t, test.expected, proxy.String())
			}
		})
	}
}

func TestWebAccessProxyWindowsBypassPatterns(t *testing.T) {
	proxyForRequest, err := buildWebAccessSystemProxy(webAccessSystemProxySettings{
		Enabled: true, Server: "127.0.0.1:8080", Override: "<local>;*.example.test;127.*;10.0.0.0/8;[::1]:443;www.cloudflare.com:443",
	})
	require.NoError(t, err)
	for _, target := range []string{"https://intranet/", "https://sub.example.test/", "https://127.0.0.2/", "https://10.2.3.4/", "https://[::1]/", "https://www.cloudflare.com/"} {
		proxy, err := proxyForRequest(webAccessProxyTestRequest(t, target))
		require.NoError(t, err)
		require.Nil(t, proxy, target)
	}
	for _, target := range []string{"https://example.test/", "https://api.ipify.org/", "https://www.cloudflare.com:8443/"} {
		proxy, err := proxyForRequest(webAccessProxyTestRequest(t, target))
		require.NoError(t, err)
		require.NotNil(t, proxy, target)
	}
}

func TestWebAccessProxyInvalidOrAutomaticConfigurationNeverBecomesDirect(t *testing.T) {
	for _, settings := range []webAccessSystemProxySettings{
		{Enabled: true},
		{Enabled: true, Server: "https=invalid:99999"},
		{Enabled: true, Server: "socks=127.0.0.1:1080"},
		{Enabled: true, Server: "https=proxy.example:8080;https=other.example:8080"},
		{Enabled: true, Server: "127.0.0.1:8080", Override: "[broken"},
		{Enabled: true, Server: "127.0.0.1:8080", AutoConfigURL: "https://private.example/fixture.pac"},
		{AutoDetect: true},
	} {
		_, err := buildWebAccessLocalProxyPolicy(webAccessProxyEnvironment{}, func() (webAccessSystemProxySettings, error) {
			return settings, nil
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private.example")
	}
	_, err := buildWebAccessLocalProxyPolicy(webAccessProxyEnvironment{}, func() (webAccessSystemProxySettings, error) {
		return webAccessSystemProxySettings{}, errors.New("access denied to private registry value")
	})
	require.EqualError(t, err, "cannot read system proxy configuration")
	_, err = buildWebAccessLocalProxyPolicy(webAccessProxyEnvironment{HasProxy: true, HTTPSProxy: "http://fixture:private-password@proxy.example:99999"}, func() (webAccessSystemProxySettings, error) {
		t.Fatal("invalid explicit environment must not fall back to Windows")
		return webAccessSystemProxySettings{}, nil
	})
	require.EqualError(t, err, "invalid environment proxy configuration")
	require.NotContains(t, err.Error(), "private-password")
}

func TestWebAccessProxyFingerprintTracksSettingsWithoutPlaintext(t *testing.T) {
	environment := webAccessProxyEnvironment{HasProxy: true, HTTPSProxy: "http://fixture:private-password@proxy.example:8080"}
	first := webAccessProxyFingerprint("environment", environment, webAccessSystemProxySettings{})
	require.Len(t, first, 64)
	require.NotContains(t, first, "private-password")
	require.NotContains(t, first, "proxy.example")
	environment.HTTPSProxy = "http://fixture:changed-password@proxy.example:8080"
	require.NotEqual(t, first, webAccessProxyFingerprint("environment", environment, webAccessSystemProxySettings{}))
	system := webAccessSystemProxySettings{Enabled: true, Server: "127.0.0.1:8080"}
	first = webAccessProxyFingerprint("system", webAccessProxyEnvironment{}, system)
	system.Override = "www.cloudflare.com"
	require.NotEqual(t, first, webAccessProxyFingerprint("system", webAccessProxyEnvironment{}, system))
}

type webAccessProxyTrackingTransport struct {
	roundTrip webAccessLocalNetworkRoundTripper
	closed    atomic.Int32
}

func (transport *webAccessProxyTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport.roundTrip(req)
}

func (transport *webAccessProxyTrackingTransport) CloseIdleConnections() {
	transport.closed.Add(1)
}

func TestWebAccessLocalNetworkProxyChangesInvalidateCacheAndClient(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	selected := "direct"
	var loadErr error
	resolver.loadProxyPolicy = func() (webAccessLocalProxyPolicy, error) {
		return webAccessLocalProxyPolicy{fingerprint: selected}, loadErr
	}
	clients := make(map[string]*webAccessProxyTrackingTransport)
	var calls atomic.Int32
	resolver.newClient = func(policy webAccessLocalProxyPolicy) *http.Client {
		transport := &webAccessProxyTrackingTransport{roundTrip: func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			ip := "223.5.5.5"
			if policy.fingerprint == "proxy" {
				ip = "8.8.8.8"
			}
			return webAccessLocalNetworkResponse(req, http.StatusOK, "ip="+ip+"\n"), nil
		}}
		clients[policy.fingerprint] = transport
		return &http.Client{Transport: transport}
	}
	address, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("223.5.5.5"), address)
	selected = "proxy"
	address, err = resolver.resolve(context.Background())
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("8.8.8.8"), address, "a proxy switch must not reuse the unexpired direct result")
	require.EqualValues(t, 1, clients["direct"].closed.Load())
	require.EqualValues(t, 2, calls.Load())
	loadErr = errors.New("cannot read system proxy configuration")
	_, err = resolver.resolve(context.Background())
	require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
	require.EqualValues(t, 1, clients["proxy"].closed.Load())
	require.EqualValues(t, 2, calls.Load(), "a configuration error must not issue a direct request or reuse an old result")
}

func TestWebAccessLocalNetworkProxyFailureNeverRetriesDirect(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	policy, err := buildWebAccessLocalProxyPolicy(webAccessProxyEnvironment{}, func() (webAccessSystemProxySettings, error) {
		return webAccessSystemProxySettings{Enabled: true, Server: "127.0.0.1:8080"}, nil
	})
	require.NoError(t, err)
	resolver.loadProxyPolicy = func() (webAccessLocalProxyPolicy, error) { return policy, nil }
	var calls atomic.Int32
	var direct atomic.Bool
	resolver.newClient = func(policy webAccessLocalProxyPolicy) *http.Client {
		return &http.Client{Transport: webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			proxy, proxyErr := policy.proxy(req)
			if proxy == nil || proxyErr != nil {
				direct.Store(true)
			}
			return nil, errors.New("fixture proxy connection unavailable")
		})}
	}
	_, err = resolver.resolve(context.Background())
	require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
	require.False(t, direct.Load())
	require.EqualValues(t, 2, calls.Load(), "both fixed providers must use the same selected proxy policy")
}

func TestWebAccessLocalNetworkSameProxyNodeRefreshHasShortCache(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	ip := "223.5.5.5"
	var calls atomic.Int32
	setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return webAccessLocalNetworkResponse(req, http.StatusOK, "ip="+ip+"\n"), nil
	}))
	_, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	cache := *resolver.state.cache.Load()
	require.LessOrEqual(t, time.Until(cache.expiresAt), 5*time.Second)
	ip = "8.8.8.8"
	cache.expiresAt = time.Now().Add(-time.Second)
	resolver.state.cache.Store(&cache)
	address, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("8.8.8.8"), address)
	require.EqualValues(t, 2, calls.Load())
	transport := newWebAccessLocalNetworkHTTPClient(webAccessLocalProxyPolicy{proxy: func(*http.Request) (*url.URL, error) { return nil, nil }}).Transport.(*http.Transport)
	require.True(t, transport.DisableKeepAlives)
	require.False(t, transport.ForceAttemptHTTP2)
	require.True(t, transport.Protocols.HTTP1())
	require.False(t, transport.Protocols.HTTP2())
}

func TestWebAccessLocalNetworkRetiresInflightOldProxy(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	var selected atomic.Value
	selected.Store("old")
	resolver.loadProxyPolicy = func() (webAccessLocalProxyPolicy, error) {
		return webAccessLocalProxyPolicy{fingerprint: selected.Load().(string)}, nil
	}
	started := make(chan struct{})
	resolver.newClient = func(policy webAccessLocalProxyPolicy) *http.Client {
		return &http.Client{Transport: webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
			if policy.fingerprint == "old" {
				close(started)
				<-req.Context().Done()
				return nil, req.Context().Err()
			}
			return webAccessLocalNetworkResponse(req, http.StatusOK, "ip=8.8.8.8\n"), nil
		})}
	}
	oldResult := make(chan error, 1)
	go func() {
		_, err := resolver.resolve(context.Background())
		oldResult <- err
	}()
	<-started
	selected.Store("new")
	address, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("8.8.8.8"), address)
	require.ErrorIs(t, <-oldResult, ErrWebAccessLocalNetworkUnavailable)
}
