import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  downloadVideoContent,
  getVideoRequestId,
  getVideoTask,
  submitVideoTask,
} from '../api'

describe('video studio API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('submits the native Grok video payload', async () => {
    const accepted = { request_id: 'video_123', status: 'pending' }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(accepted),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(submitVideoTask('site-key', {
      model: 'grok-imagine-video',
      prompt: 'camera pushes through the rain',
      resolution: '720p',
      duration: 8,
    })).resolves.toEqual(accepted)

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toMatch(/[/]v1[/]videos[/]generations$/)
    expect(init.method).toBe('POST')
    expect(init.headers).toEqual({
      Authorization: 'Bearer site-key',
      Accept: 'application/json',
      'Content-Type': 'application/json',
    })
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'grok-imagine-video',
      prompt: 'camera pushes through the rain',
      resolution: '720p',
      duration: 8,
    })
  })

  it('uses the same key for status and authenticated content downloads', async () => {
    const blob = new Blob(['video'], { type: 'video/mp4' })
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ id: 'video_123', status: 'completed' }),
      })
      .mockResolvedValueOnce({ ok: true, blob: vi.fn().mockResolvedValue(blob) })
    vi.stubGlobal('fetch', fetchMock)

    await expect(getVideoTask('same-key', 'video_123')).resolves.toMatchObject({ status: 'completed' })
    await expect(downloadVideoContent('same-key', 'video_123')).resolves.toBe(blob)

    expect(fetchMock.mock.calls[0]?.[0]).toMatch(/[/]v1[/]videos[/]video_123$/)
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toEqual({
      Authorization: 'Bearer same-key',
      Accept: 'application/json',
    })
    expect(fetchMock.mock.calls[1]?.[0]).toMatch(/[/]v1[/]videos[/]video_123[/]content$/)
    expect(fetchMock.mock.calls[1]?.[1]?.headers).toEqual({
      Authorization: 'Bearer same-key',
      Accept: 'video/mp4,video/*;q=0.9,*/*;q=0.8',
    })
  })

  it('accepts the known upstream task id fields', () => {
    expect(getVideoRequestId({ request_id: 'request-id' })).toBe('request-id')
    expect(getVideoRequestId({ task_id: 'task-id' })).toBe('task-id')
    expect(getVideoRequestId({ id: 'id' })).toBe('id')
  })
})
