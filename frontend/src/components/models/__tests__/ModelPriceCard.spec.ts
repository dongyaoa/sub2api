import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPriceCard from '../ModelPriceCard.vue'
import type { ModelPricingVariant, ModelSquareModel } from '@/utils/modelSquare'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

const model: ModelSquareModel = {
  key: 'gemini::gemini-3.1-flash-image',
  name: 'gemini-3.1-flash-image',
  platform: 'gemini',
  channelNames: ['Gemini'],
  groupIds: [1],
  variants: [],
}

function imageVariant(perRequestPrice: number | null): ModelPricingVariant {
  return {
    channelName: 'Gemini',
    groupIds: [1],
    pricing: {
      billing_mode: 'image',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: 0.0001,
      per_request_price: perRequestPrice,
      intervals: [],
    },
  }
}

function mountCard(perRequestPrice: number | null, showMultiplier = false) {
  return mount(ModelPriceCard, {
    props: {
      model,
      variant: imageVariant(perRequestPrice),
      showMultiplier,
      multiplier: showMultiplier
        ? {
            value: 0.6,
            baseValue: 0.6,
            source: 'group',
            imageIndependent: false,
            peakActive: false,
            peakFactor: 1,
          }
        : null,
    },
    global: {
      stubs: {
        Icon: true,
        PlatformIcon: true,
      },
    },
  })
}

describe('ModelPriceCard image pricing', () => {
  it('uses the configured per-request image price instead of the image token price', () => {
    const wrapper = mountCard(0.1)

    expect(wrapper.get('.price').text()).toBe('$0.1')
    expect(wrapper.text()).not.toContain('$0.0001')
  })

  it('keeps the per-request image price fixed when multiplier display is enabled', () => {
    const wrapper = mountCard(0.1, true)

    expect(wrapper.get('.price').text()).toBe('$0.1')
    expect(wrapper.find('.base-price').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('$0.06')
  })

  it('does not fall back to image token pricing when no per-request price is configured', () => {
    const wrapper = mountCard(null)

    expect(wrapper.get('.price').text()).toBe('\u2014')
    expect(wrapper.text()).not.toContain('$0.0001')
  })
})
