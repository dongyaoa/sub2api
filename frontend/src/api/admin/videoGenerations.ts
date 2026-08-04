import { apiClient } from '../client'

export interface VideoGenerationLedgerItem {
  request_id: string
  user_id: number
  user_email: string
  api_key_id: number
  api_key_name: string
  group_id?: number
  group_name?: string
  account_id: number
  account_name?: string
  operation: string
  model: string
  upstream_model?: string
  prompt: string
  resolution?: string
  aspect_ratio?: string
  duration_seconds: number
  status: string
  delivery_status: string
  billing_status: string
  http_status?: number
  task_error?: unknown
  last_upstream_error?: string
  delivery_error?: string
  billing_error?: string
  video_url?: string
  content_type?: string
  byte_size?: number
  browser_playable: boolean
  actual_cost: number
  usage_log_id?: number
  created_at: string
  last_checked_at?: string
  completed_at?: string
  delivered_at?: string
  billed_at?: string
}

export interface VideoGenerationLedgerSummary {
  total: number
  processing: number
  delivered: number
  failed: number
  charged_without_output: number
  total_charged: number
}

export interface VideoGenerationLedgerQuery {
  page?: number
  page_size?: number
  search?: string
  status?: string
  delivery_status?: string
  billing_status?: string
  model?: string
  account_id?: number
  start_time?: string
  end_time?: string
}

export interface VideoGenerationLedgerResponse {
  items: VideoGenerationLedgerItem[]
  total: number
  page: number
  page_size: number
  summary: VideoGenerationLedgerSummary
}

export async function listVideoGenerations(
  params: VideoGenerationLedgerQuery,
  options?: { signal?: AbortSignal },
): Promise<VideoGenerationLedgerResponse> {
  const { data } = await apiClient.get<VideoGenerationLedgerResponse>('/admin/usage/video-generations', {
    params,
    signal: options?.signal,
  })
  return data
}

export default { list: listVideoGenerations }
