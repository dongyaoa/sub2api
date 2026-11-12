//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceUpstreamProxyRoutesPelicanAndCandyWithoutStoringCredentials(t *testing.T) {
	for _, kind := range []string{IntelligenceMonitorTestPelican, IntelligenceMonitorTestCandy} {
		for _, mode := range []string{MonitorAPIModeResponses, MonitorAPIModeChatCompletions} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				calls := 0
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					calls++
					require.Equal(t, "intelligence-only-proxy.invalid", req.URL.Host)
					require.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
					var payload map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
					require.Equal(t, true, payload["stream"])
					w.Header().Set("Content-Type", "application/json")
					if mode == MonitorAPIModeChatCompletions {
						_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"21"},"finish_reason":"stop"}]}`))
					} else {
						_, _ = w.Write([]byte(`{"status":"completed","output_text":"21"}`))
					}
				}))
				defer proxy.Close()
				target := &UpstreamTarget{ID: 7, Provider: MonitorProviderOpenAI, Endpoint: "http://intelligence-only-proxy.invalid/v1", APIKeyEncrypted: "encrypted:test-key", AccountIDs: []int64{5}}
				route := financeTestProxy(t, proxy.URL, 1)
				route.Username, route.Password = "proxy-user", "proxy-secret"
				accounts := &financeAccounts{accounts: map[int64]*Account{5: {ID: 5, Type: AccountTypeAPIKey, Platform: MonitorProviderOpenAI, Credentials: map[string]any{"base_url": target.Endpoint, "api_key": "test-key"}, ProxyID: &route.ID, Proxy: route}}}
				// Finance fallback also covers callers that don't configure OAuth support.
				svc := NewIntelligenceMonitorService(nil, upstreamTestEncryptor{}, &upstreamTestRepo{target: target}, nil, nil, NewUpstreamFinanceService(nil, nil, nil, nil, accounts), nil)
				svc.externalClient.Transport = financeRoundTrip(func(*http.Request) (*http.Response, error) {
					t.Error("must not use direct transport")
					return nil, fmt.Errorf("direct disabled")
				})
				run := &IntelligenceMonitorRun{SourceType: "upstream", SourceEndpoint: target.Endpoint, SourceSnapshot: map[string]any{"upstream_target_id": int64(7)}, TestKind: kind, APIMode: mode, TimeoutSeconds: 60}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				status, text, message := svc.generate(ctx, run, "test-key")
				require.Empty(t, message)
				require.Equal(t, 200, *status)
				require.Equal(t, "21", text)
				require.Equal(t, 1, calls)
				require.Equal(t, map[string]any{"upstream_target_id": int64(7)}, run.SourceSnapshot)
			})
		}
	}
}

func TestIntelligenceUpstreamProxyRejectsMissingOrChangedSource(t *testing.T) {
	target := &UpstreamTarget{ID: 7, Provider: MonitorProviderOpenAI, Endpoint: "http://unresolvable.invalid/v1", APIKeyEncrypted: "encrypted:test-key", AccountIDs: []int64{5}}
	for _, failure := range []string{"missing repo", "missing snapshot", "changed key", "changed endpoint", "changed provider", "proxy unavailable"} {
		t.Run(failure, func(t *testing.T) {
			copy := *target
			svc := NewIntelligenceMonitorService(nil, upstreamTestEncryptor{}, &upstreamTestRepo{target: &copy}, nil, nil, nil, nil)
			svc.externalClient.Transport = financeRoundTrip(func(*http.Request) (*http.Response, error) {
				t.Error("invalid route must never dispatch")
				return nil, fmt.Errorf("direct disabled")
			})
			run := &IntelligenceMonitorRun{SourceType: "upstream", SourceEndpoint: target.Endpoint, SourceSnapshot: map[string]any{"upstream_target_id": int64(7)}}
			switch failure {
			case "missing repo":
				svc.upstreams = nil
			case "missing snapshot":
				run.SourceSnapshot = nil
			case "changed key":
				copy.APIKeyEncrypted = "encrypted:new-key"
			case "changed endpoint":
				copy.Endpoint = "https://other.invalid"
			case "changed provider":
				copy.Provider = MonitorProviderAnthropic
			}
			status, text, message := svc.generate(context.Background(), run, "test-key")
			require.Nil(t, status)
			require.Empty(t, text)
			require.Contains(t, message, "upstream_proxy_")
			require.NotContains(t, message, "test-key")
		})
	}
}
