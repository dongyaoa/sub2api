package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

var (
	ErrUpstreamProxyUnavailable     = infraerrors.BadRequest("UPSTREAM_PROXY_UNAVAILABLE", "upstream_proxy_unavailable")
	ErrUpstreamProxyConflict        = infraerrors.BadRequest("UPSTREAM_PROXY_CONFLICT", "upstream_proxy_conflict")
	ErrUpstreamProxyAccountMismatch = infraerrors.BadRequest("UPSTREAM_PROXY_ACCOUNT_MISMATCH", "upstream_proxy_account_mismatch")
)

// Resolve on each operation so an existing binding follows later proxy changes.
// Only the matching account's stored route may receive the target credential.
// A missing/broken configured proxy is an error, never permission to dial direct.
func resolveUpstreamAccountProxy(ctx context.Context, accounts AccountRepository, accountIDs []int64, provider, endpoint, apiKey string) (string, error) {
	if len(accountIDs) == 0 {
		return "", nil
	}
	if accounts == nil {
		return "", ErrUpstreamProxyUnavailable
	}
	var first *Account
	var route string
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			return "", ErrUpstreamProxyAccountMismatch
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		account, err := accounts.GetByID(ctx, id)
		if err != nil || account == nil {
			return "", ErrUpstreamProxyUnavailable
		}
		accountEndpoint := normalizeEndpoint(account.GetCredential("base_url"))
		if accountEndpoint == "" {
			switch account.Platform {
			case MonitorProviderOpenAI:
				accountEndpoint = "https://api.openai.com"
			case MonitorProviderAnthropic:
				accountEndpoint = "https://api.anthropic.com"
			case MonitorProviderGemini:
				accountEndpoint = "https://generativelanguage.googleapis.com"
			}
		}
		if account.Type != AccountTypeAPIKey || account.ParentAccountID != nil || account.IsSyntheticUITest() ||
			account.Platform != provider || accountEndpoint != normalizeEndpoint(endpoint) ||
			strings.TrimSpace(account.GetCredential("api_key")) != strings.TrimSpace(apiKey) || strings.TrimSpace(apiKey) == "" {
			return "", ErrUpstreamProxyAccountMismatch
		}
		snapshot := *account
		identity, err := upstreamAccountProxyIdentity(&snapshot)
		if err != nil {
			return "", err
		}
		if first == nil {
			first, route = &snapshot, identity
		} else if route != identity {
			return "", ErrUpstreamProxyConflict
		}
	}
	if first == nil {
		return "", nil
	}
	if err := selectAccountTestProxy(first, nil); err != nil {
		return "", ErrUpstreamProxyUnavailable
	}
	if first.ProxyID == nil && first.Proxy == nil {
		return "", nil
	}
	if first.ProxyID == nil || first.Proxy == nil || first.Proxy.ID != *first.ProxyID {
		return "", ErrUpstreamProxyUnavailable
	}
	value, _, err := proxyurl.Parse(first.Proxy.URL())
	if err != nil || value == "" {
		return "", ErrUpstreamProxyUnavailable
	}
	return value, nil
}

// Compare route configurations before selecting from a pool. Selecting a random
// pool member separately for each account would make identical pools conflict.
func upstreamAccountProxyIdentity(account *Account) (string, error) {
	pool := account.ProxyPool
	configuredPool, err := ParseAccountProxyPool(account.Extra[AccountProxyPoolExtraKey])
	if err != nil {
		return "", ErrUpstreamProxyUnavailable
	}
	hasPool := len(pool) > 0 || len(configuredPool) > 0
	if hasPool {
		identities := make([]string, 0, len(pool))
		for _, entry := range pool {
			if entry.Concurrency <= 0 || entry.Proxy == nil || entry.Proxy.ID != entry.ProxyID ||
				(entry.Proxy.Status != "" && !entry.Proxy.IsActive()) || entry.Proxy.IsExpired(time.Now()) {
				continue
			}
			route, _, err := proxyurl.Parse(entry.Proxy.URL())
			if err != nil || route == "" {
				return "", ErrUpstreamProxyUnavailable
			}
			identities = append(identities, fmt.Sprintf("%d:%d:%s", entry.ProxyID, entry.Concurrency, route))
		}
		if len(identities) == 0 {
			return "", ErrUpstreamProxyUnavailable
		}
		sort.Strings(identities)
		return strings.Join(identities, "\x00"), nil
	}
	if account.ProxyID == nil && account.Proxy == nil {
		return "direct", nil
	}
	if account.ProxyID == nil || account.Proxy == nil || account.Proxy.ID != *account.ProxyID ||
		(account.Proxy.Status != "" && !account.Proxy.IsActive()) || account.Proxy.IsExpired(time.Now()) {
		return "", ErrUpstreamProxyUnavailable
	}
	route, _, err := proxyurl.Parse(account.Proxy.URL())
	if err != nil || route == "" {
		return "", ErrUpstreamProxyUnavailable
	}
	return fmt.Sprintf("%d:%s", account.Proxy.ID, route), nil
}

