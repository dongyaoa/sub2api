import { onMounted, onUnmounted, ref } from 'vue'
import type { RechargePromotion, RechargePromotionTier } from '@/types/payment'
import { normalizePaymentCurrency } from '@/components/payment/currency'

export function isRechargePromotionActive(promotion: RechargePromotion | null | undefined, now = Date.now()): promotion is RechargePromotion {
  if (!promotion?.enabled || !promotion.tiers.length) return false
  const start = Date.parse(promotion.starts_at)
  const end = Date.parse(promotion.ends_at)
  // `active` is the server's snapshot. Check the window again so an open page
  // automatically enters/exits a scheduled event without retaining stale rewards.
  return Number.isFinite(start) && Number.isFinite(end) && now >= start && now < end
}

export function sortedPromotionTiers(promotion: RechargePromotion): RechargePromotionTier[] {
  return [...promotion.tiers].sort((a, b) => a.min_amount - b.min_amount)
}

function roundBalance(amount: number): number {
  return Math.round((amount + Number.EPSILON) * 100) / 100
}

export function calculateRechargePromotion(
  promotion: RechargePromotion | null | undefined,
  amount: number,
  multiplier: number,
  currency: string,
  now = Date.now(),
) {
  const safeAmount = Number.isFinite(amount) && amount > 0 ? amount : 0
  const safeMultiplier = Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
  const baseAmount = roundBalance(safeAmount * safeMultiplier)
  let tier: RechargePromotionTier | undefined
  if (isRechargePromotionActive(promotion, now)
    && normalizePaymentCurrency(promotion.currency) === normalizePaymentCurrency(currency)) {
    tier = sortedPromotionTiers(promotion).filter(item => safeAmount >= item.min_amount).slice(-1)[0]
  }
  let bonusAmount = tier ? roundBalance(baseAmount * tier.bonus_percent / 100) : 0
  if (promotion && promotion.max_bonus > 0) bonusAmount = Math.min(bonusAmount, promotion.max_bonus)
  return { baseAmount, bonusAmount, totalAmount: roundBalance(baseAmount + bonusAmount), tier }
}

/** Keep both displayed rewards and event visibility current while the page is open. */
export function useRechargePromotionClock() {
  const now = ref(Date.now())
  let timer: ReturnType<typeof setInterval> | undefined
  onMounted(() => {
    now.value = Date.now()
    timer = setInterval(() => { now.value = Date.now() }, 1000)
  })
  onUnmounted(() => { if (timer !== undefined) clearInterval(timer) })
  return now
}
