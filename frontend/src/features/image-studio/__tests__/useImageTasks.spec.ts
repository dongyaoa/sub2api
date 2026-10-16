import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import type { ImageTask } from '../types'
import { formatHistoryTime, historyDuration, useImageTasks } from '../useImageTasks'

const api = vi.hoisted(() => ({ getTask: vi.fn(), listTasks: vi.fn(), submitTask: vi.fn(), deleteTask: vi.fn() }))
vi.mock('../api', () => ({
  getImageTask: api.getTask,
  listImageTasks: api.listTasks,
  submitImageTask: api.submitTask,
  submitImageEditTask: vi.fn(), deleteImageTask: api.deleteTask,
  extractTaskImageData: (task: ImageTask) => (task.result?.data || []).flatMap((image) => image.url ? [{ url: image.url }] : []),
}))

const key = { id: 1, user_id: 1, key: 'key-1', group: { id: 1, name: 'Gemini', platform: 'gemini' } } as ApiKey
const pending: ImageTask = {
  id: 'pending-task', task_id: 'pending-task', object: 'image.generation.task', status: 'processing',
  created_at: 1_792_048_980, expires_at: 1_792_653_780,
  metadata: { model: 'gemini-image', prompt: 'a landscape', quantity: 1, resolution: '1K', aspect_ratio: '1:1' },
}

function setupManager(keys: ApiKey[] = [key]) {
  let manager!: ReturnType<typeof useImageTasks>
  const wrapper = mount({
    setup() { manager = useImageTasks(ref(keys), vi.fn(), () => 'No image returned'); return () => null },
  })
  return { manager, wrapper }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  sessionStorage.clear()
  api.listTasks.mockResolvedValue({ tasks: [], retentionDays: 7 })
})

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

describe('image task recovery', () => {
  it('formats local dates with month, day, hour and minute', () => {
    expect(formatHistoryTime(new Date(2026, 9, 15, 15, 23).getTime())).toBe('10-15 15:23')
  })

  it('restores tasks across keys and deletes a completed task using its owning key', async () => {
    const secondKey = { ...key, id: 2, key: 'key-2' }
    const second = { ...pending, id: 'second', task_id: 'second' }
    const completed = { ...pending, id: 'completed', task_id: 'completed', status: 'completed' as const, completed_at: pending.created_at + 65, result: { data: [{ url: 'https://images.example/complete.png' }] } }
    api.listTasks.mockImplementation(async (secret: string) => ({ tasks: secret === 'key-2' ? [second, completed] : [pending], retentionDays: 7 }))
    api.getTask.mockImplementation(() => new Promise(() => {}))
    const { manager, wrapper } = setupManager([key, secondKey])
    try {
      await manager.restore()
      await vi.advanceTimersByTimeAsync(0)
      expect(manager.runningCount.value).toBe(2)
      expect(api.getTask).toHaveBeenCalledWith('key-1', 'pending-task', expect.any(AbortSignal))
      expect(api.getTask).toHaveBeenCalledWith('key-2', 'second', expect.any(AbortSignal))
      await manager.remove(manager.history.value.find(item => item.id === 'completed')!)
      expect(api.deleteTask).toHaveBeenCalledWith('key-2', 'completed')
      await manager.remove(manager.history.value[0]!)
      expect(api.deleteTask).toHaveBeenCalledTimes(1)
      expect(manager.runningCount.value).toBe(2)
      const stored = sessionStorage.getItem('image_studio_active_tasks_v2:1')!
      expect(JSON.parse(stored)).toHaveLength(2)
      expect(stored).not.toContain('key-1')
      expect(stored).not.toContain('key-2')
    } finally { wrapper.unmount() }
    for (const call of api.getTask.mock.calls) expect(call[2].aborted).toBe(true)
  })

  it('does not duplicate a task discovered by history before its submission is acknowledged', async () => {
    let accept!: (value: ImageTask) => void
    api.submitTask.mockImplementation(() => new Promise<ImageTask>(resolve => { accept = resolve }))
    api.getTask.mockImplementation(() => new Promise(() => {}))
    const { manager, wrapper } = setupManager()
    try {
      const submission = manager.submit(key, { request: { model: 'gemini-image', prompt: 'draft', n: 1, size: '1K' }, operation: 'generate', platform: 'gemini', resolution: '1K', ratio: '1:1', sourceFiles: [] })
      api.listTasks.mockResolvedValue({ tasks: [pending], retentionDays: 7 })
      await manager.loadHistory(key)
      accept(pending)
      await submission
      expect(manager.history.value).toHaveLength(1)
      expect(manager.history.value[0]?.prompt).toBe('draft')
      expect(manager.activeHistoryItem.value?.id).toBe(pending.id)
    } finally { wrapper.unmount() }
  })

  it('restores a server task and retries transient polling failures without marking generation as failed', async () => {
    api.listTasks.mockResolvedValue({ tasks: [pending], retentionDays: 14 })
    api.getTask.mockRejectedValueOnce(new TypeError('network interrupted')).mockResolvedValueOnce({
      ...pending, status: 'completed', completed_at: pending.created_at + 65,
      result: { data: [{ url: 'https://images.example/completed.png' }] },
    })
    const { manager, wrapper } = setupManager()
    try {
      await manager.restore()
      await vi.advanceTimersByTimeAsync(0)
      await flushPromises()
      expect(manager.runningCount.value).toBe(1)
      expect(manager.history.value[0]?.pollWarning).toBe(true)
      expect(manager.historyRetentionDays.value).toBe(14)
      await vi.advanceTimersByTimeAsync(6000)
      await flushPromises()
      expect(manager.runningCount.value).toBe(0)
      expect(manager.history.value[0]?.status).toBe('completed')
      expect(historyDuration(manager.history.value[0]!, Date.now())).toBe(65)
      await vi.advanceTimersByTimeAsync(60_000)
      expect(historyDuration(manager.history.value[0]!, Date.now())).toBe(65)
      expect(api.getTask).toHaveBeenCalledTimes(2)
    } finally { wrapper.unmount() }
  })

  it('stops polling an expired saved task that is absent from server history', async () => {
    sessionStorage.setItem('image_studio_active_tasks_v2:1', JSON.stringify([{ taskId: 'expired-task', apiKeyId: 1 }]))
    api.getTask.mockRejectedValue(Object.assign(new Error('Task not found'), { status: 404 }))
    const { manager, wrapper } = setupManager()
    try {
      await manager.restore()
      await vi.advanceTimersByTimeAsync(18_000)
      expect(api.getTask).toHaveBeenCalledTimes(1)
      expect(manager.runningCount.value).toBe(0)
    } finally { wrapper.unmount() }
  })
})
