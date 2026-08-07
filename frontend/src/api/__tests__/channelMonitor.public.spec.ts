import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { publicList, publicStatus } from '@/api/channelMonitor'

describe('public channel monitor API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('loads the anonymous status feed', async () => {
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: { items: [] } })

    await expect(publicList({ signal: controller.signal })).resolves.toEqual({ items: [] })
    expect(get).toHaveBeenCalledWith('/public/channel-monitors', {
      signal: controller.signal,
    })
  })

  it('loads public aggregate detail for one monitor', async () => {
    get.mockResolvedValueOnce({ data: { id: 7, models: [] } })

    await expect(publicStatus(7)).resolves.toEqual({ id: 7, models: [] })
    expect(get).toHaveBeenCalledWith('/public/channel-monitors/7/status')
  })
})
