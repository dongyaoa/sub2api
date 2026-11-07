import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UpstreamFinancePanel from './UpstreamFinancePanel.vue'
import type { UpstreamFinanceSummary, UpstreamSupplier } from '@/api/admin/upstreamCenter'
import { money } from './format'

const financeSummary = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/upstreamCenter', () => ({ upstreamCenterAPI: { financeSummary } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: { count?: number }) => params?.count == null ? key : `${key} ${params.count}` }) }))
beforeEach(() => { financeSummary.mockReset() })

function summary(overrides: Partial<UpstreamFinanceSummary> = {}): UpstreamFinanceSummary {
  return {
    revenue: 12, business_cost: null, monitor_cost: null, profit: 8,
    request_count: 2, total_tokens: 0, unknown_token_requests: 0, account_billed: 0,
    cost_source: 'reported', currency: 'USD', from: '2026-10-05T00:00:00+08:00', to: '2026-10-06T00:00:00+08:00',
    remote_used: 4, remote_synced_at: '2026-10-05T12:00:00+08:00', remote_stale: false,
    reconciliation_delta: null, unpriced_monitor_count: 0, ...overrides,
  }
}
function mountPanel(supplier: UpstreamSupplier | null = null) {
  return mount(UpstreamFinancePanel, { props: { supplier, target: null }, global: { stubs: { Icon: true, teleport: true, transition: true } } })
}

