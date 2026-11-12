import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import { loadImageModelCatalogs } from '../access'
import type { ImageModelsResponse } from '../types'

const listModels = vi.hoisted(() => vi.fn())
vi.mock('@/api/keys', () => ({ keysAPI: { list: vi.fn() } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: vi.fn() }))
vi.mock('../api', () => ({ listImageModels: listModels }))

function key(id: number, groupID = id, platform = 'gemini'): ApiKey {
  return { id, key: `key-${id}`, group_id: groupID, group: { id: groupID, platform } } as ApiKey
}

function deferredCatalog() {
  let resolve!: (value: ImageModelsResponse) => void
  const promise = new Promise<ImageModelsResponse>((done) => { resolve = done })
  return { promise, resolve }
}

beforeEach(() => vi.resetAllMocks())

describe('image studio model catalogs', () => {
  it('loads one catalog per group and shares it across that group’s keys', async () => {
    const sameGroup = key(2, 10)
    delete sameGroup.group_id
    listModels.mockResolvedValue({ data: [{ id: 'gemini-3-pro-image' }] })
    const catalogs = await loadImageModelCatalogs([key(1, 10), sameGroup, key(3, 20)])

    expect(listModels).toHaveBeenCalledTimes(2)
    expect(listModels).toHaveBeenCalledWith('key-1', undefined)
    expect(listModels).toHaveBeenCalledWith('key-3', undefined)
    expect(catalogs.get(1)).toEqual([{ id: 'gemini-3-pro-image' }])
    expect(catalogs.get(2)).toBe(catalogs.get(1))
    expect(catalogs.get(3)).toEqual([{ id: 'gemini-3-pro-image' }])
  })

  it('keeps image models from mixed catalogs and leaves video-only, text-only, and empty groups unavailable', async () => {
    const responses: Record<string, unknown[]> = {
      'key-1': [{ id: 'gemini-omni-1.2-flash', image_generation: true }, { id: 'muse-video', image_generation: true }],
      'key-2': [{ id: 'gemini-2.5-pro' }],
      'key-3': [],
      'key-4': [
        { id: 'gemini-omni-1.2-flash', image_generation: true },
        { id: 'gemini-3-pro-image' },
        { id: 'custom-art', image_generation: true },
        { id: 'custom-motion', image_generation: true, video_generation: true },
      ],
      'key-5': [{ id: 'gpt-image-2' }, { id: 'gpt-5.4' }],
      'key-6': [{ id: 'grok-imagine-image' }, { id: 'grok-imagine-video' }],
    }
    listModels.mockImplementation(async (value: string) => ({ data: responses[value] }))
    const catalogs = await loadImageModelCatalogs([
      key(1), key(2), key(3), key(4), key(5, 5, 'openai'), key(6, 6, 'grok'),
    ])

    for (const id of [1, 2, 3]) expect(catalogs.get(id)).toEqual([])
    expect(catalogs.get(4)).toEqual([{ id: 'gemini-3-pro-image' }, { id: 'custom-art', image_generation: true }])
    expect(catalogs.get(5)).toEqual([{ id: 'gpt-image-2' }])
    expect(catalogs.get(6)).toEqual([{ id: 'grok-imagine-image' }])
  })

  it('tries another key in a failing group without treating failed lookups as availability', async () => {
    listModels.mockImplementation(async (value: string) => {
      if (value !== 'key-2') throw new Error('unavailable')
      return { data: [{ id: 'gemini-nano-banana-2.1' }] }
    })
    const catalogs = await loadImageModelCatalogs([key(1, 10), key(2, 10), key(3, 20), key(4, 20)])

    expect(listModels).toHaveBeenCalledWith('key-2', undefined)
    expect(listModels).toHaveBeenCalledWith('key-4', undefined)
    expect(catalogs.get(1)).toEqual([{ id: 'gemini-nano-banana-2.1' }])
    expect(catalogs.get(2)).toBe(catalogs.get(1))
    expect(catalogs.has(3)).toBe(false)
    expect(catalogs.has(4)).toBe(false)
  })

  it('limits outstanding group requests to four', async () => {
    const requests = Array.from({ length: 8 }, deferredCatalog)
    let active = 0
    let maximum = 0
    listModels.mockImplementation((value: string) => {
      active += 1
      maximum = Math.max(maximum, active)
      return requests[Number(value.slice(4)) - 1].promise.finally(() => { active -= 1 })
    })
    const loading = loadImageModelCatalogs(Array.from({ length: 8 }, (_, index) => key(index + 1)))
    expect(listModels).toHaveBeenCalledTimes(4)
    for (const request of requests.slice(0, 4)) request.resolve({ object: 'list', data: [] })
    await flushPromises()
    expect(listModels).toHaveBeenCalledTimes(8)
    expect(maximum).toBe(4)
    for (const request of requests.slice(4)) request.resolve({ object: 'list', data: [] })
    expect((await loading).size).toBe(8)
    expect(active).toBe(0)
  })

  it('does not start requests after cancellation', async () => {
    const controller = new AbortController()
    controller.abort()
    expect(await loadImageModelCatalogs([key(1)], controller.signal)).toEqual(new Map())
    expect(listModels).not.toHaveBeenCalled()
  })

  it('discards responses arriving after cancellation and does not probe remaining groups', async () => {
    const controller = new AbortController()
    const request = deferredCatalog()
    listModels.mockReturnValue(request.promise)
    const loading = loadImageModelCatalogs(Array.from({ length: 6 }, (_, index) => key(index + 1)), controller.signal)
    expect(listModels).toHaveBeenCalledTimes(4)
    expect(listModels).toHaveBeenCalledWith('key-1', controller.signal)
    controller.abort()
    request.resolve({ object: 'list', data: [{ id: 'gemini-3-pro-image' }] })

    expect(await loading).toEqual(new Map())
    expect(listModels).toHaveBeenCalledTimes(4)
  })
})
