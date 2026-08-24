import { describe, expect, it } from 'vitest'
import {
  buildGenerateImageRequest,
  filterImageModels,
  getAspectRatioOptions,
  getMaxImageQuantity,
  getMaxSourceImageQuantity,
  getOpenAIImageSize,
  getPreferredImageModel,
  getResolutionOptions,
  normalizeStudioSelection,
  supportsImageEditing,
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
      { id: 'grok-imagine-image-2.0' },
      { id: 'grok-imagine-edit' },
      { id: 'grok-imagine-video' },
    ])).toEqual([{ id: 'grok-imagine-image' }, { id: 'grok-imagine-image-2.0' }])

    expect(filterImageModels('gemini', [
      { id: 'gemini-2.5-flash-image' },
      { id: 'gemini-3.1-flash-image' },
      { id: 'gemini-3-pro-image-preview' },
      { id: 'gemini-3-pro-preview' },
    ])).toEqual([
      { id: 'gemini-3.1-flash-image' },
      { id: 'gemini-3-pro-image-preview' },
    ])
  })

  it('maps OpenAI ratio and tier to supported image sizes', () => {
    expect(getOpenAIImageSize('1:1', '2K')).toBe('1024x1024')
    expect(getOpenAIImageSize('3:2', '2K')).toBe('1536x1024')
    expect(getOpenAIImageSize('2:3', '2K')).toBe('1024x1536')
    expect(getOpenAIImageSize('16:9', '2K')).toBe('2048x1152')
    expect(getOpenAIImageSize('9:16', '2K')).toBe('1152x2048')
    expect(getOpenAIImageSize('4:3', '2K')).toBe('1536x1152')
    expect(getOpenAIImageSize('3:4', '2K')).toBe('1152x1536')
    expect(getOpenAIImageSize('21:9', '2K')).toBe('1792x768')
    expect(getOpenAIImageSize('1:1', '4K')).toBe('2880x2880')
    expect(getOpenAIImageSize('3:2', '4K')).toBe('3456x2304')
    expect(getOpenAIImageSize('2:3', '4K')).toBe('2304x3456')
    expect(getOpenAIImageSize('16:9', '4K')).toBe('3840x2160')
    expect(getOpenAIImageSize('9:16', '4K')).toBe('2160x3840')
    expect(getOpenAIImageSize('4:3', '4K')).toBe('3072x2304')
    expect(getOpenAIImageSize('3:4', '4K')).toBe('2304x3072')
    expect(getOpenAIImageSize('21:9', '4K')).toBe('3584x1536')
  })

  it('offers the extended OpenAI aspect ratios in studio order', () => {
    expect(getAspectRatioOptions('openai').map((option) => option.value)).toEqual([
      '1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9',
    ])
  })

  it('keeps every OpenAI 4K size within the upstream pixel limits', () => {
    for (const { value: ratio } of getAspectRatioOptions('openai')) {
      const [width, height] = getOpenAIImageSize(ratio, '4K').split('x').map(Number)
      expect(Math.max(width, height)).toBeLessThanOrEqual(3840)
      expect(width * height).toBeLessThanOrEqual(8_294_400)
    }
  })

  it('enables OpenAI 4K only for configured groups', () => {
    expect(getResolutionOptions('openai')).toEqual(['1K', '2K'])
    expect(getResolutionOptions('openai', true)).toEqual(['1K', '2K', '4K'])
    expect(normalizeStudioSelection('openai', '1:1', '4K', true)).toEqual({
      ratio: '1:1',
      resolution: '4K',
    })
  })

  it('limits Grok image generation to 1K and 2K', () => {
    expect(getResolutionOptions('grok')).toEqual(['1K', '2K'])
    expect(normalizeStudioSelection('grok', '1:1', '4K')).toEqual({
      ratio: '1:1',
      resolution: '1K',
    })
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

  it('builds configured OpenAI 4K requests with the selected aspect ratio', () => {
    expect(buildGenerateImageRequest({
      platform: 'openai',
      model: 'gpt-image-2',
      prompt: ' detailed landscape ',
      quantity: 1,
      ratio: '16:9',
      resolution: '4K',
      quality: 'high',
      allowOpenAI4K: true,
    })).toEqual({
      model: 'gpt-image-2',
      prompt: 'detailed landscape',
      n: 1,
      size: '3840x2160',
      quality: 'high',
    })
  })

  it('enables image editing for Grok and Gemini and limits Gemini to one image', () => {
    expect(supportsImageEditing('grok')).toBe(true)
    expect(supportsImageEditing('gemini')).toBe(true)
    expect(getMaxImageQuantity('gemini')).toBe(1)
    expect(getMaxSourceImageQuantity('openai')).toBe(4)
    expect(getMaxSourceImageQuantity('grok')).toBe(4)
    expect(getMaxSourceImageQuantity('gemini')).toBe(1)
    expect(getPreferredImageModel('gemini')).toBe('gemini-3.1-flash-image')
  })

  it('builds Gemini native image controls through the common task request', () => {
    expect(buildGenerateImageRequest({
      platform: 'gemini',
      model: 'gemini-3-pro-image-preview',
      prompt: ' edit this ',
      quantity: 4,
      ratio: '3:2',
      resolution: '4K',
      quality: 'auto',
    })).toEqual({
      model: 'gemini-3-pro-image-preview',
      prompt: 'edit this',
      n: 1,
      size: '4K',
      aspect_ratio: '3:2',
      resolution: '4k',
    })
  })
})
