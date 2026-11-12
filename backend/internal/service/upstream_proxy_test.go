package service

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamProxyAccounts struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *upstreamProxyAccounts) GetByID(_ context.Context, id int64) (*Account, error) {
	account := r.accounts[id]
	if account == nil {
		return nil, errors.New("secret repository error")
	}
	return account, nil
}

func upstreamProxyTestAccount(id int64, route *Proxy) *Account {
	account := &Account{ID: id, Type: AccountTypeAPIKey, Platform: MonitorProviderOpenAI, Credentials: map[string]any{"base_url": "https://api.example.invalid/v1/", "api_key": "account-api-secret"}}
	if route != nil {
		proxyID := route.ID
		account.ProxyID, account.Proxy = &proxyID, route
	}
	return account
}

func TestUpstreamProxyResolverFollowsAccountChangesAndRejectsConflicts(t *testing.T) {
	route := &Proxy{ID: 3, Protocol: "http", Host: "127.0.0.1", Port: 7890, Status: StatusActive}
	account := upstreamProxyTestAccount(1, route)
	repo := &upstreamProxyAccounts{accounts: map[int64]*Account{1: account}}
	resolve := func(ids ...int64) (string, error) {
		return resolveUpstreamAccountProxy(context.Background(), repo, ids, MonitorProviderOpenAI, "https://api.example.invalid/v1", "account-api-secret")
	}
	got, err := resolve(1)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:7890", got)
	account.Proxy.Port = 7891
	got, err = resolve(1)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:7891", got, "existing groups must read current proxy settings on every operation")
	second := upstreamProxyTestAccount(2, route)
	repo.accounts[2] = second
	got, err = resolve(1, 2, 1)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	second.ProxyID, second.Proxy = nil, nil
	_, err = resolve(1, 2)
	require.ErrorIs(t, err, ErrUpstreamProxyConflict, "mixed direct/proxy accounts must not silently choose direct")
	account.ProxyID, account.Proxy = nil, nil
	got, err = resolve(1, 2)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestUpstreamProxyResolverValidatesCredentialScopeAndConfiguredRoutes(t *testing.T) {
	for name, mutate := range map[string]func(*Account){
		"wrong provider": func(a *Account) { a.Platform = MonitorProviderAnthropic },
		"wrong key":      func(a *Account) { a.Credentials["api_key"] = "changed" },
		"wrong endpoint": func(a *Account) { a.Credentials["base_url"] = "https://other.example.invalid" },
		"wrong type":     func(a *Account) { a.Type = AccountTypeOAuth },
		"shadow":         func(a *Account) { id := int64(9); a.ParentAccountID = &id },
	} {
		t.Run(name, func(t *testing.T) {
			account := upstreamProxyTestAccount(1, nil)
			mutate(account)
			_, err := resolveUpstreamAccountProxy(context.Background(), &upstreamProxyAccounts{accounts: map[int64]*Account{1: account}}, []int64{1}, MonitorProviderOpenAI, "https://api.example.invalid/v1", "account-api-secret")
			require.ErrorIs(t, err, ErrUpstreamProxyAccountMismatch)
			require.NotContains(t, err.Error(), "account-api-secret")
		})
	}
	for _, scenario := range []string{"missing", "disabled", "expired", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			route := &Proxy{ID: 3, Protocol: "http", Host: "proxy.example", Port: 7890, Username: "route-secret-user", Password: "route-secret-pass"}
			account := upstreamProxyTestAccount(1, route)
			switch scenario {
			case "missing":
				account.Proxy = nil
			case "disabled":
				route.Status = "disabled"
			case "expired":
				expired := time.Now().Add(-time.Hour)
				route.ExpiresAt = &expired
			case "invalid":
				route.Protocol = "invalid"
			}
			_, err := resolveUpstreamAccountProxy(context.Background(), &upstreamProxyAccounts{accounts: map[int64]*Account{1: account}}, []int64{1}, MonitorProviderOpenAI, "https://api.example.invalid/v1", "account-api-secret")
			require.ErrorIs(t, err, ErrUpstreamProxyUnavailable)
			require.NotContains(t, err.Error(), "route-secret")
		})
	}
	value, err := resolveUpstreamAccountProxy(context.Background(), nil, nil, "openai", "https://api.example.invalid", "key")
	require.NoError(t, err)
	require.Empty(t, value)
}

