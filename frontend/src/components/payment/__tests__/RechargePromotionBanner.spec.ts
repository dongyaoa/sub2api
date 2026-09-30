import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import RechargePromotionBanner from '../RechargePromotionBanner.vue'
import type { RechargePromotion } from '@/types/payment'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale: { value: 'zh' }, t: (key: string, params?: Record<string, unknown>) => `${key} ${params ? JSON.stringify(params) : ''}` }),
}))

const promotion: RechargePromotion = {
  enabled: true, active: true, title: '国庆加赠', subtitle: '开启下一次灵感',
  starts_at: '2026-10-01T00:00:00+08:00', ends_at: '2026-10-08T00:00:00+08:00', currency: 'CNY', max_bonus: 20,
  tiers: [{ min_amount: 50, bonus_percent: 5 }, { min_amount: 100, bonus_percent: 10 }, { min_amount: 300, bonus_percent: 12 }],
}

describe('RechargePromotionBanner', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-02T00:00:00+08:00')) })
  afterEach(() => { vi.useRealTimers() })

  it('selects a threshold and highlights only the highest qualified tier', async () => {
    const wrapper = mount(RechargePromotionBanner, { props: { promotion, selectable: true, amount: 120, selectedCurrency: 'CNY' } })
    expect(wrapper.findAll('[aria-pressed="true"]')).toHaveLength(1)
    expect(wrapper.get('[aria-pressed="true"]').attributes('data-tier')).toBe('100')
    await wrapper.get('[data-tier="300"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[300]])
    expect(wrapper.get('[data-tier="300"]').text()).toContain('+12%')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.get('summary').text()).toContain('promotion.details')
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('disables ineligible currency and unsupported amounts without showing a selected bonus', async () => {
    const wrapper = mount(RechargePromotionBanner, { props: { promotion, selectable: true, amount: 100, selectedCurrency: 'USD' } })
    expect(wrapper.findAll('button').every(button => button.attributes('disabled') !== undefined)).toBe(true)
    expect(wrapper.find('[aria-pressed="true"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('promotion.currencyMismatch')
    await wrapper.get('[data-tier="100"]').trigger('click')
    expect(wrapper.emitted('select')).toBeUndefined()
    await wrapper.setProps({ selectedCurrency: 'CNY', disabledTiers: [300] })
    expect(wrapper.get('[data-tier="300"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-tier="100"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('appears at the scheduled start and disappears at the end while left open', async () => {
    vi.setSystemTime(new Date('2026-09-30T23:59:59+08:00'))
    const wrapper = mount(RechargePromotionBanner, { props: { promotion: { ...promotion, active: false } } })
    expect(wrapper.find('[data-testid="recharge-promotion"]').exists()).toBe(false)
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.find('[data-testid="recharge-promotion"]').exists()).toBe(true)
    vi.setSystemTime(new Date('2026-10-07T23:59:59+08:00'))
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.find('[data-testid="recharge-promotion"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
