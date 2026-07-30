import { buildGatewayUrl } from '@/api/url'
import type {
  GenerateVideoRequest,
  VideoModelsResponse,
  VideoStudioError,
  VideoTask,
} from './types'

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

async function parseVideoStudioError(response: Response): Promise<VideoStudioError> {
  let message = response.statusText || `HTTP ${response.status}`
  let code: string | number = response.status
  try {
    const body = await response.json()
    message = body?.error?.message || body?.message || message
    code = body?.error?.code || body?.error?.type || body?.code || code
  } catch {
    // Keep the HTTP fallback for non-JSON responses.
  }
  const error = new Error(message) as VideoStudioError
  error.code = code
  error.status = response.status
  error.requestId = response.headers.get('X-Request-Id') || ''
  return error
}

export function getVideoRequestId(task: VideoTask): string {
  return String(task.request_id || task.task_id || task.id || '').trim()
}

export async function listVideoModels(apiKey: string, signal?: AbortSignal): Promise<VideoModelsResponse> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: authHeaders(apiKey),
    signal,
  })
  if (!response.ok) throw await parseVideoStudioError(response)
  return response.json()
}

export async function submitVideoTask(
  apiKey: string,
  payload: GenerateVideoRequest,
  signal?: AbortSignal,
): Promise<VideoTask> {
  const response = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify(payload),
    signal,
  })
  if (!response.ok) throw await parseVideoStudioError(response)
  return response.json()
}

export async function getVideoTask(
  apiKey: string,
  requestId: string,
  signal?: AbortSignal,
): Promise<VideoTask> {
  const response = await fetch(
    buildGatewayUrl(`/v1/videos/${encodeURIComponent(requestId)}`),
    {
      headers: authHeaders(apiKey, { Accept: 'application/json' }),
      signal,
    },
  )
  if (!response.ok) throw await parseVideoStudioError(response)
  return response.json()
}

export async function downloadVideoContent(
  apiKey: string,
  requestId: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const response = await fetch(
    buildGatewayUrl(`/v1/videos/${encodeURIComponent(requestId)}/content`),
    {
      headers: authHeaders(apiKey, { Accept: 'video/mp4,video/*;q=0.9,*/*;q=0.8' }),
      signal,
    },
  )
  if (!response.ok) throw await parseVideoStudioError(response)
  return response.blob()
}
