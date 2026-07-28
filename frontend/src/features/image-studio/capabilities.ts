import type {
  GenerateImageRequest,
  ImageAspectRatio,
  ImageModel,
  ImageQuality,
  ImageResolutionTier,
  ImageStudioPlatform,
} from './types'

export interface StudioOption<T extends string> {
  value: T
  label: string
}

const OPENAI_RATIOS: StudioOption<ImageAspectRatio>[] = [
  { value: '1:1', label: '1:1' },
  { value: '3:2', label: '3:2' },
  { value: '2:3', label: '2:3' },
]

const GROK_RATIOS: StudioOption<ImageAspectRatio>[] = [
  { value: '1:1', label: '1:1' },
  { value: '16:9', label: '16:9' },
  { value: '9:16', label: '9:16' },
  { value: '4:3', label: '4:3' },
  { value: '3:4', label: '3:4' },
  { value: '3:2', label: '3:2' },
  { value: '2:3', label: '2:3' },
]

const OPENAI_RESOLUTIONS: ImageResolutionTier[] = ['1K', '2K']
const GROK_RESOLUTIONS: ImageResolutionTier[] = ['1K', '2K', '4K']

export function isImageStudioPlatform(platform: string | undefined): platform is ImageStudioPlatform {
  return platform === 'openai' || platform === 'grok'
}

export function isImageGenerationModel(platform: ImageStudioPlatform, modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  if (platform === 'openai') return id.startsWith('gpt-image-')
  return (
    id === 'grok-imagine' ||
    id === 'grok-imagine-image' ||
    id === 'grok-imagine-image-quality'
  )
}

export function filterImageModels(platform: ImageStudioPlatform, models: ImageModel[]): ImageModel[] {
  const seen = new Set<string>()
  return models.filter((model) => {
    const id = model.id?.trim()
    if (!id || seen.has(id) || !isImageGenerationModel(platform, id)) return false
    seen.add(id)
    return true
  })
}

export function getAspectRatioOptions(platform: ImageStudioPlatform): StudioOption<ImageAspectRatio>[] {
  return platform === 'openai' ? OPENAI_RATIOS : GROK_RATIOS
}

export function getResolutionOptions(platform: ImageStudioPlatform): ImageResolutionTier[] {
  return platform === 'openai' ? OPENAI_RESOLUTIONS : GROK_RESOLUTIONS
}

export function normalizeStudioSelection(
  platform: ImageStudioPlatform,
  ratio: ImageAspectRatio,
  resolution: ImageResolutionTier,
): { ratio: ImageAspectRatio; resolution: ImageResolutionTier } {
  const ratios = getAspectRatioOptions(platform).map((option) => option.value)
  const resolutions = getResolutionOptions(platform)
  const normalizedRatio = ratios.includes(ratio) ? ratio : '1:1'
  const normalizedResolution = resolutions.includes(resolution) ? resolution : resolutions[0]
  if (platform === 'openai') {
    if (normalizedResolution === '1K') return { ratio: '1:1', resolution: '1K' }
    return {
      ratio: normalizedRatio === '1:1' ? '3:2' : normalizedRatio,
      resolution: '2K',
    }
  }
  return { ratio: normalizedRatio, resolution: normalizedResolution }
}

export function getOpenAIImageSize(
  ratio: ImageAspectRatio,
  resolution: ImageResolutionTier,
): string {
  if (resolution === '1K' || ratio === '1:1') return '1024x1024'
  if (ratio === '2:3') return '1024x1536'
  return '1536x1024'
}

export function getPreviewAspectRatio(ratio: ImageAspectRatio): string {
  return ratio.replace(':', ' / ')
}

export function buildGenerateImageRequest(input: {
  platform: ImageStudioPlatform
  model: string
  prompt: string
  quantity: number
  ratio: ImageAspectRatio
  resolution: ImageResolutionTier
  quality: ImageQuality
}): GenerateImageRequest {
  const normalized = normalizeStudioSelection(input.platform, input.ratio, input.resolution)
  const base: GenerateImageRequest = {
    model: input.model.trim(),
    prompt: input.prompt.trim(),
    n: Math.min(4, Math.max(1, Math.round(input.quantity))),
    size:
      input.platform === 'openai'
        ? getOpenAIImageSize(normalized.ratio, normalized.resolution)
        : normalized.resolution,
  }

  if (input.platform === 'openai') {
    base.quality = input.quality
    return base
  }

  base.aspect_ratio = normalized.ratio
  base.resolution = normalized.resolution.toLowerCase()
  return base
}
