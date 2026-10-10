package service

import (
	"fmt"
	"strings"
)

// WalletRef identifies a shared supplier wallet, not a key's ability to read
// it. Failed keys retain their own observations on target.Balance; they do not
// create another supplier wallet beside a known balance for the same reference.
func upstreamSupplierWallets(targets []*UpstreamTarget) []*UpstreamBalanceSnapshot {
	knownWallets := make(map[string]bool)
	for _, target := range targets {
		if target != nil && target.Balance != nil && target.Balance.Kind == "wallet" && target.Balance.Balance != nil {
			knownWallets[upstreamWalletScope(target)] = true
		}
	}
	type walletIdentity struct {
		ref      string
		currency string
		targetID int64
		kind     string
		account  string
	}
	positions := make(map[walletIdentity]int)
	wallets := make([]*UpstreamBalanceSnapshot, 0)
	for _, target := range targets {
		if target == nil || target.Balance == nil {
			continue
		}
		observation := target.Balance
		if target.NewAPIUserID > 0 && observation.Kind == "key_quota" {
			if knownWallets[upstreamWalletScope(target)] {
				continue
			}
			// Key allowances remain available on the group. The site sidebar
			// has one account-wallet placeholder until a wallet can be verified.
			copy := *observation
			copy.Kind, copy.QuotaRemaining, copy.Balance = "unknown", nil, nil
			copy.UnlimitedQuota = false
			observation = &copy
		}
		ref := upstreamWalletRef(target)
		identity := walletIdentity{ref: ref, account: upstreamWalletScope(target)}
		switch observation.Kind {
		case "key_quota", "subscription":
			// Explicit per-key allowances are never a shared wallet, even if the
			// operator has left the wallet label at its default value.
			identity.kind, identity.targetID = observation.Kind, target.ID
		case "wallet":
			identity.kind = "wallet"
			if target.NewAPIUserID <= 0 {
				identity.currency = strings.ToUpper(strings.TrimSpace(observation.Currency))
			}
		default:
			if knownWallets[upstreamWalletScope(target)] {
				continue
			}
			// Without any confirmed balance, preserve an honest unknown/error
			// placeholder for the shared reference. No amount is synthesized.
			identity.kind = "unresolved"
		}
		if pos, exists := positions[identity]; exists {
			if target.NewAPIUserID > 0 {
				wallets[pos] = mergeUpstreamNewAPIWalletBalances(wallets[pos], observation)
			} else {
				wallets[pos] = mergeUpstreamWalletBalances(wallets[pos], observation)
			}
			wallets[pos].WalletRef = ref
		} else {
			positions[identity] = len(wallets)
			copy := *observation
			copy.WalletRef = ref
			wallets = append(wallets, &copy)
		}
	}
	return wallets
}

// New API identifies the actual account wallet, independently of key names.
// Never merge two users or two endpoint deployments sharing wallet_ref=default.
func upstreamWalletScope(target *UpstreamTarget) string {
	ref := upstreamWalletRef(target)
	if target.NewAPIUserID <= 0 {
		return ref
	}
	base := upstreamNewAPIBase(target.Endpoint)
	if base == "" {
		return fmt.Sprintf("invalid-newapi:%d", target.ID)
	}
	return fmt.Sprintf("newapi:%s:%d", base, target.NewAPIUserID)
}

func upstreamWalletRef(target *UpstreamTarget) string {
	if target.NewAPIUserID > 0 {
		return "default"
	}
	if ref := strings.TrimSpace(target.WalletRef); ref != "" {
		return ref
	}
	return "default"
}

func mergeUpstreamWalletBalances(current, candidate *UpstreamBalanceSnapshot) *UpstreamBalanceSnapshot {
	selected := current
	if preferUpstreamWalletObservation(candidate, current) {
		selected = candidate
	}
	// Keep amount, timestamp, source key and status together. Combining one
	// key's successful balance with another key's 401 creates a false failure.
	result := *selected
	return &result
}

func mergeUpstreamNewAPIWalletBalances(current, candidate *UpstreamBalanceSnapshot) *UpstreamBalanceSnapshot {
	confirmed := func(snapshot *UpstreamBalanceSnapshot) bool {
		return snapshot.Balance != nil && snapshot.Currency == "USD" && snapshot.CurrencySource == "newapi_status"
	}
	// /api/status may fail for one group while another group has already
	// confirmed quota_per_unit. Prefer that complete monetary observation over
	// a newer raw QUOTA amount for the same account; never mix their fields.
	if confirmed(current) != confirmed(candidate) {
		selected := current
		if confirmed(candidate) {
			selected = candidate
		}
		copy := *selected
		return &copy
	}
	return mergeUpstreamWalletBalances(current, candidate)
}

func preferUpstreamWalletObservation(candidate, current *UpstreamBalanceSnapshot) bool {
	if (candidate.Balance != nil) != (current.Balance != nil) {
		return candidate.Balance != nil
	}
	if candidate.Balance != nil {
		if (candidate.SyncedAt != nil) != (current.SyncedAt != nil) {
			return candidate.SyncedAt != nil
		}
		if candidate.SyncedAt != nil && !candidate.SyncedAt.Equal(*current.SyncedAt) {
			return candidate.SyncedAt.After(*current.SyncedAt)
		}
		if (candidate.Status == "ok") != (current.Status == "ok") {
			return candidate.Status == "ok"
		}
	}
	if (candidate.LastAttemptAt != nil) != (current.LastAttemptAt != nil) {
		return candidate.LastAttemptAt != nil
	}
	if candidate.LastAttemptAt != nil && !candidate.LastAttemptAt.Equal(*current.LastAttemptAt) {
		return candidate.LastAttemptAt.After(*current.LastAttemptAt)
	}
	return candidate.TargetID < current.TargetID
}
