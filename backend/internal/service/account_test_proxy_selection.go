package service

import (
	"fmt"
	"time"
)

// Selection changes only the caller's account snapshot. Provider retries reuse
// that selection and cannot change the account's durable primary proxy/pool.
func selectAccountTestProxy(account *Account, requestedID *int64) error {
	if requestedID != nil && *requestedID <= 0 {
		return fmt.Errorf("proxy_id must be a positive account-bound proxy ID")
	}
	now := time.Now()
	usable := func(proxy *Proxy, id int64) bool {
		return proxy != nil && proxy.ID == id && (proxy.Status == "" || proxy.IsActive()) && !proxy.IsExpired(now)
	}
	hasPool := len(account.ProxyPool) > 0 || len(AccountProxyPoolFromExtra(account.Extra)) > 0
	if requestedID != nil {
		if hasPool {
			for _, entry := range account.ProxyPool {
				if entry.ProxyID == *requestedID {
					if entry.Concurrency <= 0 || !usable(entry.Proxy, entry.ProxyID) {
						return fmt.Errorf("selected proxy is unavailable for this account")
					}
					id := entry.ProxyID
					account.ProxyID, account.Proxy = &id, entry.Proxy
					account.ProxyPoolSelected = true
					return nil
				}
			}
		} else if account.ProxyID != nil && *account.ProxyID == *requestedID {
			if !usable(account.Proxy, *requestedID) {
				return fmt.Errorf("selected proxy is unavailable for this account")
			}
			account.ProxyPoolSelected = true
			return nil
		}
		return fmt.Errorf("selected proxy is not bound to this account")
	}
	if hasPool {
		pool := account.ProxyPool
		available := make([]AccountProxyPoolEntry, 0, len(pool))
		for _, entry := range pool {
			if entry.Concurrency > 0 && usable(entry.Proxy, entry.ProxyID) {
				available = append(available, entry)
			}
		}
		if len(available) == 0 {
			return fmt.Errorf("no usable proxy is available for this account")
		}
		account.ProxyPool = available
		SelectAccountProxy(account)
		account.ProxyPool = pool
	} else if account.ProxyID != nil && !usable(account.Proxy, *account.ProxyID) {
		return fmt.Errorf("no usable proxy is available for this account")
	}
	return nil
}
