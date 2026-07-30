import type {
  GenerateVideoRequest,
  VideoModel,
  VideoResolution,
  VideoStudioOperation,
} from './types'

export const STANDARD_VIDEO_MODEL = 'grok-imagine-video'
export const IMAGE_VIDEO_MODEL = 'grok-imagine-video-1.5-preview'

export function isVideoGenerationModel(modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  return id === STANDARD_VIDEO_MODEL || id === IMAGE_VIDEO_MODEL
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
  return modelId.trim().toLowerCase() === IMAGE_VIDEO_MODEL
}

export function getVideoResolutionOptions(_modelId: string): VideoResolution[] {
  return ['480p', '720p', '1080p']
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
  return options.includes(resolution) ? resolution : options[0]
}

export function buildGenerateVideoRequest(input: {
  operation: VideoStudioOperation
  model: string
  prompt: string
  resolution: VideoResolution
  duration: number
  imageUrl?: string
}): GenerateVideoRequest {
  const payload: GenerateVideoRequest = {
    model: input.model.trim(),
    prompt: input.prompt.trim(),
    resolution: normalizeVideoResolution(input.model, input.resolution),
    duration: Math.min(15, Math.max(1, Math.round(input.duration))),
  }
  const imageUrl = input.imageUrl?.trim()
  if (input.operation === 'image' && imageUrl) {
    payload.image = { url: imageUrl }
  }
  return payload
}
