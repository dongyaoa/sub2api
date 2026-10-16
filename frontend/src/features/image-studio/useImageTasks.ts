import { computed, onBeforeUnmount, ref, type Ref } from 'vue'
import type { ApiKey } from '@/types'
import { deleteImageTask, extractTaskImageData, getImageTask, listImageTasks, submitImageEditTask, submitImageTask } from './api'
import type { GenerateImageRequest, ImageAspectRatio, ImageResolutionTier, ImageStudioPlatform, ImageTask, StudioOperation } from './types'

export interface StudioHistoryItem {
  id: string
  apiKeyId: number
  groupName: string
  platform: ImageStudioPlatform
  operation: StudioOperation
  prompt: string
  model: string
  resolution: ImageResolutionTier
  ratio: ImageAspectRatio
  quantity: number
  createdAt: number
  completedAt?: number
  status: 'processing' | 'completed' | 'failed'
  request: GenerateImageRequest
  task?: ImageTask
  thumbnail?: string
  errorMessage?: string
  pollWarning?: boolean
  // References only live in memory; never persist files or API Key secrets.
  sourceFiles?: File[]
}

interface TaskDraft {
  request: GenerateImageRequest
  operation: StudioOperation
  platform: ImageStudioPlatform
  resolution: ImageResolutionTier
  ratio: ImageAspectRatio
  sourceFiles: File[]
}

export function taskErrorText(error: unknown): string {
  if (typeof error === 'string') return error
  if (error && typeof error === 'object') {
    const value = error as { message?: string; error?: { message?: string } }
    return value.message || value.error?.message || ''
  }
  return ''
}

export function historyDuration(item: StudioHistoryItem, now: number): number | null {
  const end = item.status === 'processing' ? now : item.completedAt
  return end == null ? null : Math.max(0, Math.floor((end - item.createdAt) / 1000))
}

