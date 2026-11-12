import type { UpstreamBalanceSnapshot, UpstreamBillingSnapshot, UpstreamTarget } from '@/api/admin/upstreamCenter'

export function isNewapiBilling(billing?: UpstreamBillingSnapshot | null): boolean {
  return billing?.provider === 'newapi' || Boolean(billing?.source?.startsWith('newapi_'))
}

export function isNewapiTarget(target: UpstreamTarget): boolean {
  return isNewapiBilling(target.balance?.billing) || target.balance?.currency_source === 'newapi_status' ||
    Boolean(target.newapi_user_id && target.newapi_access_token_configured)
}

// Compare timestamps from the server so a browser clock skew cannot turn a
// successful wallet refresh into a false report of fresh consumption data.
export function hasFreshNewapiDailyUsage(snapshot: UpstreamBalanceSnapshot): boolean {
  const attempt = Date.parse(snapshot.last_attempt_at || '')
  const synced = Date.parse(snapshot.day_synced_at || '')
  const start = Date.parse(snapshot.day_start || '')
  const end = Date.parse(snapshot.day_end || '')
  return snapshot.status === 'ok' && snapshot.currency === 'USD' &&
    typeof snapshot.day_used === 'number' && Number.isFinite(snapshot.day_used) && snapshot.day_used >= 0 &&
    [attempt, synced, start, end].every(Number.isFinite) && start <= attempt && attempt < end &&
    synced >= attempt - 2000 && synced <= attempt + 60000
}

const usageOnlyErrors = new Set(['newapi_usage_incomplete', 'newapi_usage_unavailable'])
const syncErrorParts = (error: string | null | undefined) => (error || '').split(';').map(part => part.trim()).filter(Boolean)
const isUsageOnlyError = (error: string) => usageOnlyErrors.has(error)

// Today's usage can be incomplete even when the current wallet balance synced successfully.
export function walletSyncError(wallet: UpstreamBalanceSnapshot, error = wallet.error): string {
  const amount = wallet.kind === 'wallet' ? wallet.balance : wallet.quota_remaining ?? wallet.balance
  const knownBalance = (typeof amount === 'number' && Number.isFinite(amount)) || (wallet.kind === 'key_quota' && wallet.unlimited_quota)
  if (wallet.status !== 'ok' || !knownBalance) return error || ''
  return syncErrorParts(error).filter(part => !isUsageOnlyError(part)).join('; ')
}

export function walletUsageError(wallet?: UpstreamBalanceSnapshot): string {
  if (!wallet) return ''
  const errors = [wallet.error, wallet.billing?.error].flatMap(syncErrorParts)
  return [...new Set(errors.filter(isUsageOnlyError))].join('; ')
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

const syncErrorKeys: Record<string, string> = {
  upstream_proxy_unavailable: 'proxyUnavailable',
  upstream_proxy_conflict: 'proxyConflict',
  upstream_proxy_account_mismatch: 'proxyAccountMismatch',
  upstream_proxy_request_failed: 'proxyRequestFailed',
  upstream_timeout: 'timeout',
  upstream_dns_failed: 'dnsFailed',
  upstream_tls_failed: 'tlsFailed',
  upstream_connection_failed: 'connectionFailed',
  upstream_request_cancelled: 'cancelled',
  upstream_request_failed: 'requestFailed',
  response_read_failed: 'responseReadFailed',
  response_too_large: 'responseTooLarge',
  usage_endpoint_unsupported: 'usageUnsupported',
  usage_response_unsupported: 'usageResponseUnsupported',
  billing_endpoint_unsupported: 'billingUnsupported',
  billing_response_unsupported: 'billingResponseUnsupported',
  invalid_endpoint: 'invalidEndpoint',
  endpoint_unavailable: 'endpointUnavailable',
  credential_unavailable: 'credentialUnavailable',
  request_invalid: 'requestInvalid',
}

export function upstreamSyncError(error: string | null | undefined, translate: (key: string) => string): string {
  return (error || '').replace(/\b(?:newapi_|upstream_|response_|usage_|billing_|endpoint_|credential_|request_|invalid_)[a-z_0-9]+\b/g, code => {
    if (errorKeys[code]) return translate(`upstreamCenter.newapi.errors.${errorKeys[code]}`)
    if (syncErrorKeys[code]) return translate(`upstreamCenter.syncErrors.${syncErrorKeys[code]}`)
    const status = code.match(/^newapi_upstream_http_(\d{3})$/)?.[1]
    if (status) return `${translate('upstreamCenter.newapi.errors.httpError')} (HTTP ${status})`
    const upstreamStatus = code.match(/^upstream_http_(\d{3})$/)?.[1]
    return upstreamStatus ? `${translate('upstreamCenter.syncErrors.httpError')} (HTTP ${upstreamStatus})` : code
  })
}
