package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountTestProxyRepo struct {
	AccountRepository
	account *Account
}

func (r *accountTestProxyRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func accountTestProxyFixture() *Account {
	primaryID := int64(11)
	return &Account{
		ID: 75, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-account-token"},
		ProxyID:     &primaryID,
		Proxy:       &Proxy{ID: primaryID, Name: "Disabled primary", Status: "inactive"},
		ProxyPool: []AccountProxyPoolEntry{
			{ProxyID: primaryID, Concurrency: 1, Proxy: &Proxy{ID: primaryID, Name: "Disabled primary", Status: "inactive"}},
			{ProxyID: 12, Concurrency: 2, Proxy: &Proxy{
				ID: 12, Name: "US test node", Status: StatusActive,
				Protocol: "http", Host: "private-proxy.example", Port: 8080,
				Username: "private-proxy-user", Password: "private-proxy-password",
			}},
		},
	}
}

func runAccountProxyTest(t *testing.T, account *Account, upstream *httpUpstreamRecorder) ([]TestEvent, string, error) {
	t.Helper()
	svc := &AccountTestService{accountRepo: &accountTestProxyRepo{account: account}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/75/test", nil)
	err := svc.TestAccountConnection(c, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	var events []TestEvent
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: ") {
			var event TestEvent
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
			events = append(events, event)
		}
	}
	return events, recorder.Body.String(), err
}

func TestAccountTestConnectionProxyMatchesTransport(t *testing.T) {
	account := accountTestProxyFixture()
	selectedProxy := account.ProxyPool[1].Proxy
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n")),
	}}
	events, output, err := runAccountProxyTest(t, account, upstream)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, selectedProxy.URL(), upstream.lastProxyURL)
	require.GreaterOrEqual(t, len(events), 3)
	require.Equal(t, "proxy_info", events[0].Type)
	require.Equal(t, "managed", events[0].RouteType)
	require.NotNil(t, events[0].ProxyID)
	require.Equal(t, selectedProxy.ID, *events[0].ProxyID)
	require.Equal(t, selectedProxy.Name, events[0].ProxyName)
	require.Equal(t, "test_complete", events[len(events)-1].Type)
	require.True(t, events[len(events)-1].Success)
	// Selection must not mutate the repository's account or leak private proxy
	// endpoint/authentication data through either the event or other SSE output.
	require.Equal(t, int64(11), *account.ProxyID)
	require.False(t, account.ProxyPoolSelected)
	for _, secret := range []string{selectedProxy.URL(), selectedProxy.Host, selectedProxy.Username, selectedProxy.Password, "test-account-token"} {
		require.NotContains(t, output, secret)
	}
}

func TestAccountTestConnectionUnavailablePoolDoesNotSendRequest(t *testing.T) {
	for _, unavailable := range []string{"inactive", "expired", "missing hydration", "deleted pool"} {
		t.Run(unavailable, func(t *testing.T) {
			account := accountTestProxyFixture()
			switch unavailable {
			case "inactive":
				account.ProxyPool[1].Proxy.Status = "inactive"
			case "expired":
				expired := time.Now().Add(-time.Hour)
				account.ProxyPool[1].Proxy.ExpiresAt = &expired
			case "missing hydration":
				account.ProxyPool[1].Proxy = nil
			case "deleted pool":
				account.ProxyPool = nil
				account.Proxy, account.ProxyID = nil, nil
				account.Extra = map[string]any{AccountProxyPoolExtraKey: []any{map[string]any{"proxy_id": 12, "concurrency": 1}}}
			}
			upstream := &httpUpstreamRecorder{}
			events, _, err := runAccountProxyTest(t, account, upstream)
			require.ErrorContains(t, err, "No usable proxy")
			require.Empty(t, upstream.requests)
			require.Len(t, events, 2)
			require.Equal(t, "unknown", events[0].RouteType)
			require.Nil(t, events[0].ProxyID)
			require.Equal(t, "error", events[1].Type)
		})
	}
}

func TestAccountTestConnectionProxyFailureRedactsCredentials(t *testing.T) {
	account := accountTestProxyFixture()
	proxy := account.ProxyPool[1].Proxy
	upstream := &httpUpstreamRecorder{err: errors.New("proxyconnect " + proxy.URL() + ": rejected password=" + proxy.Password)}
	events, output, err := runAccountProxyTest(t, account, upstream)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "managed", events[0].RouteType)
	require.Equal(t, "error", events[len(events)-1].Type)
	require.Contains(t, output, "proxyconnect")
	for _, secret := range []string{proxy.URL(), proxy.Host, proxy.Username, proxy.Password} {
		require.NotContains(t, output, secret)
		require.NotContains(t, err.Error(), secret)
	}
}
