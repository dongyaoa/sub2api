import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelSquareView from '../ModelSquareView.vue'
import ModelPriceCard from '@/components/models/ModelPriceCard.vue'
import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserChannelPlatformSection,
  UserSupportedModel,
  UserSupportedModelPricing,
} from '@/api/channels'

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
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

enableAutoUnmount(afterEach)

function group(id: number, name: string, platform: string, rate = 1): UserAvailableGroup {
  return {
    id,
    name,
    platform,
    subscription_type: 'standard',
    rate_multiplier: rate,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
  }
}

function pricing(inputPrice: number): UserSupportedModelPricing {
  return {
    billing_mode: 'token',
    input_price: inputPrice,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
  }
}

function model(name: string, platform: string, inputPrice = 0.000002): UserSupportedModel {
  return { name, platform, pricing: pricing(inputPrice) }
}

function channel(name: string, platforms: UserChannelPlatformSection[]): UserAvailableChannel {
  return { name, description: '', platforms }
}

function catalogFixture() {
  const basic = group(1, 'A Basic', 'openai', 0.6)
  const premium = group(2, 'B Premium', 'openai', 1.2)
  const anthropic = group(3, 'C Anthropic', 'anthropic', 2.5)
  const composite = group(4, 'D Composite', 'composite', 0.4)
  return {
    groups: [basic, premium, anthropic, composite],
    rates: { 2: 0.25 } as Record<number, number>,
    channels: [
      channel('Basic channel', [{
        platform: 'openai',
        groups: [basic],
        supported_models: [model('shared-model', 'openai'), model('basic-only', 'openai')],
      }]),
      channel('Premium channel', [{
        platform: 'openai',
        groups: [premium],
        supported_models: [model('shared-model', 'openai', 0.000005), model('premium-only', 'openai')],
      }]),
      channel('Anthropic channel', [{
        platform: 'anthropic',
        groups: [anthropic],
        supported_models: [model('claude-only', 'anthropic')],
      }]),
      channel('Composite channel', [
        {
          platform: 'openai',
          groups: [composite],
          supported_models: [model('composite-openai', 'openai')],
        },
        {
          platform: 'anthropic',
          groups: [composite],
          supported_models: [model('composite-anthropic', 'anthropic')],
        },
      ]),
    ],
  }
}

let catalog: ReturnType<typeof catalogFixture>

beforeEach(() => {
  get.mockReset()
  showError.mockReset()
  catalog = catalogFixture()
  get.mockImplementation(async (url: string) => {
    switch (url) {
      case '/model-square/catalog':
        return { data: catalog.channels }
      case '/groups/available':
        return { data: catalog.groups }
      case '/groups/rates':
        return { data: catalog.rates }
      default:
        throw new Error(`Unexpected request: ${url}`)
    }
  })
})

async function mountView(stubCards = true) {
  const wrapper = mount(ModelSquareView, {
    global: { stubs: { Icon: true, PlatformIcon: true, ModelPriceCard: stubCards, Teleport: true } },
  })
  await flushPromises()
  return wrapper
}

function cardNames(wrapper: VueWrapper) {
  return wrapper.findAllComponents(ModelPriceCard).map((card) => card.props('model').name)
}

function cardFor(wrapper: VueWrapper, name: string) {
  const card = wrapper.findAllComponents(ModelPriceCard).find((item) => item.props('model').name === name)
  expect(card, `expected a card for ${name}`).toBeDefined()
  return card!
}

function groupIds(wrapper: VueWrapper) {
  return wrapper.findAll('[data-group-id]').map((button) => button.attributes('data-group-id'))
}

function platformIds(wrapper: VueWrapper) {
  return wrapper.findAll('[data-platform]').map((button) => button.attributes('data-platform'))
}

async function refresh(wrapper: VueWrapper) {
  await wrapper.get('button[aria-label="common.refresh"]').trigger('click')
  await flushPromises()
}

