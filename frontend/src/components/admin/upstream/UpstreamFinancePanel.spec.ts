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
    financeSummary.mockResolvedValue({ today: converted, last_30_days: converted })
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
    financeSummary.mockResolvedValue({ today: partial, last_30_days: partial })
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
  it('shows period-specific unused key exclusions without blocking reported profit', async () => {
    financeSummary.mockResolvedValue({
      today: summary({ inactive_key_count: 3 }),
      last_30_days: summary({ revenue: 120, remote_used: 40, profit: 80, inactive_key_count: 1 }),
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"] [data-testid="finance-inactive"]').text()).toBe('upstreamCenter.finance.inactiveScope 3')
    expect(wrapper.get('[data-period="last30Days"] [data-testid="finance-inactive"]').text()).toBe('upstreamCenter.finance.inactiveScope 1')
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(8))
    expect(wrapper.get('[data-period="last30Days"]').text()).toContain(money(80))
    expect(wrapper.text()).toContain('upstreamCenter.finance.aggregateScope')
    expect(wrapper.text()).not.toContain('upstreamCenter.finance.pending')
    wrapper.unmount()
  })
  it('shows zero business totals when all keys were only monitored in the period', async () => {
    const inactive = summary({ revenue: 0, remote_used: 0, profit: 0, request_count: 0, inactive_key_count: 2 })
    financeSummary.mockResolvedValue({ today: inactive, last_30_days: inactive })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(0))
    expect(wrapper.get('[data-period="today"] [data-testid="finance-inactive"]').text()).toBe('upstreamCenter.finance.inactiveScope 2')
    expect(wrapper.text()).not.toContain('upstreamCenter.finance.pending')
    expect(wrapper.text()).not.toContain('upstreamCenter.financeUnavailable')
    wrapper.unmount()
  })
  it('shows today and the last 30 days without a request ledger or date inputs', async () => {
    financeSummary.mockResolvedValue({ today: summary(), last_30_days: summary({ revenue: 120, remote_used: 40, profit: 80, from: '2026-09-06T00:00:00+08:00' }) })
    const wrapper = mountPanel({ id: 2, targets: [] } as unknown as UpstreamSupplier)
    await flushPromises()
    expect(financeSummary).toHaveBeenCalledWith({ supplier_id: 2, target_id: undefined }, expect.any(AbortSignal))
    const today = wrapper.get('[data-period="today"]')
    const month = wrapper.get('[data-period="last30Days"]')
    for (const value of [12, 4, 8]) expect(today.text()).toContain(money(value))
    for (const value of [120, 40, 80]) expect(month.text()).toContain(money(value))
    expect(wrapper.find('table').exists()).toBe(false)
    expect(wrapper.find('input[type="date"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not fabricate a zero cost or profit when upstream usage is missing', async () => {
    financeSummary.mockResolvedValue({
      today: summary({ remote_used: null, profit: -99, cost_source: 'estimated' }),
      last_30_days: summary({ remote_used: null, profit: null, cost_source: 'unknown' }),
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.findAll('[data-period]').every(period => period.text().includes('upstreamCenter.finance.pending'))).toBe(true)
    expect(wrapper.text()).toContain('upstreamCenter.financeUnavailable')
    expect(wrapper.text()).not.toContain(money(-99))
    expect(wrapper.text()).not.toContain(money(0))
    wrapper.unmount()
  })

  it('keeps actual usage and profit visible during a delayed upstream sync, including zero usage', async () => {
    const stale = summary({ remote_stale: true, remote_used: 0, profit: 12 })
    financeSummary.mockResolvedValue({ today: stale, last_30_days: stale })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(0))
    expect(wrapper.get('[data-period="today"]').text()).toContain(money(12))
    expect(wrapper.text()).toContain('upstreamCenter.finance.stale')
    expect(wrapper.text()).toContain('upstreamCenter.wallet.syncedAt')
    expect(wrapper.text()).not.toContain('upstreamCenter.finance.pending')
    wrapper.unmount()
  })

  it('preserves the last totals when a refresh fails', async () => {
    financeSummary.mockResolvedValueOnce({ today: summary(), last_30_days: summary() })
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
    financeSummary.mockResolvedValue({ today: summary(), last_30_days: summary() })
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
