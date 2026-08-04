import { buildGatewayUrl } from '@/api/url'
import type {
  GenerateVideoRequest,
  VideoModelsResponse,
  VideoStudioError,
  VideoTask,
  VideoTaskListResult,
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
  const bytes = await response.arrayBuffer()
  const data = new Uint8Array(bytes)
  const declaredType = String(response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase()
  const isMP4 = data.length >= 12
    && String.fromCharCode(data[4], data[5], data[6], data[7]) === 'ftyp'
  const isWebM = data.length >= 4
    && data[0] === 0x1a && data[1] === 0x45 && data[2] === 0xdf && data[3] === 0xa3
  if (!isMP4 && !isWebM) {
    let message = 'The video endpoint returned unsupported content'
    try {
      const body = JSON.parse(new TextDecoder().decode(data))
      message = body?.error?.message || body?.message || message
    } catch {
      if (declaredType) message += ` (${declaredType})`
    }
    const error = new Error(message) as VideoStudioError
    error.status = response.status
    error.requestId = response.headers.get('X-Request-Id') || ''
    throw error
  }
  const contentType = isMP4 ? 'video/mp4' : 'video/webm'
  return new Blob([bytes], { type: contentType })
}

export async function listVideoTasks(
  apiKey: string,
  limit = 10,
  signal?: AbortSignal,
): Promise<VideoTaskListResult> {
  const params = new URLSearchParams({ limit: String(limit) })
  const response = await fetch(buildGatewayUrl(`/v1/videos/tasks?${params.toString()}`), {
    headers: authHeaders(apiKey, { Accept: 'application/json' }),
    signal,
  })
  if (!response.ok) throw await parseVideoStudioError(response)
  const body = await response.json() as { data?: VideoTask[]; retention_days?: number; persistent_history?: boolean }
  return {
    tasks: Array.isArray(body.data) ? body.data : [],
    retentionDays: Math.max(1, Number(body.retention_days) || 7),
    persistentHistory: body.persistent_history === true,
  }
}

export async function clearVideoTasks(apiKey: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(buildGatewayUrl('/v1/videos/tasks'), {
    method: 'DELETE',
    headers: authHeaders(apiKey),
    signal,
  })
  if (!response.ok) throw await parseVideoStudioError(response)
}
