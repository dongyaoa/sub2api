import { describe, expect, it } from 'vitest'
import {
  LONG_VIDEO_POLL_INTERVAL_MS,
  normalizeVideoTaskState,
  shouldRetryVideoPollError,
  VIDEO_POLL_INTERVAL_MS,
  videoTaskPollDelay,
} from '../polling'

describe('video task polling policy', () => {
  it('only treats explicit upstream terminal states as failures', () => {
    expect(normalizeVideoTaskState('failed')).toBe('failed')
    expect(normalizeVideoTaskState('cancelled')).toBe('failed')
    expect(normalizeVideoTaskState('expired')).toBe('failed')
    expect(normalizeVideoTaskState('processing')).toBe('processing')
    expect(normalizeVideoTaskState(undefined)).toBe('processing')
  })

  it('continues polling after five minutes at a lower frequency', () => {
    const startedAt = 1_000
    expect(videoTaskPollDelay(startedAt, startedAt + 60_000)).toBe(VIDEO_POLL_INTERVAL_MS)
    expect(videoTaskPollDelay(startedAt, startedAt + 5 * 60_000)).toBe(LONG_VIDEO_POLL_INTERVAL_MS)
    expect(videoTaskPollDelay(startedAt, startedAt + 30 * 60_000)).toBe(LONG_VIDEO_POLL_INTERVAL_MS)
  })

  it('retries auth_not_found and server errors but stops an intentional abort', () => {
    expect(shouldRetryVideoPollError({ status: 500, message: 'server error' })).toBe(true)
    expect(shouldRetryVideoPollError({ code: 'internal_server_error', message: 'auth_not_found' })).toBe(true)
    expect(shouldRetryVideoPollError({ status: 429, message: 'busy' })).toBe(true)
    expect(shouldRetryVideoPollError({ status: 404, message: 'not found' })).toBe(false)
    expect(shouldRetryVideoPollError({ status: 401, message: 'unauthorized' })).toBe(false)
    expect(shouldRetryVideoPollError({ name: 'AbortError' })).toBe(false)
  })
})
