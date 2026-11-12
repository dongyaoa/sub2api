package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Reuse the active binding rather than copying proxy credentials into the
// upstream target. Changes in account management therefore take effect on the
// next probe, including groups created before proxy inheritance was added.
func (s *UpstreamCenterService) targetProxyAccountIDs(ctx context.Context, target *UpstreamTarget, key string) ([]int64, error) {
	if target.SourceTargetID == nil {
		return target.AccountIDs, nil
	}
	source, err := s.repo.GetTarget(ctx, *target.SourceTargetID)
	if err != nil || source == nil || source.SupplierID == nil || source.SourceTargetID != nil {
		return nil, ErrUpstreamProxyUnavailable
	}
	if _, err := s.repo.GetSupplier(ctx, *source.SupplierID); err != nil {
		return nil, ErrUpstreamProxyUnavailable
	}
	if source.Provider != target.Provider || normalizeEndpoint(source.Endpoint) != normalizeEndpoint(target.Endpoint) {
		return nil, ErrUpstreamProxyAccountMismatch
	}
	sourceKey, err := s.encryptor.Decrypt(source.APIKeyEncrypted)
	if err != nil || strings.TrimSpace(sourceKey) != strings.TrimSpace(key) {
		return nil, ErrUpstreamProxyAccountMismatch
	}
	return source.AccountIDs, nil
}

func (s *UpstreamCenterService) targetRequestRoute(ctx context.Context, target *UpstreamTarget, key string, timeout time.Duration) (context.Context, *http.Client, error) {
	ids, err := s.targetProxyAccountIDs(ctx, target, key)
	if err != nil {
		return ctx, nil, err
	}
	proxyURL, err := resolveUpstreamAccountProxy(ctx, s.accounts, ids, target.Provider, target.Endpoint, key)
	if err != nil {
		return ctx, nil, err
	}
	if proxyURL == "" {
		return ctx, nil, nil
	}
	client, err := newUpstreamProxyHTTPClient(proxyURL, timeout)
	if err != nil {
		return ctx, nil, err
	}
	return withUpstreamProxyClient(ctx, client), client, nil
}
