import { describe, expect, it } from 'vitest'
import type { UpstreamBalanceSnapshot, UpstreamBillingSnapshot, UpstreamTarget } from '@/api/admin/upstreamCenter'
import { hasFreshNewapiDailyUsage, isNewapiBilling, isNewapiTarget, upstreamSyncError, walletSyncError, walletUsageError } from './newapi'

describe('New API sync display', () => {
  it('recognizes only an identified New API snapshot', () => {
    expect(isNewapiBilling()).toBe(false)
    expect(isNewapiBilling({ source: 'sub2api_billing' } as UpstreamBillingSnapshot)).toBe(false)
    expect(isNewapiBilling({ source: 'newapi_account' } as UpstreamBillingSnapshot)).toBe(true)
    expect(isNewapiBilling({ provider: 'newapi' } as UpstreamBillingSnapshot)).toBe(true)
    expect(isNewapiTarget({ newapi_user_id: 9, newapi_access_token_configured: true } as UpstreamTarget)).toBe(true)
    expect(isNewapiTarget({ newapi_user_id: 9 } as UpstreamTarget)).toBe(false)
    expect(isNewapiTarget({ balance: { billing: { source: 'newapi_token' } } } as UpstreamTarget)).toBe(true)
  })
  it('requires a complete fresh daily cost, independently of wallet success and browser time', () => {
    const snapshot = { status: 'ok', currency: 'USD', day_used: 0, day_start: '2026-11-03T00:00:00Z', day_end: '2026-11-04T00:00:00Z', day_synced_at: '2026-11-03T16:00:00Z', last_attempt_at: '2026-11-03T16:00:00.950Z' } as UpstreamBalanceSnapshot
    expect(hasFreshNewapiDailyUsage(snapshot)).toBe(true)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, day_synced_at: '2026-11-03T02:00:00Z' })).toBe(false)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, day_used: null })).toBe(false)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, day_used: NaN })).toBe(false)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, currency: 'QUOTA' })).toBe(false)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, day_end: '2026-11-03T00:00:00Z' })).toBe(false)
    expect(hasFreshNewapiDailyUsage({ ...snapshot, last_attempt_at: null })).toBe(false)
  })
  it('maps known errors and HTTP status while preserving unfamiliar diagnostic details', () => {
    const t = (key: string) => key
    expect(upstreamSyncError(null, t)).toBe('')
    expect(upstreamSyncError('newapi_account_auth_failed; newapi_upstream_http_403', t)).toBe('upstreamCenter.newapi.errors.authorizationFailed; upstreamCenter.newapi.errors.httpError (HTTP 403)')
    expect(upstreamSyncError('newapi_auto_group', t)).toBe('upstreamCenter.newapi.errors.autoGroup')
    expect(upstreamSyncError('newapi_rate_limited', t)).toBe('upstreamCenter.newapi.errors.rateLimited')
    expect(upstreamSyncError('Upstream connection timed out.', t)).toBe('Upstream connection timed out.')
  })
  it.each([
    ['newapi_usage_account_auth_required', 'usageAuthorizationRequired'],
    ['newapi_usage_incomplete', 'usageIncomplete'],
    ['newapi_usage_unavailable', 'usageUnavailable'],
  ])('explains %s instead of exposing the diagnostic code', (code, key) => {
    expect(upstreamSyncError(code, value => value)).toBe(`upstreamCenter.newapi.errors.${key}`)
  })
  it.each([
    ['upstream_timeout', 'timeout'], ['upstream_dns_failed', 'dnsFailed'], ['upstream_tls_failed', 'tlsFailed'],
    ['upstream_connection_failed', 'connectionFailed'], ['upstream_request_cancelled', 'cancelled'],
    ['upstream_request_failed', 'requestFailed'], ['response_read_failed', 'responseReadFailed'],
    ['response_too_large', 'responseTooLarge'], ['usage_response_unsupported', 'usageResponseUnsupported'],
    ['credential_unavailable', 'credentialUnavailable'],
  ])('explains Sub2API failure %s', (code, key) => {
    expect(upstreamSyncError(code, value => value)).toBe(`upstreamCenter.syncErrors.${key}`)
  })
  it('keeps a partial history failure with consumption while preserving separate wallet errors', () => {
    const error = 'sub2api_history_sync_failed: upstream_timeout'
    const snapshot = { status: 'ok', kind: 'wallet', balance: 42, error } as UpstreamBalanceSnapshot
    expect(walletSyncError(snapshot)).toBe('')
    expect(walletUsageError(snapshot)).toBe(error)
    expect(upstreamSyncError(error, value => value)).toBe('upstreamCenter.syncErrors.historyFailed: upstreamCenter.syncErrors.timeout')
    expect(walletSyncError({ ...snapshot, error: `${error}; credential_unavailable` })).toBe('credential_unavailable')
    expect(walletSyncError({ ...snapshot, status: 'error' })).toBe(error)
    expect(upstreamSyncError('upstream_http_524', value => value)).toBe('upstreamCenter.syncErrors.httpError (HTTP 524)')
  })
})
