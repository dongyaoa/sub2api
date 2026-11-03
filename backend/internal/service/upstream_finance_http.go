package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

const upstreamFinanceRetryDelay = 200 * time.Millisecond

// fetchUpstreamFinanceJSON retries only transient failures of an idempotent
// metadata read. Its parent context remains the total budget across attempts.
// Error codes never include the URL, credentials, response body or raw error.
func (s *UpstreamFinanceService) fetchUpstreamFinanceJSON(ctx context.Context, requestURL, key string, maxBody int64, attemptTimeout time.Duration, maxAttempts int) ([]byte, string) {
	if maxBody <= 0 || attemptTimeout <= 0 {
		return nil, "request_invalid"
	}
	maxAttempts = max(1, min(maxAttempts, 2))
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			code, _ := upstreamFinanceRequestError(err)
			return nil, code
		}
		body, code, retry := s.fetchUpstreamFinanceAttempt(ctx, requestURL, key, maxBody, attemptTimeout)
		if code == "" || !retry || attempt+1 == maxAttempts {
			return body, code
		}
		timer := time.NewTimer(upstreamFinanceRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			code, _ := upstreamFinanceRequestError(ctx.Err())
			return nil, code
		case <-timer.C:
		}
	}
	return nil, "upstream_request_failed"
}

func (s *UpstreamFinanceService) fetchUpstreamFinanceAttempt(ctx context.Context, requestURL, key string, maxBody int64, attemptTimeout time.Duration) ([]byte, string, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "request_invalid", false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		code, retry := upstreamFinanceRequestError(err)
		return nil, code, retry
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		retry := resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode == 524
		return nil, fmt.Sprintf("upstream_http_%d", resp.StatusCode), retry
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if int64(len(body)) > maxBody {
		return nil, "response_too_large", false
	}
	if err != nil {
		code, retry := upstreamFinanceRequestError(err)
		if code != "upstream_timeout" && code != "upstream_request_cancelled" {
			code = "response_read_failed"
		}
		return nil, code, retry
	}
	return body, "", false
}

func upstreamFinanceRequestError(err error) (string, bool) {
	if errors.Is(err, context.Canceled) {
		return "upstream_request_cancelled", false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout", true
	}
	var verify *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var certificate x509.CertificateInvalidError
	var roots x509.SystemRootsError
	var record tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &verify) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &certificate) || errors.As(err, &roots) || errors.As(err, &record) || errors.As(err, &alert) {
		return "upstream_tls_failed", false
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "upstream_dns_failed", dns.IsTimeout || dns.IsTemporary
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "upstream_timeout", true
	}
	for _, transient := range []error{io.EOF, io.ErrUnexpectedEOF, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.ECONNREFUSED, syscall.EPIPE, syscall.ENETUNREACH, syscall.EHOSTUNREACH} {
		if errors.Is(err, transient) {
			return "upstream_connection_failed", true
		}
	}
	var address *net.AddrError
	if errors.As(err, &address) {
		return "upstream_connection_failed", false
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		return "upstream_connection_failed", operation.Temporary()
	}
	return "upstream_request_failed", false
}
