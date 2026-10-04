import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UpstreamFinancePanel from './UpstreamFinancePanel.vue'
import type { UpstreamSupplier } from '@/api/admin/upstreamCenter'
const finance = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/upstreamCenter', () => ({ upstreamCenterAPI: { finance } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => { finance.mockReset() })
describe('upstream finance unknown-cost state', () => {
  it('explains the required date boundary when historical finance has been rolled up hourly', async () => {
    finance.mockRejectedValue({ code: 'UPSTREAM_FINANCE_ARCHIVED_RANGE', message: 'raw technical error' })
    const wrapper = mount(UpstreamFinancePanel, { props: { supplier: null, target: null }, global: { stubs: { Pagination: true, Icon: true } } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('upstreamCenter.storage.archivedFinanceRange')
    wrapper.unmount()
  })
  it('shows unreconciled upstream usage instead of a fabricated zero profit and uses the backend default date window', async () => {
    finance.mockResolvedValue({ summary: { revenue: 12, business_cost: null, monitor_cost: null, profit: null, request_count: 2, cost_source: 'unknown', currency: 'USD', from: '2026-09-23T00:00:00Z', to: '2026-09-24T00:00:00Z', remote_used: null, reconciliation_delta: null, unpriced_monitor_count: 3 }, items: [], total: 0, page: 1, page_size: 50 })
    const wrapper = mount(UpstreamFinancePanel, { props: { supplier: { id: 2, targets: [] } as unknown as UpstreamSupplier, target: null }, global: { stubs: { Pagination: true, Icon: true } } })
    await flushPromises()
    expect(finance).toHaveBeenCalledWith(expect.objectContaining({ supplier_id: 2, from: undefined, to: undefined }), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('upstreamCenter.finance.pending')
    expect(wrapper.text()).toContain('upstreamCenter.finance.remoteUsedRange')
    expect(wrapper.text()).toContain('upstreamCenter.finance.noRows')
    expect(wrapper.text()).not.toContain('$0.00')
    wrapper.unmount()
  })
  it('hides legacy estimated costs and margins even when the old API sends negative profit', async () => {
    finance.mockResolvedValue({
      summary: { revenue: 0, remote_used: null, business_cost: 0.007152, monitor_cost: 0, profit: -0.007152, request_count: 0, cost_source: 'estimated', currency: 'USD', from: '2026-10-04T00:00:00Z', to: '2026-10-05T00:00:00Z' },
      items: [{ id: 1, created_at: '2026-10-04T10:00:00Z', target_name: 'Key', model: 'gpt', revenue: 0, business_cost: 0.007152, profit: -0.007152, billing_type: 0 }],
      total: 1, page: 1, page_size: 50,
    })
    const wrapper = mount(UpstreamFinancePanel, { props: { supplier: null, target: null }, global: { stubs: { Pagination: true, Icon: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('upstreamCenter.finance.estimated')
    expect(wrapper.text()).toContain('upstreamCenter.finance.pending')
    expect(wrapper.text()).not.toContain('0.007152')
    expect(wrapper.findAll('tbody tr')[0]?.text()).toContain('upstreamCenter.finance.unreconciled')
    wrapper.unmount()
  })
  it('does not display estimated costs or margins for requests without matched upstream records', async () => {
    finance.mockResolvedValue({
      summary: { revenue: 2, business_cost: null, monitor_cost: null, profit: 1, request_count: 2, cost_source: 'reported', currency: 'USD', from: '2026-09-23T00:00:00Z', to: '2026-09-24T00:00:00Z', remote_used: 1, remote_synced_at: '2026-09-23T12:00:00Z', reconciliation_delta: null, unpriced_monitor_count: 0 },
      items: [
        { id: 1, created_at: '2026-09-23T10:00:00Z', target_name: 'Key', model: 'gpt', account_id: 1, group_id: 2, request_id: 'unmatched', revenue: 1, business_cost: null, profit: null, billing_type: 0 },
        { id: 2, created_at: '2026-09-23T10:01:00Z', target_name: 'Key', model: 'gpt', account_id: 1, group_id: 2, request_id: 'matched', revenue: 1, business_cost: 0.3, profit: 0.7, billing_type: 0 },
      ], total: 2, page: 1, page_size: 50,
    })
    const wrapper = mount(UpstreamFinancePanel, { props: { supplier: null, target: null }, global: { stubs: { Pagination: true, Icon: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('upstreamCenter.wallet.syncedAt')
    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]?.text()).toContain('upstreamCenter.finance.unreconciled')
    expect(rows[0]?.text()).not.toContain('$0.00')
    expect(rows[1]?.text()).toContain('$0.30')
    expect(rows[1]?.text()).toContain('$0.70')
    wrapper.unmount()
  })
  it('preserves a supplier group and date filter when the same supplier refreshes', async () => {
    finance.mockResolvedValue({ summary: { revenue: 12, business_cost: null, monitor_cost: 0, profit: 8, request_count: 2, cost_source: 'reported', currency: 'USD', from: '2026-09-23T00:00:00Z', to: '2026-09-24T00:00:00Z', remote_used: 4, reconciliation_delta: null, unpriced_monitor_count: 0 }, items: [], total: 0, page: 1, page_size: 50 })
    const supplier = { id: 2, name: 'Supplier', targets: [{ id: 9, name: 'Group 9' }] } as unknown as UpstreamSupplier
    const wrapper = mount(UpstreamFinancePanel, { props: { supplier, target: null }, global: { stubs: { Pagination: true, Icon: true, teleport: true, transition: true } } })
    await flushPromises()
    await wrapper.get('#finance-target').trigger('click')
    expect(wrapper.find('.select-search-input').exists()).toBe(false)
    await wrapper.findAll('[role="option"]').find(option => option.text() === 'Group 9')!.trigger('click')
    expect(wrapper.get('#finance-target').attributes('aria-expanded')).toBe('false')
    await wrapper.get('#finance-from').setValue('2026-09-20')
    await wrapper.get('#finance-to').setValue('2026-09-21')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(finance).toHaveBeenCalledTimes(2)
    expect(finance).toHaveBeenLastCalledWith(expect.objectContaining({ supplier_id: 2, target_id: 9 }), expect.any(AbortSignal))

    await wrapper.setProps({ supplier: { ...supplier, name: 'Updated supplier' } })
    await flushPromises()
    expect(wrapper.get('#finance-target').text()).toBe('Group 9')
    expect((wrapper.get('#finance-from').element as HTMLInputElement).value).toBe('2026-09-20')
    expect((wrapper.get('#finance-to').element as HTMLInputElement).value).toBe('2026-09-21')
    expect(finance).toHaveBeenCalledTimes(2)
    await wrapper.get('#finance-target').trigger('click')
    await wrapper.findAll('[role="option"]').find(option => option.text() === 'upstreamCenter.financeDetails')!.trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(finance).toHaveBeenLastCalledWith(expect.objectContaining({ supplier_id: 2, target_id: undefined }), expect.any(AbortSignal))
    wrapper.unmount()
  })
})
