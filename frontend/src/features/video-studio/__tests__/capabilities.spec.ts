import { describe, expect, it } from 'vitest'
import {
  buildGenerateVideoRequest,
  filterVideoModels,
  getVideoResolutionOptions,
  selectPreferredVideoModel,
} from '../capabilities'

const models = [
  { id: 'grok-4.5' },
  { id: 'grok-imagine-video' },
  { id: 'grok-imagine-video-1.5' },
  { id: 'grok-imagine-video-1.5-preview' },
]

describe('video studio capabilities', () => {
  it('filters video models and chooses a model for each operation', () => {
    const filtered = filterVideoModels(models)
    expect(filtered.map((item) => item.id)).toEqual([
      'grok-imagine-video',
      'grok-imagine-video-1.5-preview',
    ])
    expect(selectPreferredVideoModel(filtered, 'text')).toBe('grok-imagine-video')
    expect(selectPreferredVideoModel(filtered, 'image')).toBe('grok-imagine-video-1.5-preview')
  })

  it('uses model-level pricing across all supported resolutions', () => {
    expect(getVideoResolutionOptions('grok-imagine-video')).toEqual(['480p', '720p', '1080p'])
    expect(getVideoResolutionOptions('grok-imagine-video-1.5-preview')).toEqual(['480p', '720p', '1080p'])
  })

  it('clamps duration and uses the gateway image object', () => {
    expect(buildGenerateVideoRequest({
      operation: 'image',
      model: 'grok-imagine-video-1.5-preview',
      prompt: ' animate ',
      resolution: '1080p',
      duration: 99,
      imageUrl: ' data:image/png;base64,aW1n ',
    })).toEqual({
      model: 'grok-imagine-video-1.5-preview',
      prompt: 'animate',
      resolution: '1080p',
      duration: 15,
      image: { url: 'data:image/png;base64,aW1n' },
    })
  })
})
