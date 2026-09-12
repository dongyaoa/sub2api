package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type selectedAccountTestProxyRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r selectedAccountTestProxyRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

type selectedAccountTestProxyUpstream struct {
	proxyURL string
	calls    int
}

func (u *selectedAccountTestProxyUpstream) Do(_ *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.proxyURL = proxyURL
	u.calls++
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n"))}, nil
}

func (u *selectedAccountTestProxyUpstream) DoWithTLS(r *http.Request, proxyURL string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, proxyURL, id, concurrency)
}

func TestAccountTestProxyRequestValidationAndSelection(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status, calls int
	}{
		{"selected", `{"proxy_id":13}`, 200, 1},
		{"empty body", "", 200, 1},
		{"wrong type", `{"proxy_id":"13"}`, 400, 0},
		{"malformed", `{"proxy_id":`, 400, 0},
		{"unbound", `{"proxy_id":99}`, 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxyID := int64(13)
			proxy := &service.Proxy{ID: proxyID, Name: "Test proxy", Status: service.StatusActive, Protocol: "http", Host: "selected.example", Port: 8080}
			account := &service.Account{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token"}, ProxyID: &proxyID, Proxy: proxy}
			upstream := &selectedAccountTestProxyUpstream{}
			svc := service.NewAccountTestService(selectedAccountTestProxyRepo{account: account}, nil, nil, nil, nil, upstream, nil, nil)
			handler := &AccountHandler{accountTestService: svc}
			router := gin.New()
			router.POST("/accounts/:id/test", handler.Test)
			request := httptest.NewRequest(http.MethodPost, "/accounts/1/test", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.calls, upstream.calls)
			if tc.calls > 0 {
				require.Equal(t, proxy.URL(), upstream.proxyURL)
				require.Contains(t, recorder.Body.String(), `"proxy_id":13`)
				require.Contains(t, recorder.Body.String(), `"type":"test_metrics"`)
			}
		})
	}
}
