//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamEndpointHTTPAndPublicIP(t *testing.T) {
	for _, endpoint := range []string{
		"http://216.158.77.26:7070", "https://8.8.8.8:8443/v1", "http://8.8.8.8:80/prefix/v1",
		"http://[2606:4700:4700::1111]:7070/v1", "https://[2606:4700:4700::1111]/v1",
	} {
		t.Run(endpoint, func(t *testing.T) { require.NoError(t, validateUpstreamEndpoint(endpoint)) })
	}
	for _, endpoint := range []string{"http://api.example.com:7070/prefix/v1", "https://api.example.com", "http://api.example.com/tenant%2Fone/v1"} {
		_, err := parseUpstreamEndpointURL(endpoint)
		require.NoError(t, err)
	}
	// The separate channel-monitor URL policy is unchanged.
	require.ErrorIs(t, validateEndpoint("http://8.8.8.8:7070"), ErrChannelMonitorEndpointScheme)
}

func TestUpstreamEndpointRejectsMalformedAddressesAndPrivateHosts(t *testing.T) {
	for _, endpoint := range []string{
		"", "8.8.8.8:7070", "//8.8.8.8:7070", "ftp://8.8.8.8", "http:///v1", "http://:7070/v1",
		"http://user:secret@8.8.8.8", "http://8.8.8.8:0", "http://8.8.8.8:65536", "http://8.8.8.8:bad",
		"http://8.8.8.8:", "http://8.8.8.8/v1?", "http://8.8.8.8/v1?key=secret", "http://8.8.8.8/v1#",
		"http://8.8.8.8/v1#fragment", "http://8.8.8.8\\v1", "http://8.8.8.8/a b", "http://[8.8.8.8]:7070",
		"http://2606:4700:4700::1111:7070", "http://[fe80::1%25eth0]:7070",
	} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := parseUpstreamEndpointURL(endpoint)
			require.ErrorIs(t, err, ErrUpstreamInvalid)
		})
	}
	for _, endpoint := range []string{
		"http://127.0.0.1:7070", "http://10.0.0.1:80", "http://192.168.1.1:7070",
		"http://169.254.169.254", "http://[::1]:7070", "http://[::ffff:127.0.0.1]:7070", "http://[fd00::1]:7070",
		"http://localhost:7070", "http://metadata.google.internal",
	} {
		t.Run(endpoint, func(t *testing.T) {
			require.ErrorIs(t, validateUpstreamEndpoint(endpoint), ErrChannelMonitorEndpointPrivate)
		})
	}
}

func TestUpstreamHTTPAccountImportAndModelDiscovery(t *testing.T) {
	endpoint := "http://8.8.8.8:7070/relay/v1"
	account := &Account{ID: 9, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"base_url": endpoint, "api_key": "test-http-key"}}
	repo := &upstreamTestRepo{}
	svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, upstreamTestAccounts{account: account}, nil)
	name, provider, ids, paused := "HTTP upstream", PlatformOpenAI, []int64{9}, false
	result, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SupplierID: json.RawMessage("2"), Name: &name,
		Provider: &provider, Endpoint: &endpoint, AccountIDs: &ids, Enabled: &paused})
	require.NoError(t, err)
	require.Equal(t, endpoint, result.Endpoint)
	require.Equal(t, ids, result.AccountIDs)
	require.Equal(t, "encrypted:test-http-key", result.APIKeyEncrypted)
	svc.modelsClient.Transport = upstreamModelsTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "http://8.8.8.8:7070/relay/v1/models", r.URL.String())
		require.Equal(t, "Bearer test-http-key", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"http-model"}]}`))}, nil
	})
	models, err := svc.Models(context.Background(), UpstreamModelsInput{AccountID: &account.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"http-model"}, models)
	// Merely changing TLS to plaintext cannot forward an existing saved secret.
	repo.target = validUpstreamTestTarget()
	endpoint = "http://8.8.8.8"
	_, err = svc.SaveTarget(context.Background(), repo.target.ID, UpstreamTargetInput{Endpoint: &endpoint})
	require.ErrorIs(t, err, ErrUpstreamInvalid)
}

func TestIntelligenceHTTPUpstreamGeneration(t *testing.T) {
	svc := NewIntelligenceMonitorService(&intelligenceTestRepository{}, upstreamTestEncryptor{}, nil, nil, nil, nil, nil)
	name, endpoint, key, source := "HTTP monitor", "http://8.8.8.8:7070/relay/v1", "test-http-key", "external"
	_, err := svc.SavePlan(context.Background(), 0, 1, IntelligenceMonitorInput{Name: &name, Endpoint: &endpoint, APIKey: &key, SourceType: &source})
	require.NoError(t, err)
	svc.externalClient.Transport = upstreamModelsTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "http://8.8.8.8:7070/relay/v1/chat/completions", r.URL.String())
		require.Equal(t, "Bearer test-http-key", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"<!doctype html><html><body>Pelican</body></html>"}}]}`))}, nil
	})
	status, text, message := svc.generate(context.Background(), &IntelligenceMonitorRun{SourceType: "upstream", SourceEndpoint: endpoint, APIMode: MonitorAPIModeChatCompletions}, key)
	require.Empty(t, message)
	require.NotNil(t, status)
	require.Equal(t, http.StatusOK, *status)
	require.Contains(t, text, "Pelican")
}