describe('upstream consumption and profit totals', () => {
  it('uses server-converted amounts without applying the supplier recharge ratio again', async () => {
    const converted = summary({ revenue: 5, remote_used: 2, remote_raw_used: 20, profit: 3, conversion_applied: true })
    financeSummary.mockResolvedValue(converted)
    const wrapper = mountPanel({ id: 2, targets: [], recharge_ratio: 10 } as unknown as UpstreamSupplier)
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(2))
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(3))
    expect(wrapper.get('[data-period="today"]').text()).not.toContain(money(0.2))
    expect(wrapper.find('[data-testid="supplier-recharge-ratio"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="finance-converted"]').exists()).toBe(true)
    wrapper.unmount()
  })
  it('keeps known partial costs visible but hides profit until all active keys are covered', async () => {
    const partial = summary({ cost_source: 'unknown', remote_used: 2, profit: 10, cost_partial: true, known_key_count: 1, missing_key_count: 1, archived_key_count: 2, inactive_key_count: 3 })
    financeSummary.mockResolvedValue(partial)
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(2))
    expect(wrapper.get('[data-period="today"]').text()).not.toContain(money(10))
    expect(wrapper.get('[data-period="today"]').text()).toContain('upstreamCenter.finance.pending')
    expect(wrapper.find('[data-testid="finance-partial"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="finance-archived"]').exists()).toBe(true)
    expect(wrapper.get('[data-period="today"] [data-testid="finance-inactive"]').text()).toBe('upstreamCenter.finance.inactiveScope 3')
    expect(wrapper.find('[data-testid="finance-converted"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="supplier-recharge-ratio"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('waits for local history imports before showing profit without blaming already synced upstream costs', async () => {
    const pending = summary({ revenue: 1, remote_used: 4, profit: -3, history_sync_pending: 2 })
    financeSummary.mockResolvedValueOnce(pending)
    const wrapper = mountPanel()
    await flushPromises()
    for (const period of wrapper.findAll('[data-period]')) {
      expect(period.text()).toContain('upstreamCenter.finance.historySyncPending')
      expect(period.get('[data-testid="finance-history-pending"]').text()).toBe('upstreamCenter.finance.historySyncPendingHint 2')
      expect(period.text()).toContain(money(4))
      expect(period.text()).not.toContain(money(-3))
      expect(period.text()).not.toContain('upstreamCenter.finance.pending')
      expect(period.text()).not.toContain('upstreamCenter.financeUnavailable')
    }
    financeSummary.mockResolvedValueOnce(summary())
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain(money(8))
    expect(wrapper.find('[data-testid="finance-history-pending"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('shows zero business totals when all keys were only monitored in the period', async () => {
    const inactive = summary({ revenue: 0, remote_used: 0, profit: 0, request_count: 0, inactive_key_count: 2 })
    financeSummary.mockResolvedValue(inactive)
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(0))
    expect(wrapper.get('[data-period="today"] [data-testid="finance-inactive"]').text()).toBe('upstreamCenter.finance.inactiveScope 2')
    expect(wrapper.text()).not.toContain('upstreamCenter.finance.pending')
    expect(wrapper.text()).not.toContain('upstreamCenter.financeUnavailable')
    wrapper.unmount()
  })
  it('shows only today without a request ledger or date inputs', async () => {
    financeSummary.mockResolvedValue(summary())
    const wrapper = mountPanel({ id: 2, targets: [] } as unknown as UpstreamSupplier)
    await flushPromises()
    expect(financeSummary).toHaveBeenCalledWith({ supplier_id: 2, target_id: undefined }, expect.any(AbortSignal))
    const today = wrapper.get('[data-period="today"]')
    expect(wrapper.findAll('[data-period]')).toHaveLength(1)
    for (const value of [12, 4, 8]) expect(today.text()).toContain(money(value))
    expect(wrapper.find('table').exists()).toBe(false)
    expect(wrapper.find('input[type="date"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('does not fabricate a zero cost or profit when upstream usage is missing', async () => {
    financeSummary.mockResolvedValue(summary({ remote_used: null, profit: -99, cost_source: 'estimated' }))
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.findAll('[data-period]').every(period => period.text().includes('upstreamCenter.finance.pending'))).toBe(true)
    expect(wrapper.text()).toContain('upstreamCenter.financeUnavailable')
    expect(wrapper.text()).not.toContain(money(-99))
    expect(wrapper.text()).not.toContain(money(0))
    wrapper.unmount()
  })

  it('keeps last reported usage visible but waits for current cost before displaying profit', async () => {
    const stale = summary({ remote_stale: true, remote_used: 0, profit: 12 })
    financeSummary.mockResolvedValue(stale)
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(0))
    expect(wrapper.get('[data-period="today"]').text()).toContain('upstreamCenter.finance.pending')
    expect(wrapper.text()).toContain('upstreamCenter.finance.stale')
    expect(wrapper.text()).toContain('upstreamCenter.wallet.syncedAt')
    expect(wrapper.findAll('[data-period] .text-lg').filter(value => value.text() === money(12))).toHaveLength(1)
    wrapper.unmount()
  })

  it('preserves the last totals when a refresh fails', async () => {
    financeSummary.mockResolvedValueOnce(summary())
    const wrapper = mountPanel()
    await flushPromises()
    financeSummary.mockRejectedValueOnce(new Error('Sync unavailable'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBeTruthy()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(4))
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(8))
    wrapper.unmount()
  })

  it('loads a selected key group and preserves the selection through supplier refreshes', async () => {
    financeSummary.mockResolvedValue(summary())
    const supplier = { id: 2, name: 'Supplier', targets: [{ id: 9, name: 'Group 9' }] } as unknown as UpstreamSupplier
    const wrapper = mountPanel(supplier)
    await flushPromises()
    await wrapper.get('#finance-target').trigger('click')
    await wrapper.findAll('[role="option"]').find(option => option.text() === 'Group 9')!.trigger('click')
    await flushPromises()
    expect(financeSummary).toHaveBeenLastCalledWith({ supplier_id: 2, target_id: 9 }, expect.any(AbortSignal))
    expect(wrapper.text()).not.toContain('upstreamCenter.finance.aggregateScope')
    expect(financeSummary).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ supplier: { ...supplier, name: 'Updated supplier' } })
    await flushPromises()
    expect(wrapper.get('#finance-target').text()).toBe('Group 9')
    expect(financeSummary).toHaveBeenCalledTimes(2)
    await wrapper.get('#finance-target').trigger('click')
    await wrapper.findAll('[role="option"]').find(option => option.text() === 'upstreamCenter.finance.allGroups')!.trigger('click')
    await flushPromises()
    expect(financeSummary).toHaveBeenLastCalledWith({ supplier_id: 2, target_id: undefined }, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('upstreamCenter.finance.aggregateScope')
    wrapper.unmount()
  })
})
