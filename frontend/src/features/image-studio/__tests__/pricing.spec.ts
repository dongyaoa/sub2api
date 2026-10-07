import { describe, expect, it } from 'vitest'
import type { ImageModelPricing } from '../types'
import { estimateImageCost, getImagePriceTiers } from '../pricing'

function quote(overrides: Partial<ImageModelPricing> = {}): ImageModelPricing {
  return {
    model: 'gemini-nano-banana-2.1',
    billing_mode: 'image',
    pricing_source: 'channel',
    rate_multiplier: 1,
    openai4k_allowed: false,
    tiers: {
      '1K': { base_price: 0.1, unit_price: 0.1 },
      '2K': { base_price: 0.2, unit_price: 0.2 },
      '4K': { base_price: 0.3, unit_price: 0.3 },
    },
    ...overrides,
  }
}

describe('image studio model pricing', () => {
  it('uses the server model quote and selected tier for multiple images', () => {
    expect(getImagePriceTiers(quote()).map((item) => item.unitPrice)).toEqual([0.1, 0.2, 0.3])
    expect(estimateImageCost(quote(), '2K', 3)).toBeCloseTo(0.6)
  })

  it('does not apply user or group multipliers a second time', () => {
    const target = quote({
      rate_multiplier: 1.5,
      tiers: {
        '1K': { base_price: 0.1, unit_price: 0.15 },
        '2K': { base_price: 0.2, unit_price: 0.3 },
        '4K': { base_price: 0.3, unit_price: 0.45 },
      },
    })
    expect(estimateImageCost(target, '1K', 2)).toBeCloseTo(0.3)
  })

  it('does not invent a flat price when quoting fails or billing is by tokens', () => {
    expect(getImagePriceTiers(null)).toEqual([])
    expect(estimateImageCost(null, '1K', 1)).toBeNull()
    expect(estimateImageCost(quote({ billing_mode: 'token', tiers: null }), '2K', 1)).toBeNull()
  })

  it('preserves explicitly free model pricing', () => {
    const free = quote({
      tiers: {
        '1K': { base_price: 0, unit_price: 0 },
        '2K': { base_price: 0, unit_price: 0 },
        '4K': { base_price: 0, unit_price: 0 },
      },
    })
    expect(estimateImageCost(free, '4K', 2)).toBe(0)
  })
})
