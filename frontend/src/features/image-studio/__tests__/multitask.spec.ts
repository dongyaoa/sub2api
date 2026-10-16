import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageStudioView from '../ImageStudioView.vue'
import type { ImageAspectRatio, ImageTask } from '../types'

const api = vi.hoisted(() => ({
  listKeys: vi.fn(), listModels: vi.fn(), getPricing: vi.fn(), listTasks: vi.fn(),
  submitTask: vi.fn(), submitEdit: vi.fn(), getTask: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(),
}))

vi.mock('../access', () => ({ listEligibleImageKeys: api.listKeys }))
vi.mock('../api', () => ({
  listImageModels: api.listModels,
  getImageModelPricing: api.getPricing,
  listImageTasks: api.listTasks,
  submitImageTask: api.submitTask,
  submitImageEditTask: api.submitEdit,
  getImageTask: api.getTask,
  clearImageTasks: vi.fn(), deleteImageTask: vi.fn(),
  extractTaskImageData: (task: ImageTask) => (task.result?.data || []).flatMap((image) => image.url ? [{ url: image.url }] : []),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1 }, isAuthenticated: true }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${Object.values(params).join(' ')}` : key }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

const keys = [1, 2].map((id) => ({
  id, user_id: 1, name: `Key ${id}`, key: `key-${id}`,
  group: { id, name: `Gemini ${id}`, platform: 'gemini', allow_image_generation: true },
}))

function task(id: string, status: ImageTask['status'] = 'processing'): ImageTask {
  return {
    id, task_id: id, object: 'image.generation.task', status,
    created_at: 1792048980, expires_at: 1792653780,
    ...(status === 'completed' ? { completed_at: 1792049045, result: { data: [{ url: `https://images.example/${id}.png` }] } } : {}),
    metadata: { model: 'gemini-nano-banana-2.1', prompt: id, quantity: 1, resolution: '1K', aspect_ratio: '1:1' },
  }
}

