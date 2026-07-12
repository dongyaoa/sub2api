import type {
  UserAvailableChannel,
  UserSupportedModelPricing,
} from '@/api/channels'
import type { Group } from '@/types'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  type BillingMode,
} from '@/constants/channel'

export interface ModelPricingVariant {
  channelName: string
  groupIds: number[]
  pricing: UserSupportedModelPricing | null
}

export interface ModelSquareModel {
  key: string
  name: string
  platform: string
  channelNames: string[]
  groupIds: number[]
  variants: ModelPricingVariant[]
}

export interface EffectiveMultiplier {
  value: number
  baseValue: number
  source: 'user' | 'group'
  imageIndependent: boolean
  peakActive: boolean
  peakFactor: number
}

function uniqueSorted(values: Iterable<number>): number[] {
  return [...new Set(values)].sort((a, b) => a - b)
}

export function buildModelSquareModels(channels: UserAvailableChannel[]): ModelSquareModel[] {
  const models = new Map<string, ModelSquareModel>()

  for (const channel of channels) {
    for (const section of channel.platforms) {
      const groupIds = uniqueSorted(section.groups.map((group) => group.id))

      for (const model of section.supported_models) {
        const platform = model.platform || section.platform
        const key = `${platform.toLowerCase()}::${model.name.toLowerCase()}`
        let entry = models.get(key)

        if (!entry) {
          entry = {
            key,
            name: model.name,
            platform,
            channelNames: [],
            groupIds: [],
            variants: [],
          }
          models.set(key, entry)
        }

        if (!entry.channelNames.includes(channel.name)) {
          entry.channelNames.push(channel.name)
        }
        entry.groupIds = uniqueSorted([...entry.groupIds, ...groupIds])

        const existingVariant = entry.variants.find(
          (variant) => variant.channelName === channel.name,
        )
        if (existingVariant) {
          existingVariant.groupIds = uniqueSorted([...existingVariant.groupIds, ...groupIds])
          if (!existingVariant.pricing && model.pricing) existingVariant.pricing = model.pricing
        } else {
          entry.variants.push({
            channelName: channel.name,
            groupIds,
            pricing: model.pricing,
          })
        }
      }
    }
  }

  return [...models.values()]
    .map((model) => ({
      ...model,
      channelNames: [...model.channelNames].sort((a, b) => a.localeCompare(b)),
      variants: [...model.variants].sort((a, b) => {
        if (Boolean(a.pricing) !== Boolean(b.pricing)) return a.pricing ? -1 : 1
        return a.channelName.localeCompare(b.channelName)
      }),
    }))
    .sort((a, b) => {
      const platformOrder = a.platform.localeCompare(b.platform)
      return platformOrder || a.name.localeCompare(b.name)
    })
}

export function resolveModelVariant(
  model: ModelSquareModel,
  groupId?: number | null,
): ModelPricingVariant | null {
  const candidates = groupId == null
    ? model.variants
    : model.variants.filter((variant) => variant.groupIds.includes(groupId))

  return candidates.find((variant) => variant.pricing) ?? candidates[0] ?? null
}

export function collectModelSquareGroups(
  channels: UserAvailableChannel[],
  groups: Group[],
): Group[] {
  const visibleIds = new Set<number>()
  for (const channel of channels) {
    for (const section of channel.platforms) {
      for (const group of section.groups) visibleIds.add(group.id)
    }
  }

  return groups
    .filter((group) => visibleIds.has(group.id))
    .sort((a, b) => a.name.localeCompare(b.name))
}

function parseTimeMinutes(value?: string | null): number | null {
  if (!value) return null
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hours = Number(match[1])
  const minutes = Number(match[2])
  if (hours > 23 || minutes > 59) return null
  return hours * 60 + minutes
}

function parseUTCOffsetMinutes(value?: string | null): number | null {
  if (!value) return null
  const match = /^([+-])(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const minutes = Number(match[2]) * 60 + Number(match[3])
  return match[1] === '-' ? -minutes : minutes
}

export function isGroupPeakActive(
  group: Group,
  serverUTCOffset?: string | null,
  now: Date = new Date(),
): boolean {
  if (
    group.subscription_type !== 'subscription' ||
    !group.peak_rate_enabled ||
    group.peak_rate_multiplier == null
  ) {
    return false
  }

  const start = parseTimeMinutes(group.peak_start)
  const end = parseTimeMinutes(group.peak_end)
  if (start == null || end == null || start >= end) return false

  const offset = parseUTCOffsetMinutes(serverUTCOffset)
  if (offset == null) return false
  const utcMinutes = now.getUTCHours() * 60 + now.getUTCMinutes()
  const serverMinutes = ((utcMinutes + offset) % 1440 + 1440) % 1440
  return serverMinutes >= start && serverMinutes < end
}

export function resolveEffectiveMultiplier(
  group: Group,
  userRate: number | undefined,
  billingMode: BillingMode,
  options?: { serverUTCOffset?: string | null; now?: Date },
): EffectiveMultiplier {
  const source = userRate == null ? 'group' : 'user'
  const baseValue = userRate ?? group.rate_multiplier
  const imageIndependent =
    billingMode === BILLING_MODE_IMAGE && Boolean(group.image_rate_independent)
  const peakActive =
    billingMode === BILLING_MODE_TOKEN &&
    isGroupPeakActive(group, options?.serverUTCOffset, options?.now)
  const peakFactor = peakActive ? group.peak_rate_multiplier : 1

  let value = imageIndependent ? group.image_rate_multiplier : baseValue
  if (peakActive) value *= peakFactor

  return {
    value: Math.max(0, value),
    baseValue: Math.max(0, baseValue),
    source,
    imageIndependent,
    peakActive,
    peakFactor,
  }
}

export function formatModelPrice(
  value: number | null,
  options: { scale?: number; multiplier?: number; symbol?: '$' | '¥' } = {},
): string {
  if (value == null) return '—'
  const scaled = value * (options.scale ?? 1) * (options.multiplier ?? 1)
  const abs = Math.abs(scaled)
  const maximumFractionDigits = abs >= 100 ? 2 : abs >= 1 ? 4 : abs >= 0.01 ? 6 : 8
  const formatted = new Intl.NumberFormat('en-US', {
    useGrouping: false,
    maximumFractionDigits,
  }).format(scaled)
  return `${options.symbol ?? '$'}${formatted}`
}

