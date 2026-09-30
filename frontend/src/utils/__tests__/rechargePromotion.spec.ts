import { describe, expect, it } from 'vitest'
import type { RechargePromotion } from '@/types/payment'
import { calculateRechargePromotion, isRechargePromotionActive } from '../rechargePromotion'

const now = Date.parse('2026-10-02T00:00:00+08:00')
const promotion: RechargePromotion = {
  enabled: true, active: true, title: 'Recharge rewards', subtitle: '',
  starts_at: '2026-10-01T00:00:00+08:00', ends_at: '2026-10-08T00:00:00+08:00',
  currency: 'CNY', max_bonus: 0,
  tiers: [{ min_amount: 300, bonus_percent: 12 }, { min_amount: 50, bonus_percent: 5 }, { min_amount: 100, bonus_percent: 10 }],
}

describe('recharge promotion preview', () => {
  it('takes only the highest eligible threshold and calculates on base credit, not the payment or fee', () => {
    expect(calculateRechargePromotion(promotion, 100, 0.25, 'CNY', now)).toEqual({
      baseAmount: 25, bonusAmount: 2.5, totalAmount: 27.5,
      tier: { min_amount: 100, bonus_percent: 10 },
    })
    expect(calculateRechargePromotion(promotion, 299, 1, 'CNY', now).bonusAmount).toBe(29.9)
    expect(calculateRechargePromotion(promotion, 300, 1, 'CNY', now).bonusAmount).toBe(36)
    expect(calculateRechargePromotion(promotion, 49.99, 1, 'CNY', now).bonusAmount).toBe(0)
  })

  it('caps bonuses in credited balance units and rounds before adding the final credit', () => {
    expect(calculateRechargePromotion({ ...promotion, max_bonus: 2 }, 100, 0.25, 'CNY', now).totalAmount).toBe(27)
    expect(calculateRechargePromotion(promotion, 100.01, 0.3333, 'CNY', now)).toMatchObject({ baseAmount: 33.33, bonusAmount: 3.33, totalAmount: 36.66 })
  })

  it('never awards a bonus for another currency or an invalid amount', () => {
    expect(calculateRechargePromotion(promotion, 300, 1, 'USD', now).bonusAmount).toBe(0)
    expect(calculateRechargePromotion(promotion, Number.NaN, 1, 'CNY', now).totalAmount).toBe(0)
    expect(calculateRechargePromotion(promotion, -100, 1, 'CNY', now).totalAmount).toBe(0)
  })

  it('starts inclusively and ends exclusively, regardless of a stale active snapshot', () => {
    expect(isRechargePromotionActive({ ...promotion, active: false }, Date.parse(promotion.starts_at))).toBe(true)
    expect(isRechargePromotionActive(promotion, Date.parse(promotion.ends_at))).toBe(false)
    expect(isRechargePromotionActive(promotion, Date.parse(promotion.starts_at) - 1)).toBe(false)
    expect(isRechargePromotionActive({ ...promotion, enabled: false }, now)).toBe(false)
    expect(isRechargePromotionActive({ ...promotion, starts_at: 'bad' }, now)).toBe(false)
    expect(calculateRechargePromotion(promotion, 300, 1, 'CNY', Date.parse(promotion.ends_at)).totalAmount).toBe(300)
  })
})