function mountStudio() {
  return mount(ImageStudioView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    MediaStudioHeader: true, Icon: true, RouterLink: true,
    BaseDialog: { props: ['show'], template: '<div v-if="show" data-test="preview-dialog"><slot /><slot name="footer" /></div>' },
    Select: {
      props: ['modelValue', 'options', 'disabled'],
      emits: ['update:modelValue', 'change'],
      template: '<select :disabled="disabled" :value="modelValue" @change="$emit(\'update:modelValue\', options.find(option => String(option.value) === $event.target.value).value); $emit(\'change\')"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
    },
  } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  api.listKeys.mockResolvedValue(keys)
  api.listModels.mockResolvedValue({ object: 'list', data: [{ id: 'gemini-nano-banana-2.1', image_generation: true }] })
  api.getPricing.mockImplementation(async (_key: string, model: string) => ({
    model, billing_mode: 'image', pricing_source: 'channel', rate_multiplier: 1, openai4k_allowed: false,
    tiers: Object.fromEntries(['1K', '2K', '4K'].map((tier) => [tier, { base_price: 0.1, unit_price: 0.1 }])),
  }))
  api.listTasks.mockResolvedValue({ tasks: [], retentionDays: 7 })
})

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('image studio independent tasks', () => {
  it.each(['1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9'] as ImageAspectRatio[])(
    'contains %s result images without letting their intrinsic size drive the canvas layout', async (ratio) => {
      const completed = task('canvas-result', 'completed')
      completed.metadata = { ...completed.metadata, aspect_ratio: ratio, resolution: '4K', quantity: 2 }
      completed.result = { data: [{ url: 'https://images.example/first.png' }, { url: 'https://images.example/second.png' }] }
      api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [completed] : [], retentionDays: 7 }))
      const wrapper = mountStudio()
      try {
        await flushPromises()
        const canvas = wrapper.find('.studio-canvas')
        const idleCanvasClasses = canvas.attributes('class')
        expect(canvas.classes()).not.toContain('flex-1')
        expect(canvas.find(':scope > div').classes()).toEqual(expect.arrayContaining(['h-full', 'min-h-0']))
        await wrapper.find('button[title="canvas-result"]').trigger('click')
        const media = canvas.find('.studio-single-media')
        expect(media.classes()).toEqual(expect.arrayContaining(['relative', 'min-h-0', 'min-w-0', 'overflow-hidden']))
        expect(media.find('img').classes()).toEqual(expect.arrayContaining(['absolute', 'inset-0', 'h-full', 'w-full', 'object-contain']))
        expect(canvas.find('figcaption').classes()).toContain('shrink-0')
        await canvas.find('button[title="imageStudio.nextImage"]').trigger('click')
        expect(media.find('img').attributes('src')).toBe('https://images.example/second.png')
        expect(canvas.attributes('class')).toBe(idleCanvasClasses)
        expect(canvas.attributes('style')).toBeUndefined()
        expect(canvas.find(':scope > div').classes()).toEqual(expect.arrayContaining(['h-full', 'min-h-0']))
      } finally { wrapper.unmount() }
    },
  )

  it.each(['processing', 'completed', 'failed'] as const)('aligns %s status and duration with the timestamp and uses a compact mint new-task button', async (status) => {
    const historical = task('long-prompt-task', status)
    historical.metadata = { ...historical.metadata, prompt: 'A very long image prompt '.repeat(30) }
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [historical] : [], retentionDays: 7 }))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const timestamp = wrapper.find('.studio-history-list time')
      const metadata = wrapper.find('[data-testid="history-meta"]')
      const statusRow = metadata.find('[data-testid="history-status"]')
      expect(metadata.classes()).toEqual(expect.arrayContaining(['col-span-2', 'min-w-0', 'grid-cols-[minmax(0,1fr)_auto]', 'items-center']))
      expect(timestamp.classes()).toEqual(expect.arrayContaining(['justify-self-end', 'whitespace-nowrap']))
      expect(timestamp.classes()).not.toContain('absolute')
      expect(timestamp.element.parentElement).toBe(metadata.element)
      expect(metadata.element.firstElementChild).toBe(statusRow.element)
      expect(statusRow.text()).toContain('imageStudio.historyStatus.' + status)
      expect(statusRow.text()).toContain('imageStudio.duration')
      const historyButton = wrapper.find('button[title="' + historical.metadata.prompt + '"]')
      expect(historyButton.classes()).toContain('row-start-2')
      expect(historyButton.classes()).toContain('min-w-0')
      expect(historyButton.text()).not.toContain('imageStudio.duration')
      expect(wrapper.find('[data-testid="new-task"]').classes()).toEqual(expect.arrayContaining(['h-7', 'bg-emerald-50', 'text-emerald-700', 'text-xs', 'ml-auto']))
    } finally { wrapper.unmount() }
  })

  it('accepts another task while the first is running and keeps completed results attached to their task', async () => {
    const finish = new Map<string, (value: ImageTask) => void>()
    api.submitTask.mockImplementation(async (_key: string, request: { prompt: string }) => task(request.prompt))
    api.getTask.mockImplementation((_key: string, id: string) => new Promise<ImageTask>((resolve) => { finish.set(id, resolve) }))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-prompt').setValue('first-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.getTask).toHaveBeenCalledWith('key-1', 'first-task', expect.any(AbortSignal))
      expect(wrapper.find('#image-studio-prompt').attributes('disabled')).toBeUndefined()
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()

      await wrapper.find('#image-studio-prompt').setValue('second-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.submitTask).toHaveBeenCalledTimes(2)
      expect(api.getTask).toHaveBeenCalledWith('key-1', 'second-task', expect.any(AbortSignal))

      finish.get('first-task')!(task('first-task', 'completed'))
      await flushPromises()
      expect(wrapper.find('.studio-canvas img[src="https://images.example/first-task.png"]').exists()).toBe(false)
      finish.get('second-task')!(task('second-task', 'completed'))
      await flushPromises()
      expect(wrapper.find('img[src="https://images.example/second-task.png"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('01:05')
    } finally { wrapper.unmount() }
  })

  it('keeps polling the original key when a new task uses another key', async () => {
    const finish = new Map<string, (value: ImageTask) => void>()
    const signals = new Map<string, AbortSignal>()
    api.submitTask.mockImplementation(async (_key: string, request: { prompt: string }) => task(request.prompt))
    api.getTask.mockImplementation((_key: string, id: string, signal: AbortSignal) => {
      signals.set(id, signal)
      return new Promise<ImageTask>((resolve) => { finish.set(id, resolve) })
    })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-prompt').setValue('key-one-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      await wrapper.find('#image-studio-key').setValue('2')
      await flushPromises()
      expect(signals.get('key-one-task')?.aborted).toBe(false)
      await wrapper.find('#image-studio-prompt').setValue('key-two-task')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.getTask).toHaveBeenCalledWith('key-2', 'key-two-task', expect.any(AbortSignal))
      finish.get('key-one-task')!(task('key-one-task', 'completed'))
      await flushPromises()
      expect(wrapper.find('.studio-canvas img[src="https://images.example/key-one-task.png"]').exists()).toBe(false)
      finish.get('key-two-task')!(task('key-two-task', 'completed'))
      await flushPromises()
      expect(wrapper.find('img[src="https://images.example/key-two-task.png"]').exists()).toBe(true)
      expect(api.showError).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('remembers the last selected key when the workbench is reopened', async () => {
    const first = mountStudio()
    await flushPromises()
    await first.find('#image-studio-key').setValue('2')
    await flushPromises()
    first.unmount()
    const reopened = mountStudio()
    try {
      await flushPromises()
      expect((reopened.find('#image-studio-key').element as HTMLSelectElement).value).toBe('2')
      expect(api.getPricing).toHaveBeenLastCalledWith('key-2', 'gemini-nano-banana-2.1', expect.any(AbortSignal))
    } finally { reopened.unmount() }
  })

  it('can submit multiple image-edit tasks with independent reference-image snapshots', async () => {
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:reference')
      static revokeObjectURL = vi.fn()
    })
    api.submitEdit.mockImplementation(async (_key: string, _files: File[], request: { prompt: string }) => task(request.prompt))
    api.getTask.mockImplementation(() => new Promise<ImageTask>(() => {}))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const editTab = wrapper.findAll('[role="tab"]').find((tab) => tab.text().includes('imageStudio.editMode'))
      await editTab!.trigger('click')
      const input = wrapper.find('input[type="file"]')
      const firstImage = new File(['first'], 'first.png', { type: 'image/png' })
      const secondImage = new File(['second'], 'second.png', { type: 'image/png' })
      Object.defineProperty(input.element, 'files', { value: [firstImage], configurable: true })
      await input.trigger('change')
      await wrapper.find('#image-studio-prompt').setValue('first-edit')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(wrapper.find('input[type="file"]').attributes('disabled')).toBeUndefined()
      Object.defineProperty(input.element, 'files', { value: [secondImage], configurable: true })
      await input.trigger('change')
      await wrapper.find('#image-studio-prompt').setValue('second-edit')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.submitEdit).toHaveBeenCalledTimes(2)
      expect(api.submitEdit.mock.calls[0]?.[1]).toEqual([firstImage])
      expect(api.submitEdit.mock.calls[1]?.[1]).toEqual([firstImage, secondImage])
      expect(api.submitTask).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('previews history metadata and explicitly reuses its prompt or full parameters', async () => {
    const historical = task('historical-task', 'completed')
    historical.metadata = { ...historical.metadata, prompt: 'Saved landscape prompt', quantity: 1, resolution: '4K', aspect_ratio: '16:9' }
    api.listTasks.mockImplementation(async (key: string) => ({ tasks: key === 'key-1' ? [historical] : [], retentionDays: 7 }))
    api.submitTask.mockResolvedValue(task('reused-task'))
    api.getTask.mockImplementation(() => new Promise<ImageTask>(() => {}))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-key').setValue('2')
      await flushPromises()
      await wrapper.find('#image-studio-prompt').setValue('Current draft')
      await wrapper.find('button[title="Saved landscape prompt"]').trigger('click')
      expect((wrapper.find('#image-studio-prompt').element as HTMLTextAreaElement).value).toBe('Current draft')
      await wrapper.find('button[title="imageStudio.preview"]').trigger('click')
      const preview = wrapper.find('[data-test="preview-dialog"]')
      expect(preview.text()).toContain('gemini-nano-banana-2.1')
      expect(preview.text()).toContain('Gemini 1')
      expect(preview.text()).toContain('Saved landscape prompt')
      expect(preview.text()).toContain('01:05')
      expect(wrapper.find('.studio-history-list time').text()).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/)
      await preview.findAll('button').find(button => button.text() === 'imageStudio.reusePrompt')!.trigger('click')
      expect((wrapper.find('#image-studio-prompt').element as HTMLTextAreaElement).value).toBe('Saved landscape prompt')
      expect((wrapper.find('#image-studio-key').element as HTMLSelectElement).value).toBe('2')

      await wrapper.find('button[title="imageStudio.preview"]').trigger('click')
      await wrapper.find('[data-test="preview-dialog"]').findAll('button').find(button => button.text() === 'imageStudio.reuseParameters')!.trigger('click')
      await flushPromises()
      expect((wrapper.find('#image-studio-key').element as HTMLSelectElement).value).toBe('1')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(api.submitTask).toHaveBeenCalledWith('key-1', expect.objectContaining({
        model: 'gemini-nano-banana-2.1', prompt: 'Saved landscape prompt', n: 1, size: '4K', resolution: '4k', aspect_ratio: '16:9',
      }), expect.any(AbortSignal))
    } finally { wrapper.unmount() }
  })
})
