import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import { estimateVideoCost, getVideoPriceTiers } from '../pricing'

function group(overrides: Partial<Group> = {}): Group {
  return {
    id: 1,
    name: 'Grok video',
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
    image_rate_independent: false,
    image_rate_multiplier: 1,
    batch_image_discount_multiplier: 1,
    batch_image_hold_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
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

describe('video studio pricing', () => {
  it('calculates standard video cost per second with the user rate', () => {
    expect(estimateVideoCost(group(), 'grok-imagine-video', '720p', 10, 0.5)).toBeCloseTo(0.25)
  })

  it('uses the independent video multiplier and custom group price', () => {
    const target = group({
      video_rate_independent: true,
      video_rate_multiplier: 1.5,
      video_price_720p: 0.2,
    })
    expect(estimateVideoCost(target, 'grok-imagine-video-1.5-preview', '1080p', 5, 9)).toBeCloseTo(1.5)
  })

  it('uses video 1.5 defaults when group prices are absent', () => {
    const target = group({ rate_multiplier: 1 })
    expect(getVideoPriceTiers(target, 'grok-imagine-video-1.5-preview').map((item) => item.unitPrice))
      .toEqual([0.15, 0.15, 0.15])
  })
})
