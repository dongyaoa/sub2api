import type { ImageModelPricing, ImageResolutionTier } from './types'

export interface ImagePriceTier {
  tier: ImageResolutionTier
  basePrice: number
  unitPrice: number
  configured: boolean
}

// The server quote already includes the effective user/image multiplier.
// Never substitute flat group prices when a model quote is unavailable.
export function getImagePriceTiers(pricing: ImageModelPricing | null): ImagePriceTier[] {
  if (!pricing?.tiers) return []
  return (['1K', '2K', '4K'] as ImageResolutionTier[]).flatMap((tier) => {
    const price = pricing.tiers?.[tier]
    if (!price || !Number.isFinite(price.base_price) || !Number.isFinite(price.unit_price)) return []
    return [{
      tier,
      basePrice: price.base_price,
      unitPrice: price.unit_price,
      configured: pricing.pricing_source !== 'default',
    }]
  })
}

export function estimateImageCost(
  pricing: ImageModelPricing | null,
  tier: ImageResolutionTier,
  quantity: number,
): number | null {
  const selected = getImagePriceTiers(pricing).find((item) => item.tier === tier)
  return selected ? selected.unitPrice * Math.max(0, quantity) : null
}

export function formatUSD(value: number): string {
  const maximumFractionDigits = Math.abs(value) >= 1 ? 4 : 6
  return `$${new Intl.NumberFormat('en-US', {
    useGrouping: false,
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(value)}`
}
