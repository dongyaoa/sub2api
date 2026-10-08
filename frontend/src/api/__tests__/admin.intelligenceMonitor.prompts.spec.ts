import { beforeEach, describe, expect, it, vi } from 'vitest'
const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: client }))
import { intelligenceMonitorAPI, type IntelligencePlanInput } from '@/api/admin/intelligenceMonitor'

beforeEach(() => { vi.resetAllMocks(); client.get.mockResolvedValue({ data: { items: [] } }); client.post.mockResolvedValue({ data: {} }); client.put.mockResolvedValue({ data: {} }) })

describe('local monitoring prompt API', () => {
  it('loads channels for exactly one group and forwards cancellation', async () => {
    const signal = new AbortController().signal
    await expect(intelligenceMonitorAPI.localChannels(42, signal)).resolves.toEqual({ items: [] })
    expect(client.get).toHaveBeenCalledWith('/admin/intelligence-monitors/local-channels', { signal, params: { group_id: 42 } })
  })

  it('preserves prompt text on creation and explicit clears on update', async () => {
    const input = { source_type: 'local_group', group_id: 42, custom_prompt: 'Draw HTML\nwith a pelican', channel_prompts: [{ account_id: 7, prompt: 'Draw SVG' }] } as IntelligencePlanInput
    await intelligenceMonitorAPI.create(input)
    expect(client.post).toHaveBeenCalledWith('/admin/intelligence-monitors/plans', input)
    await intelligenceMonitorAPI.update(5, { custom_prompt: '', channel_prompts: [] })
    expect(client.put).toHaveBeenCalledWith('/admin/intelligence-monitors/plans/5', { custom_prompt: '', channel_prompts: [] })
  })
})
