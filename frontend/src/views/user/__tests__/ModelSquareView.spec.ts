import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import ModelSquareView from '../ModelSquareView.vue'
import ModelPriceCard from '@/components/models/ModelPriceCard.vue'

const { get, showError } = vi.hoisted(() => ({ get: vi.fn(), showError: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { available_channels_enabled: false },
    showError,
  }),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ meta: {} }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<main><slot /></main>' },
}))

describe('ModelSquareView independent catalog', () => {
  it('loads model prices while Available Channels is disabled', async () => {
    const pricing = { billing_mode: 'image', per_request_price: 0.04, intervals: [] }
    get.mockImplementation(async (url: string) => {
      switch (url) {
        case '/model-square/catalog':
          return { data: [{
            name: 'Image channel',
            description: '',
            platforms: [{
              platform: 'openai',
              groups: [{ id: 1 }],
              supported_models: [{ name: 'test-image', platform: 'openai', pricing }],
            }],
          }] }
        case '/groups/available':
          return { data: [{ id: 1, name: 'Public', platform: 'openai', rate_multiplier: 1 }] }
        case '/groups/rates':
          return { data: {} }
        case '/channels/available':
          return { data: [] }
        default:
          throw new Error(`Unexpected request: ${url}`)
      }
    })

    const wrapper = mount(ModelSquareView, {
      global: { stubs: { Icon: true, PlatformIcon: true, Select: true, ModelPriceCard: true } },
    })
    try {
      await flushPromises()
      const card = wrapper.findComponent(ModelPriceCard)
      expect(card.exists()).toBe(true)
      expect(card.props('model').name).toBe('test-image')
      expect(card.props('variant').pricing).toEqual(pricing)
      expect(get.mock.calls.some(([url]) => url === '/channels/available')).toBe(false)
      expect(showError).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })
})
