package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	WebAccessLocationClientIP                = "client_ip"
	WebAccessLocationLocalNetwork            = "local_network"
	WebAccessLocationLocalNetworkUnavailable = "local_network_unavailable"
	webAccessLocalNetworkTimeout             = 3 * time.Second
	webAccessLocalNetworkAttemptTimeout      = 1500 * time.Millisecond
	webAccessLocalNetworkCacheTTL            = 5 * time.Second
	webAccessLocalNetworkErrorCacheTTL       = 5 * time.Second
	webAccessLocalNetworkMaxBodyBytes        = 1024
)

// ErrWebAccessLocalNetworkUnavailable distinguishes an explicitly enabled
// local-preview lookup failure from an ordinary, unknown visitor IP. Callers
// must show a temporary unavailable state rather than silently allow entry.
var ErrWebAccessLocalNetworkUnavailable = errors.New("local network location is temporarily unavailable")

// This mode is intentionally opt-in and limited to a loopback-only listener.
// Public deployments must never use the server's egress IP for visitor access.
func (s *SettingService) shouldCheckLocalWebAccessNetwork(clientIP netip.Addr) bool {
	if s == nil || s.cfg == nil || !clientIP.IsLoopback() {
		return false
	}
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("WEB_ACCESS_LOCAL_NETWORK_CHECK")))
	if !enabled {
		return false
	}
	listener, err := netip.ParseAddr(strings.TrimSpace(s.cfg.Server.Host))
	return err == nil && listener.Zone() == "" && listener.Unmap().IsLoopback()
}

type cachedWebAccessLocalNetwork struct {
	address   netip.Addr
	err       error
	expiresAt time.Time
}

type webAccessLocalNetworkResolver struct {
	loadProxyPolicy func() (webAccessLocalProxyPolicy, error)
	newClient       func(webAccessLocalProxyPolicy) *http.Client
	mu              sync.Mutex
	state           *webAccessLocalNetworkState
}

type webAccessLocalNetworkState struct {
	fingerprint string
	ctx         context.Context
	cancel      context.CancelFunc
	retired     atomic.Bool
	client      *http.Client
	cache       atomic.Pointer[cachedWebAccessLocalNetwork]
	inflight    singleflight.Group
}

func (s *SettingService) localWebAccessNetwork() *webAccessLocalNetworkResolver {
	if resolver := s.webAccessLocalNetwork.Load(); resolver != nil {
		return resolver
	}
	resolver := newWebAccessLocalNetworkResolver()
	if s.webAccessLocalNetwork.CompareAndSwap(nil, resolver) {
		return resolver
	}
	return s.webAccessLocalNetwork.Load()
}

func newWebAccessLocalNetworkResolver() *webAccessLocalNetworkResolver {
	return &webAccessLocalNetworkResolver{
		loadProxyPolicy: loadWebAccessLocalProxyPolicy,
		newClient:       newWebAccessLocalNetworkHTTPClient,
	}
}

