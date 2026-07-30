import type { GroupPlatform } from '@/types'

export type ImageStudioPlatform = Extract<GroupPlatform, 'openai' | 'gemini' | 'grok'>
export type StudioOperation = 'generate' | 'edit'
export type ImageResolutionTier = '1K' | '2K' | '4K'
export type ImageQuality = 'auto' | 'low' | 'medium' | 'high'
export type ImageAspectRatio = '1:1' | '3:2' | '2:3' | '16:9' | '9:16' | '4:3' | '3:4'

export interface ImageModel {
  id: string
  object?: string
  display_name?: string
}

export interface ImageModelsResponse {
  object: string
  data: ImageModel[]
}

export interface GenerateImageRequest {
  model: string
  prompt: string
  n: number
  size: string
  quality?: ImageQuality
  aspect_ratio?: ImageAspectRatio
  resolution?: string
}

export interface ImageGenerationData {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface ImageGenerationResult {
  created?: number
  data?: ImageGenerationData[]
  image_url?: string
}

export type ImageTaskStatus = 'processing' | 'completed' | 'failed'

export interface ImageTaskMetadata {
  operation?: StudioOperation
  model?: string
  prompt?: string
  quantity?: number
  size?: string
  quality?: string
  aspect_ratio?: ImageAspectRatio
  resolution?: ImageResolutionTier
}

export interface ImageTask {
  id: string
  task_id: string
  object: string
  status: ImageTaskStatus
  http_status?: number
  image_url?: string
  result?: ImageGenerationResult
  error?: unknown
  created_at: number
  completed_at?: number
  expires_at: number
  metadata?: ImageTaskMetadata
}

export interface ImageStudioError extends Error {
  code?: string | number
  status?: number
  requestId?: string
}

export interface StoredImageTask {
  taskId: string
  apiKeyId: number
  operation?: StudioOperation
  request: GenerateImageRequest
  platform: ImageStudioPlatform
  ratio: ImageAspectRatio
  resolution: ImageResolutionTier
  startedAt: number
}
