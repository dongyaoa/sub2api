//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type webAccessLocalNetworkRoundTripper func(*http.Request) (*http.Response, error)

func (fn webAccessLocalNetworkRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func webAccessLocalNetworkResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func setWebAccessLocalNetworkTestTransport(resolver *webAccessLocalNetworkResolver, transport http.RoundTripper) {
	resolver.loadProxyPolicy = func() (webAccessLocalProxyPolicy, error) {
		return webAccessLocalProxyPolicy{fingerprint: "isolated-test-policy", proxy: http.ProxyFromEnvironment}, nil
	}
	resolver.newClient = func(policy webAccessLocalProxyPolicy) *http.Client {
		client := newWebAccessLocalNetworkHTTPClient(policy)
		client.Transport = transport
		return client
	}
}

func webAccessLocalNetworkTestService(t *testing.T, transport http.RoundTripper) *SettingService {
	t.Helper()
	svc := NewSettingService(&webAccessSettingsRepoStub{values: map[string]string{
		SettingKeyWebAccessCNBlockEnabled: "true",
	}}, &config.Config{Server: config.ServerConfig{Host: "127.0.0.1"}})
	resolver := newWebAccessLocalNetworkResolver()
	setWebAccessLocalNetworkTestTransport(resolver, transport)
	svc.webAccessLocalNetwork.Store(resolver)
	return svc
}

func TestWebAccessLocalNetworkDecision(t *testing.T) {
	t.Setenv("WEB_ACCESS_LOCAL_NETWORK_CHECK", "true")
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	for _, test := range []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"mainland IPv4", "223.5.5.5", true},
		{"mainland IPv6", "2400:3200::1", true},
		{"Hong Kong", "1.36.0.1", false},
		{"Taiwan", "168.95.1.1", false},
		{"other IPv4", "8.8.8.8", false},
		{"other IPv6", "2001:4860:4860::8888", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			svc := webAccessLocalNetworkTestService(t, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if req.Method != http.MethodGet || req.URL.String() != "https://www.cloudflare.com/cdn-cgi/trace" || req.Body != nil {
					return nil, errors.New("unexpected public IP request")
				}
				if len(req.Header) != 2 || req.Header.Get("Accept") != "text/plain" || req.Header.Get("User-Agent") != "Sub2API-WebAccess/1.0" {
					return nil, errors.New("unexpected forwarded request headers")
				}
				return webAccessLocalNetworkResponse(req, http.StatusOK, "fl=test\nip="+test.ip+"\nloc=CN\n"), nil
			}))
			decision, err := svc.GetWebAccessDecision(context.Background(), "127.0.0.1")
			require.NoError(t, err)
			require.True(t, decision.Enabled)
			require.Equal(t, test.blocked, decision.Blocked)
			require.Equal(t, WebAccessLocationLocalNetwork, decision.LocationSource)
			serialized, err := json.Marshal(decision)
			require.NoError(t, err)
			require.NotContains(t, string(serialized), test.ip, "public responses must not expose the observed egress IP")
			_, err = svc.GetWebAccessDecision(context.Background(), "::1")
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load(), "the instance should reuse its egress result for local visitors")
		})
	}
}

func TestWebAccessLocalNetworkAppliesAllowlistToEgress(t *testing.T) {
	t.Setenv("WEB_ACCESS_LOCAL_NETWORK_CHECK", "true")
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	svc := webAccessLocalNetworkTestService(t, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		return webAccessLocalNetworkResponse(req, http.StatusOK, "ip=223.5.5.5\n"), nil
	}))
	repo := svc.settingRepo.(*webAccessSettingsRepoStub)
	repo.values[SettingKeyWebAccessIPAllowlist] = `["223.5.5.0/24"]`
	decision, err := svc.GetWebAccessDecision(context.Background(), "127.0.0.1")
	require.NoError(t, err)
	require.False(t, decision.Blocked)
	require.Equal(t, WebAccessLocationLocalNetwork, decision.LocationSource)

	repo.values[SettingKeyWebAccessIPAllowlist] = `["127.0.0.1"]`
	svc.invalidateWebAccessSettings()
	decision, err = svc.GetWebAccessDecision(context.Background(), "127.0.0.1")
	require.NoError(t, err)
	require.True(t, decision.Blocked, "explicit local-network checks must use the resolved egress for allowlisting")
}

