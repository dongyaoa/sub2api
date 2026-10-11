import type { UpstreamBalanceSnapshot, UpstreamBillingSnapshot } from '@/api/admin/upstreamCenter'

export function isNewapiBilling(billing?: UpstreamBillingSnapshot | null): boolean {
  return billing?.provider === 'newapi' || Boolean(billing?.source?.startsWith('newapi_'))
}

const usageOnlyErrors = new Set(['newapi_usage_incomplete', 'newapi_usage_unavailable'])

// Usage history can be incomplete even when the current wallet balance synced successfully.
export function walletSyncError(wallet: UpstreamBalanceSnapshot, error = wallet.error): string {
  const amount = wallet.kind === 'wallet' ? wallet.balance : wallet.quota_remaining ?? wallet.balance
  const knownBalance = (typeof amount === 'number' && Number.isFinite(amount)) || (wallet.kind === 'key_quota' && wallet.unlimited_quota)
  if (!isNewapiBilling(wallet.billing) || wallet.status !== 'ok' || !knownBalance) return error || ''
  return (error || '').split(';').map(part => part.trim()).filter(part => part && !usageOnlyErrors.has(part)).join('; ')
}

export function walletUsageError(wallet?: UpstreamBalanceSnapshot): string {
  if (!wallet || !isNewapiBilling(wallet.billing)) return ''
  const errors = [wallet.error, wallet.billing?.error].flatMap(error => (error || '').split(';').map(part => part.trim()))
  return [...new Set(errors.filter(error => usageOnlyErrors.has(error)))].join('; ')
}

const errorKeys: Record<string, string> = {
  newapi_account_auth_required: 'authorizationRequired',
  newapi_account_auth_failed: 'authorizationFailed',
  newapi_account_identity_mismatch: 'identityMismatch',
  newapi_token_not_found: 'tokenNotFound',
  newapi_token_ambiguous: 'tokenAmbiguous',
  newapi_auto_group: 'autoGroup',
  newapi_group_rate_unavailable: 'rateUnavailable',
  newapi_quota_unit_unknown: 'quotaUnitUnknown',
  newapi_token_lookup_unsupported: 'lookupUnsupported',
  newapi_request_failed: 'requestFailed',
  newapi_response_unsupported: 'responseUnsupported',
  newapi_rate_limited: 'rateLimited',
  newapi_usage_account_auth_required: 'usageAuthorizationRequired',
  newapi_usage_incomplete: 'usageIncomplete',
  newapi_usage_unavailable: 'usageUnavailable',
}

export function upstreamSyncError(error: string | null | undefined, translate: (key: string) => string): string {
  return (error || '').replace(/\bnewapi_[a-z_0-9]+\b/g, code => {
    if (errorKeys[code]) return translate(`upstreamCenter.newapi.errors.${errorKeys[code]}`)
    const status = code.match(/^newapi_upstream_http_(\d{3})$/)?.[1]
    return status ? `${translate('upstreamCenter.newapi.errors.httpError')} (HTTP ${status})` : code
  })
}
