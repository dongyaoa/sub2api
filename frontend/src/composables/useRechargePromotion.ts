import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useRechargePromotionStore } from '@/stores/rechargePromotion'
import { isRechargePromotionActive, useRechargePromotionClock } from '@/utils/rechargePromotion'

export function useRechargePromotion(fetchEnabled: MaybeRefOrGetter<boolean> = true) {
  const store = useRechargePromotionStore()
  const now = useRechargePromotionClock()
  watch(() => [store.enabled, toValue(fetchEnabled)], ([enabled, fetch]) => {
    if (enabled && fetch) void store.load()
  }, { immediate: true })
  const activePromotion = computed(() => store.enabled && isRechargePromotionActive(store.promotion, now.value)
    ? store.promotion
    : null)
  return { activePromotion }
}
