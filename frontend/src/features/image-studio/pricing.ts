import { BILLING_MODE_IMAGE } from '@/constants/channel'
import type { Group } from '@/types'
import { resolveEffectiveMultiplier } from '@/utils/modelSquare'
import type { ImageResolutionTier, ImageStudioPlatform } from './types'

export interface ImagePriceTier {
  tier: ImageResolutionTier
  basePrice: number
  unitPrice: number
  configured: boolean
}

const DEFAULT_OPENAI_PRICES: Record<ImageResolutionTier, number> = {
  '1K': 0.134,
  '2K': 0.201,
  '4K': 0.268,
}

const DEFAULT_GROK_PRICES: Record<ImageResolutionTier, number> = {
  '1K': 0.005,
  '2K': 0.005,
  '4K': 0.005,
}

const DEFAULT_GROK_QUALITY_PRICES: Record<ImageResolutionTier, number> = {
  '1K': 0.01,
  '2K': 0.01,
  '4K': 0.01,
}

const DEFAULT_GROK_20_PRICES: Record<ImageResolutionTier, number> = {
  '1K': 0.06,
  '2K': 0.08,
  '4K': 0.08,
}

function configuredPrice(group: Group, model: string, tier: ImageResolutionTier): number | null {
  const value = group.platform === 'grok'
    ? model.trim().toLowerCase() === 'grok-imagine-image-quality'
      ? group.image_price_2k
      : group.image_price_1k
    : tier === '1K'
      ? group.image_price_1k
      : tier === '2K'
        ? group.image_price_2k
        : group.image_price_4k
  return value == null || !Number.isFinite(Number(value)) ? null : Math.max(0, Number(value))
}

export function getDefaultImagePrice(
  platform: ImageStudioPlatform,
  model: string,
  tier: ImageResolutionTier,
): number {
  if (platform === 'grok') {
    if (model.trim().toLowerCase() === 'grok-imagine-image-2.0') return DEFAULT_GROK_20_PRICES[tier]
    return model.trim().toLowerCase() === 'grok-imagine-image-quality'
      ? DEFAULT_GROK_QUALITY_PRICES[tier]
      : DEFAULT_GROK_PRICES[tier]
  }
  return DEFAULT_OPENAI_PRICES[tier]
}

export function getImagePriceTiers(
  group: Group,
  model: string,
  userRate?: number,
): ImagePriceTier[] {
  const platform = group.platform === 'grok' ? 'grok' : 'openai'
  const multiplier = resolveEffectiveMultiplier(group, userRate, BILLING_MODE_IMAGE).value
  return (['1K', '2K', '4K'] as ImageResolutionTier[]).map((tier) => {
    const custom = configuredPrice(group, model, tier)
    const basePrice = custom ?? getDefaultImagePrice(platform, model, tier)
    return {
      tier,
      basePrice,
      unitPrice: basePrice * multiplier,
      configured: custom != null,
    }
  })
}

export function estimateImageCost(
  group: Group,
  model: string,
  tier: ImageResolutionTier,
  quantity: number,
  userRate?: number,
): number {
  const selected = getImagePriceTiers(group, model, userRate).find((item) => item.tier === tier)
  return (selected?.unitPrice ?? 0) * Math.max(0, quantity)
}

export function formatUSD(value: number): string {
  const maximumFractionDigits = Math.abs(value) >= 1 ? 4 : 6
  return `$${new Intl.NumberFormat('en-US', {
    useGrouping: false,
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(value)}`
}
