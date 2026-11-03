package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceHTTPClassifiesErrorsWithoutLeakingDetails(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		err        error
		retry      bool
	}{
		{"cancelled", "upstream_request_cancelled", context.Canceled, false},
		{"deadline", "upstream_timeout", context.DeadlineExceeded, true},
		{"dns temporary", "upstream_dns_failed", &net.DNSError{Err: "private-host", IsTemporary: true}, true},
		{"dns not found", "upstream_dns_failed", &net.DNSError{Err: "private-host", IsNotFound: true}, false},
		{"untrusted certificate", "upstream_tls_failed", x509.UnknownAuthorityError{}, false},
		{"certificate verification", "upstream_tls_failed", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, false},
		{"tls alert", "upstream_tls_failed", tls.AlertError(40), false},
		{"connection reset", "upstream_connection_failed", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, true},
		{"connection refused", "upstream_connection_failed", syscall.ECONNREFUSED, true},
		{"eof", "upstream_connection_failed", io.EOF, true},
		{"blocked address", "upstream_connection_failed", &net.AddrError{Err: "blocked by SSRF policy", Addr: "private-host"}, false},
		{"other", "upstream_request_failed", errors.New("credential-secret private-host"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &url.Error{Op: "Get", URL: "https://private-host/usage?key=credential-secret", Err: tc.err}
			code, retry := upstreamFinanceRequestError(err)
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.retry, retry)
			require.NotContains(t, code, "credential-secret")
			require.NotContains(t, code, "private-host")
		})
	}
}

func TestUpstreamFinanceHTTPRetriesTransientGETAndKeepsRequestContract(t *testing.T) {
	svc := NewUpstreamFinanceService(nil, nil, nil, nil, nil)
	calls := 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "Bearer credential-secret", req.Header.Get("Authorization"))
		require.Equal(t, "application/json", req.Header.Get("Accept"))
		require.Equal(t, "1", req.URL.Query().Get("days"))
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		require.Positive(t, time.Until(deadline))
		require.LessOrEqual(t, time.Until(deadline), time.Second)
		if calls == 1 {
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"balance":2}`)), Header: http.Header{}, Request: req}, nil
	})
	body, code := svc.fetchUpstreamFinanceJSON(context.Background(), "https://example.com/v1/usage?days=1", "credential-secret", 100, time.Second, 2)
	require.Empty(t, code)
	require.JSONEq(t, `{"balance":2}`, string(body))
	require.Equal(t, 2, calls)
}

func TestUpstreamFinanceHTTPStatusRetryIsBounded(t *testing.T) {
	for _, status := range []int{401, 403, 404, 405, 429, 500, 502, 503, 504, 524, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			svc := NewUpstreamFinanceService(nil, nil, nil, nil, nil)
			calls := 0
			svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("credential-secret")), Header: http.Header{"Location": {"https://other.example/secret"}}, Request: req}, nil
			})
			body, code := svc.fetchUpstreamFinanceJSON(context.Background(), "https://example.com/v1/usage", "credential-secret", 100, time.Second, 9)
			require.Nil(t, body)
			require.True(t, strings.HasPrefix(code, "upstream_http_"))
			require.NotContains(t, code, "credential-secret")
			wantCalls := 1
			if status == 502 || status == 503 || status == 504 || status == 524 {
				wantCalls = 2
			}
			require.Equal(t, wantCalls, calls)
		})
	}
}

type financeHTTPErrorReader struct{ err error }

func (r financeHTTPErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestUpstreamFinanceHTTPBodyFailures(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		reader     io.Reader
		calls      int
	}{
		{"oversized", "response_too_large", strings.NewReader("123456"), 1},
		{"truncated", "response_read_failed", financeHTTPErrorReader{io.ErrUnexpectedEOF}, 2},
		{"unclassified", "response_read_failed", financeHTTPErrorReader{errors.New("credential-secret")}, 1},
		{"timeout", "upstream_timeout", financeHTTPErrorReader{context.DeadlineExceeded}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewUpstreamFinanceService(nil, nil, nil, nil, nil)
			calls := 0
			svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(tc.reader), Header: http.Header{}, Request: req}, nil
			})
			body, code := svc.fetchUpstreamFinanceJSON(context.Background(), "https://example.com/v1/usage", "secret", 5, time.Second, 2)
			require.Nil(t, body)
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.calls, calls)
		})
	}
}

func TestUpstreamFinanceHTTPParentCancellationStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := NewUpstreamFinanceService(nil, nil, nil, nil, nil)
	calls := 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, io.EOF
	})
	_, code := svc.fetchUpstreamFinanceJSON(ctx, "https://example.com/v1/usage", "secret", 100, time.Second, 2)
	require.Equal(t, "upstream_request_cancelled", code)
	require.Equal(t, 1, calls)
}

func TestUpstreamFinanceHTTPParentDeadlineBoundsAllAttempts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	svc := NewUpstreamFinanceService(nil, nil, nil, nil, nil)
	calls := 0
	svc.client.Transport = financeRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	_, code := svc.fetchUpstreamFinanceJSON(ctx, "https://example.com/v1/usage", "secret", 100, time.Second, 2)
	require.Equal(t, "upstream_timeout", code)
	require.Equal(t, 1, calls)
}
