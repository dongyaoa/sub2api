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
  { value: '16:9', label: '16:9' },
  { value: '9:16', label: '9:16' },
  { value: '4:3', label: '4:3' },
  { value: '3:4', label: '3:4' },
  { value: '21:9', label: '21:9' },
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
const OPENAI_RESOLUTIONS_WITH_4K: ImageResolutionTier[] = ['1K', '2K', '4K']
const GROK_RESOLUTIONS: ImageResolutionTier[] = ['1K', '2K']
const GEMINI_RESOLUTIONS: ImageResolutionTier[] = ['1K', '2K', '4K']

const OPENAI_2K_SIZES: Record<ImageAspectRatio, string> = {
  '1:1': '1024x1024',
  '3:2': '1536x1024',
  '2:3': '1024x1536',
  '16:9': '2048x1152',
  '9:16': '1152x2048',
  '4:3': '1536x1152',
  '3:4': '1152x1536',
  '21:9': '1792x768',
}

const OPENAI_4K_SIZES: Record<ImageAspectRatio, string> = {
  '1:1': '2880x2880',
  '3:2': '3456x2304',
  '2:3': '2304x3456',
  '16:9': '3840x2160',
  '9:16': '2160x3840',
  '4:3': '3072x2304',
  '3:4': '2304x3072',
  '21:9': '3584x1536',
}

const GEMINI_IMAGE_MODELS = new Set([
  'gemini-3.1-flash-image',
  'gemini-3-pro-image-preview',
])

export function isImageStudioPlatform(platform: string | undefined): platform is ImageStudioPlatform {
  return platform === 'openai' || platform === 'gemini' || platform === 'grok'
}

export function isImageGenerationModel(platform: ImageStudioPlatform, modelId: string): boolean {
  const id = modelId.trim().toLowerCase()
  if (platform === 'openai') return id.startsWith('gpt-image-')
  if (platform === 'gemini') return GEMINI_IMAGE_MODELS.has(id)
  return id === 'grok-imagine-image' || id === 'grok-imagine-image-quality'
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

export function getResolutionOptions(
  platform: ImageStudioPlatform,
  allowOpenAI4K = false,
): ImageResolutionTier[] {
  if (platform === 'openai') return allowOpenAI4K ? OPENAI_RESOLUTIONS_WITH_4K : OPENAI_RESOLUTIONS
  return platform === 'gemini' ? GEMINI_RESOLUTIONS : GROK_RESOLUTIONS
}

export function supportsImageEditing(platform: ImageStudioPlatform): boolean {
  return platform === 'openai' || platform === 'gemini' || platform === 'grok'
}

export function getMaxImageQuantity(platform: ImageStudioPlatform): number {
  return platform === 'gemini' ? 1 : 4
}

export function getPreferredImageModel(platform: ImageStudioPlatform): string {
  if (platform === 'openai') return 'gpt-image-2'
  if (platform === 'gemini') return 'gemini-3.1-flash-image'
  return 'grok-imagine-image'
}

export function normalizeStudioSelection(
  platform: ImageStudioPlatform,
  ratio: ImageAspectRatio,
  resolution: ImageResolutionTier,
  allowOpenAI4K = false,
): { ratio: ImageAspectRatio; resolution: ImageResolutionTier } {
  const ratios = getAspectRatioOptions(platform).map((option) => option.value)
  const resolutions = getResolutionOptions(platform, allowOpenAI4K)
  const normalizedRatio = ratios.includes(ratio) ? ratio : '1:1'
  const normalizedResolution = resolutions.includes(resolution) ? resolution : resolutions[0]
  if (platform === 'openai') {
    if (normalizedResolution === '1K') return { ratio: '1:1', resolution: '1K' }
    if (normalizedResolution === '4K') {
      return { ratio: normalizedRatio, resolution: '4K' }
    }
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
  if (resolution === '4K') return OPENAI_4K_SIZES[ratio]
  if (resolution === '1K') return '1024x1024'
  return OPENAI_2K_SIZES[ratio]
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
  allowOpenAI4K?: boolean
}): GenerateImageRequest {
  const normalized = normalizeStudioSelection(
    input.platform,
    input.ratio,
    input.resolution,
    input.allowOpenAI4K,
  )
  const base: GenerateImageRequest = {
    model: input.model.trim(),
    prompt: input.prompt.trim(),
    n: Math.min(getMaxImageQuantity(input.platform), Math.max(1, Math.round(input.quantity))),
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
