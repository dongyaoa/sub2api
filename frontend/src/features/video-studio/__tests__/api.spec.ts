import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  clearVideoTasks,
  downloadVideoContent,
  getVideoRequestId,
  getVideoTask,
  listVideoTasks,
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
      aspect_ratio: '3:2',
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
      aspect_ratio: '3:2',
      duration: 8,
    })
  })

  it('uses the same key for status and authenticated content downloads', async () => {
    const mp4 = new Uint8Array([0, 0, 0, 24, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d])
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ id: 'video_123', status: 'completed' }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: { get: vi.fn((name: string) => name === 'Content-Type' ? 'application/octet-stream' : '') },
        arrayBuffer: vi.fn().mockResolvedValue(mp4.buffer),
      })
    vi.stubGlobal('fetch', fetchMock)

    await expect(getVideoTask('same-key', 'video_123')).resolves.toMatchObject({ status: 'completed' })
    const blob = await downloadVideoContent('same-key', 'video_123')
    expect(blob.type).toBe('video/mp4')
    expect(blob.size).toBe(mp4.byteLength)

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

  it('rejects a JSON success body instead of creating an unplayable blob', async () => {
    const data = new TextEncoder().encode('{"error":{"message":"video expired"}}')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: { get: vi.fn((name: string) => name === 'Content-Type' ? 'application/json' : '') },
      arrayBuffer: vi.fn().mockResolvedValue(data.buffer),
    }))

    await expect(downloadVideoContent('same-key', 'video_123')).rejects.toThrow('video expired')
  })

  it('lists and clears persisted video tasks with the same API key', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({
          data: [{ id: 'video_123', status: 'completed' }],
          retention_days: 14,
        }),
      })
      .mockResolvedValueOnce({ ok: true })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listVideoTasks('history-key')).resolves.toEqual({
      tasks: [{ id: 'video_123', status: 'completed' }],
      retentionDays: 14,
    })
    await expect(clearVideoTasks('history-key')).resolves.toBeUndefined()

    expect(fetchMock.mock.calls[0]?.[0]).toMatch(/[/]v1[/]videos[/]tasks[?]limit=10$/)
    expect(fetchMock.mock.calls[1]?.[0]).toMatch(/[/]v1[/]videos[/]tasks$/)
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({
      method: 'DELETE',
      headers: { Authorization: 'Bearer history-key' },
    })
  })

  it('accepts the known upstream task id fields', () => {
    expect(getVideoRequestId({ request_id: 'request-id' })).toBe('request-id')
    expect(getVideoRequestId({ task_id: 'task-id' })).toBe('task-id')
    expect(getVideoRequestId({ id: 'id' })).toBe('id')
  })
})
