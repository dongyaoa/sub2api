import type { VideoViewState } from './types'

export const VIDEO_POLL_INTERVAL_MS = 5000
export const LONG_VIDEO_TASK_THRESHOLD_MS = 5 * 60 * 1000
export const LONG_VIDEO_POLL_INTERVAL_MS = 15000

export function normalizeVideoTaskState(status: string | undefined): Exclude<VideoViewState, 'idle'> {
  const normalized = String(status || '').trim().toLowerCase()
  if (['completed', 'succeeded', 'success', 'done'].includes(normalized)) return 'completed'
  if (['failed', 'cancelled', 'canceled', 'expired', 'error'].includes(normalized)) return 'failed'
  return 'processing'
}

export function videoTaskPollDelay(startedAt: number, now = Date.now()): number {
  return now - startedAt >= LONG_VIDEO_TASK_THRESHOLD_MS
    ? LONG_VIDEO_POLL_INTERVAL_MS
    : VIDEO_POLL_INTERVAL_MS
}

export function shouldRetryVideoPollError(error: unknown): boolean {
  const candidate = error as { name?: string; status?: number } | null
  if (candidate?.name === 'AbortError') return false
  const status = Number(candidate?.status)
  if (!Number.isFinite(status) || status <= 0) return true
  return status === 408 || status === 409 || status === 425 || status === 429 || status >= 500
}
