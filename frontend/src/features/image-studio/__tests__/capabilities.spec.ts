import { describe, expect, it } from 'vitest'
import {
  buildGenerateImageRequest,
  filterImageModels,
  getOpenAIImageSize,
  normalizeStudioSelection,
} from '../capabilities'

describe('image studio capabilities', () => {
  it('filters generation-only models by platform', () => {
    expect(filterImageModels('openai', [
      { id: 'gpt-5.4' },
      { id: 'gpt-image-2' },
      { id: 'gpt-image-2' },
    ])).toEqual([{ id: 'gpt-image-2' }])

    expect(filterImageModels('grok', [
      { id: 'grok-4.5' },
      { id: 'grok-imagine-image' },
      { id: 'grok-imagine-edit' },
      { id: 'grok-imagine-video' },
    ])).toEqual([{ id: 'grok-imagine-image' }])
  })

  it('maps OpenAI ratio and tier to supported image sizes', () => {
    expect(getOpenAIImageSize('1:1', '2K')).toBe('1024x1024')
    expect(getOpenAIImageSize('3:2', '2K')).toBe('1536x1024')
    expect(getOpenAIImageSize('2:3', '2K')).toBe('1024x1536')
  })

  it('normalizes unsupported options after platform changes', () => {
    expect(normalizeStudioSelection('openai', '16:9', '4K')).toEqual({
      ratio: '1:1',
      resolution: '1K',
    })
  })

  it('keeps Grok resolution for billing while forwarding native controls', () => {
    expect(buildGenerateImageRequest({
      platform: 'grok',
      model: 'grok-imagine-image',
      prompt: ' city at night ',
      quantity: 2,
      ratio: '16:9',
      resolution: '2K',
      quality: 'auto',
    })).toEqual({
      model: 'grok-imagine-image',
      prompt: 'city at night',
      n: 2,
      size: '2K',
      aspect_ratio: '16:9',
      resolution: '2k',
    })
  })
})
