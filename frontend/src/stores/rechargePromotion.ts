import { defineStore } from 'pinia'
import { computed, shallowRef, watch } from 'vue'
import { paymentAPI } from '@/api/payment'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { CheckoutInfoResponse, RechargePromotion } from '@/types/payment'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'

/** Small shared cache for the sidebar and dashboard; checkout supplies its own data. */
export const useRechargePromotionStore = defineStore('rechargePromotion', () => {
  const app = useAppStore()
  const auth = useAuthStore()
  const promotion = shallowRef<RechargePromotion | null>(null)
  const enabled = computed(() => auth.isAuthenticated && !auth.isSimpleMode
    && resolveFeatureFlag(app.cachedPublicSettings, FeatureFlags.payment)
    && resolveFeatureFlag(app.cachedPublicSettings, FeatureFlags.rechargePromotion))
  let pending: Promise<void> | null = null
  let generation = 0
  let fetchedAt: number | null = null

  function clear() {
    generation += 1
    promotion.value = null
    pending = null
    fetchedAt = null
  }

  watch([() => auth.user?.id, () => auth.isAuthenticated, enabled], clear, { flush: 'sync' })

  async function load() {
    if (!enabled.value) return
    if (pending) return pending
    if (fetchedAt !== null && Date.now() - fetchedAt < 60_000) return
    const requestGeneration = generation
    pending = (async () => {
      try {
        const { data } = await paymentAPI.getConfig()
        if (requestGeneration !== generation) return
        promotion.value = data.enabled && !data.balance_disabled ? data.recharge_promotion ?? null : null
        fetchedAt = Date.now()
      } catch {
        if (requestGeneration === generation) {
          promotion.value = null
          fetchedAt = null
        }
      } finally {
        if (requestGeneration === generation) pending = null
      }
    })()
    return pending
  }

  function acceptCheckout(checkout: CheckoutInfoResponse) {
    clear()
    if (!enabled.value) return
    // The checkout API already removes the promotion when payment is disabled.
    promotion.value = checkout.balance_disabled ? null : checkout.recharge_promotion ?? null
    fetchedAt = Date.now()
  }

  return { promotion, enabled, load, clear, acceptCheckout }
})
