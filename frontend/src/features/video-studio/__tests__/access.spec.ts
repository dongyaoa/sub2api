import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import { keyAllowsVideoStudio, listEligibleVideoKeys, loadVideoModelCatalogs } from '../access'

const list = vi.hoisted(() => vi.fn())
const listModels = vi.hoisted(() => vi.fn())
vi.mock('@/api/keys', () => ({ keysAPI: { list } }))
vi.mock('../api', () => ({ listVideoModels: listModels }))

function key(platform: string, allow = true, status = 'active'): ApiKey {
  return { status, group: { platform, allow_image_generation: allow } } as ApiKey
}

beforeEach(() => vi.resetAllMocks())

describe('video studio access', () => {
  it('allows OpenAI, Grok, Gemini and composite keys while enforcing media permission and active status', () => {
    for (const platform of ['openai', 'grok', 'gemini', 'composite']) expect(keyAllowsVideoStudio(key(platform))).toBe(true)
    expect(keyAllowsVideoStudio(key('anthropic'))).toBe(false)
    expect(keyAllowsVideoStudio(key('openai', false))).toBe(false)
    expect(keyAllowsVideoStudio(key('openai', true, 'disabled'))).toBe(false)
  })

  it('includes eligible keys from every page', async () => {
    const openai = key('openai')
    const composite = key('composite')
    const gemini = key('gemini')
    list.mockResolvedValueOnce({ items: [openai, gemini], pages: 2 })
      .mockResolvedValueOnce({ items: [composite], pages: 2 })
    await expect(listEligibleVideoKeys()).resolves.toEqual([openai, gemini, composite])
    expect(list).toHaveBeenNthCalledWith(2, 2, 100, expect.objectContaining({ status: 'active' }))
  })

  it('checks each group once and excludes text-only and unknown model names', async () => {
    const keys = [1, 2, 3, 4].map(id => ({ id, key: `key-${id}`, group_id: id === 2 ? 1 : id } as ApiKey))
    listModels.mockImplementation(async (value: string) => ({ data: value === 'key-1'
      ? [{ id: 'muse-video' }, { id: 'svggrok-imagine-video-1.5' }]
      : [{ id: value === 'key-3' ? 'gemini-2.5-flash' : 'unsupported-video-name' }] }))
    const catalogs = await loadVideoModelCatalogs(keys)
    expect(listModels).toHaveBeenCalledTimes(3)
    expect(catalogs.get(1)).toEqual([{ id: 'muse-video' }, { id: 'svggrok-imagine-video-1.5' }])
    expect(catalogs.get(2)).toBe(catalogs.get(1))
    expect(catalogs.get(3)).toEqual([])
    expect(catalogs.get(4)).toEqual([])
  })

  it('limits catalog requests to four and retries another key after a lookup fails', async () => {
    const keys = Array.from({ length: 7 }, (_, index) => ({ id: index + 1, key: `key-${index + 1}`, group_id: index === 1 ? 1 : index + 1 } as ApiKey))
    let active = 0, maximum = 0
    listModels.mockImplementation(async (value: string) => {
      active += 1
      maximum = Math.max(maximum, active)
      await Promise.resolve()
      active -= 1
      if (value === 'key-1' || value === 'key-7') throw new Error('unavailable')
      return { data: [{ id: 'gemini-omni-1.2-flash' }] }
    })
    const catalogs = await loadVideoModelCatalogs(keys)
    expect(maximum).toBeLessThanOrEqual(4)
    expect(catalogs.get(1)).toEqual([{ id: 'gemini-omni-1.2-flash' }])
    expect(catalogs.has(7)).toBe(false)
    expect(listModels).toHaveBeenCalledWith('key-2', undefined)
  })

  it('does not keep probing catalogs after cancellation', async () => {
    const controller = new AbortController()
    controller.abort()
    expect(await loadVideoModelCatalogs([{ id: 1, key: 'key-1' } as ApiKey], controller.signal)).toEqual(new Map())
    expect(listModels).not.toHaveBeenCalled()
  })
})