describe('ModelSquareView independent catalog', () => {
  it('loads prices with Available Channels disabled and selects the first group on all platforms', async () => {
    const wrapper = await mountView()

    expect(get.mock.calls.map(([url]) => url)).toEqual([
      '/model-square/catalog', '/groups/available', '/groups/rates',
    ])
    expect(get.mock.calls.some(([url]) => url === '/channels/available')).toBe(false)
    expect(wrapper.get('[data-platform=""]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['basic-only', 'shared-model'])
    expect(cardFor(wrapper, 'shared-model').props('showMultiplier')).toBe(false)
    expect(cardFor(wrapper, 'shared-model').props('variant').pricing).toEqual(pricing(0.000002))
    expect(showError).not.toHaveBeenCalled()
  })

  it('loads independently configured image prices while Available Channels is disabled', async () => {
    const imagePrice: UserSupportedModelPricing = {
      ...pricing(0), billing_mode: 'image', input_price: null, per_request_price: 0.04,
    }
    catalog.channels = [channel('Image channel', [{
      platform: 'openai',
      groups: [catalog.groups[0]],
      supported_models: [{ name: 'test-image', platform: 'openai', pricing: imagePrice }],
    }])]
    const wrapper = await mountView(false)

    expect(cardNames(wrapper)).toEqual(['test-image'])
    expect(cardFor(wrapper, 'test-image').props('variant').pricing).toEqual(imagePrice)
    expect(cardFor(wrapper, 'test-image').get('.price').text()).toBe('$0.04')
    await wrapper.get('input[role="switch"]').setValue(true)
    expect(cardFor(wrapper, 'test-image').get('.price').text()).toBe('$0.04')
    expect(cardFor(wrapper, 'test-image').find('.base-price').exists()).toBe(false)
    expect(get.mock.calls.some(([url]) => url === '/channels/available')).toBe(false)
    expect(showError).not.toHaveBeenCalled()
  })

  it('uses platform and group buttons without search or a platform dropdown', async () => {
    const wrapper = await mountView()

    expect(platformIds(wrapper)).toEqual(['', 'anthropic', 'openai'])
    expect(groupIds(wrapper)).toEqual(['1', '2', '3', '4'])
    expect(wrapper.find('input[type="search"], input[type="text"], select, [role="combobox"], select-stub').exists())
      .toBe(false)
    expect(wrapper.findAll('input')).toHaveLength(1)
    expect(wrapper.get('input').attributes('role')).toBe('switch')
    expect(wrapper.text()).not.toContain('modelSquare.searchLabel')
  })

  it('keeps the only multiplier switch in the group heading without a footer or summary row', async () => {
    const wrapper = await mountView()

    expect(wrapper.findAll('input[role="switch"]')).toHaveLength(1)
    expect(wrapper.get('.group-section-heading .multiplier-control input[role="switch"]').element)
      .toBe(wrapper.get('input[role="switch"]').element)
    expect(wrapper.find('.filter-panel-footer, .selection-summary, .selection-summary-notes').exists()).toBe(false)
    expect(wrapper.find('.selected-group-name, .selected-model-count, .filter-status-pill').exists()).toBe(false)
    expect(wrapper.find('.custom-rate-note, .peak-rate-note').exists()).toBe(false)
  })

  it('switches the price and model list to the selected group without borrowing another channel price', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-group-id="2"]').trigger('click')

    expect(cardNames(wrapper)).toEqual(['premium-only', 'shared-model'])
    expect(cardFor(wrapper, 'shared-model').props('variant')).toMatchObject({
      channelName: 'Premium channel', groupIds: [2], pricing: pricing(0.000005),
    })
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.get('[data-group-id="2"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-group-id="1"]').trigger('click')

    expect(cardNames(wrapper)).toEqual(['basic-only', 'shared-model'])
    expect(cardFor(wrapper, 'shared-model').props('variant')).toMatchObject({
      channelName: 'Basic channel', groupIds: [1], pricing: pricing(0.000002),
    })
  })

  it('renders each group rate and resets adjusted prices when changing groups', async () => {
    const wrapper = await mountView(false)

    expect(cardFor(wrapper, 'shared-model').get('.price').text()).toBe('$2')
    await wrapper.get('input[role="switch"]').setValue(true)
    expect(cardFor(wrapper, 'shared-model').get('.price').text()).toBe('$1.2')
    expect(cardFor(wrapper, 'shared-model').get('.base-price').text()).toBe('$2 / 1M')
    expect(cardFor(wrapper, 'shared-model').get('.multiplier-pill').text()).toBe('×0.6')

    await wrapper.get('[data-group-id="2"]').trigger('click')

    expect((wrapper.get('input[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(cardFor(wrapper, 'shared-model').get('.price').text()).toBe('$5')
    expect(cardFor(wrapper, 'shared-model').find('.base-price').exists()).toBe(false)
    expect(cardFor(wrapper, 'shared-model').props('multiplier')).toMatchObject({ value: 0.25, source: 'user' })
    expect(wrapper.get('[data-group-id="2"] .group-rate').text()).toBe('×0.25')
    expect(wrapper.get('.multiplier-control').attributes('title')).toContain('modelSquare.customRate')
    expect(wrapper.text()).not.toContain('modelSquare.customRate')

    await wrapper.get('input[role="switch"]').setValue(true)
    expect(cardFor(wrapper, 'shared-model').get('.price').text()).toBe('$1.25')
    expect(cardFor(wrapper, 'shared-model').get('.base-price').text()).toBe('$5 / 1M')
    expect(cardFor(wrapper, 'shared-model').get('.multiplier-pill').text()).toBe('×0.25')
  })

  it('keeps every catalog platform and its count visible when changing groups', async () => {
    const wrapper = await mountView()
    const initialPlatforms = wrapper.findAll('[data-platform]').map((button) => button.text())

    await wrapper.get('[data-group-id="3"]').trigger('click')

    expect(cardNames(wrapper)).toEqual(['claude-only'])
    expect(platformIds(wrapper)).toEqual(['', 'anthropic', 'openai'])
    expect(wrapper.findAll('[data-platform]').map((button) => button.text())).toEqual(initialPlatforms)
    expect(wrapper.get('[data-platform=""]').attributes('aria-pressed')).toBe('true')
  })

  it('filters groups by catalog platform and selects its first group if the old group is unavailable', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-platform="openai"]').trigger('click')
    expect(groupIds(wrapper)).toEqual(['1', '2', '4'])
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('input[role="switch"]').setValue(true)

    await wrapper.get('[data-platform="anthropic"]').trigger('click')

    expect(groupIds(wrapper)).toEqual(['3', '4'])
    expect(wrapper.get('[data-group-id="3"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['claude-only'])
    expect(cardFor(wrapper, 'claude-only').props('showMultiplier')).toBe(false)
    expect(cardFor(wrapper, 'claude-only').props('multiplier')).toMatchObject({ value: 2.5, source: 'group' })
    expect(platformIds(wrapper)).toEqual(['', 'anthropic', 'openai'])
  })

  it('keeps a composite group selected across its concrete catalog platforms', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-group-id="4"]').trigger('click')
    expect(cardNames(wrapper)).toEqual(['composite-anthropic', 'composite-openai'])

    await wrapper.get('[data-platform="openai"]').trigger('click')
    expect(groupIds(wrapper)).toContain('4')
    expect(wrapper.get('[data-group-id="4"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['composite-openai'])

    await wrapper.get('[data-platform="anthropic"]').trigger('click')
    expect(groupIds(wrapper)).toContain('4')
    expect(wrapper.get('[data-group-id="4"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['composite-anthropic'])
    expect(cardFor(wrapper, 'composite-anthropic').props('variant').groupIds).toEqual([4])
    expect(cardFor(wrapper, 'composite-anthropic').props('multiplier')).toMatchObject({ value: 0.4 })
  })

  it('defaults to the first group with models but preserves a manually selected empty group', async () => {
    const emptyGroup = group(5, '0 Empty', 'openai')
    catalog.groups.push(emptyGroup)
    catalog.channels.push(channel('Empty channel', [{
      platform: 'openai', groups: [emptyGroup], supported_models: [],
    }]))
    const wrapper = await mountView()

    expect(groupIds(wrapper)).toEqual(['5', '1', '2', '3', '4'])
    expect(wrapper.get('[data-group-id="5"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['basic-only', 'shared-model'])

    await wrapper.get('[data-group-id="5"]').trigger('click')
    await wrapper.get('[data-platform="openai"]').trigger('click')
    await refresh(wrapper)

    expect(wrapper.get('[data-group-id="5"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('false')
    expect(cardNames(wrapper)).toEqual([])
    expect(wrapper.text()).toContain('modelSquare.emptyFiltered')
  })

  it('shows an empty state for a group with no models instead of displaying another group models', async () => {
    const emptyGroup = group(5, 'E Empty', 'openai')
    catalog.groups.push(emptyGroup)
    catalog.channels.push(channel('Empty channel', [{
      platform: 'openai', groups: [emptyGroup], supported_models: [],
    }]))
    const wrapper = await mountView()

    await wrapper.get('[data-group-id="5"]').trigger('click')

    expect(cardNames(wrapper)).toEqual([])
    expect(wrapper.get('[data-group-id="5"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('modelSquare.emptyFiltered')
    expect(wrapper.text()).toContain('modelSquare.emptyFilteredHint')
    expect(platformIds(wrapper)).toEqual(['', 'anthropic', 'openai'])
  })

  it('keeps missing group pricing empty even when another group configures the same model price', async () => {
    catalog.channels[1].platforms[0].supported_models[0].pricing = null
    const wrapper = await mountView()

    await wrapper.get('[data-group-id="2"]').trigger('click')

    expect(cardFor(wrapper, 'shared-model').props('variant')).toEqual({
      channelName: 'Premium channel', groupIds: [2], pricing: null,
    })
  })

  it('does not fall back to catalog models when no catalog group is accessible', async () => {
    catalog.groups = [group(99, 'Unbound group', 'openai')]
    const wrapper = await mountView()

    expect(groupIds(wrapper)).toEqual([])
    expect(cardNames(wrapper)).toEqual([])
    expect(platformIds(wrapper)).toEqual([''])
    expect(wrapper.get('input[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('modelSquare.noGroups')
    expect(wrapper.get('.multiplier-control').attributes('title')).toContain('modelSquare.selectGroupHint')
    expect(wrapper.text()).toContain('modelSquare.emptyFiltered')
  })

  it('selects the first remaining group on refresh when the active group is removed', async () => {
    const wrapper = await mountView()
    await wrapper.get('[data-platform="openai"]').trigger('click')
    await wrapper.get('[data-group-id="2"]').trigger('click')
    await wrapper.get('input[role="switch"]').setValue(true)
    catalog.groups = catalog.groups.filter((item) => item.id !== 2)

    await refresh(wrapper)

    expect(wrapper.get('[data-platform="openai"]').attributes('aria-pressed')).toBe('true')
    expect(groupIds(wrapper)).toEqual(['1', '4'])
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['basic-only', 'shared-model'])
    expect(cardFor(wrapper, 'shared-model').props('variant').pricing).toEqual(pricing(0.000002))
    expect(cardFor(wrapper, 'shared-model').props('showMultiplier')).toBe(false)
  })

  it('returns to all platforms and a valid group when refresh removes the selected platform', async () => {
    const wrapper = await mountView()
    await wrapper.get('[data-platform="anthropic"]').trigger('click')
    await wrapper.get('input[role="switch"]').setValue(true)
    catalog.channels = catalog.channels.map((item) => ({
      ...item, platforms: item.platforms.filter((section) => section.platform !== 'anthropic'),
    }))

    await refresh(wrapper)

    expect(platformIds(wrapper)).toEqual(['', 'openai'])
    expect(wrapper.get('[data-platform=""]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-group-id="1"]').attributes('aria-pressed')).toBe('true')
    expect(cardNames(wrapper)).toEqual(['basic-only', 'shared-model'])
    expect(cardFor(wrapper, 'shared-model').props('showMultiplier')).toBe(false)
  })

  it('clears stale cards and disables adjusted prices when refresh removes every available group', async () => {
    const wrapper = await mountView()
    await wrapper.get('input[role="switch"]').setValue(true)
    catalog.groups = []

    await refresh(wrapper)

    expect(groupIds(wrapper)).toEqual([])
    expect(cardNames(wrapper)).toEqual([])
    expect(wrapper.find('.selected-group-name').exists()).toBe(false)
    expect((wrapper.get('input[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.get('input[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('modelSquare.noGroups')
    expect(wrapper.text()).toContain('modelSquare.emptyFiltered')
  })

  it('shows the unavailable empty state after refreshing to an empty catalog', async () => {
    const wrapper = await mountView()
    await wrapper.get('[data-platform="openai"]').trigger('click')
    catalog.channels = []

    await refresh(wrapper)

    expect(platformIds(wrapper)).toEqual([''])
    expect(wrapper.get('[data-platform=""]').attributes('aria-pressed')).toBe('true')
    expect(groupIds(wrapper)).toEqual([])
    expect(cardNames(wrapper)).toEqual([])
    expect(wrapper.get('input[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('modelSquare.emptyUnavailable')
    expect(wrapper.text()).toContain('modelSquare.emptyUnavailableHint')
    expect(showError).not.toHaveBeenCalled()
  })
})
