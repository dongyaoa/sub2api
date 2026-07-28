import { buildGatewayUrl } from '@/api/url'
import type {
  GenerateImageRequest,
  ImageGenerationResult,
  ImageModelsResponse,
  ImageStudioError,
  ImageTask,
} from './types'

export type ExternalRequestFailureKind = 'invalid-url' | 'connection-interrupted' | 'network' | 'api'

export function classifyExternalRequestFailure(
  error: unknown,
  elapsedMs: number,
  hasSuccessfulRequest: boolean,
): ExternalRequestFailureKind {
  const message = (error as Error)?.message || ''
  if (/Base URL|Invalid URL/i.test(message)) return 'invalid-url'
  if (error instanceof TypeError) {
    return hasSuccessfulRequest || elapsedMs >= 15_000
      ? 'connection-interrupted'
      : 'network'
  }
  return 'api'
}

async function parseImageStudioError(response: Response): Promise<ImageStudioError> {
  let message = response.statusText || `HTTP ${response.status}`
  let code: string | number = response.status
  try {
    const body = await response.json()
    message = body?.error?.message || body?.message || message
    code = body?.error?.code || body?.error?.type || body?.code || code
  } catch {
    // Keep the HTTP fallback when the response is not JSON.
  }
  const error = new Error(message) as ImageStudioError
  error.code = code
  error.status = response.status
  error.requestId = response.headers.get('X-Request-Id') || ''
  return error
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

export function normalizeExternalBaseUrl(value: string): string {
  const parsed = new URL(value.trim())
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('Base URL must use http or https')
  }
  if (parsed.username || parsed.password) {
    throw new Error('Base URL must not contain credentials')
  }
  parsed.search = ''
  parsed.hash = ''
  return parsed.toString().replace(/\/$/, '')
}

export async function generateExternalImages(
  baseUrl: string,
  apiKey: string,
  payload: GenerateImageRequest,
  signal?: AbortSignal,
): Promise<ImageGenerationResult> {
  const endpoint = `${normalizeExternalBaseUrl(baseUrl)}/images/generations`
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: authHeaders(apiKey.trim(), {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify(payload),
    signal,
  })
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}

export function buildImageEditForm(file: File, payload: GenerateImageRequest): FormData {
  const form = new FormData()
  form.append('image', file, file.name)
  form.append('model', payload.model)
  form.append('prompt', payload.prompt)
  form.append('n', String(payload.n))
  form.append('size', payload.size)
  if (payload.quality) form.append('quality', payload.quality)
  if (payload.aspect_ratio) form.append('aspect_ratio', payload.aspect_ratio)
  if (payload.resolution) form.append('resolution', payload.resolution)
  return form
}

export async function generateExternalImageEdit(
  baseUrl: string,
  apiKey: string,
  file: File,
  payload: GenerateImageRequest,
  signal?: AbortSignal,
): Promise<ImageGenerationResult> {
  const endpoint = normalizeExternalBaseUrl(baseUrl) + '/images/edits'
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: authHeaders(apiKey.trim(), { Accept: 'application/json' }),
    body: buildImageEditForm(file, payload),
    signal,
  })
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}
export async function listImageModels(apiKey: string, signal?: AbortSignal): Promise<ImageModelsResponse> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: authHeaders(apiKey),
    signal,
  })
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}

export async function submitImageTask(
  apiKey: string,
  payload: GenerateImageRequest,
  signal?: AbortSignal,
): Promise<ImageTask> {
  const response = await fetch(buildGatewayUrl('/v1/images/generations/async'), {
    method: 'POST',
    headers: authHeaders(apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
    signal,
  })
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}

export async function submitImageEditTask(
  apiKey: string,
  file: File,
  payload: GenerateImageRequest,
  signal?: AbortSignal,
): Promise<ImageTask> {
  const response = await fetch(buildGatewayUrl('/v1/images/edits/async'), {
    method: 'POST',
    headers: authHeaders(apiKey, { Accept: 'application/json' }),
    body: buildImageEditForm(file, payload),
    signal,
  })
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}
export async function getImageTask(
  apiKey: string,
  taskId: string,
  signal?: AbortSignal,
): Promise<ImageTask> {
  const response = await fetch(
    buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(taskId)}`),
    { headers: authHeaders(apiKey), signal },
  )
  if (!response.ok) throw await parseImageStudioError(response)
  return response.json()
}

export function extractTaskImageData(task: ImageTask): Array<{ url: string; revisedPrompt?: string }> {
  const result = task.result
  const data = Array.isArray(result?.data) ? result.data : []
  const images = data.flatMap((item) => {
    const url = item?.url?.trim() || (item?.b64_json ? `data:image/png;base64,${item.b64_json}` : '')
    return url ? [{ url, revisedPrompt: item.revised_prompt?.trim() || undefined }] : []
  })
  if (images.length > 0) return images
  const fallback = task.image_url?.trim() || result?.image_url?.trim()
  return fallback ? [{ url: fallback }] : []
}
