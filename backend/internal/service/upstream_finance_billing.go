package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const upstreamRemoteBillingFreshness = 150 * time.Second

func pendingUpstreamRemoteBilling() *UpstreamRemoteBillingSnapshot {
	return &UpstreamRemoteBillingSnapshot{Status: "pending", Source: "unknown", BillingScope: "token", Stale: true}
}

// LatestRemoteBilling only reads a stored observation. Page refreshes never wait
// on an upstream HTTP request.
func (s *UpstreamFinanceService) LatestRemoteBilling(ctx context.Context, targetID int64) (*UpstreamRemoteBillingSnapshot, error) {
	snapshot, err := s.LatestBalance(ctx, targetID)
	if err != nil {
		return nil, err
	}
	return snapshot.Billing, nil
}

// SyncRemoteBilling shares the balance lease and credentials CAS. Concurrent
// callers must not silently turn an old observation into a fresh declaration.
func (s *UpstreamFinanceService) SyncRemoteBilling(ctx context.Context, targetID int64) (*UpstreamRemoteBillingSnapshot, error) {
	snapshot, err := s.SyncBalance(ctx, targetID)
	if err != nil {
		return nil, err
	}
	return snapshot.Billing, nil
}

func markUpstreamRemoteBillingStale(snapshot *UpstreamRemoteBillingSnapshot, now time.Time) {
	if snapshot == nil {
		return
	}
	snapshot.Stale = snapshot.Status != "ok" || snapshot.SyncedAt == nil || now.Sub(*snapshot.SyncedAt) > upstreamRemoteBillingFreshness
	if snapshot.ObservedAt != nil && now.Sub(*snapshot.ObservedAt) > upstreamRemoteBillingFreshness {
		snapshot.Stale = true
	}
}

func upstreamRemoteBillingURL(endpoint string) (string, error) {
	usageURL, err := upstreamUsageURL(endpoint)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(usageURL, "/v1/usage") + "/v1/sub2api/billing", nil
}

func (s *UpstreamFinanceService) fetchRemoteBilling(ctx context.Context, target *UpstreamFinanceTarget) *UpstreamRemoteBillingSnapshot {
	now := s.now().UTC()
	snapshot := &UpstreamRemoteBillingSnapshot{Status: "error", Source: "unknown", BillingScope: "token", Stale: true, LastAttemptAt: &now}
	fail := func(code string) *UpstreamRemoteBillingSnapshot { snapshot.Error = code; return snapshot }
	requestURL, err := upstreamRemoteBillingURL(target.Endpoint)
	if err != nil {
		return fail("invalid_endpoint")
	}
	if err = validateUpstreamEndpoint(target.Endpoint); err != nil {
		return fail("endpoint_unavailable")
	}
	if s.encryptor == nil {
		return fail("credential_unavailable")
	}
	key, err := s.encryptor.Decrypt(target.APIKeyEncrypted)
	if err != nil || strings.TrimSpace(key) == "" {
		return fail("credential_unavailable")
	}
	reqCtx, cancel := context.WithTimeout(ctx, upstreamBalanceTimeout)
	defer cancel()
	snapshot.Source = "sub2api_billing"
	body, code := s.fetchUpstreamFinanceJSON(reqCtx, requestURL, key, upstreamBillingProbeMaxBodyBytes, upstreamBalanceTimeout, 2)
	if code == "upstream_http_404" || code == "upstream_http_405" {
		snapshot.Status = "unsupported"
		return fail("billing_endpoint_unsupported")
	}
	if code != "" {
		return fail(code)
	}
	parsed, err := parseUpstreamRemoteBilling(body, "sub2api_billing")
	if err != nil {
		snapshot.Status = "unsupported"
		return fail("billing_response_unsupported")
	}
	completedAt := s.now().UTC()
	parsed.SyncedAt = &completedAt
	parsed.LastAttemptAt = &completedAt
	markUpstreamRemoteBillingStale(parsed, completedAt)
	return parsed
}

// The already established /v1/sub2api/billing v1 schema defines these rates.
// Reuse its validation (including personal overrides and peak consistency) so
// arbitrary usage/price fields can never be mistaken for a key multiplier.
func parseUpstreamRemoteBilling(body []byte, source string) (*UpstreamRemoteBillingSnapshot, error) {
	data, err := parseUpstreamBillingProbeResponse(body)
	if err != nil {
		return nil, err
	}
	var fields struct {
		GroupID   *int64  `json:"group_id"`
		GroupName *string `json:"group_name"`
	}
	if err = json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields.GroupID != nil && *fields.GroupID <= 0 {
		fields.GroupID = nil
	}
	if fields.GroupName != nil {
		trimmed := strings.TrimSpace(*fields.GroupName)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > 100 {
			fields.GroupName = nil
		} else {
			fields.GroupName = &trimmed
		}
	}
	// parseUpstreamBillingProbeResponse validates and normalizes these fields.
	groupRate, _ := data["group_rate_multiplier"].(float64)
	resolvedRate, _ := data["resolved_rate_multiplier"].(float64)
	effectiveRate, _ := data["effective_rate_multiplier"].(float64)
	observedAt, _ := data["observed_at"].(string)
	observed, _ := time.Parse(time.RFC3339Nano, observedAt)
	snapshot := &UpstreamRemoteBillingSnapshot{
		GroupID: fields.GroupID, GroupName: fields.GroupName, GroupRateMultiplier: &groupRate,
		ResolvedRateMultiplier: &resolvedRate, EffectiveRateMultiplier: &effectiveRate,
		BillingScope: "token", Source: source, Status: "ok", ObservedAt: &observed,
	}
	if value, ok := data["user_rate_multiplier"].(float64); ok {
		snapshot.UserRateMultiplier = &value
	}
	return snapshot, nil
}

// Some compatible deployments embed the same declared schema as usage.billing.
// The current project's /v1/usage does not. Missing/incomplete metadata triggers
// the dedicated billing endpoint; do not guess rates from cost/actual_cost.
func parseUpstreamBillingFromUsage(body []byte) *UpstreamRemoteBillingSnapshot {
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	if raw, exists := payload["data"]; exists {
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil {
			payload = nested
		}
	}
	raw, exists := payload["billing"]
	if !exists {
		return nil
	}
	snapshot, err := parseUpstreamRemoteBilling(raw, "sub2api_usage")
	if err != nil {
		return nil
	}
	return snapshot
}
