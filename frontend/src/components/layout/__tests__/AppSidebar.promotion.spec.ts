import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick, ref, type Ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import { useRechargePromotionStore } from '@/stores/rechargePromotion'
import type { PublicSettings, User } from '@/types'
import type { CheckoutInfoResponse, RechargePromotion } from '@/types/payment'
import AppSidebar from '../AppSidebar.vue'

const state = vi.hoisted(() => ({ locale: undefined as unknown as Ref<string> }))
const getConfig = vi.hoisted(() => vi.fn())
vi.mock('vue-i18n', async (importOriginal) => {
  const { ref } = await import('vue')
  state.locale = ref('zh')
  return { ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key, locale: state.locale }) }
})
vi.mock('@/api/payment', () => ({ paymentAPI: { getConfig } }))
vi.mock('@/api/pelicanMonitor', () => ({
  pelicanMonitorAPI: { config: async () => ({ enabled: false, title: '', description: '', notice: '' }) },
}))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
}))
const promotion: RechargePromotion = {
  enabled: true, active: true, title: '国庆加赠', subtitle: '', currency: 'CNY', max_bonus: 0,
  starts_at: '2026-10-01T00:00:00+08:00', ends_at: '2026-10-08T00:00:00+08:00', tiers: [{ min_amount: 100, bonus_percent: 10 }],
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-02T00:00:00+08:00'))
  state.locale.value = 'zh'
  getConfig.mockReset().mockResolvedValue({ data: { enabled: true, balance_disabled: false, recharge_promotion: promotion } })
})
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

async function renderSidebar(role: 'user' | 'admin', path = '/dashboard') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  app.cachedPublicSettings = { payment_enabled: true, recharge_promotion_enabled: true } as PublicSettings
  const auth = useAuthStore()
  auth.user = { id: 1, role } as User
  auth.token = 'test-token'
  vi.spyOn(useAdminSettingsStore(), 'fetch').mockResolvedValue(undefined)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push(path)
  const wrapper = mount(AppSidebar, { global: { plugins: [pinia, router], stubs: { VersionBadge: true, Icon: true } } })
  await flushPromises()
  return { wrapper, app }
}

describe('Recharge promotion navigation badge', () => {
  it.each(['user', 'admin'] as const)('shows a localized badge beside the personal purchase entry for %s', async (role) => {
    const { wrapper, app } = await renderSidebar(role)
    try {
      const purchase = wrapper.get('a[href="/purchase"]')
      expect(purchase.get('.sidebar-promotion-badge').text()).toBe('活动')
      expect(wrapper.findAll('.sidebar-promotion-badge')).toHaveLength(1)
      state.locale.value = 'en'
      await nextTick()
      expect(purchase.get('.sidebar-promotion-badge').text()).toBe('BONUS')
      app.sidebarCollapsed = true
      await nextTick()
      expect(wrapper.find('.sidebar-promotion-badge').exists()).toBe(false)
      app.sidebarCollapsed = false
      app.cachedPublicSettings!.recharge_promotion_enabled = false
      await nextTick()
      expect(wrapper.find('.sidebar-promotion-badge').exists()).toBe(false)
    } finally { wrapper.unmount() }
  })

  it('uses the checkout result on the purchase page without fetching config', async () => {
    const { wrapper } = await renderSidebar('user', '/purchase')
    try {
      expect(getConfig).not.toHaveBeenCalled()
      expect(wrapper.find('.sidebar-promotion-badge').exists()).toBe(false)
      useRechargePromotionStore().acceptCheckout({ balance_disabled: false, recharge_promotion: promotion } as CheckoutInfoResponse)
      await nextTick()
      expect(wrapper.get('.sidebar-promotion-badge').text()).toBe('活动')
    } finally { wrapper.unmount() }
  })
})
