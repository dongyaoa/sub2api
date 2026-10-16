import { computed, onBeforeUnmount, ref, type Ref } from 'vue'
import type { ApiKey } from '@/types'
import { clearVideoTasks, downloadVideoContent, getVideoRequestId, getVideoTask, listVideoTasks, submitVideoTask } from './api'
import { VIDEO_ASPECT_RATIOS } from './capabilities'
import { normalizeVideoTaskState, shouldRetryVideoPollError, videoTaskPollDelay } from './polling'
import type { GenerateVideoRequest, StoredVideoHistoryItem, VideoResolution, VideoStudioOperation, VideoTask } from './types'

export interface VideoHistoryItem extends StoredVideoHistoryItem {
  completedAt?: number
  task?: VideoTask
  pollWarning?: string
  delivering?: boolean
  deliveryError?: string
  groupName: string
}

export function videoTaskError(error: unknown): string {
  if (typeof error === 'string') return error
  if (!error || typeof error !== 'object') return ''
  const value = error as { message?: string; code?: string | number }
  return String(value.message || value.code || '')
}

export function videoElapsedSeconds(item: VideoHistoryItem, now: number): number | null {
  const end = item.status === 'processing' ? now : item.completedAt
  return end == null ? null : Math.max(0, Math.floor((end - item.startedAt) / 1000))
}

