package service

import (
	"context"
	"errors"
	"time"
)

// Implemented by the persistent repository. Legacy test doubles without this
// optional projection keep their explicit transport, never query an unrelated
// account or infer a proxy from endpoint similarity.
type UpstreamProxyAccountsRepository interface {
	ProxyAccountIDs(context.Context, int64) ([]int64, error)
}

func upstreamProxyErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUpstreamProxyConflict):
		return "upstream_proxy_conflict"
	case errors.Is(err, ErrUpstreamProxyAccountMismatch):
		return "upstream_proxy_account_mismatch"
	default:
		return "upstream_proxy_unavailable"
	}
}

// A synchronization resolves the current bound account route once. All of its
// usage, wallet and billing reads share that route without changing s.client.
func (s *UpstreamFinanceService) fetchBalanceWithAccountProxy(ctx context.Context, target *UpstreamFinanceTarget) *UpstreamBalanceSnapshot {
	fail := func(err error) *UpstreamBalanceSnapshot {
		now := s.now().UTC()
		code := upstreamProxyErrorCode(err)
		return &UpstreamBalanceSnapshot{TargetID: target.ID, WalletRef: target.WalletRef,
			Kind: "unknown", Status: "error", SyncedAt: &now, Error: code,
			Billing: &UpstreamRemoteBillingSnapshot{Source: "unknown", Status: "error", Stale: true, Error: code, LastAttemptAt: &now}}
	}
	if routes, ok := s.repo.(UpstreamProxyAccountsRepository); ok {
		ids, err := routes.ProxyAccountIDs(ctx, target.ID)
		if err != nil {
			return fail(err)
		}
		if len(ids) > 0 {
			if s.encryptor == nil {
				return fail(ErrUpstreamProxyUnavailable)
			}
			key, err := s.encryptor.Decrypt(target.APIKeyEncrypted)
			if err != nil {
				return fail(ErrUpstreamProxyUnavailable)
			}
			proxyURL, err := resolveUpstreamAccountProxy(ctx, s.accounts, ids, target.Provider, target.Endpoint, key)
			if err != nil {
				return fail(err)
			}
			if proxyURL != "" {
				client, err := newUpstreamProxyHTTPClient(proxyURL, 30*time.Second)
				if err != nil {
					return fail(err)
				}
				defer client.CloseIdleConnections()
				ctx = withUpstreamProxyClient(ctx, client)
			}
		}
	}
	snapshot := s.fetchBalance(ctx, target)
	if snapshot.Billing == nil {
		snapshot.Billing = s.fetchRemoteBilling(ctx, target)
	}
	return snapshot
}