func TestUpstreamProxyResolverUsesHydratedPoolsWithoutMutatingAccount(t *testing.T) {
	routes := []*Proxy{{ID: 1, Protocol: "http", Host: "127.0.0.1", Port: 7890}, {ID: 2, Protocol: "socks5", Host: "127.0.0.1", Port: 7891}}
	first, second := upstreamProxyTestAccount(1, nil), upstreamProxyTestAccount(2, nil)
	first.ProxyPool = []AccountProxyPoolEntry{{ProxyID: 1, Concurrency: 1, Proxy: routes[0]}, {ProxyID: 2, Concurrency: 1, Proxy: routes[1]}}
	second.ProxyPool = []AccountProxyPoolEntry{first.ProxyPool[1], first.ProxyPool[0]}
	repo := &upstreamProxyAccounts{accounts: map[int64]*Account{1: first, 2: second}}
	seen := map[string]bool{}
	for range 4 {
		value, err := resolveUpstreamAccountProxy(context.Background(), repo, []int64{1, 2}, "openai", "https://api.example.invalid/v1", "account-api-secret")
		require.NoError(t, err)
		seen[value] = true
	}
	require.Len(t, seen, 2)
	require.True(t, seen["socks5h://127.0.0.1:7891"])
	require.Nil(t, first.ProxyID)
	require.Nil(t, first.Proxy)
	require.False(t, first.ProxyPoolSelected)
	first.ProxyPool = nil
	SetAccountProxyPoolExtra(first.Extra, second.ProxyPool)
	first.Extra = map[string]any{AccountProxyPoolExtraKey: []map[string]any{{"proxy_id": 2, "concurrency": 1}}}
	_, err := resolveUpstreamAccountProxy(context.Background(), repo, []int64{1}, "openai", "https://api.example.invalid/v1", "account-api-secret")
	require.ErrorIs(t, err, ErrUpstreamProxyUnavailable, "a configured but unhydrated pool cannot mean direct")
	first.Extra[AccountProxyPoolExtraKey] = "malformed-pool"
	_, err = resolveUpstreamAccountProxy(context.Background(), repo, []int64{1}, "openai", "https://api.example.invalid/v1", "account-api-secret")
	require.ErrorIs(t, err, ErrUpstreamProxyUnavailable, "malformed legacy proxy configuration cannot mean direct")
}

func TestUpstreamProxyHTTPRoutesUnresolvableDestinationsAndRejectsRedirects(t *testing.T) {
	calls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "api.example.invalid", r.URL.Host)
		require.Equal(t, "Bearer account-api-secret", r.Header.Get("Authorization"))
		require.Equal(t, "http://api.example.invalid/v1/usage?days=1", r.RequestURI)
		w.Header().Set("Location", "http://other.example.invalid/collect")
		w.WriteHeader(http.StatusFound)
	}))
	defer proxy.Close()
	client, err := newUpstreamProxyHTTPClient(proxy.URL, time.Second)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	ctx := withUpstreamProxyClient(context.Background(), client)
	require.NoError(t, validateUpstreamRequestEndpoint(ctx, "http://api.example.invalid/v1"))
	require.Same(t, client, upstreamRequestClient(ctx, http.DefaultClient))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://api.example.invalid/v1/usage?days=1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer account-api-secret")
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, 1, calls)
	for _, endpoint := range []string{"http://127.0.0.1/v1", "http://169.254.169.254/", "http://[::1]/", "http://metadata.google.internal/", "http://LOCALHOST./", "http://127.1/", "http://2130706433/", "http://0x7f000001/", "http://0177.0.0.1/"} {
		_, err := client.Get(endpoint)
		require.Error(t, err)
	}
	require.Equal(t, 1, calls, "blocked destinations must never reach the trusted proxy")
}

