import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageStudioView from '../ImageStudioView.vue'
import type { ImageModelPricing } from '../types'

const { listKeys, listModels, getPricing, showError } = vi.hoisted(() => ({
  listKeys: vi.fn(), listModels: vi.fn(), getPricing: vi.fn(), showError: vi.fn(),
}))

vi.mock('../access', async (importOriginal) => ({
  ...await importOriginal<typeof import('../access')>(), listEligibleImageKeys: listKeys,
}))
vi.mock('../api', () => ({
  listImageModels: listModels,
  getImageModelPricing: getPricing,
  listImageTasks: async () => ({ tasks: [], retentionDays: 7 }),
  extractTaskImageData: () => [],
  clearImageTasks: vi.fn(), deleteImageTask: vi.fn(), getImageTask: vi.fn(),
  submitImageTask: vi.fn(), submitImageEditTask: vi.fn(),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({
  t: (key: string, params?: Record<string, unknown>) => params?.amount ? `${key} ${params.amount}` : key,
}) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

const keys = [1, 2].map((id) => ({
  id, name: `Key ${id}`, key: `key-${id}`,
  group: { id, name: `Gemini ${id}`, platform: 'gemini', image_price_1k: 0.06, image_price_2k: 0.06, image_price_4k: 0.06 },
}))

function quote(model = 'gemini-nano-banana-2.1', price = 0.1): ImageModelPricing {
  return {
    model, billing_mode: 'image', pricing_source: 'channel', rate_multiplier: 1, openai4k_allowed: false,
    tiers: {
      '1K': { base_price: price, unit_price: price },
      '2K': { base_price: price, unit_price: price },
      '4K': { base_price: price, unit_price: price },
    },
  }
}

function mountStudio() {
  return mount(ImageStudioView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    MediaStudioHeader: true, Icon: true, BaseDialog: true, RouterLink: true,
    Select: {
      props: ['modelValue', 'options'],
      emits: ['update:modelValue', 'change'],
      template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', options.find(option => String(option.value) === $event.target.value).value); $emit(\'change\')"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
    },
  } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  listKeys.mockResolvedValue(keys)
  listModels.mockResolvedValue({ object: 'list', data: [
    { id: 'gemini-nano-banana-2.1', image_generation: true },
    { id: 'custom-art', image_generation: true },
  ] })
  getPricing.mockImplementation(async (_key: string, model: string) => quote(model))
})

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('image studio model and billing integration', () => {
  it('selects only image models from a mixed catalog with incorrect video image flags', async () => {
    listModels.mockResolvedValue({ object: 'list', data: [
      { id: 'gemini-omni-1.2-flash', image_generation: true },
      { id: 'muse-video', image_generation: true },
      { id: 'gemini-nano-banana-2.1', image_generation: true },
    ] })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const select = wrapper.find('#image-studio-model')
      expect(select.findAll('option').map(option => option.attributes('value'))).toEqual(['gemini-nano-banana-2.1'])
      expect((select.element as HTMLSelectElement).value).toBe('gemini-nano-banana-2.1')
      expect(getPricing.mock.calls.every(([, model]) => model === 'gemini-nano-banana-2.1')).toBe(true)
    } finally { wrapper.unmount() }
  })

  it('disables image generation when the group only contains video models', async () => {
    listModels.mockResolvedValue({ object: 'list', data: [
      { id: 'gemini-omni-1.2-flash', image_generation: true },
      { id: 'omni-featured', image_generation: true },
    ] })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-prompt').setValue('A landscape')
      expect(wrapper.find('#image-studio-key').exists()).toBe(false)
      expect(wrapper.text()).toContain('imageStudio.noKeys')
      expect(wrapper.find('#image-studio-model').findAll('option')).toHaveLength(0)
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
      expect(getPricing).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('hides video-only and empty groups and falls back from a remembered video key to an image key', async () => {
    const imageKey = { ...keys[0], id: 3, key: 'key-3', group: { ...keys[0].group, id: 3, platform: 'openai', name: 'Images' } }
    const emptyKey = { ...keys[0], id: 4, key: 'key-4', group: { ...keys[0].group, id: 4, name: 'Empty' } }
    listKeys.mockResolvedValue([...keys, imageKey, emptyKey])
    localStorage.setItem('image_studio_last_key_v1:local', '1')
    listModels.mockImplementation(async (key: string) => ({ object: 'list', data:
      key === 'key-1' ? [{ id: 'gemini-omni-1.2-flash', image_generation: true }]
        : key === 'key-2' ? [{ id: 'gemini-omni-1.2-flash', image_generation: true }, { id: 'custom-art', image_generation: true }]
        : key === 'key-3' ? [{ id: 'gpt-image-2', image_generation: true }] : [],
    }))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const selector = wrapper.find('#image-studio-key')
      expect(selector.findAll('option').map(option => option.attributes('value'))).toEqual(['2', '3'])
      expect((selector.element as HTMLSelectElement).value).toBe('3')
      expect((wrapper.find('#image-studio-model').element as HTMLSelectElement).value).toBe('gpt-image-2')
      expect(listModels).toHaveBeenCalledTimes(4)
      await selector.setValue('2')
      await flushPromises()
      expect(wrapper.find('#image-studio-model').findAll('option').map(option => option.attributes('value'))).toEqual(['custom-art'])
    } finally { wrapper.unmount() }
  })

  it('removes a group if its refreshed catalog no longer has image models', async () => {
    const wrapper = mountStudio()
    try {
      await flushPromises()
      listModels.mockResolvedValue({ object: 'list', data: [{ id: 'gemini-omni-1.2-flash', image_generation: true }] })
      await wrapper.find('#image-studio-key').setValue('2')
      await flushPromises()
      expect(wrapper.find('#image-studio-key').findAll('option').map(option => option.attributes('value'))).toEqual(['1'])
      expect(wrapper.find('#image-studio-model').findAll('option')).toHaveLength(0)
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    } finally { wrapper.unmount() }
  })

  it('accepts fourteen Gemini references, refuses excess files, and allows replacing a removed image', async () => {
    const revokeURL = vi.fn()
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:test-reference')
      static revokeObjectURL = revokeURL
    })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      const editTab = wrapper.findAll('[role="tab"]').find((tab) => tab.text().includes('imageStudio.editMode'))
      await editTab!.trigger('click')
      const input = wrapper.find('input[type="file"]')
      expect(input.attributes('multiple')).toBeDefined()
      const files = Array.from({ length: 15 }, (_, i) => new File([`reference-${i}`], `reference-${i}.png`, { type: 'image/png' }))
      Object.defineProperty(input.element, 'files', { value: files, configurable: true })
      await input.trigger('change')
      expect(wrapper.findAll('img')).toHaveLength(14)
      expect(showError).toHaveBeenCalledWith('imageStudio.sourceImageCountError')
      expect(wrapper.findAll('button').some((button) => button.text().includes('imageStudio.addSourceImages'))).toBe(false)
      const remove = wrapper.findAll('button').find((button) => button.attributes('title') === 'imageStudio.removeImage')
      await remove!.trigger('click')
      expect(wrapper.findAll('img')).toHaveLength(13)
      expect(revokeURL).toHaveBeenCalledTimes(1)
      Object.defineProperty(input.element, 'files', { value: [files[14]], configurable: true })
      await input.trigger('change')
      expect(wrapper.findAll('img')).toHaveLength(14)
      expect(wrapper.text()).toContain('reference-14.png')
    } finally { wrapper.unmount() }
  })

  it('shows a newly configured model and its channel price instead of the group flat price', async () => {
    const wrapper = mountStudio()
    try {
      await flushPromises()
      expect(wrapper.find('#image-studio-model').text()).toContain('gemini-nano-banana-2.1')
      expect(wrapper.text()).toContain('$0.1')
      expect(wrapper.text()).not.toContain('$0.06')
      expect(getPricing).toHaveBeenCalledWith('key-1', 'gemini-nano-banana-2.1', expect.any(AbortSignal))
      await wrapper.find('#image-studio-model').setValue('custom-art')
      await flushPromises()
      expect(getPricing).toHaveBeenLastCalledWith('key-1', 'custom-art', expect.any(AbortSignal))
    } finally { wrapper.unmount() }
  })

  it('does not allow generation with a failed quote and reloads prices on retry', async () => {
    getPricing.mockRejectedValueOnce(new Error('pricing unavailable'))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-prompt').setValue('A landscape')
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
      expect(wrapper.text()).toContain('imageStudio.pricingFailed')
      expect(wrapper.text()).not.toContain('$0.06')
      const retry = wrapper.findAll('button').find((button) => button.text() === 'imageStudio.retryPricing')
      await retry!.trigger('click')
      await flushPromises()
      expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
      expect(wrapper.text()).toContain('$0.1')
    } finally { wrapper.unmount() }
  })

  it('ignores a late quote from the previous key after switching groups', async () => {
    let finishOldQuote!: (pricing: ImageModelPricing) => void
    const oldQuote = new Promise<ImageModelPricing>((resolve) => { finishOldQuote = resolve })
    getPricing.mockImplementation((key: string, model: string) => key === 'key-1' ? oldQuote : Promise.resolve(quote(model, 0.2)))
    const wrapper = mountStudio()
    try {
      await flushPromises()
      await wrapper.find('#image-studio-key').setValue('2')
      await flushPromises()
      expect(wrapper.text()).toContain('$0.2')
      finishOldQuote(quote())
      await flushPromises()
      expect(wrapper.text()).toContain('$0.2')
      expect(wrapper.text()).not.toContain('$0.1')
    } finally { wrapper.unmount() }
  })

  it('does not clear the new model when the previous key catalog request fails late', async () => {
    let failOldCatalog!: (error: Error) => void
    const oldCatalog = new Promise((_, reject) => { failOldCatalog = reject })
    const wrapper = mountStudio()
    try {
      await flushPromises()
      listModels.mockImplementation((key: string) => key === 'key-2' ? oldCatalog : Promise.resolve({
        object: 'list', data: [{ id: 'gemini-nano-banana-2.1', image_generation: true }],
      }))
      await wrapper.find('#image-studio-key').setValue('2')
      await flushPromises()
      await wrapper.find('#image-studio-key').setValue('1')
      await flushPromises()
      failOldCatalog(new Error('old upstream catalog failed'))
      await flushPromises()
      expect((wrapper.find('#image-studio-model').element as HTMLSelectElement).value).toBe('gemini-nano-banana-2.1')
      expect(wrapper.text()).toContain('$0.1')
      expect(showError).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })
})
