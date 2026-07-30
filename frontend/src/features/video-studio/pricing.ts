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
  '720p': 0.05,
  '1080p': 0.05,
}

const VIDEO_15_DEFAULTS: Record<VideoResolution, number> = {
  '480p': 0.15,
  '720p': 0.15,
  '1080p': 0.15,
}

function configuredPrice(group: Group, model: string): number | null {
  const value = isVideo15Model(model) ? group.video_price_720p : group.video_price_480p
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
    const custom = configuredPrice(group, model)
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