func TestWebAccessLocalNetworkOnlyRunsForExplicitLocalMode(t *testing.T) {
	for _, test := range []struct {
		name, enabled, forceDisabled, listener, client string
		featureEnabled                                 bool
	}{
		{"default off", "", "", "127.0.0.1", "127.0.0.1", true},
		{"explicit false", "false", "", "127.0.0.1", "127.0.0.1", true},
		{"feature off", "true", "", "127.0.0.1", "127.0.0.1", false},
		{"emergency disabled", "true", "true", "127.0.0.1", "127.0.0.1", true},
		{"public IPv4 visitor", "true", "", "127.0.0.1", "223.5.5.5", true},
		{"public IPv6 visitor", "true", "", "::1", "2400:3200::1", true},
		{"LAN visitor", "true", "", "127.0.0.1", "192.168.1.2", true},
		{"all IPv4 interfaces", "true", "", "0.0.0.0", "127.0.0.1", true},
		{"all IPv6 interfaces", "true", "", "::", "::1", true},
		{"public listener", "true", "", "8.8.8.8", "127.0.0.1", true},
		{"unresolved listener", "true", "", "localhost", "127.0.0.1", true},
		{"unknown visitor", "true", "", "127.0.0.1", "unknown", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("WEB_ACCESS_LOCAL_NETWORK_CHECK", test.enabled)
			t.Setenv("WEB_ACCESS_FORCE_DISABLED", test.forceDisabled)
			var calls atomic.Int32
			svc := webAccessLocalNetworkTestService(t, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("network lookup must not occur")
			}))
			svc.cfg.Server.Host = test.listener
			if !test.featureEnabled {
				svc.settingRepo.(*webAccessSettingsRepoStub).values[SettingKeyWebAccessCNBlockEnabled] = "false"
			}
			decision, err := svc.GetWebAccessDecision(context.Background(), test.client)
			require.NoError(t, err)
			require.Equal(t, WebAccessLocationClientIP, decision.LocationSource)
			require.Zero(t, calls.Load())
		})
	}
}

func TestWebAccessLocalNetworkFailureIsExplicitAndCached(t *testing.T) {
	t.Setenv("WEB_ACCESS_LOCAL_NETWORK_CHECK", "true")
	t.Setenv("WEB_ACCESS_FORCE_DISABLED", "")
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"provider error", http.StatusServiceUnavailable, "unavailable"},
		{"invalid JSON", http.StatusOK, "not json"},
		{"empty IP", http.StatusOK, `{}`},
		{"hostname", http.StatusOK, `{"ip":"example.test"}`},
		{"private IPv4", http.StatusOK, `{"ip":"192.168.1.1"}`},
		{"loopback", http.StatusOK, `{"ip":"127.0.0.1"}`},
		{"mapped loopback", http.StatusOK, `{"ip":"::ffff:127.0.0.1"}`},
		{"multicast", http.StatusOK, `{"ip":"224.0.0.1"}`},
		{"shared address", http.StatusOK, `{"ip":"100.64.0.1"}`},
		{"documentation IPv4", http.StatusOK, `{"ip":"192.0.2.1"}`},
		{"documentation IPv6", http.StatusOK, `{"ip":"2001:db8::1"}`},
		{"scoped IPv6", http.StatusOK, `{"ip":"fe80::1%eth0"}`},
		{"oversized response", http.StatusOK, strings.Repeat(" ", webAccessLocalNetworkMaxBodyBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			svc := webAccessLocalNetworkTestService(t, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return webAccessLocalNetworkResponse(req, test.status, test.body), nil
			}))
			decision, err := svc.GetWebAccessDecision(context.Background(), "127.0.0.1")
			require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
			require.True(t, decision.Enabled)
			require.False(t, decision.Blocked)
			require.Equal(t, WebAccessLocationLocalNetworkUnavailable, decision.LocationSource)
			require.Equal(t, ErrWebAccessLocalNetworkUnavailable.Error(), err.Error(), "provider response text must not be exposed")
			_, err = svc.GetWebAccessDecision(context.Background(), "127.0.0.1")
			require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
			require.EqualValues(t, 2, calls.Load(), "one fixed fallback is allowed, failures are briefly cached")
		})
	}
}

