import { describe, expect, it } from 'vitest'
import {
  buildGenerateVideoRequest,
  filterVideoModels,
  getVideoResolutionOptions,
  getVideoMaxDuration,
  getVideoMinDuration,
  getVideoAspectRatioOptions,
  normalizeVideoAspectRatio,
  selectPreferredVideoModel,
} from '../capabilities'

const models = [
  { id: 'grok-4.5' },
  { id: 'grok-imagine-video' },
  { id: 'grok-imagine-video-1.5' },
  { id: 'grok-imagine-video-1.5-preview' },
]

describe('video studio capabilities', () => {
  it('recognizes Gemini Omni and its aliases with the full documented output range', () => {
    const aliases = ['gemini-omni-1.2-flash', 'gemini-omni-1.2', 'omni-1.2', 'omni-featured']
    expect(filterVideoModels([...aliases.map(id => ({ id })), { id: 'gemini-flash' }]).map(item => item.id)).toEqual(aliases)
    for (const model of aliases) {
      expect(getVideoMinDuration(model)).toBe(3)
      expect(getVideoMaxDuration(model)).toBe(15)
      expect(getVideoResolutionOptions(model)).toEqual(['360p', '720p', '1080p', '4k'])
      expect(getVideoAspectRatioOptions(model)).toEqual(['16:9', '9:16', 'auto'])
    }
  })

  it('builds matching Omni dimensions and leaves auto framing to the provider', () => {
    const input = { operation: 'text' as const, model: 'gemini-omni-1.2-flash', prompt: 'camera motion', resolution: '4k' as const, aspectRatio: '9:16' as const, duration: 20 }
    expect(buildGenerateVideoRequest(input)).toMatchObject({ resolution: '4k', aspect_ratio: '9:16', image_size: '2160x3840', duration: 15 })
    expect(buildGenerateVideoRequest({ ...input, aspectRatio: 'auto', duration: 1 })).toEqual({ model: input.model, prompt: input.prompt, resolution: '4k', aspect_ratio: 'auto', duration: 3 })
    expect(buildGenerateVideoRequest({ ...input, resolution: '480p', aspectRatio: '1:1' })).toMatchObject({ resolution: '720p', aspect_ratio: '16:9', image_size: '1280x720' })
  })

  it('keeps Omni frame and multiple-reference modes mutually exclusive and supports seed zero', () => {
    const input = { operation: 'image' as const, model: 'gemini-omni-1.2-flash', prompt: 'animate', resolution: '360p' as const, aspectRatio: '16:9' as const, duration: 5, imageUrl: 'https://example.com/first.png', lastFrameUrl: 'https://example.com/last.png', imageReferences: ['https://example.com/subject.png'], videoReferences: ['https://example.com/motion.mp4'], seed: 0 }
    const frames = buildGenerateVideoRequest(input)
    expect(frames).toMatchObject({ first_frame: input.imageUrl, last_frame: input.lastFrameUrl, seed: 0, image_size: '640x360' })
    expect(frames.image_references).toBeUndefined()
    const references = buildGenerateVideoRequest({ ...input, referenceMode: 'references' })
    expect(references).toMatchObject({ image_references: input.imageReferences, video_references: input.videoReferences })
    expect(references.first_frame).toBeUndefined()
    expect(references.last_frame).toBeUndefined()
  })

  it('filters video models and chooses a model for each operation', () => {
    const filtered = filterVideoModels(models)
    expect(filtered.map((item) => item.id)).toEqual([
      'grok-imagine-video',
      'grok-imagine-video-1.5',
    ])
    expect(selectPreferredVideoModel(filtered, 'text')).toBe('grok-imagine-video')
    expect(selectPreferredVideoModel(filtered, 'image')).toBe('grok-imagine-video-1.5')
  })

  it('recognizes the configured SVG Grok 1.5 alias without accepting arbitrary video names', () => {
    expect(filterVideoModels([{ id: 'svggrok-imagine-video-1.5' }, { id: 'random-video' }])).toEqual([{ id: 'svggrok-imagine-video-1.5' }])
    expect(getVideoResolutionOptions('svggrok-imagine-video-1.5')).toEqual(['480p', '720p', '1080p'])
    expect(selectPreferredVideoModel([{ id: 'svggrok-imagine-video-1.5' }], 'image')).toBe('svggrok-imagine-video-1.5')
  })

  it('uses model-level pricing across all supported resolutions', () => {
    expect(getVideoResolutionOptions('grok-imagine-video')).toEqual(['480p', '720p'])
    expect(getVideoResolutionOptions('grok-imagine-video-1.5')).toEqual(['480p', '720p', '1080p'])
    expect(getVideoResolutionOptions('grok-imagine-video-1.5-preview')).toEqual(['480p', '720p', '1080p'])
  })

  it('clamps duration and uses the gateway image object', () => {
    expect(buildGenerateVideoRequest({
      operation: 'image',
      model: 'grok-imagine-video-1.5-preview',
      prompt: ' animate ',
      resolution: '1080p',
      aspectRatio: '3:2',
      duration: 99,
      imageUrl: ' data:image/png;base64,aW1n ',
    })).toEqual({
      model: 'grok-imagine-video-1.5-preview',
      prompt: 'animate',
      resolution: '1080p',
      aspect_ratio: '3:2',
      duration: 15,
      image: { url: 'data:image/png;base64,aW1n' },
    })
  })

  it('supports every documented Grok video aspect ratio and defaults invalid values', () => {
    for (const ratio of ['1:1', '16:9', '9:16', '4:3', '3:4', '3:2', '2:3']) {
      expect(normalizeVideoAspectRatio(ratio)).toBe(ratio)
    }
    expect(normalizeVideoAspectRatio('auto')).toBe('16:9')
    expect(normalizeVideoAspectRatio(undefined)).toBe('16:9')
  })

  it('offers Muse for both creation modes with up to 60 seconds and no unsupported resolution', () => {
    const available = filterVideoModels([{ id: 'muse-chat' }, { id: 'muse-video' }, { id: 'muse-video' }])
    expect(available).toEqual([{ id: 'muse-video' }])
    expect(selectPreferredVideoModel(available, 'text')).toBe('muse-video')
    expect(selectPreferredVideoModel(available, 'image')).toBe('muse-video')
    expect(getVideoMaxDuration('muse-video')).toBe(60)
    expect(getVideoMaxDuration('grok-imagine-video')).toBe(15)
    expect(getVideoResolutionOptions('muse-video')).toEqual(['auto'])
    expect(buildGenerateVideoRequest({
      operation: 'image', model: 'muse-video', prompt: ' animate ',
      resolution: '1080p', aspectRatio: '9:16', duration: 90,
      imageUrl: 'data:image/png;base64,aW1n',
    })).toEqual({
      model: 'muse-video', prompt: 'animate', resolution: 'auto', aspect_ratio: '9:16', duration: 60,
      image: { url: 'data:image/png;base64,aW1n' },
    })
  })
})
