package service

import "strings"

func isNewAPIFinanceTarget(target *UpstreamFinanceTarget, balance *UpstreamBalanceSnapshot) bool {
	if target != nil && target.NewAPIUserID > 0 && target.NewAPIAccessTokenEncrypted != "" {
		return true
	}
	return balance != nil && (balance.CurrencySource == "newapi_status" ||
		balance.Billing != nil && strings.HasPrefix(balance.Billing.Source, "newapi_"))
}
