//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBoundAccountProxySaveProbeAndModels(t *testing.T) {
	paths := []string{}
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, "public-upstream.invalid", r.URL.Host)
		require.Equal(t, "Bearer account-api-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-test"}]}`))
			return
		}
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		question := regexp.MustCompile(`Q: (\d+) ([+-]) (\d+) = \?`).FindAllStringSubmatch(body.Messages[0].Content, -1)
		require.NotEmpty(t, question)
		last := question[len(question)-1]
		a, _ := strconv.Atoi(last[1])
		b, _ := strconv.Atoi(last[3])
		answer := a + b
		if last[2] == "-" {
			answer = a - b
		}
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":"%d"}}]}`, answer)
	}))
	defer proxyServer.Close()
	route := financeTestProxy(t, proxyServer.URL, 3)
	account := upstreamProxyTestAccount(9, route)
	account.Status, account.Schedulable = StatusActive, true
	account.Credentials["base_url"] = "http://public-upstream.invalid/v1"
	repo := &upstreamTestRepo{}
	accounts := &upstreamProxyAccounts{accounts: map[int64]*Account{9: account}}
	svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, accounts, nil)
	name, endpoint, ids := "Proxied group", "http://public-upstream.invalid/v1", []int64{9}
	created, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SupplierID: json.RawMessage("2"), Name: &name, Endpoint: &endpoint, AccountIDs: &ids})
	require.NoError(t, err, "binding an account proxy must not resolve the upstream using local DNS")
	created.ID = 7
	repo.target = created
	for _, manual := range []bool{true, false} {
		entries, err := svc.runCheck(context.Background(), 7, manual)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, MonitorStatusOperational, entries[0].Status)
	}
	for _, input := range []UpstreamModelsInput{{TargetID: &created.ID}, {AccountID: &account.ID}, {TargetID: &created.ID, APIKey: "account-api-secret"}} {
		models, err := svc.Models(context.Background(), input)
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-test"}, models)
	}
	require.Equal(t, []string{"/v1/chat/completions", "/v1/chat/completions", "/v1/models", "/v1/models", "/v1/models"}, paths)
	account.Proxy.Status = "disabled"
	entries, err := svc.runCheck(context.Background(), 7, true)
	require.NoError(t, err)
	require.Equal(t, "upstream_proxy_unavailable", entries[0].Message)
	require.Equal(t, MonitorStatusError, entries[0].Status)
	require.Len(t, paths, 5, "a disabled proxy cannot fall back to direct")
	_, err = svc.Models(context.Background(), UpstreamModelsInput{TargetID: &created.ID})
	require.ErrorIs(t, err, ErrUpstreamProxyUnavailable)
}

func TestUpstreamProxySourceGroupRequiresMatchingCredentials(t *testing.T) {
	svc, repo := monitorSourceFixture()
	source := repo.source
	svc.accounts = &upstreamProxyAccounts{accounts: map[int64]*Account{
		101: {ID: 101, Type: AccountTypeAPIKey, Platform: source.Provider, Credentials: map[string]any{"base_url": source.Endpoint, "api_key": "secret-testing-key"}},
		102: {ID: 102, Type: AccountTypeAPIKey, Platform: source.Provider, Credentials: map[string]any{"base_url": source.Endpoint, "api_key": "secret-testing-key"}},
	}}
	key := "secret-testing-key"
	monitor := &UpstreamTarget{Provider: source.Provider, Endpoint: source.Endpoint, SourceTargetID: &source.ID}
	ids, err := svc.targetProxyAccountIDs(context.Background(), monitor, key)
	require.NoError(t, err)
	require.Equal(t, source.AccountIDs, ids)
	_, err = svc.targetProxyAccountIDs(context.Background(), monitor, "rotated-key")
	require.ErrorIs(t, err, ErrUpstreamProxyAccountMismatch)
}
