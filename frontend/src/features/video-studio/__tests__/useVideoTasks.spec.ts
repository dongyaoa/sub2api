import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import type { GenerateVideoRequest, VideoTask } from '../types'
import { formatVideoHistoryTime, useVideoTasks, videoElapsedSeconds } from '../useVideoTasks'

const api = vi.hoisted(() => ({
  getTask: vi.fn(), listTasks: vi.fn(), submitTask: vi.fn(), clearTasks: vi.fn(), downloadContent: vi.fn(),
}))
vi.mock('../api', () => ({
  getVideoTask: api.getTask,
  listVideoTasks: api.listTasks,
  submitVideoTask: api.submitTask,
  clearVideoTasks: api.clearTasks,
  downloadVideoContent: api.downloadContent,
  getVideoRequestId: (task: VideoTask) => task.request_id || task.task_id || task.id || '',
}))

const firstKey = {
  id: 1, user_id: 7, key: 'first-secret', group: { id: 1, name: 'Video A', platform: 'grok' },
} as ApiKey
const secondKey = {
  ...firstKey, id: 2, key: 'second-secret', group: { ...firstKey.group, id: 2, name: 'Video B' },
} as ApiKey
const now = new Date(2026, 9, 16, 15, 23).getTime()
const storageKey = 'video_studio_active_tasks_v2:7'
const wrappers: Array<{ unmount: () => void }> = []

function request(prompt = 'A moving landscape'): GenerateVideoRequest {
  return { model: 'grok-imagine-video', prompt, resolution: '720p', aspect_ratio: '16:9', duration: 5 }
}

