import type { RechargePromotion } from '@/types/payment'

export type RechargePromotionDraft = Omit<RechargePromotion, 'active'>

// datetime-local has no timezone. Always interpret this form's wall clock as Beijing time.
export function toBeijingInput(value: string): string {
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? new Date(timestamp + 8 * 3600_000).toISOString().slice(0, 16) : ''
}

export function fromBeijingInput(value: string): string {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return ''
  const timestamp = Date.parse(`${value}:00+08:00`)
  if (!Number.isFinite(timestamp)) return ''
  const result = new Date(timestamp).toISOString()
  return toBeijingInput(result) === value ? result : ''
}

export function defaultRechargePromotion(now = new Date()): RechargePromotionDraft {
  const beijing = new Date(now.getTime() + 8 * 3600_000)
  const start = Date.UTC(beijing.getUTCFullYear(), beijing.getUTCMonth(), beijing.getUTCDate() + 1) - 8 * 3600_000
  return {
    enabled: false,
    title: '充值加赠',
    subtitle: '',
    starts_at: new Date(start).toISOString(),
    ends_at: new Date(start + 7 * 86400_000).toISOString(),
    currency: 'CNY',
    tiers: [{ min_amount: 50, bonus_percent: 5 }, { min_amount: 100, bonus_percent: 10 }, { min_amount: 300, bonus_percent: 12 }],
    max_bonus: 0,
  }
}

export function writablePromotion(config: RechargePromotionDraft): RechargePromotionDraft {
  return {
    enabled: config.enabled, title: config.title.trim(), subtitle: config.subtitle.trim(),
    starts_at: config.starts_at, ends_at: config.ends_at, currency: config.currency,
    tiers: config.tiers.map(tier => ({ ...tier })).sort((a, b) => a.min_amount - b.min_amount),
    max_bonus: config.max_bonus,
  }
}

function isAmount(value: number): boolean {
  return Number.isFinite(value) && Math.abs(value * 100 - Math.round(value * 100)) < 0.000001
}

export function promotionValidationError(config: RechargePromotionDraft): string | null {
  if (!config.title.trim() || config.title.trim().length > 80 || config.subtitle.trim().length > 200) return 'invalidTitle'
  if (!Number.isFinite(Date.parse(config.starts_at)) || !Number.isFinite(Date.parse(config.ends_at)) || Date.parse(config.ends_at) <= Date.parse(config.starts_at)) return 'invalidDates'
  if (!/^[A-Z]{3}$/.test(config.currency)) return 'invalidCurrency'
  if (config.tiers.length < 1 || config.tiers.length > 10) return 'invalidTiers'
  const thresholds = new Set<number>()
  for (const tier of config.tiers) {
    if (!isAmount(tier.min_amount) || tier.min_amount <= 0 || tier.min_amount > 1e9 || !isAmount(tier.bonus_percent) || tier.bonus_percent <= 0 || tier.bonus_percent > 100) return 'invalidTier'
    if (thresholds.has(tier.min_amount)) return 'duplicateTier'
    thresholds.add(tier.min_amount)
  }
  if (!isAmount(config.max_bonus) || config.max_bonus < 0 || config.max_bonus > 1e9) return 'invalidCap'
  return null
}
