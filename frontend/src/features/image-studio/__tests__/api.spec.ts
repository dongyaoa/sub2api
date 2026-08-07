import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  classifyExternalRequestFailure,
  generateExternalImageEdit,
  generateExternalImages,
  normalizeExternalBaseUrl,
  clearImageTasks,
  listImageTasks,
  submitImageEditTask,
} from '../api'

describe('image studio external API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('normalizes supported base URLs and rejects unsafe URL forms', () => {
    expect(normalizeExternalBaseUrl(' https://images.example.com/v1/ ')).toBe('https://images.example.com/v1')
    expect(() => normalizeExternalBaseUrl('ftp://images.example.com/v1')).toThrow(/http or https/)
    expect(() => normalizeExternalBaseUrl('https://user:pass@images.example.com/v1')).toThrow(/credentials/)
  })

  it('posts a synchronous OpenAI-compatible image request', async () => {
    const result = { created: 1, data: [{ url: 'https://cdn.example.com/image.png' }] }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(result),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(generateExternalImages(
      'https://images.example.com/v1/',
      ' test-key ',
      { model: 'gpt-image-2', prompt: 'test image', n: 1, size: '1024x1024' },
    )).resolves.toEqual(result)

    expect(fetchMock).toHaveBeenCalledWith(
      'https://images.example.com/v1/images/generations',
      expect.objectContaining({
        method: 'POST',
        headers: {
          Authorization: 'Bearer test-key',
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
      }),
    )
  })

  it('posts external edits as multipart without overriding the boundary header', async () => {
    const result = { created: 1, data: [{ url: 'https://cdn.example.com/edited.png' }] }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(result),
    })
    vi.stubGlobal('fetch', fetchMock)
    const file = new File(['source'], 'source.png', { type: 'image/png' })
    const secondFile = new File(['second'], 'second.webp', { type: 'image/webp' })

    await expect(generateExternalImageEdit(
      'https://images.example.com/v1/',
      ' test-key ',
      [file, secondFile],
      { model: 'gpt-image-2', prompt: 'replace background', n: 2, size: '1536x1024', quality: 'high' },
    )).resolves.toEqual(result)

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://images.example.com/v1/images/edits')
    expect(init.headers).toEqual({
      Authorization: 'Bearer test-key',
      Accept: 'application/json',
    })
    expect(init.body).toBeInstanceOf(FormData)
    const form = init.body as FormData
    const uploaded = form.getAll('image') as File[]
    expect(uploaded).toHaveLength(2)
    expect(uploaded[0].name).toBe('source.png')
    expect(uploaded[0].type).toBe('image/png')
    expect(uploaded[0].size).toBe(file.size)
    expect(uploaded[1].name).toBe('second.webp')
    expect(uploaded[1].type).toBe('image/webp')
    expect(uploaded[1].size).toBe(secondFile.size)
    expect(form.get('model')).toBe('gpt-image-2')
    expect(form.get('prompt')).toBe('replace background')
    expect(form.get('n')).toBe('2')
    expect(form.get('size')).toBe('1536x1024')
    expect(form.get('quality')).toBe('high')
  })

  it('submits site-group edits to the asynchronous edit endpoint', async () => {
    const accepted = {
      id: 'imgtask_1',
      task_id: 'imgtask_1',
      object: 'image.task',
      status: 'processing',
      created_at: 1,
      expires_at: 2,
    }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(accepted),
    })
    vi.stubGlobal('fetch', fetchMock)
    const file = new File(['source'], 'source.webp', { type: 'image/webp' })

    await expect(submitImageEditTask(
      'site-key',
      file,
      { model: 'gpt-image-2', prompt: 'remove the sign', n: 1, size: '1024x1024' },
    )).resolves.toEqual(accepted)

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toMatch(/[/]v1[/]images[/]edits[/]async$/)
    expect(init.headers).toEqual({
      Authorization: 'Bearer site-key',
      Accept: 'application/json',
    })
    expect(init.body).toBeInstanceOf(FormData)
  })
  it('distinguishes initial network failures from interrupted long generations', () => {
    const networkError = new TypeError('Failed to fetch')

    expect(classifyExternalRequestFailure(networkError, 500, false)).toBe('network')
    expect(classifyExternalRequestFailure(networkError, 20_000, false)).toBe('connection-interrupted')
    expect(classifyExternalRequestFailure(networkError, 500, true)).toBe('connection-interrupted')
    expect(classifyExternalRequestFailure(new Error('upstream rejected prompt'), 30_000, true)).toBe('api')
  })
})

describe('image studio history API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists and clears server-side image history for the selected key', async () => {
    const tasks = [{
      id: 'imgtask_1',
      task_id: 'imgtask_1',
      object: 'image.generation.task',
      status: 'completed',
      created_at: 1,
      expires_at: 2,
    }]
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ object: 'list', data: tasks, retention_days: 7 }),
      })
      .mockResolvedValueOnce({ ok: true })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listImageTasks('site-key', 10)).resolves.toEqual({
      tasks,
      retentionDays: 7,
    })
    await expect(clearImageTasks('site-key')).resolves.toBeUndefined()

    const [listUrl, listInit] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(listUrl).toMatch(/[/]v1[/]images[/]tasks[?]limit=10$/)
    expect(listInit.headers).toEqual({ Authorization: 'Bearer site-key' })

    const [clearUrl, clearInit] = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(clearUrl).toMatch(/[/]v1[/]images[/]tasks$/)
    expect(clearInit.method).toBe('DELETE')
    expect(clearInit.headers).toEqual({ Authorization: 'Bearer site-key' })
  })
})