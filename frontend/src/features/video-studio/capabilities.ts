import type {
  GenerateVideoRequest,
  VideoAspectRatio,
  VideoModel,
  VideoResolution,
  VideoStudioOperation,
  VideoReferenceMode,
} from './types'

export const STANDARD_VIDEO_MODEL = 'grok-imagine-video'
export const IMAGE_VIDEO_MODEL = 'grok-imagine-video-1.5'
export const SVG_IMAGE_VIDEO_MODEL = 'svggrok-imagine-video-1.5'
export const LEGACY_IMAGE_VIDEO_MODEL = 'grok-imagine-video-1.5-preview'
export const MUSE_VIDEO_MODEL = 'muse-video'
export const GEMINI_OMNI_VIDEO_MODEL = 'gemini-omni-1.2-flash'
const GEMINI_OMNI_MODELS = new Set([GEMINI_OMNI_VIDEO_MODEL, 'gemini-omni-1.2', 'omni-1.2', 'omni-featured'])
export const DEFAULT_VIDEO_ASPECT_RATIO: VideoAspectRatio = '16:9'
export const VIDEO_ASPECT_RATIOS: VideoAspectRatio[] = ['16:9', '9:16', '1:1', '4:3', '3:4', '3:2', '2:3']

export function isVideoGenerationModel(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  return id === STANDARD_VIDEO_MODEL || id === IMAGE_VIDEO_MODEL || id === SVG_IMAGE_VIDEO_MODEL || id === MUSE_VIDEO_MODEL || isGeminiOmniVideoModel(id)
}

export function isGeminiOmniVideoModel(modelId: string): boolean {
  return GEMINI_OMNI_MODELS.has(modelId.trim().toLowerCase())
}

export function filterVideoModels(models: VideoModel[]): VideoModel[] {
  const seen = new Set<string>()
  return models.filter((item) => {
    const id = item.id?.trim()
    if (!id || seen.has(id) || !isVideoGenerationModel(id)) return false
    seen.add(id)
    return true
  })
}

export function isVideo15Model(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  return id === IMAGE_VIDEO_MODEL || id === LEGACY_IMAGE_VIDEO_MODEL || id === SVG_IMAGE_VIDEO_MODEL
}

export function isGrokVideoModel(modelId: string): boolean {
  return modelId.trim().toLowerCase() === STANDARD_VIDEO_MODEL || isVideo15Model(modelId)
}

export function getVideoResolutionOptions(modelId: string): VideoResolution[] {
  if (isMuseVideoModel(modelId)) return ['auto']
  if (isGeminiOmniVideoModel(modelId)) return ['360p', '720p', '1080p', '4k']
  return isVideo15Model(modelId) ? ['480p', '720p', '1080p'] : ['480p', '720p']
}

export function isMuseVideoModel(modelId: string): boolean {
  return modelId.trim().toLowerCase() === MUSE_VIDEO_MODEL
}

export function getVideoMaxDuration(modelId: string): number {
  return isMuseVideoModel(modelId) ? 60 : 15
}

export function getVideoMinDuration(modelId: string): number {
  return isGeminiOmniVideoModel(modelId) ? 3 : 1
}

export function getVideoAspectRatioOptions(modelId: string): VideoAspectRatio[] {
  return isGeminiOmniVideoModel(modelId) ? ['16:9', '9:16', 'auto'] : VIDEO_ASPECT_RATIOS
}

export function normalizeVideoDuration(modelId: string, duration: number): number {
  return Math.min(getVideoMaxDuration(modelId), Math.max(getVideoMinDuration(modelId), Math.round(Number(duration) || 1)))
}

export function selectPreferredVideoModel(
  models: VideoModel[],
  operation: VideoStudioOperation,
  currentModel = '',
): string {
  if (models.some((item) => item.id === currentModel)) {
    if (operation === 'image' || !isVideo15Model(currentModel)) return currentModel
  }
  if (operation === 'image') {
    return models.find((item) => isVideo15Model(item.id))?.id || models[0]?.id || ''
  }
  return models.find((item) => item.id === STANDARD_VIDEO_MODEL)?.id
    || models.find((item) => !isVideo15Model(item.id))?.id
    || models[0]?.id
    || ''
}

export function normalizeVideoResolution(modelId: string, resolution: VideoResolution): VideoResolution {
  const options = getVideoResolutionOptions(modelId)
  return options.includes(resolution) ? resolution : isGeminiOmniVideoModel(modelId) ? '720p' : options[0]
}

export function normalizeVideoAspectRatio(aspectRatio: string | undefined, modelId = ''): VideoAspectRatio {
  return getVideoAspectRatioOptions(modelId).includes(aspectRatio as VideoAspectRatio)
    ? aspectRatio as VideoAspectRatio
    : DEFAULT_VIDEO_ASPECT_RATIO
}

export function geminiOmniVideoSize(resolution: VideoResolution, aspectRatio: VideoAspectRatio): string | undefined {
  if (aspectRatio === 'auto') return undefined
  const sizes: Partial<Record<VideoResolution, [number, number]>> = {
    '360p': [640, 360], '720p': [1280, 720], '1080p': [1920, 1080], '4k': [3840, 2160],
  }
  const dimensions = sizes[resolution]
  if (!dimensions) return undefined
  return (aspectRatio === '9:16' ? [...dimensions].reverse() : dimensions).join('x')
}

export function buildGenerateVideoRequest(input: {
  operation: VideoStudioOperation
  model: string
  prompt: string
  resolution: VideoResolution
  aspectRatio: VideoAspectRatio
  duration: number
  imageUrl?: string
  referenceMode?: VideoReferenceMode
  lastFrameUrl?: string
  imageReferences?: string[]
  videoReferences?: string[]
  seed?: number
}): GenerateVideoRequest {
  const payload: GenerateVideoRequest = {
    model: input.model.trim(),
    prompt: input.prompt.trim(),
    resolution: normalizeVideoResolution(input.model, input.resolution),
    aspect_ratio: normalizeVideoAspectRatio(input.aspectRatio, input.model),
    duration: normalizeVideoDuration(input.model, input.duration),
  }
  const imageUrl = input.imageUrl?.trim()
  if (isGeminiOmniVideoModel(input.model)) {
    const size = geminiOmniVideoSize(payload.resolution, payload.aspect_ratio!)
    if (size) payload.image_size = size
    if (input.seed != null && Number.isSafeInteger(input.seed)) payload.seed = input.seed
    if (input.operation === 'image') {
      if (input.referenceMode === 'references') {
        if (input.imageReferences?.length) payload.image_references = [...input.imageReferences]
        if (input.videoReferences?.length) payload.video_references = [...input.videoReferences]
      } else if (imageUrl) {
        payload.first_frame = imageUrl
        if (input.lastFrameUrl?.trim()) payload.last_frame = input.lastFrameUrl.trim()
      }
    }
    return payload
  }
  if (input.operation === 'image' && imageUrl) {
    payload.image = { url: imageUrl }
  }
  return payload
}