func TestUpstreamProxyHTTPSConnectSeparatesCredentialsAndKeepsTLSVerification(t *testing.T) {
	const destination = "unresolvable.example.invalid"
	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: destination}, DNSNames: []string{destination},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &signer.PublicKey, signer)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	tlsCertificate := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: signer}
	type capturedRequest struct{ method, target, proxyAuth, auth string }
	for _, status := range []int{http.StatusOK, http.StatusProxyAuthRequired} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			connects := make(chan capturedRequest, 2)
			upstream := make(chan capturedRequest, 1)
			failures := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				connects <- capturedRequest{r.Method, r.Host, r.Header.Get("Proxy-Authorization"), r.Header.Get("Authorization")}
				if status != http.StatusOK {
					w.Header().Set("Proxy-Authenticate", `Basic realm="fixture"`)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "proxy-user:proxy-pass must not appear in an error")
					return
				}
				conn, buffered, err := w.(http.Hijacker).Hijack()
				if err != nil {
					failures <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				if err := buffered.Flush(); err != nil {
					failures <- err
					return
				}
				tunnel := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{tlsCertificate}, MinVersion: tls.VersionTLS12})
				if err := tunnel.Handshake(); err != nil {
					failures <- err
					return
				}
				request, err := http.ReadRequest(bufio.NewReader(tunnel))
				if err != nil {
					failures <- err
					return
				}
				upstream <- capturedRequest{request.Method, request.Host + request.URL.Path, request.Header.Get("Proxy-Authorization"), request.Header.Get("Authorization")}
				_, err = io.WriteString(tunnel, "HTTP/1.1 302 Found\r\nLocation: https://other.example.invalid/collect\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				failures <- err
			}))
			defer proxy.Close()
			route, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			route.User = url.UserPassword("proxy-user", "proxy-pass")
			client, err := newUpstreamProxyHTTPClient(route.String(), 3*time.Second)
			require.NoError(t, err)
			defer client.CloseIdleConnections()
			// Trust only this fixture's certificate. Hostname and TLS verification
			// remain enabled for the unresolvable HTTPS destination.
			client.Transport.(*upstreamProxyTransport).transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			request, err := http.NewRequest(http.MethodGet, "https://"+destination+"/v1/models", nil)
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer account-api-secret")
			response, err := client.Do(request)
			require.Len(t, connects, 1, "the configured proxy must receive the CONNECT request")
			connect := <-connects
			require.Equal(t, http.MethodConnect, connect.method)
			require.Equal(t, destination+":443", connect.target, "the proxy receives the unresolved destination name")
			require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-pass")), connect.proxyAuth)
			require.Empty(t, connect.auth, "upstream bearer credentials must not be sent in CONNECT")
			if status == http.StatusOK {
				require.NoError(t, err)
				response.Body.Close()
				require.Equal(t, http.StatusFound, response.StatusCode, "cross-origin redirects remain disabled after tunneling")
				sent := <-upstream
				require.Equal(t, http.MethodGet, sent.method)
				require.Equal(t, destination+"/v1/models", sent.target)
				require.Equal(t, "Bearer account-api-secret", sent.auth)
				require.Empty(t, sent.proxyAuth, "proxy credentials must never enter the upstream TLS request")
				require.NoError(t, <-failures)
			} else {
				require.Error(t, err)
				require.Nil(t, response)
				require.Contains(t, err.Error(), "upstream_proxy_request_failed")
				for _, secret := range []string{"proxy-user", "proxy-pass", "account-api-secret", route.Host} {
					require.NotContains(t, err.Error(), secret)
				}
				require.Empty(t, upstream, "failed CONNECT must not transmit an upstream request")
			}
			require.Empty(t, connects, "no direct fallback, auth retry or redirect is allowed")
		})
	}
}

func TestUpstreamProxySOCKSUsesProxyDNS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		reader := bufio.NewReader(conn)
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil {
			result <- err
			return
		}
		methods := make([]byte, int(header[1]))
		_, _ = io.ReadFull(reader, methods)
		_, _ = conn.Write([]byte{5, 0})
		request := make([]byte, 4)
		if _, err := io.ReadFull(reader, request); err != nil {
			result <- err
			return
		}
		if request[3] != 3 {
			result <- fmt.Errorf("SOCKS must receive a domain, got type %d", request[3])
			return
		}
		size, err := reader.ReadByte()
		if err != nil {
			result <- err
			return
		}
		domain := make([]byte, size)
		_, _ = io.ReadFull(reader, domain)
		port := make([]byte, 2)
		_, _ = io.ReadFull(reader, port)
		if string(domain) != "unresolvable.example.invalid" {
			result <- fmt.Errorf("unexpected target domain")
			return
		}
		_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
		r, err := http.ReadRequest(reader)
		if err != nil {
			result <- err
			return
		}
		if r.URL.Path != "/v1/models" {
			result <- fmt.Errorf("unexpected request path")
			return
		}
		_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}")
		result <- err
	}()
	client, err := newUpstreamProxyHTTPClient("socks5://"+listener.Addr().String(), 3*time.Second)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://unresolvable.example.invalid/v1/models")
	require.NoError(t, err)
	resp.Body.Close()
	require.NoError(t, <-result)
}

func TestUpstreamProxyErrorsDoNotExposeProxySecrets(t *testing.T) {
	client, err := newUpstreamProxyHTTPClient("invalid://secret-user:secret-pass@proxy.invalid:7890", time.Second)
	require.Nil(t, client)
	require.ErrorIs(t, err, ErrUpstreamProxyUnavailable)
	require.NotContains(t, err.Error(), "secret")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	route := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), User: url.UserPassword("secret-user", "secret-pass")}
	client, err = newUpstreamProxyHTTPClient(route.String(), time.Second)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	_, err = client.Get("http://upstream.example.invalid/v1/models")
	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream_proxy_request_failed")
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), route.Host)
	var op *net.OpError
	require.True(t, errors.As(err, &op), "preserve the network error type for safe structured classification")
	cause := errors.New("proxy-secret-user:proxy-secret-password")
	wrapped := &upstreamProxyRequestError{cause: cause}
	require.False(t, strings.Contains(wrapped.Error(), "password"))
	require.ErrorIs(t, wrapped, cause)
}
