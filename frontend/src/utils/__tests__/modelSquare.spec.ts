import { describe, expect, it } from 'vitest'
import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserChannelPlatformSection,
  UserSupportedModelPricing,
} from '@/api/channels'
import type { Group } from '@/types'
import {
  buildModelSquareModels,
  collectModelSquareGroups,
  resolveModelVariant,
} from '../modelSquare'

function catalogGroup(id: number, name: string, platform: string): UserAvailableGroup {
  return {
    id,
    name,
    platform,
    subscription_type: 'standard',
    rate_multiplier: 1,
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

function channel(name: string, platforms: UserChannelPlatformSection[]): UserAvailableChannel {
  return { name, description: '', platforms }
}

describe('Model Square group pricing variants', () => {
  const basicGroup = catalogGroup(1, 'Basic', 'openai')
  const premiumGroup = catalogGroup(2, 'Premium', 'openai')
  const basicPrice = pricing(0.000002)
  const premiumPrice = pricing(0.000005)
  const channels = [
    channel('A channel', [{
      platform: 'openai',
      groups: [basicGroup],
      supported_models: [{ name: 'shared-model', platform: 'openai', pricing: basicPrice }],
    }]),
    channel('B channel', [{
      platform: 'openai',
      groups: [premiumGroup],
      supported_models: [{ name: 'shared-model', platform: 'openai', pricing: premiumPrice }],
    }]),
  ]

  it('keeps the configured price for each group when the model exists in multiple channels', () => {
    const models = buildModelSquareModels(channels)

    expect(models).toHaveLength(1)
    expect(models[0].groupIds).toEqual([1, 2])
    expect(resolveModelVariant(models[0], 1)).toEqual({
      channelName: 'A channel', groupIds: [1], pricing: basicPrice,
    })
    expect(resolveModelVariant(models[0], 2)).toEqual({
      channelName: 'B channel', groupIds: [2], pricing: premiumPrice,
    })
  })

  it('does not fall back to another group price when the requested group has no variant', () => {
    const [model] = buildModelSquareModels(channels)

    expect(resolveModelVariant(model, 999)).toBeNull()
  })

  it('keeps identically named models on different platforms separate', () => {
    const [openaiChannel] = channels
    const anthropicPrice = pricing(0.000003)
    const models = buildModelSquareModels([
      openaiChannel,
      channel('Anthropic channel', [{
        platform: 'anthropic',
        groups: [catalogGroup(3, 'Anthropic', 'anthropic')],
        supported_models: [{ name: 'shared-model', platform: 'anthropic', pricing: anthropicPrice }],
      }]),
    ])

    expect(models).toHaveLength(2)
    const anthropicModel = models.find((model) => model.platform === 'anthropic')!
    const openaiModel = models.find((model) => model.platform === 'openai')!
    expect(resolveModelVariant(anthropicModel, 3)?.pricing).toEqual(anthropicPrice)
    expect(resolveModelVariant(openaiModel, 3)).toBeNull()
    expect(resolveModelVariant(anthropicModel, 1)).toBeNull()
  })
})

describe('Model Square platform groups', () => {
  const openaiGroup = catalogGroup(1, 'A OpenAI', 'openai')
  const anthropicGroup = catalogGroup(2, 'B Anthropic', 'anthropic')
  const compositeGroup = catalogGroup(3, 'C Composite', 'composite')
  const unrelatedGroup = catalogGroup(4, 'D Unrelated', 'openai')
  const inaccessibleGroup = catalogGroup(99, 'E Inaccessible', 'openai')
  const availableGroups = [openaiGroup, anthropicGroup, compositeGroup, unrelatedGroup] as Group[]
  const channels = [channel('Combined channel', [
    {
      platform: 'openai',
      groups: [openaiGroup, compositeGroup, inaccessibleGroup],
      supported_models: [],
    },
    {
      platform: 'anthropic',
      groups: [anthropicGroup, compositeGroup],
      supported_models: [],
    },
  ])]

  it('keeps composite groups available in each matching catalog platform section', () => {
    expect(collectModelSquareGroups(channels, availableGroups, 'openai').map((group) => group.id))
      .toEqual([1, 3])
    expect(collectModelSquareGroups(channels, availableGroups, 'anthropic').map((group) => group.id))
      .toEqual([2, 3])
  })

  it('excludes groups outside the user available list or outside the selected platform catalog', () => {
    const groups = collectModelSquareGroups(channels, availableGroups, 'openai')

    expect(groups.map((group) => group.id)).not.toContain(inaccessibleGroup.id)
    expect(groups.map((group) => group.id)).not.toContain(unrelatedGroup.id)
    expect(groups.map((group) => group.id)).not.toContain(anthropicGroup.id)
    expect(collectModelSquareGroups(channels, availableGroups, 'gemini')).toEqual([])
  })

  it('preserves the all-platform group intersection when no platform is provided', () => {
    expect(collectModelSquareGroups(channels, availableGroups).map((group) => group.id))
      .toEqual([1, 2, 3])
    expect(collectModelSquareGroups(channels, availableGroups, '').map((group) => group.id))
      .toEqual([1, 2, 3])
  })
})