func newWebAccessLocalNetworkHTTPClient(policy webAccessLocalProxyPolicy) *http.Client {
	// A fresh connection per probe also observes a node switch behind an
	// unchanged local proxy address after the five-second result cache expires.
	// This transport is used only for the opt-in local webpage check.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport := &http.Transport{
		Proxy:     policy.proxy,
		Protocols: protocols,
		DialContext: (&net.Dialer{
			Timeout:   webAccessLocalNetworkAttemptTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    webAccessLocalNetworkAttemptTimeout,
		ResponseHeaderTimeout:  webAccessLocalNetworkAttemptTimeout,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           2,
		MaxIdleConnsPerHost:    1,
		MaxResponseHeaderBytes: 4096,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   webAccessLocalNetworkTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (r *webAccessLocalNetworkResolver) resolve(ctx context.Context) (netip.Addr, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return netip.Addr{}, fmt.Errorf("%w: %w", ErrWebAccessLocalNetworkUnavailable, ctx.Err())
	}
	// Re-read proxy settings before consulting the egress cache: Windows proxy
	// switches must not keep a formerly direct result (or the old proxy's IP).
	r.mu.Lock()
	policy, policyErr := r.loadProxyPolicy()
	if policyErr != nil {
		if r.state != nil {
			r.state.retire()
			r.state = nil
		}
		r.mu.Unlock()
		return netip.Addr{}, fmt.Errorf("%w: %w", ErrWebAccessLocalNetworkUnavailable, policyErr)
	}
	if r.state == nil || r.state.fingerprint != policy.fingerprint {
		if r.state != nil {
			r.state.retire()
		}
		stateCtx, cancel := context.WithCancel(context.Background())
		r.state = &webAccessLocalNetworkState{
			fingerprint: policy.fingerprint, ctx: stateCtx, cancel: cancel, client: r.newClient(policy),
		}
	}
	state := r.state
	r.mu.Unlock()
	return state.resolve(ctx)
}

func (state *webAccessLocalNetworkState) retire() {
	state.retired.Store(true)
	state.cancel()
	state.client.CloseIdleConnections()
}

func (state *webAccessLocalNetworkState) resolve(ctx context.Context) (netip.Addr, error) {
	if state.retired.Load() {
		return netip.Addr{}, ErrWebAccessLocalNetworkUnavailable
	}
	if cached := state.cache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.address, cached.err
	}
	result := state.inflight.DoChan("local_network", func() (any, error) {
		if cached := state.cache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
			return cached.address, cached.err
		}
		// One cancelled page does not cancel a shared lookup for other pages.
		// The independent deadline still bounds all network work to 3 seconds.
		lookupCtx, cancel := context.WithTimeout(state.ctx, webAccessLocalNetworkTimeout)
		defer cancel()
		address, err := state.lookup(lookupCtx)
		ttl := webAccessLocalNetworkCacheTTL
		if err != nil {
			ttl = webAccessLocalNetworkErrorCacheTTL
		}
		state.cache.Store(&cachedWebAccessLocalNetwork{address: address, err: err, expiresAt: time.Now().Add(ttl)})
		return address, err
	})
	select {
	case <-ctx.Done():
		return netip.Addr{}, fmt.Errorf("%w: %w", ErrWebAccessLocalNetworkUnavailable, ctx.Err())
	case completed := <-result:
		if state.retired.Load() {
			return netip.Addr{}, ErrWebAccessLocalNetworkUnavailable
		}
		if completed.Err != nil {
			return netip.Addr{}, completed.Err
		}
		return completed.Val.(netip.Addr), nil
	}
}

func (state *webAccessLocalNetworkState) lookup(ctx context.Context) (netip.Addr, error) {
	// Fixed public endpoints only. The primary supports IPv4 and IPv6. Country
	// labels in trace output are ignored: only the observed IP is used locally.
	for _, endpoint := range []webAccessPublicIPProvider{
		{url: "https://www.cloudflare.com/cdn-cgi/trace", trace: true},
		{url: "https://api.ipify.org?format=json"},
	} {
		if ctx.Err() != nil {
			break
		}
		attemptCtx, cancel := context.WithTimeout(ctx, webAccessLocalNetworkAttemptTimeout)
		address, err := state.lookupEndpoint(attemptCtx, endpoint)
		cancel()
		if err == nil {
			return address, nil
		}
	}
	// Do not return endpoint response text or the observed public IP in errors.
	return netip.Addr{}, ErrWebAccessLocalNetworkUnavailable
}

type webAccessPublicIPProvider struct {
	url   string
	trace bool
}

func (state *webAccessLocalNetworkState) lookupEndpoint(ctx context.Context, endpoint webAccessPublicIPProvider) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.url, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	req.Header.Set("Accept", "application/json")
	if endpoint.trace {
		req.Header.Set("Accept", "text/plain")
	}
	req.Header.Set("User-Agent", "Sub2API-WebAccess/1.0")
	resp, err := state.client.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("unexpected public IP response status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webAccessLocalNetworkMaxBodyBytes+1))
	if err != nil || len(body) > webAccessLocalNetworkMaxBodyBytes {
		return netip.Addr{}, fmt.Errorf("invalid public IP response body")
	}
	ip := ""
	if endpoint.trace {
		foundIP := false
		for _, line := range strings.Split(string(body), "\n") {
			key, value, exists := strings.Cut(strings.TrimSpace(line), "=")
			if exists && key == "ip" {
				if foundIP {
					return netip.Addr{}, fmt.Errorf("duplicate public IP trace field")
				}
				foundIP = true
				ip = value
			}
		}
	} else {
		var payload struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return netip.Addr{}, fmt.Errorf("invalid public IP response JSON")
		}
		ip = payload.IP
	}
	address, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || address.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("invalid public IP address")
	}
	address = address.Unmap()
	if !isWebAccessPublicAddress(address) {
		return netip.Addr{}, fmt.Errorf("public IP response is not globally routable")
	}
	return address, nil
}

func isWebAccessPublicAddress(address netip.Addr) bool {
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	// IsGlobalUnicast includes several special-purpose/documentation ranges.
	// They cannot be a real public egress address returned by the provider.
	for _, prefix := range webAccessNonPublicNetworks {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var webAccessNonPublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("3fff::/20"),
}
