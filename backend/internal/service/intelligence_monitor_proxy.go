package service

import (
	"context"
	"net/http"
	"time"
)

// Queued runs snapshot inference credentials, not proxy secrets. Resolve the
// still-matching group now so account proxy changes apply to artwork and candy
// just as they apply to availability and finance checks.
func (s *IntelligenceMonitorService) upstreamGenerationRoute(ctx context.Context, run *IntelligenceMonitorRun, key string) (context.Context, *http.Client, error) {
	id := intelligenceSnapshotID(run.SourceSnapshot["upstream_target_id"])
	if id <= 0 || s.upstreams == nil || s.encryptor == nil {
		return ctx, nil, ErrUpstreamProxyUnavailable
	}
	target, err := s.upstreams.GetTarget(ctx, id)
	if err != nil || target == nil {
		return ctx, nil, ErrUpstreamProxyUnavailable
	}
	currentKey, err := s.encryptor.Decrypt(target.APIKeyEncrypted)
	if err != nil || currentKey != key || target.Endpoint != run.SourceEndpoint || target.Provider != MonitorProviderOpenAI {
		return ctx, nil, ErrUpstreamProxyAccountMismatch
	}
	accounts := s.accounts
	if accounts == nil && s.finance != nil {
		accounts = s.finance.accounts
	}
	timeout := time.Duration(run.TimeoutSeconds) * time.Second
	if timeout <= 0 || run.TimeoutSeconds > IntelligenceMonitorMaxTimeoutSeconds {
		timeout = time.Duration(IntelligenceMonitorMaxTimeoutSeconds) * time.Second
	}
	routes := &UpstreamCenterService{repo: s.upstreams, accounts: accounts, encryptor: s.encryptor}
	return routes.targetRequestRoute(ctx, target, key, timeout)
}
