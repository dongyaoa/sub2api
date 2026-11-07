import type { VideoModelPricing, VideoResolution } from './types'
import type { Group } from '@/types'
import { isGrokVideoModel, isVideo15Model } from './capabilities'

export interface VideoPriceTier {
  resolution: VideoResolution
  basePrice: number
  unitPrice: number
  configured: boolean
}

const GROK_RESOLUTIONS = ['480p', '720p', '1080p'] as const
const STANDARD_DEFAULTS = { '480p': 0.05, '720p': 0.07, '1080p': 0.07 }
const VIDEO_15_DEFAULTS = { '480p': 0.08, '720p': 0.14, '1080p': 0.25 }

export function getVideoRateMultiplier(group: Group, userRate?: number): number {
  if (group.video_rate_independent) return Math.max(0, Number(group.video_rate_multiplier) || 0)
  return Math.max(0, userRate == null ? Number(group.rate_multiplier) || 0 : userRate)
}

function configuredGrokPrice(group: Group, model: string, resolution: typeof GROK_RESOLUTIONS[number]): number | null {
  const family = isVideo15Model(model) ? 'grok-imagine-video-1.5' : 'grok-imagine-video'
  const familyPrices = Object.entries(group.video_model_prices || {})
    .find(([key]) => key.trim().toLowerCase() === family)?.[1]
  const override = familyPrices?.[resolution]
  if (override != null && Number.isFinite(Number(override))) return Math.max(0, Number(override))
  const value = group[`video_price_${resolution}`]
  return value == null || !Number.isFinite(Number(value)) ? null : Math.max(0, Number(value))
}

// Restore Grok's original group/default per-second prices when the quote is
// unavailable or is a generic token card. Explicit media quotes stay authoritative.
export function resolveVideoStudioPricing(
  group: Group | null,
  model: string,
  pricing: VideoModelPricing | null,
  userRate?: number,
): VideoModelPricing | null {
  if (!group || !isGrokVideoModel(model)
    || (pricing && ['per_request', 'image', 'video'].includes(pricing.billing_mode))) return pricing
  const multiplier = getVideoRateMultiplier(group, userRate)
  const defaults = isVideo15Model(model) ? VIDEO_15_DEFAULTS : STANDARD_DEFAULTS
  const tiers: VideoModelPricing['tiers'] = {}
  let configured = false
  for (const resolution of GROK_RESOLUTIONS) {
    const custom = configuredGrokPrice(group, model, resolution)
    const basePrice = custom ?? defaults[resolution]
    configured ||= custom != null
    tiers[resolution] = { base_price: basePrice, unit_price: basePrice * multiplier, configured: custom != null }
  }
  return { model, billing_mode: 'video', pricing_source: configured ? 'group' : 'default', rate_multiplier: multiplier, tiers }
}

export function isVideoPerRequestPricing(pricing: VideoModelPricing | null): boolean {
  return pricing?.billing_mode === 'per_request' || pricing?.billing_mode === 'image'
}

export function getVideoPriceTiers(pricing: VideoModelPricing | null): VideoPriceTier[] {
  if (!pricing?.tiers || !['per_request', 'image', 'video'].includes(pricing.billing_mode)) return []
  return (['auto', '360p', '480p', '720p', '1080p', '4k'] as VideoResolution[]).flatMap(resolution => {
    const tier = pricing.tiers?.[resolution]
    if (!tier || !Number.isFinite(tier.unit_price) || tier.unit_price < 0
      || !Number.isFinite(tier.base_price) || tier.base_price < 0) return []
    return [{ resolution, basePrice: tier.base_price, unitPrice: tier.unit_price, configured: tier.configured }]
  })
}

export function estimateVideoCost(
  pricing: VideoModelPricing | null,
  resolution: VideoResolution,
  duration: number,
): number | null {
  const tier = getVideoPriceTiers(pricing)
    .find((item) => item.resolution === resolution)
  if (!tier) return null
  return tier.unitPrice * (isVideoPerRequestPricing(pricing) ? 1 : Math.max(1, Math.round(duration)))
}

export function formatUSD(value: number): string {
  const maximumFractionDigits = Math.abs(value) >= 1 ? 4 : 6
  return `$${new Intl.NumberFormat('en-US', {
    useGrouping: false,
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(value)}`
}
