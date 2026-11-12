//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamProxyFinanceRepo struct {
	*upstreamScheduledBillingRepo
	ids []int64
	err error
}

func (r *upstreamProxyFinanceRepo) ProxyAccountIDs(context.Context, int64) ([]int64, error) {
	return r.ids, r.err
}

func financeTestProxy(t *testing.T, endpoint string, id int64) *Proxy {
	t.Helper()
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return &Proxy{ID: id, Protocol: "http", Host: u.Hostname(), Port: port, Status: StatusActive}
}

func proxyFinanceService(t *testing.T, target *UpstreamFinanceTarget, proxy *Proxy) (*UpstreamFinanceService, *upstreamProxyFinanceRepo, *Account) {
	t.Helper()
	repo := &upstreamProxyFinanceRepo{upstreamScheduledBillingRepo: &upstreamScheduledBillingRepo{target: target}, ids: []int64{5}}
	account := &Account{ID: 5, Type: AccountTypeAPIKey, Platform: target.Provider, Status: StatusActive,
		Credentials: map[string]any{"base_url": target.Endpoint, "api_key": target.APIKeyEncrypted}, ProxyID: &proxy.ID, Proxy: proxy}
	svc := NewUpstreamFinanceService(repo, financeTestCipher{}, nil, nil, &financeAccounts{accounts: map[int64]*Account{5: account}})
	svc.client.Transport = financeRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Error("a configured proxy must not fall back to the direct client")
		return nil, fmt.Errorf("direct route disabled")
	})
	return svc, repo, account
}

func TestUpstreamFinanceProxyUsesRemoteDNSForSub2APIUsageAndBilling(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	paths := []string{}
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, r.URL.IsAbs(), "HTTP forward proxy must receive the original authority")
		require.Equal(t, "unresolvable-upstream.invalid", r.URL.Host)
		require.Equal(t, "Bearer test-secret-key", r.Header.Get("Authorization"))
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/prefix/v1/usage":
			require.Equal(t, "1", r.URL.Query().Get("days"))
			_, _ = w.Write([]byte(`{"balance":25,"usage":{"today":{"actual_cost":3}}}`))
		case "/prefix/v1/sub2api/billing":
			_, _ = w.Write([]byte(remoteBillingFixture(.12, now)))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer proxyServer.Close()
	target := &UpstreamFinanceTarget{ID: 7, Provider: MonitorProviderOpenAI, Endpoint: "http://unresolvable-upstream.invalid/prefix/v1", APIKeyEncrypted: "test-secret-key"}
	svc, repo, _ := proxyFinanceService(t, target, financeTestProxy(t, proxyServer.URL, 1))
	svc.now = func() time.Time { return now }
	originalClient := svc.client
	got, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", got.Status)
	require.Equal(t, 25.0, *got.Balance)
	require.Equal(t, 3.0, *got.DayUsed)
	require.Equal(t, .12, *got.Billing.EffectiveRateMultiplier)
	require.Equal(t, []string{"/prefix/v1/usage", "/prefix/v1/sub2api/billing"}, paths)
	require.Same(t, originalClient, svc.client)
	require.Empty(t, repo.token)
}

func TestUpstreamFinanceProxyNewAPIForceUsesAccountRouteForAllReads(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	paths := []string{}
	fixtures := newAPIFixtures()
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "newapi-private-dns.invalid", r.URL.Host)
		path := strings.TrimPrefix(r.URL.Path, "/prefix")
		paths = append(paths, path)
		w.Header().Set("Content-Type", "application/json")
		if path == "/api/status" {
			require.Empty(t, r.Header.Get("Authorization"))
		} else {
			require.Equal(t, "Bearer console-secret", r.Header.Get("Authorization"))
			require.Equal(t, "42", r.Header.Get("New-Api-User"))
		}
		if path == "/api/log/self" {
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"user_id":42,"token_id":13,"created_at":%d,"type":2,"quota":500000}]}}`, now.Unix()-1)
			return
		}
		body, ok := fixtures[path]
		if !ok {
			t.Errorf("unexpected path %s", path)
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer proxyServer.Close()
	target := &UpstreamFinanceTarget{ID: 7, Provider: MonitorProviderOpenAI, Endpoint: "http://newapi-private-dns.invalid/prefix/v1", APIKeyEncrypted: "sk-inference-secret", NewAPIUserID: 42, NewAPIAccessTokenEncrypted: "console-secret"}
	svc, _, _ := proxyFinanceService(t, target, financeTestProxy(t, proxyServer.URL, 2))
	svc.now = func() time.Time { return now }
	got, err := svc.ForceSyncNewAPIBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", got.Status)
	require.Equal(t, "synced", got.UsageSyncStatus)
	require.Equal(t, 1.0, *got.DayUsed)
	require.Equal(t, 20.0, *got.Balance)
	require.Equal(t, .35, *got.Billing.EffectiveRateMultiplier)
	require.Equal(t, []string{"/api/status", "/api/user/self", "/api/token/search", "/api/log/self", "/api/user/self/groups"}, paths)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "console-secret")
	require.NotContains(t, string(encoded), "sk-inference-secret")
}

func TestUpstreamFinanceProxyFollowsAccountChangesAndFailsClosed(t *testing.T) {
	calls := [2]int{}
	servers := make([]*httptest.Server, 2)
	for i := range servers {
		i := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[i]++
			_, _ = w.Write([]byte(`{"balance":2,"usage":{"today":{"actual_cost":1}},"billing":` + remoteBillingFixture(.2, time.Now()) + `}`))
		}))
		defer servers[i].Close()
	}
	target := &UpstreamFinanceTarget{ID: 7, Provider: MonitorProviderOpenAI, Endpoint: "http://only-via-proxy.invalid/v1", APIKeyEncrypted: "test-secret"}
	svc, repo, account := proxyFinanceService(t, target, financeTestProxy(t, servers[0].URL, 1))
	first, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", first.Status)
	account.Proxy = financeTestProxy(t, servers[1].URL, 2)
	account.ProxyID = &account.Proxy.ID
	second, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", second.Status)
	require.Equal(t, [2]int{1, 1}, calls)
	account.Proxy = nil
	failed, err := svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "error", failed.Status)
	require.Equal(t, "upstream_proxy_unavailable", failed.Error)
	require.Equal(t, [2]int{1, 1}, calls)
	require.Empty(t, repo.token)
	repo.err = ErrUpstreamProxyAccountMismatch
	failed, err = svc.SyncBalance(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "upstream_proxy_account_mismatch", failed.Error)
	require.Equal(t, [2]int{1, 1}, calls)
}