export function formatHistoryTime(timestamp: number): string {
  const date = new Date(timestamp)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function taskResolution(task: ImageTask): ImageResolutionTier {
  const value = (task.metadata?.resolution || task.metadata?.size || '').toUpperCase()
  if (value === '1K' || value === '2K' || value === '4K') return value
  const [width, height] = value.toLowerCase().split('x').map(Number)
  if (Math.max(width || 0, height || 0) > 2048) return '4K'
  return Math.max(width || 0, height || 0) > 1024 ? '2K' : '1K'
}

function taskRatio(task: ImageTask): ImageAspectRatio {
  const supported: ImageAspectRatio[] = ['1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9']
  if (task.metadata?.aspect_ratio && supported.includes(task.metadata.aspect_ratio)) return task.metadata.aspect_ratio
  const [width, height] = (task.metadata?.size || '').split('x').map(Number)
  if (!width || !height) return '1:1'
  return supported.reduce((best, ratio) => {
    const value = (text: string) => { const [a, b] = text.split(':').map(Number); return a / b }
    return Math.abs(value(ratio) - width / height) < Math.abs(value(best) - width / height) ? ratio : best
  }, '1:1')
}

function fromTask(task: ImageTask, key: ApiKey): StudioHistoryItem {
  const metadata = task.metadata || {}
  const quality = ['auto', 'low', 'medium', 'high'].includes(metadata.quality || '')
    ? metadata.quality as GenerateImageRequest['quality'] : undefined
  return {
    id: task.task_id || task.id,
    apiKeyId: key.id,
    groupName: metadata.group_name || key.group?.name || '-',
    platform: key.group?.platform as ImageStudioPlatform,
    operation: metadata.operation === 'edit' ? 'edit' : 'generate',
    prompt: metadata.prompt || '',
    model: metadata.model || '',
    resolution: taskResolution(task),
    ratio: taskRatio(task),
    quantity: Math.max(1, metadata.quantity || extractTaskImageData(task).length || 1),
    createdAt: task.created_at * 1000,
    completedAt: task.completed_at ? task.completed_at * 1000 : undefined,
    status: task.status,
    request: { model: metadata.model || '', prompt: metadata.prompt || '', n: metadata.quantity || 1,
      size: metadata.size || '', quality, aspect_ratio: metadata.aspect_ratio, resolution: metadata.resolution },
    task,
    thumbnail: extractTaskImageData(task)[0]?.url,
    errorMessage: taskErrorText(task.error),
  }
}

export function useImageTasks(keys: Ref<ApiKey[]>, notifyError: (message: string) => void, emptyResult: () => string) {
  const history = ref<StudioHistoryItem[]>([])
  const activeHistoryId = ref('')
  const activeHistoryItem = computed(() => history.value.find(item => item.id === activeHistoryId.value) || null)
  const runningCount = computed(() => history.value.filter(item => item.status === 'processing').length)
  const historyRetentionDays = ref(7)
  const loadingHistory = ref(false)
  const submitting = ref(false)
  const deletingHistoryId = ref('')
  const clearingHistory = ref(false)
  const controllers = new Map<string, AbortController>()
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  const loadingKeys = new Set<number>()
  const removedTaskIds = new Set<string>()
  let disposed = false
  let sequence = 0
  const storageKey = () => `image_studio_active_tasks_v2:${keys.value[0]?.user_id || 'local'}`

  function persist() {
    try {
      sessionStorage.setItem(storageKey(), JSON.stringify(history.value
        .filter(item => item.status === 'processing' && item.task)
        .map(item => ({ taskId: item.task!.task_id || item.task!.id, apiKeyId: item.apiKeyId }))))
    } catch { /* Storage may be unavailable; server history still owns tasks. */ }
  }

  function applyTask(id: string, task: ImageTask, key: ApiKey) {
    const index = history.value.findIndex(item => item.id === id)
    const previous = history.value[index]
    const item = { ...previous, ...fromTask(task, key) }
    // Submission snapshots remain authoritative if an older server omits metadata.
    if (previous) {
      Object.assign(item, { request: previous.request, prompt: previous.prompt, model: previous.model,
        resolution: previous.resolution, ratio: previous.ratio, quantity: previous.quantity,
        operation: previous.operation, groupName: previous.groupName, sourceFiles: previous.sourceFiles })
    }
    if (task.status === 'completed' && !extractTaskImageData(task).length) {
      item.status = 'failed'
      item.errorMessage = emptyResult()
    }
    item.pollWarning = false
    if (item.status !== 'processing') item.completedAt ||= Date.now()
    if (index >= 0) history.value.splice(index, 1, item)
    else history.value.unshift(item)
    // A history request can discover the server ID before the submission response arrives.
    history.value = history.value.filter((entry, position) => entry.id !== item.id || position === Math.max(index, 0))
    if (activeHistoryId.value === id) activeHistoryId.value = item.id
    persist()
    return item
  }

  function schedule(id: string, key: ApiKey, delay = 3000) {
    if (disposed || timers.has(id)) return
    timers.set(id, setTimeout(() => { timers.delete(id); void poll(id, key) }, delay))
  }

  async function poll(id: string, key: ApiKey) {
    if (disposed || controllers.has(id)) return
    const controller = new AbortController()
    controllers.set(id, controller)
    try {
      const task = await getImageTask(key.key, id, controller.signal)
      if (disposed) return
      const item = applyTask(id, task, key)
      if (item.status === 'processing') schedule(id, key)
    } catch (error) {
      if (disposed || (error as Error).name === 'AbortError') return
      const item = history.value.find(entry => entry.id === id)
      const status = (error as { status?: number }).status
      if ([401, 403, 404, 410].includes(status || 0)) {
        if (item) {
          item.status = 'failed'
          item.completedAt = Date.now()
          item.errorMessage = taskErrorText(error)
        }
        persist()
      } else {
        if (item) item.pollWarning = true
        // A network error is not a generation failure. Leave the server task running.
        schedule(id, key, 6000)
      }
    } finally { controllers.delete(id) }
  }

  async function loadHistory(key: ApiKey) {
    if (loadingKeys.has(key.id) || disposed) return
    loadingKeys.add(key.id)
    loadingHistory.value = true
    const controller = new AbortController()
    controllers.set(`history:${key.id}`, controller)
    try {
      const result = await listImageTasks(key.key, 50, controller.signal)
      if (disposed) return
      historyRetentionDays.value = result.retentionDays
      for (const task of result.tasks) {
        const id = task.task_id || task.id
        // Do not overwrite a fresher poll or a live submission with an older list snapshot.
        if (removedTaskIds.has(id)) continue
        if (!history.value.some(item => item.id === id)) {
          history.value.push(fromTask(task, key))
        }
        if (history.value.find(item => item.id === id)?.status === 'processing') schedule(id, key, 0)
      }
      history.value.sort((a, b) => b.createdAt - a.createdAt)
      persist()
    } catch (error) {
      if (!disposed && (error as Error).name !== 'AbortError') notifyError(taskErrorText(error))
    } finally {
      controllers.delete(`history:${key.id}`)
      loadingKeys.delete(key.id)
      loadingHistory.value = loadingKeys.size > 0
    }
  }

  async function restore() {
    // Read before list calls can refresh the persisted pending IDs.
    let pending: Array<{ taskId: string; apiKeyId: number }> = []
    try {
      const stored = JSON.parse(sessionStorage.getItem(storageKey()) || '[]')
      if (Array.isArray(stored)) pending = stored
      const legacy = JSON.parse(sessionStorage.getItem('image_studio_active_task_v1') || 'null')
      if (legacy?.taskId) pending.push(legacy)
    } catch { /* Ignore invalid browser state. */ }
    for (let offset = 0; offset < keys.value.length && !disposed; offset += 3) {
      await Promise.all(keys.value.slice(offset, offset + 3).map(loadHistory))
    }
    await Promise.all(pending.map(async item => {
      if (!item || typeof item.taskId !== 'string') return
      const key = keys.value.find(key => key.id === item.apiKeyId)
      if (key && !history.value.some(entry => entry.id === item.taskId)) await poll(item.taskId, key)
    }))
    persist()
  }

  async function submit(key: ApiKey, draft: TaskDraft) {
    if (submitting.value || disposed) return
    const id = `local-${Date.now()}-${++sequence}`
    const request = { ...draft.request }
    const sourceFiles = [...draft.sourceFiles]
    history.value.unshift({
      ...draft, request, sourceFiles, id, apiKeyId: key.id, groupName: key.group?.name || '-',
      prompt: request.prompt, model: request.model, quantity: request.n, createdAt: Date.now(), status: 'processing',
    })
    activeHistoryId.value = id
    submitting.value = true
    const controller = new AbortController()
    controllers.set(id, controller)
    try {
      const task = draft.operation === 'edit'
        ? await submitImageEditTask(key.key, sourceFiles, request, controller.signal)
        : await submitImageTask(key.key, request, controller.signal)
      if (disposed) return
      const item = applyTask(id, task, key)
      if (item.status === 'processing') void poll(item.id, key)
    } catch (error) {
      if (disposed || (error as Error).name === 'AbortError') return
      const item = history.value.find(entry => entry.id === id)
      if (item) Object.assign(item, { status: 'failed', completedAt: Date.now(), errorMessage: taskErrorText(error) })
    } finally {
      controllers.delete(id)
      submitting.value = false
    }
  }

  async function remove(item: StudioHistoryItem) {
    if (item.status === 'processing') return
    const key = keys.value.find(key => key.id === item.apiKeyId)
    if (!key) throw new Error('API Key unavailable')
    if (item.task) await deleteImageTask(key.key, item.task.task_id || item.task.id)
    removedTaskIds.add(item.id)
    const timer = timers.get(item.id)
    if (timer) clearTimeout(timer)
    timers.delete(item.id)
    controllers.get(item.id)?.abort()
    history.value = history.value.filter(entry => entry.id !== item.id)
    if (activeHistoryId.value === item.id) activeHistoryId.value = ''
  }

  onBeforeUnmount(() => {
    disposed = true
    controllers.forEach(controller => controller.abort())
    timers.forEach(timer => clearTimeout(timer))
    controllers.clear()
    timers.clear()
  })

  return { history, activeHistoryId, activeHistoryItem, runningCount, historyRetentionDays,
    loadingHistory, submitting, deletingHistoryId, clearingHistory, loadHistory, restore, submit, remove }
}