export function formatVideoHistoryTime(timestamp: number): string {
  const date = new Date(timestamp)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function safeRequest(request: GenerateVideoRequest): GenerateVideoRequest {
  // Neither source images nor API Key secrets belong in task history/browser storage.
  return { model: request.model, prompt: request.prompt, resolution: request.resolution,
    aspect_ratio: request.aspect_ratio, duration: request.duration }
}

function serverTime(value?: number): number | undefined {
  return Number.isFinite(value) && Number(value) > 0 ? Number(value) * 1000 : undefined
}

function fromTask(task: VideoTask, key: ApiKey): VideoHistoryItem {
  const metadata = task.metadata || {}
  return {
    requestId: getVideoRequestId(task), apiKeyId: key.id, groupName: key.group?.name || '-',
    operation: metadata.operation === 'image' ? 'image' : 'text',
    request: { model: metadata.model || '', prompt: metadata.prompt || '',
      resolution: ['480p', '720p', '1080p'].includes(metadata.resolution || '') ? metadata.resolution as VideoResolution : '480p',
      aspect_ratio: metadata.aspect_ratio && VIDEO_ASPECT_RATIOS.includes(metadata.aspect_ratio) ? metadata.aspect_ratio : undefined,
      duration: Math.max(1, Number(metadata.duration) || 4) },
    startedAt: serverTime(task.created_at) || Date.now(),
    completedAt: serverTime(task.completed_at),
    status: normalizeVideoTaskState(task.status), task, errorMessage: videoTaskError(task.error),
  }
}

export function useVideoTasks(keys: Ref<ApiKey[]>, messages: { error: (message: string) => void; missingId: () => string; failed: () => string }) {
  const history = ref<VideoHistoryItem[]>([])
  const activeHistoryId = ref('')
  const activeHistoryItem = computed(() => history.value.find(item => item.requestId === activeHistoryId.value) || null)
  const runningCount = computed(() => history.value.filter(item => item.status === 'processing').length)
  const deliveringCount = computed(() => history.value.filter(item => item.delivering).length)
  const submitting = ref(false)
  const restoring = ref(false)
  const listingHistory = ref(false)
  const loadingHistory = computed(() => restoring.value || listingHistory.value)
  const clearingHistory = ref(false)
  const retentionDays = ref(7)
  const persistentHistory = ref(false)
  const controllers = new Map<string, AbortController>()
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  const loadingKeys = new Set<number>()
  const removed = new Set<string>()
  const restoredSnapshots = new Set<string>()
  const contentCache = new Map<string, Blob>()
  const contentPromises = new Map<string, Promise<Blob>>()
  const deliveryQueue: Array<{ id: string; key: ApiKey; resolve: (blob: Blob) => void; reject: (error: unknown) => void }> = []
  let activeDeliveries = 0
  let sequence = 0
  let disposed = false
  const storageKey = () => `video_studio_active_tasks_v2:${keys.value[0]?.user_id || 'local'}`

  function pumpDeliveries() {
    // Content retrieval performs backend persistence/billing. Keep it independent
    // of the player, with at most two transfers and a bounded in-memory cache.
    while (!disposed && activeDeliveries < 2 && deliveryQueue.length) {
      const job = deliveryQueue.shift()!
      const controller = new AbortController()
      const controllerID = `content:${job.id}`
      controllers.set(controllerID, controller)
      activeDeliveries++
      void downloadVideoContent(job.key.key, job.id, controller.signal).then(blob => {
        if (disposed) throw new DOMException('Disposed', 'AbortError')
        if (blob.size <= 128 * 1024 * 1024) {
          contentCache.set(job.id, blob)
          while (contentCache.size > 2 || [...contentCache.values()].reduce((total, item) => total + item.size, 0) > 128 * 1024 * 1024) {
            contentCache.delete(contentCache.keys().next().value!)
          }
        }
        const item = history.value.find(entry => entry.requestId === job.id)
        if (item?.task) item.task.content_available = true
        job.resolve(blob)
      }).catch(error => {
        const item = history.value.find(entry => entry.requestId === job.id)
        if (!disposed && item && (error as Error).name !== 'AbortError') item.deliveryError = videoTaskError(error)
        job.reject(error)
      }).finally(() => {
        const item = history.value.find(entry => entry.requestId === job.id)
        if (item) item.delivering = false
        controllers.delete(controllerID)
        contentPromises.delete(job.id)
        activeDeliveries--
        pumpDeliveries()
      })
    }
  }

  function getContent(item: VideoHistoryItem, refresh = false): Promise<Blob> {
    if (disposed) return Promise.reject(new DOMException('Disposed', 'AbortError'))
    if (clearingHistory.value) return Promise.reject(new Error('Video history is being cleared'))
    if (refresh) contentCache.delete(item.requestId)
    const cached = contentCache.get(item.requestId)
    if (cached) return Promise.resolve(cached)
    const pending = contentPromises.get(item.requestId)
    if (pending) return pending
    const key = keys.value.find(key => key.id === item.apiKeyId)
    if (!key) return Promise.reject(new Error('The API key used for this video is unavailable'))
    item.delivering = true
    item.deliveryError = ''
    const promise = new Promise<Blob>((resolve, reject) => {
      deliveryQueue.push({ id: item.requestId, key, resolve, reject })
    })
    contentPromises.set(item.requestId, promise)
    pumpDeliveries()
    return promise
  }

  function deliverCompleted(item: VideoHistoryItem) {
    if (item.status === 'completed' && item.task?.content_available !== true) {
      // The error is displayed on its history card and can be retried by opening it.
      void getContent(item).catch(() => {})
    }
  }

  function persist() {
    try {
      sessionStorage.setItem(storageKey(), JSON.stringify(history.value
        .filter(item => item.status === 'processing' && item.task)
        .map(item => ({ requestId: item.requestId, apiKeyId: item.apiKeyId,
          operation: item.operation, request: safeRequest(item.request), startedAt: item.startedAt }))))
    } catch { /* Server-side history still owns tasks when browser storage is unavailable. */ }
  }

  function applyTask(id: string, task: VideoTask, key: ApiKey): VideoHistoryItem {
    restoredSnapshots.delete(id)
    const index = history.value.findIndex(item => item.requestId === id)
    const previous = history.value[index]
    const item = fromTask(task, key)
    if (previous) {
      item.request = previous.request
      item.operation = previous.operation
      item.groupName = previous.groupName
      item.startedAt = serverTime(task.created_at) || previous.startedAt
      item.completedAt = serverTime(task.completed_at) || previous.completedAt
      item.delivering = previous.delivering
      item.deliveryError = previous.deliveryError
    }
    if (item.status !== 'processing' && previous?.status === 'processing') item.completedAt ||= Date.now()
    if (item.status === 'failed') item.errorMessage ||= messages.failed()
    if (index >= 0) history.value.splice(index, 1, item)
    else history.value.unshift(item)
    history.value = history.value.filter((entry, position) => entry.requestId !== item.requestId || position === Math.max(index, 0))
    if (activeHistoryId.value === id) activeHistoryId.value = item.requestId
    deliverCompleted(history.value.find(entry => entry.requestId === item.requestId)!)
    persist()
    return item
  }

  function schedule(id: string, key: ApiKey, startedAt: number, immediate = false) {
    if (disposed || timers.has(id) || removed.has(id)) return
    timers.set(id, setTimeout(() => { timers.delete(id); void poll(id, key) }, immediate ? 0 : videoTaskPollDelay(startedAt)))
  }

  async function poll(id: string, key: ApiKey) {
    if (disposed || controllers.has(id) || removed.has(id)) return
    const controller = new AbortController()
    controllers.set(id, controller)
    try {
      const task = await getVideoTask(key.key, id, controller.signal)
      if (disposed || removed.has(id)) return
      const item = applyTask(id, { ...task, request_id: getVideoRequestId(task) || id }, key)
      if (item.status === 'processing') schedule(item.requestId, key, item.startedAt)
    } catch (error) {
      if (disposed || (error as Error).name === 'AbortError') return
      const item = history.value.find(entry => entry.requestId === id)
      if (shouldRetryVideoPollError(error)) {
        if (item) item.pollWarning = videoTaskError(error)
        schedule(id, key, item?.startedAt || Date.now())
      } else {
        if (item) Object.assign(item, { status: 'failed', completedAt: Date.now(), errorMessage: videoTaskError(error) || messages.failed() })
        persist()
      }
    } finally { controllers.delete(id) }
  }

  async function loadHistory(key: ApiKey) {
    if (disposed || loadingKeys.has(key.id)) return
    loadingKeys.add(key.id)
    listingHistory.value = true
    const controller = new AbortController()
    controllers.set(`history:${key.id}`, controller)
    try {
      const result = await listVideoTasks(key.key, 50, controller.signal)
      if (disposed) return
      retentionDays.value = result.retentionDays
      persistentHistory.value = result.persistentHistory
      for (const task of result.tasks) {
        const id = getVideoRequestId(task)
        if (!id || removed.has(id)) continue
        // A late list response must not overwrite newer local/polled task state.
        if (restoredSnapshots.has(id)) applyTask(id, task, key)
        else if (!history.value.some(item => item.requestId === id)) history.value.push(fromTask(task, key))
        const item = history.value.find(entry => entry.requestId === id)!
        if (item.status === 'processing') schedule(id, key, item.startedAt, true)
        else if (item.task?.content_available === false) deliverCompleted(item)
      }
      history.value.sort((a, b) => b.startedAt - a.startedAt)
      persist()
    } catch (error) {
      if (!disposed && (error as Error).name !== 'AbortError') messages.error(videoTaskError(error))
    } finally {
      controllers.delete(`history:${key.id}`)
      loadingKeys.delete(key.id)
      listingHistory.value = loadingKeys.size > 0
    }
  }

  async function restore() {
    if (disposed || restoring.value) return
    restoring.value = true
    let pending: Array<{ requestId: string; apiKeyId: number; request?: GenerateVideoRequest; operation?: VideoStudioOperation; startedAt?: number }> = []
    try {
      const stored = JSON.parse(sessionStorage.getItem(storageKey()) || '[]')
      if (Array.isArray(stored)) pending = stored
      const legacy = JSON.parse(sessionStorage.getItem('video_studio_active_task_v1') || 'null')
      if (legacy?.requestId) pending.push(legacy)
    } catch { /* Ignore invalid browser state. */ }
    try {
      // Restore safe snapshots before listing so an interrupted list response or a
      // metadata-free status response cannot erase the prompt and generation settings.
      for (const item of pending) {
        const key = keys.value.find(key => key.id === item?.apiKeyId)
        if (!key || typeof item.requestId !== 'string' || !item.request?.model || history.value.some(entry => entry.requestId === item.requestId)) continue
        history.value.push({ requestId: item.requestId, apiKeyId: key.id, groupName: key.group?.name || '-',
          operation: item.operation === 'image' ? 'image' : 'text', request: safeRequest(item.request),
          startedAt: Number(item.startedAt) || Date.now(), status: 'processing',
          task: { request_id: item.requestId, status: 'processing' } })
        restoredSnapshots.add(item.requestId)
      }
      for (let offset = 0; offset < keys.value.length && !disposed; offset += 3) {
        await Promise.all(keys.value.slice(offset, offset + 3).map(loadHistory))
      }
      await Promise.all(pending.map(async item => {
        if (!item || typeof item.requestId !== 'string') return
        const key = keys.value.find(key => key.id === item.apiKeyId)
        const existing = history.value.find(entry => entry.requestId === item.requestId)
        if (key && (!existing || existing.status === 'processing') && !timers.has(item.requestId)) await poll(item.requestId, key)
      }))
      persist()
      try { sessionStorage.removeItem('video_studio_active_task_v1') } catch { /* Optional migration cleanup. */ }
    } finally { restoring.value = false }
  }

  async function submit(key: ApiKey, operation: VideoStudioOperation, request: GenerateVideoRequest, prepareImage?: () => Promise<string>) {
    if (submitting.value || clearingHistory.value || disposed) return
    const id = `local-video-${Date.now()}-${++sequence}`
    const snapshot = safeRequest(request)
    history.value.unshift({ requestId: id, apiKeyId: key.id, operation, request: snapshot,
      startedAt: Date.now(), status: 'processing', groupName: key.group?.name || '-' })
    activeHistoryId.value = id
    submitting.value = true
    const controller = new AbortController()
    controllers.set(id, controller)
    try {
      const imageURL = prepareImage ? await prepareImage() : request.image?.url
      if (disposed) return
      const payload = { ...snapshot, ...(operation === 'image' && imageURL ? { image: { url: imageURL } } : {}) }
      const task = await submitVideoTask(key.key, payload, controller.signal)
      if (disposed) return
      if (!getVideoRequestId(task)) throw new Error(messages.missingId())
      const item = applyTask(id, task, key)
      if (item.status === 'processing') schedule(item.requestId, key, item.startedAt)
    } catch (error) {
      if (!disposed && (error as Error).name !== 'AbortError') {
        const item = history.value.find(entry => entry.requestId === id)
        if (item) Object.assign(item, { status: 'failed', completedAt: Date.now(), errorMessage: videoTaskError(error) || messages.failed() })
      }
    } finally { controllers.delete(id); submitting.value = false }
  }

  async function clearHistory() {
    // The API clears a whole Key's history: never call it while any task is running.
    if (runningCount.value || deliveringCount.value || submitting.value || loadingHistory.value || clearingHistory.value) return
    clearingHistory.value = true
    try {
      for (const key of keys.value.filter(key => history.value.some(item => item.apiKeyId === key.id))) {
        await clearVideoTasks(key.key)
        history.value.filter(item => item.apiKeyId === key.id).forEach(item => {
          removed.add(item.requestId)
          contentCache.delete(item.requestId)
        })
        history.value = history.value.filter(item => item.apiKeyId !== key.id)
        if (!activeHistoryItem.value) activeHistoryId.value = ''
      }
      persist()
    } finally { clearingHistory.value = false }
  }

  onBeforeUnmount(() => {
    disposed = true
    controllers.forEach(controller => controller.abort())
    timers.forEach(timer => clearTimeout(timer))
    controllers.clear()
    timers.clear()
    deliveryQueue.splice(0).forEach(job => job.reject(new DOMException('Disposed', 'AbortError')))
    contentCache.clear()
  })

  return { history, activeHistoryId, activeHistoryItem, runningCount, submitting, loadingHistory,
    clearingHistory, deliveringCount, retentionDays, persistentHistory, loadHistory, restore, submit, clearHistory, getContent }
}