function task(id: string, overrides: Partial<VideoTask> = {}): VideoTask {
  return {
    request_id: id, status: 'processing', created_at: now / 1000,
    metadata: { ...request(), operation: 'text' }, ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function setupManager(initialKeys: ApiKey[] = [firstKey]) {
  let manager!: ReturnType<typeof useVideoTasks>
  const keys = ref(initialKeys)
  const error = vi.fn()
  const wrapper = mount({
    setup() {
      manager = useVideoTasks(keys, { error, missingId: () => 'Missing task ID', failed: () => 'Generation failed' })
      return () => null
    },
  })
  wrappers.push(wrapper)
  return { manager, keys, wrapper, error }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
  vi.resetAllMocks()
  sessionStorage.clear()
  api.listTasks.mockResolvedValue({ tasks: [], retentionDays: 7, persistentHistory: true })
  api.clearTasks.mockResolvedValue(undefined)
  api.downloadContent.mockResolvedValue(new Blob(['video'], { type: 'video/mp4' }))
})

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('video multitask submission and polling', () => {
  it('unlocks after submission and polls two text jobs independently with their owning keys', async () => {
    api.submitTask.mockResolvedValueOnce(task('first')).mockResolvedValueOnce(task('second'))
    api.getTask.mockImplementation(async (_secret: string, id: string) => id === 'first'
      ? task(id, { status: 'completed', completed_at: now / 1000 + 12, progress: 100 })
      : task(id, { status: 'failed', completed_at: now / 1000 + 10, error: { message: 'Upstream rejected task' } }))
    const { manager } = setupManager([firstKey, secondKey])

    await manager.submit(firstKey, 'text', request('First draft'))
    expect(manager.submitting.value).toBe(false)
    expect(manager.runningCount.value).toBe(1)
    await manager.submit(secondKey, 'text', request('Second draft'))
    expect(manager.runningCount.value).toBe(2)
    expect(manager.activeHistoryId.value).toBe('second')

    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(api.getTask).toHaveBeenCalledWith('first-secret', 'first', expect.any(AbortSignal))
    expect(api.getTask).toHaveBeenCalledWith('second-secret', 'second', expect.any(AbortSignal))
    expect(manager.history.value.find(item => item.requestId === 'first')).toMatchObject({
      status: 'completed', apiKeyId: 1, groupName: 'Video A', request: { prompt: 'First draft' },
    })
    expect(manager.history.value.find(item => item.requestId === 'second')).toMatchObject({
      status: 'failed', apiKeyId: 2, groupName: 'Video B', errorMessage: 'Upstream rejected task',
    })
    expect(manager.runningCount.value).toBe(0)
    expect(manager.activeHistoryId.value).toBe('second')
    const completed = manager.history.value.find(item => item.requestId === 'first')!
    expect(videoElapsedSeconds(completed, Date.now())).toBe(12)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(videoElapsedSeconds(completed, Date.now())).toBe(12)
    expect(api.getTask).toHaveBeenCalledTimes(2)
  })

  it('keeps request and source image snapshots isolated while image preparation is pending', async () => {
    const preparedImage = deferred<string>()
    const firstRequest = request('Animate the original reference')
    const prepareFirstImage = vi.fn(() => preparedImage.promise)
    api.submitTask.mockResolvedValueOnce(task('image-first')).mockResolvedValueOnce(task('image-second'))
    const { manager } = setupManager()

    const submission = manager.submit(firstKey, 'image', firstRequest, prepareFirstImage)
    expect(manager.submitting.value).toBe(true)
    firstRequest.prompt = 'A different draft'
    firstRequest.duration = 12
    firstRequest.resolution = '480p'
    preparedImage.resolve('data:image/png;base64,first-reference')
    await submission
    await manager.submit(firstKey, 'image', request('Animate the second reference'), async () => 'data:image/png;base64,second-reference')

    expect(api.submitTask.mock.calls[0]?.[1]).toEqual({
      ...request('Animate the original reference'), image: { url: 'data:image/png;base64,first-reference' },
    })
    expect(api.submitTask.mock.calls[1]?.[1]).toEqual({
      ...request('Animate the second reference'), image: { url: 'data:image/png;base64,second-reference' },
    })
    expect(manager.runningCount.value).toBe(2)
    expect(manager.history.value.every(item => !item.request.image)).toBe(true)
    const stored = sessionStorage.getItem(storageKey)!
    expect(JSON.parse(stored)).toHaveLength(2)
    expect(stored).not.toContain('first-reference')
    expect(stored).not.toContain('second-reference')
    expect(stored).not.toContain('first-secret')
    expect(JSON.parse(stored).map((item: { request: GenerateVideoRequest }) => item.request.prompt).sort()).toEqual([
      'Animate the original reference', 'Animate the second reference',
    ])
  })

  it('does not reselect a background job after the user opens a new empty task', async () => {
    api.submitTask.mockResolvedValue(task('background'))
    api.getTask.mockResolvedValue(task('background', { status: 'completed', completed_at: now / 1000 + 5 }))
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    manager.activeHistoryId.value = ''
    await vi.advanceTimersByTimeAsync(5000)
    expect(manager.history.value[0]?.status).toBe('completed')
    expect(manager.activeHistoryId.value).toBe('')
    expect(manager.activeHistoryItem.value).toBeNull()
  })

  it('freezes elapsed time at the observed terminal transition when the upstream omits completed_at', async () => {
    api.submitTask.mockResolvedValue(task('without-end-time'))
    api.getTask.mockResolvedValue(task('without-end-time', { status: 'completed' }))
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    await vi.advanceTimersByTimeAsync(3000)
    expect(videoElapsedSeconds(manager.history.value[0]!, Date.now())).toBe(3)
    await vi.advanceTimersByTimeAsync(2000)
    expect(videoElapsedSeconds(manager.history.value[0]!, Date.now())).toBe(5)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(videoElapsedSeconds(manager.history.value[0]!, Date.now())).toBe(5)
  })

  it('does not duplicate a task discovered by history before submission acknowledgement', async () => {
    const accepted = deferred<VideoTask>()
    api.submitTask.mockReturnValue(accepted.promise)
    api.listTasks.mockResolvedValue({ tasks: [task('discovered')], retentionDays: 7, persistentHistory: true })
    const { manager } = setupManager()
    const submission = manager.submit(firstKey, 'text', request('Original local draft'))
    await manager.loadHistory(firstKey)
    accepted.resolve(task('discovered'))
    await submission
    expect(manager.history.value).toHaveLength(1)
    expect(manager.history.value[0]?.request.prompt).toBe('Original local draft')
    expect(manager.activeHistoryId.value).toBe('discovered')
    expect(manager.runningCount.value).toBe(1)
  })

  it('does not overwrite a completed local task with a stale history response', async () => {
    const listed = deferred<{ tasks: VideoTask[]; retentionDays: number; persistentHistory: boolean }>()
    api.submitTask.mockResolvedValue(task('stale-list'))
    api.getTask.mockResolvedValue(task('stale-list', { status: 'completed', completed_at: now / 1000 + 5 }))
    api.listTasks.mockReturnValue(listed.promise)
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    const loading = manager.loadHistory(firstKey)
    await vi.advanceTimersByTimeAsync(5000)
    listed.resolve({ tasks: [task('stale-list')], retentionDays: 7, persistentHistory: true })
    await loading
    expect(manager.history.value).toHaveLength(1)
    expect(manager.history.value[0]?.status).toBe('completed')
    expect(manager.runningCount.value).toBe(0)
  })
})

describe('video task history and lifecycle', () => {
  it('formats timestamps as local month-day and hour-minute without inventing old elapsed times', async () => {
    expect(formatVideoHistoryTime(new Date(2026, 9, 15, 15, 23).getTime())).toBe('10-15 15:23')
    expect(formatVideoHistoryTime(new Date(2026, 0, 2, 3, 4).getTime())).toBe('01-02 03:04')
    api.listTasks.mockResolvedValue({ tasks: [task('old-complete', { status: 'completed' })], retentionDays: 7, persistentHistory: true })
    const { manager } = setupManager()
    await manager.restore()
    expect(videoElapsedSeconds(manager.history.value[0]!, Date.now())).toBeNull()
  })

  it('restores multiple running jobs from server history and saved IDs across keys', async () => {
    sessionStorage.setItem(storageKey, JSON.stringify([
      { requestId: 'server-first', apiKeyId: 1 },
      { requestId: 'saved-second', apiKeyId: 2 },
    ]))
    api.listTasks.mockImplementation(async (secret: string) => ({
      tasks: secret === 'first-secret' ? [task('server-first')] : [],
      retentionDays: 14, persistentHistory: true,
    }))
    api.getTask.mockImplementation(async (_secret: string, id: string) => task(id))
    const { manager } = setupManager([firstKey, secondKey])
    await manager.restore()
    await vi.advanceTimersByTimeAsync(0)
    expect(manager.runningCount.value).toBe(2)
    expect(manager.history.value.map(item => item.requestId).sort()).toEqual(['saved-second', 'server-first'])
    expect(api.getTask).toHaveBeenCalledWith('first-secret', 'server-first', expect.any(AbortSignal))
    expect(api.getTask).toHaveBeenCalledWith('second-secret', 'saved-second', expect.any(AbortSignal))
    expect(manager.retentionDays.value).toBe(14)
    expect(manager.persistentHistory.value).toBe(true)
    expect(JSON.parse(sessionStorage.getItem(storageKey)!)).toHaveLength(2)
    expect(manager.activeHistoryId.value).toBe('')
  })

  it('retries transient polling errors without failing the task or disturbing selection', async () => {
    api.listTasks.mockResolvedValue({ tasks: [task('retry')], retentionDays: 7, persistentHistory: true })
    api.getTask.mockRejectedValueOnce(new TypeError('Temporary connection failure'))
      .mockResolvedValueOnce(task('retry', { status: 'completed', completed_at: now / 1000 + 5 }))
    const { manager } = setupManager()
    await manager.restore()
    manager.activeHistoryId.value = 'retry'
    await vi.advanceTimersByTimeAsync(0)
    expect(manager.history.value[0]).toMatchObject({ status: 'processing', pollWarning: 'Temporary connection failure' })
    expect(manager.runningCount.value).toBe(1)
    await vi.advanceTimersByTimeAsync(5000)
    expect(manager.history.value[0]).toMatchObject({ status: 'completed' })
    expect(manager.history.value[0]?.pollWarning).toBeUndefined()
    expect(manager.activeHistoryId.value).toBe('retry')
    expect(api.getTask).toHaveBeenCalledTimes(2)
  })

  it('stops retrying an expired saved job missing from server history', async () => {
    sessionStorage.setItem(storageKey, JSON.stringify([{ requestId: 'expired', apiKeyId: 1 }]))
    api.getTask.mockRejectedValue(Object.assign(new Error('Task not found'), { status: 404 }))
    const { manager } = setupManager()
    await manager.restore()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getTask).toHaveBeenCalledTimes(1)
    expect(manager.runningCount.value).toBe(0)
    expect(JSON.parse(sessionStorage.getItem(storageKey)!)).toEqual([])
  })

  it('blocks clearing while jobs run and clears each owning key only after completion', async () => {
    api.listTasks.mockImplementation(async (secret: string) => ({
      tasks: secret === 'first-secret' ? [task('running')] : [task('done', { status: 'completed' })],
      retentionDays: 7, persistentHistory: true,
    }))
    api.getTask.mockResolvedValue(task('running', { status: 'completed', completed_at: now / 1000 + 5 }))
    const { manager } = setupManager([firstKey, secondKey])
    await manager.restore()
    await manager.clearHistory()
    expect(api.clearTasks).not.toHaveBeenCalled()
    expect(manager.history.value).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(0)
    manager.activeHistoryId.value = 'done'
    await manager.clearHistory()
    expect(api.clearTasks.mock.calls.map(call => call[0])).toEqual(['first-secret', 'second-secret'])
    expect(manager.history.value).toEqual([])
    expect(manager.activeHistoryId.value).toBe('')
    expect(manager.clearingHistory.value).toBe(false)
  })

  it('aborts in-flight polling on unmount and ignores a late response without losing saved jobs', async () => {
    const pendingPoll = deferred<VideoTask>()
    api.submitTask.mockResolvedValue(task('unmount'))
    api.getTask.mockReturnValue(pendingPoll.promise)
    const { manager, wrapper } = setupManager()
    await manager.submit(firstKey, 'text', request())
    await vi.advanceTimersByTimeAsync(5000)
    const signal = api.getTask.mock.calls[0]?.[2] as AbortSignal
    expect(signal.aborted).toBe(false)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    pendingPoll.resolve(task('unmount', { status: 'completed' }))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getTask).toHaveBeenCalledTimes(1)
    expect(manager.history.value[0]?.status).toBe('processing')
    expect(JSON.parse(sessionStorage.getItem(storageKey)!)).toEqual([{
      requestId: 'unmount', apiKeyId: 1, operation: 'text', request: request(), startedAt: now,
    }])
  })

  it('cancels scheduled polls on unmount before they start', async () => {
    api.submitTask.mockResolvedValue(task('timer'))
    const { manager, wrapper } = setupManager()
    await manager.submit(firstKey, 'text', request())
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getTask).not.toHaveBeenCalled()
  })

  it.each(['empty', 'failed'] as const)('preserves saved task parameters when history is %s and status has no metadata', async (historyResult) => {
    const savedRequest: GenerateVideoRequest = {
      model: 'grok-imagine-video-1.5', prompt: 'A saved detailed portrait animation',
      resolution: '1080p', aspect_ratio: '9:16', duration: 12,
    }
    const startedAt = now - 32_000
    sessionStorage.setItem(storageKey, JSON.stringify([{
      requestId: 'snapshot', apiKeyId: 2, operation: 'image',
      request: { ...savedRequest, image: { url: 'data:image/png;base64,do-not-restore' } }, startedAt,
    }]))
    if (historyResult === 'failed') api.listTasks.mockRejectedValue(new Error('History temporarily unavailable'))
    const status = deferred<VideoTask>()
    api.getTask.mockReturnValue(status.promise)
    const { manager } = setupManager([firstKey, secondKey])
    const restoring = manager.restore()
    await flushPromises()
    expect(manager.loadingHistory.value).toBe(true)
    expect(api.getTask).toHaveBeenCalledWith('second-secret', 'snapshot', expect.any(AbortSignal))
    status.resolve({ request_id: 'snapshot', status: 'processing' })
    await restoring
    expect(manager.loadingHistory.value).toBe(false)
    expect(manager.history.value).toHaveLength(1)
    expect(manager.history.value[0]).toMatchObject({
      requestId: 'snapshot', apiKeyId: 2, groupName: 'Video B', operation: 'image',
      request: savedRequest, startedAt, status: 'processing',
    })
    expect(manager.history.value[0]?.request.image).toBeUndefined()
    expect(videoElapsedSeconds(manager.history.value[0]!, Date.now())).toBe(32)
    const restored = JSON.parse(sessionStorage.getItem(storageKey)!)
    expect(restored[0].request).toEqual(savedRequest)
    expect(restored[0].startedAt).toBe(startedAt)
    expect(sessionStorage.getItem(storageKey)).not.toContain('do-not-restore')
  })

  it('removes the legacy saved task after migration so clearing history cannot revive it', async () => {
    sessionStorage.setItem('video_studio_active_task_v1', JSON.stringify({
      requestId: 'legacy-migration', apiKeyId: 1, operation: 'text', request: request('Legacy prompt'), startedAt: now,
    }))
    api.getTask.mockResolvedValue({ request_id: 'legacy-migration', status: 'completed', content_available: true })
    const { manager } = setupManager()
    await manager.restore()
    expect(sessionStorage.getItem('video_studio_active_task_v1')).toBeNull()
    expect(manager.history.value[0]?.request.prompt).toBe('Legacy prompt')
    await manager.clearHistory()
    expect(manager.history.value).toEqual([])
    expect(api.clearTasks).toHaveBeenCalledWith('first-secret')
    await manager.restore()
    expect(manager.history.value).toEqual([])
    expect(api.getTask).toHaveBeenCalledTimes(1)
  })

  it('applies the authoritative server completion time to a restored snapshot without polling again', async () => {
    const startedAt = now - 120_000
    const completedAt = startedAt + 42_000
    const savedRequest = request('Preserve these original settings')
    sessionStorage.setItem(storageKey, JSON.stringify([{
      requestId: 'completed-snapshot', apiKeyId: 1, operation: 'image', request: savedRequest, startedAt,
    }]))
    api.listTasks.mockResolvedValue({
      tasks: [{
        request_id: 'completed-snapshot', status: 'completed',
        created_at: startedAt / 1000, completed_at: completedAt / 1000, content_available: true,
      }], retentionDays: 7, persistentHistory: true,
    })
    const { manager } = setupManager()
    await manager.restore()
    expect(manager.history.value).toHaveLength(1)
    const restored = manager.history.value[0]!
    expect(restored).toMatchObject({
      status: 'completed', operation: 'image', request: savedRequest, startedAt, completedAt,
    })
    expect(videoElapsedSeconds(restored, Date.now())).toBe(42)
    expect(manager.runningCount.value).toBe(0)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(videoElapsedSeconds(restored, Date.now())).toBe(42)
    expect(api.getTask).not.toHaveBeenCalled()
    expect(api.downloadContent).not.toHaveBeenCalled()
    expect(JSON.parse(sessionStorage.getItem(storageKey)!)).toEqual([])
  })
})

