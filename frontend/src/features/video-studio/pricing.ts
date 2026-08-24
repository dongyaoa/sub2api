import type { Group } from '@/types'
import { isVideo15Model } from './capabilities'
import type { VideoResolution } from './types'

export interface VideoPriceTier {
  resolution: VideoResolution
  basePrice: number
  unitPrice: number
  configured: boolean
}

const STANDARD_DEFAULTS: Record<VideoResolution, number> = {
  '480p': 0.05,
  '720p': 0.07,
  '1080p': 0.07,
}

const VIDEO_15_DEFAULTS: Record<VideoResolution, number> = {
  '480p': 0.08,
  '720p': 0.14,
  '1080p': 0.25,
}

function canonicalVideoPriceFamily(model: string): string {
  const normalized = model.trim().toLowerCase().replace(/^(xai|x-ai|grok)\//, '')
  if (normalized.startsWith('grok-imagine-video-1.5') || normalized === 'grok-video-1.5') {
    return 'grok-imagine-video-1.5'
  }
  if (normalized === 'grok-imagine-video' || normalized === 'grok-imagine-video-preview' || normalized === 'grok-video' || normalized === 'grok-video-latest') {
    return 'grok-imagine-video'
  }
  return normalized
}

function configuredPrice(group: Group, model: string, resolution: VideoResolution): number | null {
  const family = canonicalVideoPriceFamily(model)
  const familyPrices = Object.entries(group.video_model_prices || {})
    .find(([key]) => key.trim().toLowerCase() === family)?.[1]
  const override = familyPrices?.[resolution]
  if (override != null && Number.isFinite(Number(override))) return Math.max(0, Number(override))

  const flatKey = `video_price_${resolution}` as 'video_price_480p' | 'video_price_720p' | 'video_price_1080p'
  const value = group[flatKey]
  return value == null || !Number.isFinite(Number(value)) ? null : Math.max(0, Number(value))
}

export function getVideoRateMultiplier(group: Group, userRate?: number): number {
  if (group.video_rate_independent) return Math.max(0, Number(group.video_rate_multiplier) || 0)
  return Math.max(0, userRate == null ? Number(group.rate_multiplier) || 0 : userRate)
}

export function getDefaultVideoPrice(model: string, resolution: VideoResolution): number {
  return (isVideo15Model(model) ? VIDEO_15_DEFAULTS : STANDARD_DEFAULTS)[resolution]
}

export function getVideoPriceTiers(group: Group, model: string, userRate?: number): VideoPriceTier[] {
  const multiplier = getVideoRateMultiplier(group, userRate)
  return (['480p', '720p', '1080p'] as VideoResolution[]).map((resolution) => {
    const custom = configuredPrice(group, model, resolution)
    const basePrice = custom ?? getDefaultVideoPrice(model, resolution)
    return {
      resolution,
      basePrice,
      unitPrice: basePrice * multiplier,
      configured: custom != null,
    }
  })
}

export function estimateVideoCost(
  group: Group,
  model: string,
  resolution: VideoResolution,
  duration: number,
  userRate?: number,
): number {
  const tier = getVideoPriceTiers(group, model, userRate)
    .find((item) => item.resolution === resolution)
  return (tier?.unitPrice || 0) * Math.min(15, Math.max(1, Math.round(duration)))
}

export function formatUSD(value: number): string {
  const maximumFractionDigits = Math.abs(value) >= 1 ? 4 : 6
  return `$${new Intl.NumberFormat('en-US', {
    useGrouping: false,
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(value)}`
}
