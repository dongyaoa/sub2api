import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPriceCard from '../ModelPriceCard.vue'
import type { UserPricingInterval } from '@/api/channels'
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

function imageVariant(
  perRequestPrice: number | null,
  intervals: UserPricingInterval[] = [],
): ModelPricingVariant {
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
      intervals,
    },
  }
}

function mountCard(
  perRequestPrice: number | null,
  showMultiplier = false,
  intervals: UserPricingInterval[] = [],
) {
  return mount(ModelPriceCard, {
    props: {
      model,
      variant: imageVariant(perRequestPrice, intervals),
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
        Teleport: true,
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

  it('shows configured image resolution tiers when the default price is empty', () => {
    const intervals: UserPricingInterval[] = [
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '1K',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.04,
      },
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '2K',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.04,
      },
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '4K',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.09,
      },
    ]
    const wrapper = mountCard(null, true, intervals)

    expect(wrapper.findAll('.metric-tier').map((tier) => tier.text())).toEqual(['1K', '2K', '4K'])
    expect(wrapper.findAll('.price').map((price) => price.text())).toEqual(['$0.04', '$0.04', '$0.09'])
    expect(wrapper.find('.base-price').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('$0.0001')
  })

  it('uses the per-request price column in the image tier dialog', async () => {
    const intervals: UserPricingInterval[] = [{
      min_tokens: 0,
      max_tokens: null,
      tier_label: '4K',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      per_request_price: 0.09,
    }]
    const wrapper = mountCard(null, false, intervals)

    await wrapper.get('.tier-btn').trigger('click')
    const dialog = wrapper.get('[role="dialog"]')

    expect(dialog.findAll('th').map((heading) => heading.text())).toEqual([
      'modelSquare.interval',
      'modelSquare.imageOutput',
    ])
    expect(dialog.text()).toContain('4K')
    expect(dialog.text()).toContain('$0.09')
    expect(dialog.text()).toContain('modelSquare.perRequest')
    expect(dialog.text()).not.toContain('modelSquare.input')
  })

})

function videoVariant(
  perSecondPrice: number | null,
  intervals: UserPricingInterval[] = [],
): ModelPricingVariant {
  return {
    channelName: 'SuperGrok Heavy',
    groupIds: [1],
    pricing: {
      billing_mode: 'video',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: perSecondPrice,
      intervals,
    },
  }
}

function mountVideoCard(
  perSecondPrice: number | null,
  intervals: UserPricingInterval[] = [],
) {
  return mount(ModelPriceCard, {
    props: {
      model: {
        ...model,
        name: 'grok-imagine-video-1.5',
        platform: 'grok',
      },
      variant: videoVariant(perSecondPrice, intervals),
      showMultiplier: false,
      multiplier: null,
    },
    global: {
      stubs: {
        Icon: true,
        PlatformIcon: true,
        Teleport: true,
      },
    },
  })
}

describe('ModelPriceCard video pricing', () => {
  it('shows the configured default video price per second', () => {
    const wrapper = mountVideoCard(0.1)

    expect(wrapper.get('.price').text()).toBe('$0.1')
    expect(wrapper.get('.unit').text()).toBe('modelSquare.perSecond')
    expect(wrapper.get('.type-badge').text()).toBe('modelSquare.billingVideo')
  })

  it('shows configured video resolution tiers instead of token placeholders', () => {
    const wrapper = mountVideoCard(null, [
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '480p',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.1,
      },
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '720p',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.2,
      },
    ])

    expect(wrapper.findAll('.metric-tier').map((tier) => tier.text())).toEqual(['480p', '720p'])
    expect(wrapper.findAll('.price').map((price) => price.text())).toEqual(['$0.1', '$0.2'])
    expect(wrapper.text()).not.toContain('modelSquare.input')
  })
})

function tokenVariant(inputPrice: number | null = 0.00000075): ModelPricingVariant {
  return {
    channelName: 'Codex GPT',
    groupIds: [1],
    pricing: {
      billing_mode: 'token',
      input_price: inputPrice,
      output_price: 0.0000045,
      cache_write_price: 0,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [],
    },
  }
}

function mountAdjustedCard(variant: ModelPricingVariant, rate = 0.15) {
  return mount(ModelPriceCard, {
    props: {
      model: { ...model, name: 'gpt-5.4-mini', platform: 'openai' },
      variant,
      showMultiplier: true,
      multiplier: {
        value: rate,
        baseValue: rate,
        source: 'group',
        imageIndependent: false,
        peakActive: false,
        peakFactor: 1,
      },
    },
    global: {
      stubs: { Icon: true, PlatformIcon: true, Teleport: true },
    },
  })
}

describe('ModelPriceCard group multiplier prices', () => {
  it('shows adjusted token prices with their base prices, multiplier and units', () => {
    const wrapper = mountAdjustedCard(tokenVariant())
    const rows = wrapper.findAll('.metric-box')

    expect(rows.map((row) => row.get('.price').text())).toEqual(['$0.1125', '$0.675', '$0', '—'])
    expect(rows.map((row) => row.get('.unit').text())).toEqual(['/ 1M', '/ 1M', '/ 1M', '/ 1M'])
    expect(rows[0].get('.base-price').text()).toBe('$0.75 / 1M')
    expect(rows[1].get('.base-price').text()).toBe('$4.5 / 1M')
    expect(rows[2].get('.base-price').text()).toBe('$0 / 1M')
    expect(wrapper.findAll('.multiplier-pill').map((pill) => pill.text())).toEqual(['×0.15', '×0.15', '×0.15'])
    expect(rows[3].find('.adjusted-meta').exists()).toBe(false)
  })

  it('restores the original token prices and removes multiplier metadata when switched off', async () => {
    const wrapper = mountAdjustedCard(tokenVariant())

    await wrapper.setProps({ showMultiplier: false })

    expect(wrapper.findAll('.price').map((price) => price.text())).toEqual(['$0.75', '$4.5', '$0', '—'])
    expect(wrapper.find('.base-price').exists()).toBe(false)
    expect(wrapper.find('.multiplier-pill').exists()).toBe(false)
    expect(wrapper.find('.price--adjusted').exists()).toBe(false)

    await wrapper.setProps({ showMultiplier: true })

    expect(wrapper.get('.price').text()).toBe('$0.1125')
    expect(wrapper.get('.base-price').text()).toBe('$0.75 / 1M')
    expect(wrapper.get('.multiplier-pill').text()).toBe('×0.15')
  })

  it('keeps long decimal prices and fractional multipliers in the visible text', () => {
    const wrapper = mountAdjustedCard(tokenVariant(0.00000000123456), 0.12345678)
    const input = wrapper.findAll('.metric-box')[0]

    expect(input.get('.price').text()).toBe('$0.00015241')
    expect(input.get('.base-price').text()).toBe('$0.00123456 / 1M')
    expect(input.get('.multiplier-pill').text()).toBe('×0.12345678')
  })

  it('preserves per-second units for both the adjusted and original video price', () => {
    const wrapper = mountAdjustedCard(videoVariant(0.3))

    expect(wrapper.get('.price').text()).toBe('$0.045')
    expect(wrapper.get('.unit').text()).toBe('modelSquare.perSecond')
    expect(wrapper.get('.base-price').text()).toBe('$0.3 modelSquare.perSecond')
    expect(wrapper.get('.multiplier-pill').text()).toBe('×0.15')
    expect(wrapper.get('.type-badge').text()).toBe('modelSquare.billingVideo')
  })
})
