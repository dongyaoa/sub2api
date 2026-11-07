import { describe, expect, it } from 'vitest'
import type { VideoModelPricing } from '../types'
import type { Group } from '@/types'
import { estimateVideoCost, getVideoPriceTiers, resolveVideoStudioPricing } from '../pricing'

function quote(overrides: Partial<VideoModelPricing> = {}): VideoModelPricing {
  return {
    model: 'muse-video', billing_mode: 'per_request', pricing_source: 'group',
    rate_multiplier: 0.5,
    tiers: { auto: { base_price: 0.8, unit_price: 0.4, configured: true } },
    ...overrides,
  }
}

describe('video studio pricing', () => {
  it('restores Grok per-second group prices and user/video multipliers when quotes fail', () => {
    const group = { rate_multiplier: 2, video_price_480p: 9, video_model_prices: {
      'grok-imagine-video': { '480p': 0.06, '720p': 0.08, '1080p': 0.1 },
      'grok-imagine-video-1.5': { '480p': 0.12, '720p': 0.16, '1080p': 0.2 },
    } } as Group
    const pricing = resolveVideoStudioPricing(group, 'grok-imagine-video', null, 0.5)
    expect(pricing?.billing_mode).toBe('video')
    expect(estimateVideoCost(pricing, '480p', 4)).toBeCloseTo(0.12)
    expect(estimateVideoCost(pricing, '720p', 15)).toBeCloseTo(0.6)
    expect(resolveVideoStudioPricing({ ...group, video_rate_independent: true, video_rate_multiplier: 0.25 },
      'svggrok-imagine-video-1.5', null, 10)?.tiers?.['1080p']?.unit_price).toBe(0.05)
    expect(resolveVideoStudioPricing(group, 'grok-imagine-video', quote({ billing_mode: 'token' }))?.billing_mode).toBe('video')
  })

  it('keeps valid media quotes authoritative and never applies Grok defaults to other models', () => {
    const group = { rate_multiplier: 1 } as Group
    expect(resolveVideoStudioPricing(group, 'grok-imagine-video', quote())).toEqual(quote())
    const video = quote({ billing_mode: 'video' })
    expect(resolveVideoStudioPricing(group, 'grok-imagine-video', video)).toBe(video)
    expect(resolveVideoStudioPricing(group, 'muse-video', null)).toBeNull()
    expect(resolveVideoStudioPricing(group, 'gemini-omni-1.2-flash', null)).toBeNull()
    expect(resolveVideoStudioPricing(group, 'grok-imagine-video', null)?.tiers?.['480p']?.unit_price).toBe(0.05)
    expect(resolveVideoStudioPricing(group, 'svggrok-imagine-video-1.5', null)?.tiers?.['1080p']?.unit_price).toBe(0.25)
  })

  it('quotes Omni 360p and 4K using configured prices and the returned billing unit', () => {
    const pricing = quote({ model: 'gemini-omni-1.2-flash', tiers: {
      '360p': { base_price: 0.5, unit_price: 0.25, configured: true },
      '4k': { base_price: 2, unit_price: 1, configured: true },
    } })
    expect(estimateVideoCost(pricing, '360p', 15)).toBe(0.25)
    expect(estimateVideoCost(pricing, '4k', 15)).toBe(1)
    expect(estimateVideoCost({ ...pricing, billing_mode: 'video' }, '4k', 15)).toBe(15)
    expect(estimateVideoCost(pricing, '720p', 15)).toBeNull()
  })

  it('uses the resolved model-square price per request regardless of duration', () => {
    expect(estimateVideoCost(quote(), 'auto', 1)).toBe(0.4)
    expect(estimateVideoCost(quote(), 'auto', 60)).toBe(0.4)
    expect(getVideoPriceTiers(quote())).toEqual([
      { resolution: 'auto', basePrice: 0.8, unitPrice: 0.4, configured: true },
    ])
  })

  it('uses all requested seconds when the server returns per-second billing', () => {
    expect(estimateVideoCost(quote({ billing_mode: 'video' }), 'auto', 60)).toBe(24)
    expect(estimateVideoCost(quote({
      model: 'grok-imagine-video', billing_mode: 'video',
      tiers: { '720p': { base_price: 0.07, unit_price: 0.035, configured: true } },
    }), '720p', 10)).toBeCloseTo(0.35)
  })

  it('does not multiply the already resolved rate a second time', () => {
    expect(estimateVideoCost(quote({ rate_multiplier: 12 }), 'auto', 10)).toBe(0.4)
  })

  it('preserves legacy Grok image-mode prices as per-request billing', () => {
    expect(estimateVideoCost(quote({
      model: 'grok-imagine-video', billing_mode: 'image',
      tiers: { '720p': { base_price: 1, unit_price: 0.5, configured: true } },
    }), '720p', 15)).toBe(0.5)
  })

  it('does not invent prices for missing, unsupported, or invalid quotes', () => {
    expect(estimateVideoCost(null, 'auto', 10)).toBeNull()
    expect(estimateVideoCost(quote(), '720p', 10)).toBeNull()
    expect(getVideoPriceTiers(quote({ billing_mode: 'token' }))).toEqual([])
    expect(getVideoPriceTiers(quote({ tiers: null }))).toEqual([])
    expect(getVideoPriceTiers(quote({
      tiers: { auto: { base_price: 1, unit_price: Number.NaN, configured: true } },
    }))).toEqual([])
  })

  it('keeps intentionally free model prices', () => {
    expect(estimateVideoCost(quote({
      tiers: { auto: { base_price: 0, unit_price: 0, configured: true } },
    }), 'auto', 60)).toBe(0)
  })
})