type upstreamProxyClientKey struct{}

func withUpstreamProxyClient(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}
	return context.WithValue(ctx, upstreamProxyClientKey{}, client)
}

func upstreamRequestClient(ctx context.Context, fallback *http.Client) *http.Client {
	if client, ok := ctx.Value(upstreamProxyClientKey{}).(*http.Client); ok && client != nil {
		return client
	}
	return fallback
}

func validateUpstreamRequestEndpoint(ctx context.Context, endpoint string) error {
	if client, ok := ctx.Value(upstreamProxyClientKey{}).(*http.Client); ok && client != nil {
		return validateProxiedUpstreamEndpoint(endpoint)
	}
	return validateUpstreamEndpoint(endpoint)
}

// A configured account proxy owns destination DNS. Local DNS availability must
// not prevent the proxy from resolving a public hostname. URL policy still
// rejects local/metadata hostnames and private literal destinations.
func validateProxiedUpstreamEndpoint(endpoint string) error {
	u, err := parseUpstreamEndpointURL(endpoint)
	if err != nil {
		return err
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if isBlockedHostname(host) || strings.HasSuffix(host, ".localhost") {
		return ErrChannelMonitorEndpointPrivate
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return ErrChannelMonitorEndpointPrivate
		}
	} else {
		// Some proxy stacks accept inet_aton-style short, integer, hexadecimal
		// or octal IPv4 addresses. Require canonical IP syntax for numeric hosts
		// so these cannot disguise a loopback/private destination from the check.
		numeric := true
		for _, label := range strings.Split(host, ".") {
			if _, err := strconv.ParseUint(label, 0, 32); err != nil {
				numeric = false
				break
			}
		}
		if numeric {
			return ErrChannelMonitorEndpointPrivate
		}
	}
	return nil
}

func newUpstreamProxyHTTPClient(rawProxyURL string, timeout time.Duration) (*http.Client, error) {
	_, route, err := proxyurl.Parse(rawProxyURL)
	if err != nil || route == nil {
		return nil, ErrUpstreamProxyUnavailable
	}
	transport := newMonitorHTTPTransport(timeout)
	// The proxy itself is an explicitly configured account route and may be a
	// local sidecar. Do not apply the destination's public-IP policy to it.
	transport.DialContext = (&net.Dialer{Timeout: monitorDialTimeout, KeepAlive: monitorDialKeepAlive}).DialContext
	if err := proxyutil.ConfigureTransportProxy(transport, route); err != nil {
		return nil, ErrUpstreamProxyUnavailable
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     &upstreamProxyTransport{transport: transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

type upstreamProxyTransport struct{ transport *http.Transport }

func (t *upstreamProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, ErrChannelMonitorInvalidEndpoint
	}
	u := *req.URL
	// Internal adapters add their own query parameters after validating the
	// configured endpoint. Validate the authority/path on every outbound read.
	u.RawQuery, u.ForceQuery = "", false
	if err := validateProxiedUpstreamEndpoint(u.String()); err != nil {
		return nil, err
	}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return resp, &upstreamProxyRequestError{cause: err}
	}
	if resp != nil && resp.Body != nil {
		resp.Body = &upstreamProxyResponseBody{ReadCloser: resp.Body}
	}
	return resp, nil
}

func (t *upstreamProxyTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

type upstreamProxyRequestError struct{ cause error }

func (e *upstreamProxyRequestError) Error() string { return "upstream_proxy_request_failed" }
func (e *upstreamProxyRequestError) Unwrap() error { return e.cause }

type upstreamProxyResponseBody struct{ io.ReadCloser }

func (b *upstreamProxyResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		return n, &upstreamProxyRequestError{cause: err}
	}
	return n, err
}