func TestWebAccessLocalNetworkFallbackAndRedirectPolicy(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	var hosts []string
	setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Host)
		if req.URL.Host == "www.cloudflare.com" {
			response := webAccessLocalNetworkResponse(req, http.StatusFound, "")
			response.Header.Set("Location", "https://example.test/redirect")
			return response, nil
		}
		return webAccessLocalNetworkResponse(req, http.StatusOK, `{"ip":"2400:3200::1"}`), nil
	}))
	address, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("2400:3200::1"), address)
	require.Equal(t, []string{"www.cloudflare.com", "api.ipify.org"}, hosts)
	require.Nil(t, resolver.state.client.Jar)

	transport := newWebAccessLocalNetworkHTTPClient(webAccessLocalProxyPolicy{proxy: http.ProxyFromEnvironment}).Transport.(*http.Transport)
	require.NotNil(t, transport.Proxy)
	require.True(t, transport.DisableKeepAlives, "each uncached local probe needs a fresh proxy tunnel")
	require.False(t, transport.ForceAttemptHTTP2, "local probes must not reuse HTTP/2 tunnels")
}

func TestWebAccessLocalNetworkTraceRequiresOnePublicIP(t *testing.T) {
	for _, body := range []string{
		"loc=CN\n",
		"ip=223.5.5.5\nip=8.8.8.8\n",
		"ip=\nip=223.5.5.5\n",
		"ip=127.0.0.1\nloc=CN\n",
		"ip=2001:db8::1\n",
	} {
		resolver := newWebAccessLocalNetworkResolver()
		setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
			return webAccessLocalNetworkResponse(req, http.StatusOK, body), nil
		}))
		_, err := resolver.resolve(context.Background())
		require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable, body)
	}
}

func TestWebAccessLocalNetworkConcurrentLookupAndExpiry(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
			return webAccessLocalNetworkResponse(req, http.StatusOK, "ip=223.5.5.5\n"), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}))
	var group sync.WaitGroup
	errorsFound := make(chan error, 20)
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := resolver.resolve(context.Background())
			errorsFound <- err
		}()
	}
	<-started
	close(release)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls.Load())
	cache := *resolver.state.cache.Load()
	cache.expiresAt = time.Now().Add(-time.Second)
	resolver.state.cache.Store(&cache)
	_, err := resolver.resolve(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load(), "network changes become visible after the short cache expires")
}

func TestWebAccessLocalNetworkCancelledRequest(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	var calls atomic.Int32
	setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolver.resolve(ctx)
	require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
}

func TestWebAccessLocalNetworkAttemptTimeoutIsBounded(t *testing.T) {
	resolver := newWebAccessLocalNetworkResolver()
	var calls atomic.Int32
	setWebAccessLocalNetworkTestTransport(resolver, webAccessLocalNetworkRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	started := time.Now()
	_, err := resolver.resolve(context.Background())
	require.ErrorIs(t, err, ErrWebAccessLocalNetworkUnavailable)
	require.Less(t, time.Since(started), 4*time.Second)
	require.EqualValues(t, 2, calls.Load())
}
