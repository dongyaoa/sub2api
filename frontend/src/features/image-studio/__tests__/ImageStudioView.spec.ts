import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImageStudioView from '../ImageStudioView.vue'
import type { ImageModelPricing } from '../types'

const { listKeys, listModels, getPricing, showError } = vi.hoisted(() => ({
  listKeys: vi.fn(), listModels: vi.fn(), getPricing: vi.fn(), showError: vi.fn(),
}))

vi.mock('../access', () => ({ listEligibleImageKeys: listKeys }))
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
  listKeys.mockResolvedValue(keys)
  listModels.mockResolvedValue({ object: 'list', data: [
    { id: 'gemini-nano-banana-2.1', image_generation: true },
    { id: 'custom-art', image_generation: true },
  ] })
  getPricing.mockImplementation(async (_key: string, model: string) => quote(model))
})

describe('image studio model and billing integration', () => {
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
