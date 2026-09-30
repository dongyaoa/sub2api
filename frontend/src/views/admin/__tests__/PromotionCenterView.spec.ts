import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import PromotionCenterView from '../PromotionCenterView.vue'
import RechargePromotionSettings from '@/components/admin/payment/RechargePromotionSettings.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { PromotionOrder, PromotionSummary } from '@/api/admin/promotions'
import type { RechargePromotion } from '@/types/payment'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(), summary: vi.fn(), orders: vi.fn(), replace: vi.fn(),
  route: { query: {} as Record<string, string> },
  setEnabled: (_value: boolean | undefined) => {},
}))
vi.mock('@/api/admin/promotions', () => ({ adminPromotionsAPI: { summary: mocks.summary, orders: mocks.orders } }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getConfig: mocks.getConfig }, default: { getConfig: mocks.getConfig } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-router', async importOriginal => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => mocks.route, useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ locale: { value: 'zh' }, t: (key: string) => key }) }))
vi.mock('@/stores/app', async () => {
  const { reactive } = await import('vue')
  const store = reactive({ cachedPublicSettings: { recharge_promotion_enabled: true as boolean | undefined } })
  mocks.setEnabled = value => { store.cachedPublicSettings.recharge_promotion_enabled = value }
  return { useAppStore: () => store }
})

enableAutoUnmount(afterEach)
const campaign: RechargePromotion = {
  enabled: true, active: false, title: '实际充值活动', subtitle: '', currency: 'CNY', max_bonus: 10,
  starts_at: '2026-10-01T00:00:00+08:00', ends_at: '2026-10-08T00:00:00+08:00',
  tiers: [{ min_amount: 100, bonus_percent: 10 }],
}
const summary: PromotionSummary = {
  order_count: 3, pending_order_count: 1, issued_order_count: 2, issued_user_count: 2,
  base_amount: 987.65, bonus_amount: 98.76, refunded_base_amount: 98.7, refunded_bonus_amount: 9.87,
  net_base_amount: 888.95, net_bonus_amount: 88.89,
  cash_by_currency: [{ currency: 'CNY', paid_amount: 1234.5, refunded_amount: 100, net_amount: 1134.5 }],
}
const order: PromotionOrder = {
  id: 105, user_id: 23, user_username: '真实用户', user_email: 'actual@example.com', amount: 110,
  pay_amount: 102, currency: 'CNY', base_amount: 100, bonus_amount: 10,
  promotion_snapshot: { title: '下单时活动名称', base_amount: 100, bonus_amount: 10, bonus_percent: 10, currency: 'CNY' },
  fee_rate: 2, payment_type: 'alipay', out_trade_no: 'payment-105', status: 'COMPLETED', order_type: 'balance',
  created_at: '2026-10-01T01:00:00Z', completed_at: '2026-10-01T01:01:00Z', expires_at: '2026-10-01T01:10:00Z', refund_amount: 0,
}

function render(tab = 'overview') {
  mocks.route.query = { tab }
  return shallowMount(PromotionCenterView, {
    global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      RouterLink: { template: '<a><slot /></a>' },
    } },
  })
}
function findButton(wrapper: ReturnType<typeof render>, text: string) {
  const button = wrapper.findAll('button').find(item => item.text() === text)
  if (!button) throw new Error(`Button not found: ${text}`)
  return button
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-01T02:00:00+08:00'))
  vi.resetAllMocks()
  mocks.setEnabled(true)
  mocks.replace.mockResolvedValue(undefined)
  mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: campaign } })
  mocks.summary.mockResolvedValue({ data: summary })
  mocks.orders.mockImplementation(({ page = 1, page_size = 20 }) => Promise.resolve({
    data: { items: [order], total: 45, page, page_size, pages: Math.ceil(45 / page_size) },
  }))
})
afterEach(() => { vi.useRealTimers() })