describe('video content delivery', () => {
  it('rejects content retrieval while clearing history without starting a download', async () => {
    const clearing = deferred<void>()
    api.clearTasks.mockReturnValue(clearing.promise)
    api.listTasks.mockResolvedValue({
      tasks: [task('clearing-video', { status: 'completed', content_available: true })],
      retentionDays: 7, persistentHistory: true,
    })
    const { manager } = setupManager()
    await manager.restore()
    const item = manager.history.value[0]!
    const cleanup = manager.clearHistory()
    expect(manager.clearingHistory.value).toBe(true)
    await expect(manager.getContent(item)).rejects.toThrow('Video history is being cleared')
    expect(api.downloadContent).not.toHaveBeenCalled()
    expect(manager.deliveringCount.value).toBe(0)
    clearing.resolve(undefined)
    await cleanup
    expect(manager.history.value).toEqual([])
    expect(manager.clearingHistory.value).toBe(false)
  })

  it('delivers completed background videos with their owning keys without selecting them', async () => {
    const delivery = deferred<Blob>()
    api.submitTask.mockResolvedValue(task('background-content'))
    api.getTask.mockResolvedValue(task('background-content', { status: 'completed', content_available: false }))
    api.downloadContent.mockReturnValue(delivery.promise)
    const { manager } = setupManager([firstKey, secondKey])
    await manager.submit(secondKey, 'text', request())
    manager.activeHistoryId.value = ''
    await vi.advanceTimersByTimeAsync(5000)

    expect(api.downloadContent).toHaveBeenCalledWith('second-secret', 'background-content', expect.any(AbortSignal))
    expect(manager.deliveringCount.value).toBe(1)
    expect(manager.history.value[0]?.delivering).toBe(true)
    expect(manager.activeHistoryId.value).toBe('')
    delivery.resolve(new Blob(['background-video']))
    await flushPromises()
    expect(manager.deliveringCount.value).toBe(0)
    expect(manager.history.value[0]?.task?.content_available).toBe(true)
    expect(manager.activeHistoryId.value).toBe('')
  })

  it('shares one pending download between player and background delivery and reuses the cached result', async () => {
    const delivery = deferred<Blob>()
    const video = new Blob(['shared-video'])
    api.submitTask.mockResolvedValue(task('shared', { status: 'completed', content_available: false }))
    api.downloadContent.mockReturnValue(delivery.promise)
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    const item = manager.history.value[0]!
    const firstPlayback = manager.getContent(item)
    const secondPlayback = manager.getContent(item)
    expect(firstPlayback).toBe(secondPlayback)
    expect(api.downloadContent).toHaveBeenCalledTimes(1)
    expect(manager.deliveringCount.value).toBe(1)
    delivery.resolve(video)
    expect(await firstPlayback).toBe(video)
    expect(await secondPlayback).toBe(video)
    await flushPromises()
    expect(await manager.getContent(item)).toBe(video)
    expect(api.downloadContent).toHaveBeenCalledTimes(1)
    expect(manager.deliveringCount.value).toBe(0)
  })

  it('forces a new content download after cached playback failure while deduplicating concurrent reloads', async () => {
    const staleVideo = new Blob(['cached-video'])
    const freshVideo = new Blob(['fresh-video'])
    const reloading = deferred<Blob>()
    api.listTasks.mockResolvedValue({
      tasks: [task('force-refresh', { status: 'completed', content_available: true })],
      retentionDays: 7, persistentHistory: true,
    })
    api.downloadContent.mockResolvedValueOnce(staleVideo).mockReturnValueOnce(reloading.promise)
    const { manager } = setupManager()
    await manager.restore()
    const item = manager.history.value[0]!
    expect(await manager.getContent(item)).toBe(staleVideo)
    await flushPromises()
    expect(await manager.getContent(item)).toBe(staleVideo)
    expect(api.downloadContent).toHaveBeenCalledTimes(1)
    const firstReload = manager.getContent(item, true)
    const secondReload = manager.getContent(item, true)
    const concurrentPlayback = manager.getContent(item)
    expect(firstReload).toBe(secondReload)
    expect(firstReload).toBe(concurrentPlayback)
    expect(api.downloadContent).toHaveBeenCalledTimes(2)
    expect(manager.deliveringCount.value).toBe(1)
    reloading.resolve(freshVideo)
    expect(await firstReload).toBe(freshVideo)
    await flushPromises()
    expect(await manager.getContent(item)).toBe(freshVideo)
    expect(api.downloadContent).toHaveBeenCalledTimes(2)
    expect(manager.deliveringCount.value).toBe(0)
  })

  it('does not preload stored or legacy completed history and only delivers explicitly missing content', async () => {
    api.listTasks.mockResolvedValue({
      tasks: [
        task('stored', { status: 'completed', content_available: true }),
        task('legacy', { status: 'completed' }),
        task('undelivered', { status: 'completed', content_available: false }),
      ], retentionDays: 7, persistentHistory: true,
    })
    const { manager } = setupManager()
    await manager.restore()
    await flushPromises()
    expect(api.downloadContent).toHaveBeenCalledTimes(1)
    expect(api.downloadContent).toHaveBeenCalledWith('first-secret', 'undelivered', expect.any(AbortSignal))
    expect(manager.history.value).toHaveLength(3)
    expect(manager.activeHistoryId.value).toBe('')
  })

  it('blocks history clearing throughout content delivery even after generation has completed', async () => {
    const delivery = deferred<Blob>()
    api.submitTask.mockResolvedValue(task('delivering', { status: 'completed', content_available: false }))
    api.downloadContent.mockReturnValue(delivery.promise)
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    expect(manager.runningCount.value).toBe(0)
    expect(manager.deliveringCount.value).toBe(1)
    await manager.clearHistory()
    expect(api.clearTasks).not.toHaveBeenCalled()
    expect(manager.history.value).toHaveLength(1)
    delivery.resolve(new Blob(['video']))
    await flushPromises()
    expect(manager.deliveringCount.value).toBe(0)
    await manager.clearHistory()
    expect(api.clearTasks).toHaveBeenCalledWith('first-secret')
    expect(manager.history.value).toEqual([])
  })

  it('limits background content transfers to two and starts the next queued transfer when a slot frees', async () => {
    const deliveries = new Map<string, ReturnType<typeof deferred<Blob>>>()
    for (const id of ['one', 'two', 'three']) deliveries.set(id, deferred<Blob>())
    api.listTasks.mockResolvedValue({
      tasks: [...deliveries.keys()].map(id => task(id, { status: 'completed', content_available: false })),
      retentionDays: 7, persistentHistory: true,
    })
    api.downloadContent.mockImplementation((_secret: string, id: string) => deliveries.get(id)!.promise)
    const { manager } = setupManager()
    await manager.restore()
    expect(manager.deliveringCount.value).toBe(3)
    expect(api.downloadContent.mock.calls.map(call => call[1])).toEqual(['one', 'two'])
    deliveries.get('one')!.resolve(new Blob(['one']))
    await flushPromises()
    expect(api.downloadContent.mock.calls.map(call => call[1])).toEqual(['one', 'two', 'three'])
    expect(manager.deliveringCount.value).toBe(2)
    deliveries.get('two')!.resolve(new Blob(['two']))
    deliveries.get('three')!.resolve(new Blob(['three']))
    await flushPromises()
    expect(manager.deliveringCount.value).toBe(0)
  })

  it('keeps generation completed after delivery failure and permits retry without resubmission', async () => {
    api.submitTask.mockResolvedValue(task('retry-content', { status: 'completed', content_available: false }))
    api.downloadContent.mockRejectedValueOnce(new Error('Content temporarily unavailable'))
      .mockResolvedValueOnce(new Blob(['retried-video']))
    const { manager } = setupManager()
    await manager.submit(firstKey, 'text', request())
    await flushPromises()
    const item = manager.history.value[0]!
    expect(item).toMatchObject({ status: 'completed', deliveryError: 'Content temporarily unavailable', delivering: false })
    expect(manager.deliveringCount.value).toBe(0)
    await manager.getContent(item)
    await flushPromises()
    expect(item.deliveryError).toBe('')
    expect(item.task?.content_available).toBe(true)
    expect(api.submitTask).toHaveBeenCalledTimes(1)
    expect(api.downloadContent).toHaveBeenCalledTimes(2)
  })

  it('aborts active content downloads on unmount and rejects queued playback without starting more transfers', async () => {
    api.listTasks.mockResolvedValue({
      tasks: ['active-one', 'active-two', 'queued'].map(id => task(id, { status: 'completed', content_available: false })),
      retentionDays: 7, persistentHistory: true,
    })
    api.downloadContent.mockImplementation((_secret: string, _id: string, signal: AbortSignal) => new Promise<Blob>((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))
    const { manager, wrapper } = setupManager()
    await manager.restore()
    const queued = manager.getContent(manager.history.value.find(item => item.requestId === 'queued')!)
    const rejected = expect(queued).rejects.toMatchObject({ name: 'AbortError' })
    const signals = api.downloadContent.mock.calls.map(call => call[2] as AbortSignal)
    wrapper.unmount()
    await rejected
    await flushPromises()
    expect(signals).toHaveLength(2)
    expect(signals.every(signal => signal.aborted)).toBe(true)
    expect(api.downloadContent).toHaveBeenCalledTimes(2)
  })
})
