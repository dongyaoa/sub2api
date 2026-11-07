export type VideoStudioOperation = 'text' | 'image'
export type VideoResolution = 'auto' | '360p' | '480p' | '720p' | '1080p' | '4k'
export type VideoAspectRatio = 'auto' | '1:1' | '16:9' | '9:16' | '4:3' | '3:4' | '3:2' | '2:3'
export type VideoReferenceMode = 'frames' | 'references'
export interface VideoReferenceInputValue { url: string; file?: File }
export type VideoViewState = 'idle' | 'processing' | 'completed' | 'failed'

export interface VideoModel {
  id: string
  object?: string
  display_name?: string
}

export interface VideoModelsResponse {
  object: string
  data: VideoModel[]
}

export interface VideoModelPricing {
  model: string
  billing_model?: string
  billing_mode: 'per_request' | 'image' | 'video' | 'token'
  pricing_source: string
  rate_multiplier: number
  tiers: Partial<Record<VideoResolution, { base_price: number; unit_price: number; configured: boolean }>> | null
}

export interface GenerateVideoRequest {
  model: string
  prompt: string
  resolution: VideoResolution
  aspect_ratio?: VideoAspectRatio
  duration: number
  image_size?: string
  seed?: number
  first_frame?: string
  last_frame?: string
  image_references?: string[]
  video_references?: string[]
  image?: {
    url: string
  }
}

export interface VideoTaskError {
  code?: string | number
  message?: string
}

export interface VideoTask {
  request_id?: string
  task_id?: string
  id?: string
  object?: string
  status?: string
  progress?: number
  error?: VideoTaskError | string
  video?: {
    url?: string
  }
  created_at?: number
  completed_at?: number
  expires_at?: number
  content_available?: boolean
  content_type?: string
  byte_size?: number
  metadata?: {
    operation?: VideoStudioOperation
    model?: string
    prompt?: string
    resolution?: VideoResolution
    aspect_ratio?: VideoAspectRatio
    duration?: number
    image_size?: string
    seed?: number
  }
}

export interface VideoTaskListResult {
  tasks: VideoTask[]
  retentionDays: number
  persistentHistory: boolean
}

export interface VideoStudioError extends Error {
  code?: string | number
  status?: number
  requestId?: string
}

export interface StoredVideoTask {
  requestId: string
  apiKeyId: number
  operation: VideoStudioOperation
  request: GenerateVideoRequest
  startedAt: number
}

export interface StoredVideoHistoryItem extends StoredVideoTask {
  status: Exclude<VideoViewState, 'idle'>
  errorMessage?: string
}