describe('PromotionCenterView', () => {
  it('renders real metrics, separate cash totals and the immutable order snapshot', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('$987.65')
    expect(wrapper.text()).toContain('$98.76')
    expect(wrapper.text()).toContain('$88.89')
    expect(wrapper.text()).toContain('1,234.50')
    expect(wrapper.text()).toContain('1,134.50')
    expect(wrapper.text()).toContain('实际充值活动')
    expect(wrapper.text()).toContain('真实用户')
    const recent = wrapper.findAll('button').find(item => item.text().includes('#105'))!
    await recent.trigger('click')
    expect(wrapper.text()).toContain('下单时活动名称')
    expect(wrapper.text()).toContain('payment-105')
    expect(wrapper.text()).toContain('$110.00')
    expect(wrapper.text()).toContain('未到账订单的额度仅为预计值')
  })

  it('shows a real empty state only after successful empty responses', async () => {
    mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: null } })
    mocks.summary.mockResolvedValue({ data: { ...summary, order_count: 0, pending_order_count: 0, issued_order_count: 0, issued_user_count: 0, base_amount: 0, bonus_amount: 0, net_bonus_amount: 0, refunded_bonus_amount: 0, cash_by_currency: [] } })
    mocks.orders.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 0 } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('暂无参与记录')
    expect(wrapper.text()).toContain('暂无已收款记录')
    expect(wrapper.text()).toContain('尚未配置活动')
    expect(wrapper.text()).not.toContain('真实用户')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it.each(['overview', 'records'])('shows a %s request error without fabricated zeros or an empty result', async tab => {
    mocks.orders.mockRejectedValueOnce(new Error('订单服务不可用'))
    const wrapper = render(tab)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('订单服务不可用')
    expect(wrapper.text()).not.toContain('暂无参与记录')
    expect(wrapper.text()).not.toContain('$0.00')
    expect(wrapper.find(`#promotion-panel-${tab}`).exists()).toBe(false)
    await findButton(wrapper, '重试').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('真实用户')
  })

  it('does not describe an unknown configuration as disabled or unconfigured', async () => {
    let rejectConfig: ((error: Error) => void) | undefined
    mocks.getConfig.mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectConfig = reject }))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="campaign-status"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('尚未配置活动')
    rejectConfig?.(new Error('配置读取失败'))
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('配置读取失败')
    expect(wrapper.text()).not.toContain('尚未配置活动')
    expect(wrapper.text()).toContain('$987.65')
    await findButton(wrapper, '重试').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('实际充值活动')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('uses Beijing calendar boundaries and an exclusive next-day end for custom filters', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-range"]').setValue('custom')
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-10-01')
    await dates[1].setValue('2026-10-07')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.summary).toHaveBeenLastCalledWith({ start_time: '2026-09-30T16:00:00.000Z', end_time: '2026-10-07T16:00:00.000Z' })
    expect(mocks.orders).toHaveBeenLastCalledWith({ start_time: '2026-09-30T16:00:00.000Z', end_time: '2026-10-07T16:00:00.000Z', page: 1, page_size: 20 })
    const callCount = mocks.orders.mock.calls.length
    await dates[1].setValue('2026-09-30')
    await wrapper.get('form').trigger('submit')
    expect(mocks.orders).toHaveBeenCalledTimes(callCount)
    expect(wrapper.get('[role="alert"]').text()).toContain('结束日期不能早于开始日期')
  })

  it('uses today in Beijing when selecting the last seven calendar days', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-range"]').setValue('7')
    await flushPromises()
    expect(mocks.summary).toHaveBeenLastCalledWith({ start_time: '2026-09-24T16:00:00.000Z', end_time: '2026-10-01T16:00:00.000Z' })
  })

  it('applies trimmed search/status filters, preserves them for pagination and resets the page', async () => {
    const wrapper = render('records')
    await flushPromises()
    await wrapper.get('[data-testid="promotion-status-filter"]').setValue('COMPLETED')
    await wrapper.get('[data-testid="promotion-search"]').setValue('  actual@example.com  ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const filters = { status: 'COMPLETED', keyword: 'actual@example.com' }
    expect(mocks.orders).toHaveBeenLastCalledWith({ ...filters, page: 1, page_size: 20 })
    wrapper.getComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    expect(mocks.orders).toHaveBeenLastCalledWith({ ...filters, page: 2, page_size: 20 })
    wrapper.getComponent(Pagination).vm.$emit('update:page-size', 50)
    await flushPromises()
    expect(mocks.orders).toHaveBeenLastCalledWith({ ...filters, page: 1, page_size: 50 })
    await findButton(wrapper, '重置').trigger('click')
    await flushPromises()
    expect(mocks.orders).toHaveBeenLastCalledWith({ page: 1, page_size: 50 })
    expect((wrapper.get('[data-testid="promotion-search"]').element as HTMLInputElement).value).toBe('')
  })

  it('starts a campaign inclusively and ends it exclusively while the page remains open', async () => {
    vi.setSystemTime(new Date('2026-09-30T23:59:59+08:00'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-testid="campaign-status"]').text()).toBe('未开始')
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.get('[data-testid="campaign-status"]').text()).toBe('进行中')
    vi.setSystemTime(new Date('2026-10-07T23:59:59+08:00'))
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.get('[data-testid="campaign-status"]').text()).toBe('已结束')
  })

  it('ignores an older report response after a newer filter request has completed', async () => {
    const wrapper = render('records')
    await flushPromises()
    let resolveOlder: ((value: unknown) => void) | undefined
    mocks.orders.mockImplementationOnce(() => new Promise(resolve => { resolveOlder = resolve }))
    await findButton(wrapper, '刷新数据').trigger('click')
    mocks.orders.mockResolvedValueOnce({ data: { items: [{ ...order, id: 999 }], total: 1, page: 1, page_size: 20, pages: 1 } })
    await wrapper.get('[data-testid="promotion-range"]').setValue('7')
    await flushPromises()
    expect(wrapper.get('#promotion-panel-records').text()).toContain('#999')
    resolveOlder?.({ data: { items: [order], total: 45, page: 1, page_size: 20, pages: 3 } })
    await flushPromises()
    expect(wrapper.get('#promotion-panel-records').text()).toContain('#999')
    expect(wrapper.get('#promotion-panel-records').text()).not.toContain('#105')
  })

  it('leaves the page when the master switch is disabled and makes no further report calls', async () => {
    const wrapper = render()
    await flushPromises()
    mocks.setEnabled(false)
    await nextTick()
    expect(mocks.replace).toHaveBeenLastCalledWith('/admin/settings?tab=features')
    const callCount = mocks.orders.mock.calls.length
    await findButton(wrapper, '刷新数据').trigger('click')
    expect(mocks.orders).toHaveBeenCalledTimes(callCount)
  })

  it('does not load campaign data when the feature was already disabled on mount', async () => {
    mocks.setEnabled(false)
    render()
    await flushPromises()
    expect(mocks.replace).toHaveBeenCalledWith('/admin/settings?tab=features')
    expect(mocks.getConfig).not.toHaveBeenCalled()
    expect(mocks.summary).not.toHaveBeenCalled()
    expect(mocks.orders).not.toHaveBeenCalled()
  })

  it('refreshes the current campaign after a saved event without replacing the settings editor', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#promotion-tab-settings').trigger('click')
    const editor = wrapper.getComponent(RechargePromotionSettings)
    mocks.getConfig.mockResolvedValueOnce({ data: { recharge_promotion: { ...campaign, title: '保存后的活动', enabled: false } } })
    editor.vm.$emit('saved')
    await flushPromises()
    await wrapper.get('#promotion-tab-overview').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('保存后的活动')
    expect(wrapper.get('[data-testid="campaign-status"]').text()).toBe('未启用')
    expect(wrapper.getComponent(RechargePromotionSettings).vm).toBe(editor.vm)
  })
})
