import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VideoStudioView from '../VideoStudioView.vue'
import type { GenerateVideoRequest, VideoTask } from '../types'

const api = vi.hoisted(() => ({
  listKeys: vi.fn(), listModels: vi.fn(), listTasks: vi.fn(), submitTask: vi.fn(),
  getTask: vi.fn(), downloadContent: vi.fn(), clearTasks: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(),
}))
vi.mock('../access', () => ({ listEligibleVideoKeys: api.listKeys }))
vi.mock('../api', () => ({
  listVideoModels: api.listModels, listVideoTasks: api.listTasks,
  submitVideoTask: api.submitTask, getVideoTask: api.getTask,
  downloadVideoContent: api.downloadContent, clearVideoTasks: api.clearTasks,
  getVideoRequestId: (task: VideoTask) => task.request_id || task.task_id || task.id || '',
}))
vi.mock('@/api/groups', () => ({ userGroupsAPI: { getUserGroupRates: async () => ({}) } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? key + ' ' + Object.values(params).join(' ') : key }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

const START = new Date('2026-10-16T07:23:00Z').getTime()
const keys = [1, 2].map((id) => ({
  id, user_id: 1, name: 'Key ' + id, key: 'key-' + id,
  group: { id, name: 'Grok ' + id, platform: 'grok', rate_multiplier: 1, allow_video_generation: true },
}))

function task(id: string, status = 'processing'): VideoTask {
  return { request_id: id, status, progress: 25, created_at: START / 1000,
    ...(status !== 'processing' ? { completed_at: START / 1000 + 65 } : {}),
    ...(status === 'completed' ? { content_available: true } : {}),
    metadata: { operation: 'text', model: 'grok-imagine-video', prompt: id, resolution: '480p', aspect_ratio: '16:9', duration: 4 },
  }
}

function content(id: string) {
  return Object.assign(new Blob([id], { type: 'video/mp4' }), { testId: id })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

function mountStudio() {
  return mount(VideoStudioView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, MediaStudioHeader: true, Icon: true, RouterLink: true,
    Select: {
      props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue', 'change'],
      template: '<select :disabled="disabled" :value="modelValue" @change="$emit(\'update:modelValue\', options.find(option => String(option.value) === $event.target.value).value); $emit(\'change\')"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
    },
  } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(START)
  sessionStorage.clear()
  localStorage.clear()
  api.listKeys.mockResolvedValue(keys)
  api.listModels.mockResolvedValue({ data: [{ id: 'grok-imagine-video' }, { id: 'grok-imagine-video-1.5' }] })
  api.listTasks.mockResolvedValue({ tasks: [], retentionDays: 7, persistentHistory: false })
  api.submitTask.mockImplementation(async (_key: string, request: GenerateVideoRequest) => task(request.prompt))
  api.getTask.mockImplementation(() => new Promise<VideoTask>(() => {}))
  api.downloadContent.mockImplementation(async (_key: string, id: string) => content(id))
  vi.stubGlobal('URL', class extends URL {
    static createObjectURL = vi.fn((blob: Blob & { testId?: string; name?: string }) => 'blob:' + (blob.testId || blob.name || 'source'))
    static revokeObjectURL = vi.fn()
  })
})

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('video studio independent tasks', () => {
  it('unlocks after acceptance, starts a new task, and leaves old-key jobs running independently', async () => {
    const firstSubmission = deferred<VideoTask>()
    const polls = new Map<string, ReturnType<typeof deferred<VideoTask>>>()
    const signals = new Map<string, AbortSignal>()
    api.submitTask.mockImplementationOnce(() => firstSubmission.promise)
    api.getTask.mockImplementation((_key: string, id: string, signal: AbortSignal) => {
      const pending = deferred<VideoTask>()
      polls.set(id, pending)
      signals.set(id, signal)
      return pending.promise
    })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#video-studio-prompt').setValue('first-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(wrapper.find('#video-studio-prompt').attributes('disabled')).toBeDefined()
      firstSubmission.resolve(task('first-task'))
      await flushPromises()
      expect(wrapper.find('#video-studio-prompt').attributes('disabled')).toBeUndefined()
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
      await vi.advanceTimersByTimeAsync(5_000)
      expect(api.getTask).toHaveBeenCalledWith('key-1', 'first-task', expect.any(AbortSignal))

      await wrapper.find('[data-testid="new-task"]').trigger('click')
      expect((wrapper.find('#video-studio-prompt').element as HTMLTextAreaElement).value).toBe('')
      expect(wrapper.find('.video-canvas').text()).toContain('videoStudio.empty')
      expect(wrapper.find('.video-history-list').text()).toContain('first-task')
      await wrapper.find('#video-studio-key').setValue('2')
      await flushPromises()
      await wrapper.find('#video-studio-prompt').setValue('second-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5_000)
      expect(signals.get('first-task')?.aborted).toBe(false)
      expect(api.getTask).toHaveBeenCalledWith('key-2', 'second-task', expect.any(AbortSignal))
      expect(wrapper.text()).toContain('videoStudio.runningTasks 2')
      expect(wrapper.find('button[aria-label="videoStudio.clearHistory"]').attributes('disabled')).toBeDefined()

      polls.get('first-task')!.resolve({ ...task('first-task', 'completed'), content_available: false })
      await flushPromises()
      expect(api.downloadContent).toHaveBeenCalledWith('key-1', 'first-task', expect.any(AbortSignal))
      expect(wrapper.find('video').exists()).toBe(false)
      expect(wrapper.find('[data-testid="task-status"]').text()).toContain('second-task')
      polls.get('second-task')!.resolve(task('second-task', 'completed'))
      await flushPromises()
      expect(wrapper.find('video').attributes('src')).toBe('blob:second-task')
      expect(api.clearTasks).not.toHaveBeenCalled()
      expect(api.showError).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('submits independent image-to-video source snapshots while previous tasks run', async () => {
    vi.stubGlobal('FileReader', class {
      result = ''
      onload: (() => void) | null = null
      readAsDataURL(file: File) { this.result = 'data:image/png;base64,' + file.name; this.onload?.() }
    })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.findAll('[role="tab"]').find(tab => tab.text() === 'videoStudio.imageMode')!.trigger('click')
      await wrapper.find('#video-studio-model').setValue('grok-imagine-video-1.5')
      const input = wrapper.find('input[type="file"]')
      const first = new File(['one'], 'one.png', { type: 'image/png' })
      Object.defineProperty(input.element, 'files', { value: [first], configurable: true })
      await input.trigger('change')
      await wrapper.find('#video-studio-prompt').setValue('first-image')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(input.attributes('disabled')).toBeUndefined()
      await wrapper.find('[data-testid="new-task"]').trigger('click')
      expect(wrapper.find('img[alt="videoStudio.sourcePreview"]').exists()).toBe(false)
      const second = new File(['two'], 'two.png', { type: 'image/png' })
      Object.defineProperty(input.element, 'files', { value: [second], configurable: true })
      await input.trigger('change')
      await wrapper.find('#video-studio-prompt').setValue('second-image')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.submitTask).toHaveBeenCalledTimes(2)
      expect(api.submitTask.mock.calls[0]?.[1]).toEqual(expect.objectContaining({ prompt: 'first-image', model: 'grok-imagine-video-1.5', image: { url: 'data:image/png;base64,one.png' } }))
      expect(api.submitTask.mock.calls[1]?.[1]).toEqual(expect.objectContaining({ prompt: 'second-image', model: 'grok-imagine-video-1.5', image: { url: 'data:image/png;base64,two.png' } }))
      expect(wrapper.findAll('.video-history-list > button')).toHaveLength(2)
    } finally { wrapper.unmount() }
  })

  it('selects history without replacing the draft or stopping an in-flight task', async () => {
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [task('saved-result', 'completed')] : [], retentionDays: 7, persistentHistory: false }))
    let pollingSignal: AbortSignal | undefined
    api.getTask.mockImplementation((_key: string, _id: string, signal: AbortSignal) => {
      pollingSignal = signal
      return new Promise<VideoTask>(() => {})
    })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#video-studio-key').setValue('2')
      await flushPromises()
      await wrapper.find('#video-studio-prompt').setValue('running-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5_000)
      await wrapper.find('#video-studio-prompt').setValue('An unsent new draft')
      await wrapper.find('button[title="saved-result"]').trigger('click')
      await flushPromises()
      expect((wrapper.find('#video-studio-prompt').element as HTMLTextAreaElement).value).toBe('An unsent new draft')
      expect((wrapper.find('#video-studio-key').element as HTMLSelectElement).value).toBe('2')
      expect(pollingSignal?.aborted).toBe(false)
      expect(api.downloadContent).toHaveBeenCalledWith('key-1', 'saved-result', expect.any(AbortSignal))
      expect(wrapper.find('video').attributes('src')).toBe('blob:saved-result')
    } finally { wrapper.unmount() }
  })

  it.each(['processing', 'completed', 'failed'])('aligns %s history metadata with its timestamp and uses a compact mint task button', async (status) => {
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [task('history-result', status)] : [], retentionDays: 7, persistentHistory: false }))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const meta = wrapper.find('[data-testid="history-meta"]')
      const timestamp = meta.find('time')
      const statusRow = meta.find('[data-testid="history-status"]')
      expect(meta.classes()).toEqual(expect.arrayContaining(['col-span-2', 'min-w-0', 'grid-cols-[minmax(0,1fr)_auto]', 'items-center']))
      expect(timestamp.classes()).toEqual(expect.arrayContaining(['justify-self-end', 'whitespace-nowrap']))
      expect(timestamp.classes()).not.toContain('absolute')
      expect(timestamp.text()).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/)
      expect(timestamp.element.parentElement).toBe(meta.element)
      expect(meta.element.firstElementChild).toBe(statusRow.element)
      expect(statusRow.text()).toContain('videoStudio.historyStatus.' + status)
      expect(statusRow.find('[title="videoStudio.elapsedTime"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="new-task"]').classes()).toEqual(expect.arrayContaining(['h-7', 'text-xs', 'bg-emerald-50', 'text-emerald-700', 'ml-auto']))
      await wrapper.find('button[title="history-result"]').trigger('click')
      await flushPromises()
      if (status !== 'processing') {
        expect(wrapper.find('[data-testid="task-status"]').text()).toContain('01:05')
        await vi.advanceTimersByTimeAsync(60_000)
        expect(wrapper.find('[data-testid="task-status"]').text()).toContain('01:05')
        expect(statusRow.text()).toContain('01:05')
      }
    } finally { wrapper.unmount() }
  })

  it('keeps the canvas independent of video proportions and sizes', async () => {
    const wide = task('wide-video', 'completed')
    const portrait = task('portrait-video', 'completed')
    portrait.metadata = { ...portrait.metadata, resolution: '1080p', aspect_ratio: '9:16' }
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [wide, portrait] : [], retentionDays: 7, persistentHistory: false }))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const canvas = wrapper.find('.video-canvas')
      const initialClasses = canvas.attributes('class')
      expect(canvas.classes()).not.toContain('flex-1')
      for (const id of ['wide-video', 'portrait-video']) {
        await wrapper.find('button[title="' + id + '"]').trigger('click')
        await flushPromises()
        expect(canvas.find('video').classes()).toEqual(expect.arrayContaining(['absolute', 'inset-0', 'h-full', 'w-full', 'object-contain']))
        expect(canvas.find('.video-player-frame').classes()).toEqual(expect.arrayContaining(['relative', 'min-h-0', 'min-w-0', 'flex-1', 'overflow-hidden']))
        expect(canvas.attributes('class')).toBe(initialClasses)
        expect(canvas.attributes('style')).toBeUndefined()
      }
    } finally { wrapper.unmount() }
  })

  it('remembers the last selected API key when reopening the workbench', async () => {
    const first = mountStudio()
    await flushPromises()
    await first.find('#video-studio-key').setValue('2')
    await flushPromises()
    first.unmount()
    const reopened = mountStudio()
    try {
      await flushPromises()
      expect((reopened.find('#video-studio-key').element as HTMLSelectElement).value).toBe('2')
      expect(api.listModels).toHaveBeenLastCalledWith('key-2', expect.any(AbortSignal))
    } finally { reopened.unmount() }
  })

  it('ignores stale video content when another history result is selected', async () => {
    const first = deferred<Blob>()
    const second = deferred<Blob>()
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: [task(key === 'key-1' ? 'first-result' : 'second-result', 'completed')], retentionDays: 7, persistentHistory: false }))
    api.downloadContent.mockImplementation((_key: string, id: string) => id === 'first-result' ? first.promise : second.promise)
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('button[title="first-result"]').trigger('click')
      await wrapper.find('button[title="second-result"]').trigger('click')
      second.resolve(content('second-result'))
      await flushPromises()
      expect(wrapper.find('video').attributes('src')).toBe('blob:second-result')
      first.resolve(content('first-result'))
      await flushPromises()
      expect(wrapper.find('video').attributes('src')).toBe('blob:second-result')
      expect(URL.createObjectURL).not.toHaveBeenCalledWith(expect.objectContaining({ testId: 'first-result' }))
      expect(api.downloadContent).toHaveBeenCalledWith('key-2', 'second-result', expect.any(AbortSignal))
    } finally { wrapper.unmount() }
  })
})
