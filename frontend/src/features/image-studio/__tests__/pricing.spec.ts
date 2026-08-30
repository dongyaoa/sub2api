import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import { estimateImageCost, getImagePriceTiers } from '../pricing'

function group(overrides: Partial<Group> = {}): Group {
  return {
    id: 1,
    name: 'Image',
    description: null,
    platform: 'grok',
    rate_multiplier: 1.2,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    allow_image_generation: true,
    allow_batch_image_generation: false,
    image_rate_independent: true,
    image_rate_multiplier: 1.5,
    batch_image_discount_multiplier: 1,
    batch_image_hold_multiplier: 1,
    image_price_1k: 0.02,
    image_price_2k: 0.03,
    image_price_4k: 0.04,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null,
    video_price_720p: null,
    video_price_1080p: null,
    web_search_price_per_call: null,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

describe('image studio pricing', () => {
  it('uses the standard-model group price for every resolution', () => {
    expect(getImagePriceTiers(group(), 'grok-imagine-image').map((item) => item.unitPrice))
      .toEqual([0.03, 0.03, 0.03])
    expect(getImagePriceTiers(group(), 'grok-imagine-image-quality').map((item) => item.unitPrice))
      .toEqual([0.045, 0.045, 0.045])
  })

  it('uses the user rate when image billing is not independent', () => {
    const target = group({ image_rate_independent: false, image_rate_multiplier: 9 })
    expect(estimateImageCost(target, 'grok-imagine-image', '2K', 3, 0.5)).toBeCloseTo(0.03)
  })

  it('uses model defaults for missing prices', () => {
    const target = group({
      image_rate_independent: false,
      rate_multiplier: 1,
      image_price_1k: null,
      image_price_2k: null,
      image_price_4k: null,
    })
    expect(getImagePriceTiers(target, 'grok-imagine-image-quality').map((item) => item.unitPrice))
      .toEqual([0.01, 0.01, 0.01])
    expect(getImagePriceTiers(target, 'grok-imagine-image-2.0').map((item) => item.unitPrice))
      .toEqual([0.06, 0.08, 0.08])
  })

  it('uses Gemini model-square image prices before flat group prices', () => {
    const target = group({
      platform: 'gemini',
      image_price_1k: 0.9,
      image_price_2k: 0.9,
      image_price_4k: 0.9,
      image_rate_independent: false,
      rate_multiplier: 1,
    })
    const modelPricing = {
      billing_mode: 'image' as const,
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: 0.11,
      intervals: [
        { min_tokens: 0, max_tokens: null, tier_label: '2K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.22 },
      ],
    }
    expect(getImagePriceTiers(target, 'gemini-3-pro-image', undefined, modelPricing).map((item) => item.unitPrice))
      .toEqual([0.11, 0.22, 0.11])
    expect(estimateImageCost(target, 'gemini-3-pro-image', '2K', 2, undefined, modelPricing)).toBeCloseTo(0.44)
  })
})
