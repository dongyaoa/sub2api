import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { useRechargePromotionStore } from '../rechargePromotion'
import { useRechargePromotion } from '@/composables/useRechargePromotion'
import type { RechargePromotion, PaymentConfig, CheckoutInfoResponse } from '@/types/payment'

const getConfig = vi.hoisted(() => vi.fn())
const state = vi.hoisted(() => ({ auth: {} as any, app: {} as any }))
vi.mock('@/api/payment', () => ({ paymentAPI: { getConfig } }))
vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue')
  state.auth = reactive({ user: { id: 1 }, isAuthenticated: true, isSimpleMode: false })
  return { useAuthStore: () => state.auth }
})
vi.mock('@/stores/app', async () => {
  const { reactive } = await import('vue')
  state.app = reactive({ cachedPublicSettings: { payment_enabled: true, recharge_promotion_enabled: true } })
  return { useAppStore: () => state.app }
})

const promotion: RechargePromotion = {
  enabled: true, active: false, title: '国庆加赠', subtitle: '', currency: 'CNY', max_bonus: 0,
  starts_at: '2026-10-01T00:00:00+08:00', ends_at: '2026-10-08T00:00:00+08:00',
  tiers: [{ min_amount: 100, bonus_percent: 10 }],
}
const config = { enabled: true, balance_disabled: false, recharge_promotion: promotion } as PaymentConfig
const checkout = { balance_disabled: false, recharge_promotion: promotion } as CheckoutInfoResponse

beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-02T00:00:00+08:00'))
  state.auth.user = { id: 1 }
  state.auth.isAuthenticated = true
  state.auth.isSimpleMode = false
  state.app.cachedPublicSettings = { payment_enabled: true, recharge_promotion_enabled: true }
  getConfig.mockReset().mockResolvedValue({ data: config })
})
afterEach(() => vi.useRealTimers())

describe('shared promotion visibility', () => {
  it('shares an in-flight request and caches successful configuration', async () => {
    let resolve!: (value: { data: PaymentConfig }) => void
    getConfig.mockReturnValue(new Promise(done => { resolve = done }))
    const store = useRechargePromotionStore()
    const first = store.load()
    const second = store.load()
    expect(getConfig).toHaveBeenCalledTimes(1)
    resolve({ data: config })
    await Promise.all([first, second])
    await store.load()
    expect(store.promotion).toEqual(promotion)
    expect(getConfig).toHaveBeenCalledTimes(1)
    state.auth.user = { id: 1 }
    expect(store.promotion).toEqual(promotion)
  })

  it('uses checkout data without a separate config request', async () => {
    const store = useRechargePromotionStore()
    store.acceptCheckout(checkout)
    await store.load()
    expect(store.promotion).toEqual(promotion)
    expect(getConfig).not.toHaveBeenCalled()
  })

  it('clears on logout and ignores a response from the previous session', async () => {
    let resolve!: (value: { data: PaymentConfig }) => void
    const store = useRechargePromotionStore()
    store.acceptCheckout(checkout)
    state.auth.isAuthenticated = false
    expect(store.promotion).toBeNull()
    state.auth.isAuthenticated = true
    getConfig.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const pending = store.load()
    state.auth.user = { id: 2 }
    resolve({ data: config })
    await pending
    expect(store.promotion).toBeNull()
    await store.load()
    expect(getConfig).toHaveBeenCalledTimes(2)
  })

  it.each([
    { enabled: false },
    { balance_disabled: true },
    { recharge_promotion: null },
  ])('hides unavailable server configuration: %j', async (patch) => {
    getConfig.mockResolvedValue({ data: { ...config, ...patch } })
    const store = useRechargePromotionStore()
    await store.load()
    expect(store.promotion).toBeNull()
  })

  it('clears stale promotion on failed refresh and permits retry', async () => {
    const store = useRechargePromotionStore()
    await store.load()
    vi.advanceTimersByTime(60_000)
    getConfig.mockRejectedValueOnce(new Error('offline'))
    await store.load()
    expect(store.promotion).toBeNull()
    await store.load()
    expect(store.promotion).toEqual(promotion)
  })

  it('keeps scheduled starts and ends current and responds to the master switch', async () => {
    vi.setSystemTime(new Date('2026-09-30T23:59:59+08:00'))
    const Host = defineComponent({
      setup: () => useRechargePromotion(),
      template: '<span v-if="activePromotion" data-active>active</span>',
    })
    const wrapper = mount(Host)
    await flushPromises()
    expect(wrapper.find('[data-active]').exists()).toBe(false)
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.find('[data-active]').exists()).toBe(true)
    state.app.cachedPublicSettings.recharge_promotion_enabled = false
    await nextTick()
    expect(wrapper.find('[data-active]').exists()).toBe(false)
    state.app.cachedPublicSettings.recharge_promotion_enabled = true
    await flushPromises()
    expect(wrapper.find('[data-active]').exists()).toBe(true)
    vi.setSystemTime(new Date('2026-10-07T23:59:59+08:00'))
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.find('[data-active]').exists()).toBe(false)
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not request config on purchase but can load when navigating away', async () => {
    const fetchEnabled = ref(false)
    const Host = defineComponent({ setup: () => useRechargePromotion(fetchEnabled), template: '<span />' })
    const wrapper = mount(Host)
    await flushPromises()
    expect(getConfig).not.toHaveBeenCalled()
    fetchEnabled.value = true
    await flushPromises()
    expect(getConfig).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it.each(['payment_enabled', 'recharge_promotion_enabled'])('does not fetch while %s is disabled', async (key) => {
    state.app.cachedPublicSettings[key] = false
    const store = useRechargePromotionStore()
    await store.load()
    expect(store.enabled).toBe(false)
    expect(getConfig).not.toHaveBeenCalled()
  })
})
