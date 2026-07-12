<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] space-y-5">
      <section class="premium-filter-panel">
        <div class="filter-platform-header flex flex-col gap-2 border-b sm:flex-row sm:items-center">
          <span class="filter-section-label inline-flex flex-shrink-0 items-center gap-1.5">
            <Icon name="chartBar" size="sm" />
            {{ t('modelSquare.platformLabel') }}
          </span>

          <div class="platform-tabs flex min-w-0 gap-2 overflow-x-auto pb-1 sm:pb-0">
            <button
              type="button"
              class="platform-tab inline-flex flex-shrink-0 items-center gap-2 border px-3 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              :class="selectedPlatform === ''
                ? 'border-blue-600 bg-blue-600 text-white hover:bg-blue-700 dark:border-blue-500 dark:bg-blue-500 dark:hover:bg-blue-400'
                : 'border-gray-200 bg-gray-50 text-gray-600 hover:border-gray-300 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500'"
              @click="selectedPlatform = ''"
            >
              <Icon name="grid" size="xs" />
              <span>{{ t('modelSquare.allPlatforms') }}</span>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-semibold"
                :class="selectedPlatform === '' ? 'bg-white/15 dark:bg-gray-900/10' : 'bg-white text-gray-700 dark:bg-dark-700 dark:text-gray-200'"
              >{{ groupScopedModels.length }}</span>
            </button>

            <button
              v-for="stat in platformStats"
              :key="stat.platform"
              type="button"
              class="platform-tab inline-flex flex-shrink-0 items-center gap-2 border px-3 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              :class="selectedPlatform === stat.platform
                ? ['border-transparent', platformButtonClass(stat.platform)]
                : platformBadgeClass(stat.platform)"
              @click="selectedPlatform = stat.platform"
            >
              <PlatformIcon
                :platform="stat.platform as GroupPlatform"
                size="xs"
              />
              <span class="uppercase">{{ stat.platform }}</span>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-semibold"
                :class="selectedPlatform === stat.platform ? 'bg-white/15 dark:bg-gray-900/10' : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'"
              >{{ stat.count }}</span>
            </button>
          </div>
        </div>

        <div class="filter-control-grid grid grid-cols-1 md:grid-cols-2 xl:grid-cols-[minmax(240px,1fr)_200px_240px_auto_auto] xl:items-end">
          <label class="block">
            <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('modelSquare.searchLabel') }}
            </span>
            <span class="relative block">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="searchQuery"
                type="search"
                class="input premium-control pl-9"
                :placeholder="t('modelSquare.searchPlaceholder')"
              />
            </span>
          </label>

          <label class="block">
            <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('modelSquare.platformLabel') }}
            </span>
            <Select
              v-model="selectedPlatform"
              :options="platformSelectOptions"
              :searchable="false"
              class="model-square-select"
            >
              <template #selected="{ option }">
                <span class="model-select-content">
                  <PlatformIcon
                    v-if="(option as ModelSquareSelectOption)?.platform"
                    :platform="(option as ModelSquareSelectOption).platform as GroupPlatform"
                    size="xs"
                  />
                  <Icon v-else name="grid" size="xs" class="text-gray-400" />
                  <span class="truncate">{{ (option as ModelSquareSelectOption)?.label }}</span>
                </span>
              </template>
              <template #option="{ option, selected }">
                <span class="model-select-option">
                  <span class="model-select-content">
                    <PlatformIcon
                      v-if="(option as ModelSquareSelectOption).platform"
                      :platform="(option as ModelSquareSelectOption).platform as GroupPlatform"
                      size="xs"
                    />
                    <Icon v-else name="grid" size="xs" class="text-gray-400" />
                    <span class="truncate">{{ (option as ModelSquareSelectOption).label }}</span>
                  </span>
                  <Icon v-if="selected" name="check" size="sm" class="model-select-check" :stroke-width="2" />
                </span>
              </template>
            </Select>
          </label>

          <label class="block">
            <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('modelSquare.groupLabel') }}
            </span>
            <Select
              v-model="selectedGroupId"
              :options="groupSelectOptions"
              :searchable="false"
              class="model-square-select"
            >
              <template #selected="{ option }">
                <span class="model-select-content">
                  <PlatformIcon
                    v-if="(option as ModelSquareSelectOption)?.platform"
                    :platform="(option as ModelSquareSelectOption).platform as GroupPlatform"
                    size="xs"
                  />
                  <Icon v-else name="grid" size="xs" class="text-gray-400" />
                  <span class="truncate">{{ (option as ModelSquareSelectOption)?.label }}</span>
                </span>
              </template>
              <template #option="{ option, selected }">
                <span class="model-select-option">
                  <span class="model-select-content">
                    <PlatformIcon
                      v-if="(option as ModelSquareSelectOption).platform"
                      :platform="(option as ModelSquareSelectOption).platform as GroupPlatform"
                      size="xs"
                    />
                    <Icon v-else name="grid" size="xs" class="text-gray-400" />
                    <span class="truncate">{{ (option as ModelSquareSelectOption).label }}</span>
                  </span>
                  <Icon v-if="selected" name="check" size="sm" class="model-select-check" :stroke-width="2" />
                </span>
              </template>
            </Select>
          </label>

          <label
            class="multiplier-control flex select-none items-center justify-between gap-3 border px-3 transition-colors"
            :class="selectedGroup
              ? 'cursor-pointer border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
              : 'cursor-not-allowed border-gray-100 bg-gray-50/60 opacity-60 dark:border-dark-800 dark:bg-dark-900'"
          >
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('modelSquare.showMultiplier') }}
            </span>
            <span class="relative inline-flex h-6 w-11 flex-shrink-0">
              <input
                v-model="showMultiplier"
                type="checkbox"
                class="peer sr-only"
                :disabled="!selectedGroup"
              />
              <span class="absolute inset-0 rounded-full bg-gray-300 transition-colors peer-checked:bg-emerald-600 peer-focus-visible:ring-2 peer-focus-visible:ring-emerald-500 peer-focus-visible:ring-offset-2 dark:bg-dark-600"></span>
              <span class="absolute left-1 top-1 h-4 w-4 rounded-full bg-white shadow-sm transition-transform peer-checked:translate-x-5"></span>
            </span>
          </label>

          <button
            type="button"
            class="filter-refresh-button btn btn-secondary justify-center"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="loadData"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            <span class="xl:hidden">{{ t('common.refresh') }}</span>
          </button>
        </div>

        <div class="filter-panel-footer flex flex-col gap-3 border-t sm:flex-row sm:items-center sm:justify-between">
          <div class="flex flex-wrap items-center gap-2 text-xs">
            <span class="inline-flex items-center gap-1.5 text-gray-600 dark:text-gray-300">
              <Icon name="grid" size="xs" />
              {{ t('modelSquare.modelCount', { count: filteredModels.length }) }}
            </span>
            <span class="text-gray-300 dark:text-dark-600">|</span>
            <span class="text-gray-500 dark:text-gray-400">{{ t('modelSquare.rechargeRate') }}</span>
          </div>

          <div v-if="selectedGroup" class="flex flex-wrap items-center gap-2 text-xs">
            <span class="text-gray-500 dark:text-gray-400">{{ selectedGroup.name }}</span>
            <span
              class="filter-status-pill px-2 py-1 font-semibold"
              :class="showMultiplier
                ? activeMultiplier?.peakActive
                  ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
                  : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
                : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
            >
              {{ showMultiplier
                ? t('modelSquare.multiplierEnabled', { rate: currentRateLabel })
                : t('modelSquare.originalPriceActive') }}
            </span>
            <span v-if="activeMultiplier?.source === 'user'" class="text-blue-600 dark:text-blue-400">
              {{ t('modelSquare.customRate') }}
            </span>
            <span v-if="activeMultiplier?.peakActive" class="text-amber-700 dark:text-amber-300">
              {{ t('modelSquare.peakActive', { rate: activeMultiplier.peakFactor }) }}
            </span>
          </div>
          <span v-else class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('modelSquare.selectGroupHint') }}
          </span>
        </div>
      </section>

      <div v-if="loading" class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <div
          v-for="index in 6"
          :key="index"
          class="h-[268px] animate-pulse rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
        >
          <div class="mb-4 h-5 w-24 rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="mb-8 h-4 w-3/4 rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="grid grid-cols-2 gap-4">
            <div v-for="cell in 4" :key="cell" class="space-y-2">
              <div class="h-3 w-16 rounded bg-gray-100 dark:bg-dark-800"></div>
              <div class="h-5 w-24 rounded bg-gray-200 dark:bg-dark-700"></div>
            </div>
          </div>
        </div>
      </div>

      <section
        v-else-if="filteredModels.length === 0"
        class="flex min-h-72 flex-col items-center justify-center rounded-lg border border-dashed border-gray-300 bg-white/60 px-6 text-center dark:border-dark-600 dark:bg-dark-900/50"
      >
        <Icon name="inbox" size="xl" class="mb-3 text-gray-300 dark:text-dark-500" />
        <h2 class="text-sm font-semibold text-gray-800 dark:text-gray-200">{{ emptyTitle }}</h2>
        <p class="mt-1 max-w-md text-xs leading-5 text-gray-500 dark:text-gray-400">{{ emptyDescription }}</p>
      </section>

      <section v-else class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <ModelPriceCard
          v-for="model in filteredModels"
          :key="model.key"
          :model="model"
          :variant="variantFor(model)"
          :show-multiplier="showMultiplier"
          :multiplier="multiplierFor(model)"
          :price-platform="selectedGroup?.platform || model.platform"
        />
      </section>

      <p v-if="filteredModels.length > 0" class="pb-2 text-center text-[11px] text-gray-400 dark:text-gray-500">
        {{ t('modelSquare.referenceNotice') }}
      </p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import modelSquareMessages from '@/i18n/modelSquare'
import ModelPriceCard from '@/components/models/ModelPriceCard.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import type { Group, GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  buildModelSquareModels,
  collectModelSquareGroups,
  resolveEffectiveMultiplier,
  resolveModelVariant,
  type EffectiveMultiplier,
  type ModelPricingVariant,
  type ModelSquareModel,
} from '@/utils/modelSquare'
import { BILLING_MODE_TOKEN } from '@/constants/channel'
import { platformBadgeClass, platformButtonClass } from '@/utils/platformColors'

interface ModelSquareSelectOption extends SelectOption {
  platform?: string
}

const route = useRoute()
const { t, locale } = useI18n({ useScope: 'local', messages: modelSquareMessages })

watch(
  locale,
  () => {
    route.meta.title = t('modelSquare.title')
    route.meta.description = t('modelSquare.description')
  },
  { immediate: true },
)
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const groups = ref<Group[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const loaded = ref(false)
const searchQuery = ref('')
const selectedPlatform = ref('')
const selectedGroupId = ref('')
const showMultiplier = ref(false)

const models = computed(() => buildModelSquareModels(channels.value))
const availableGroups = computed(() => collectModelSquareGroups(channels.value, groups.value))
const selectedGroup = computed(() => {
  if (!selectedGroupId.value) return null
  const id = Number(selectedGroupId.value)
  return availableGroups.value.find((group) => group.id === id) ?? null
})
const platforms = computed(() => [...new Set(models.value.map((model) => model.platform))].sort())
const platformSelectOptions = computed<ModelSquareSelectOption[]>(() => [
  { value: '', label: t('modelSquare.allPlatforms') },
  ...platforms.value.map((platform) => ({
    value: platform,
    label: platform.toUpperCase(),
    platform,
  })),
])
const groupSelectOptions = computed<ModelSquareSelectOption[]>(() => [
  { value: '', label: t('modelSquare.allModelsOriginal') },
  ...availableGroups.value.map((group) => ({
    value: String(group.id),
    label: `${group.name} · ×${groupRateLabel(group)}`,
    platform: group.platform,
  })),
])

const groupScopedModels = computed(() => {
  const groupId = selectedGroup.value?.id
  if (groupId == null) return models.value
  return models.value.filter((model) => model.groupIds.includes(groupId))
})

const platformStats = computed(() => {
  const counts = new Map<string, number>()
  for (const model of groupScopedModels.value) {
    counts.set(model.platform, (counts.get(model.platform) ?? 0) + 1)
  }
  return [...counts.entries()]
    .map(([platform, count]) => ({ platform, count }))
    .sort((a, b) => a.platform.localeCompare(b.platform))
})

const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const groupId = selectedGroup.value?.id
  return models.value.filter((model) => {
    if (groupId != null && !model.groupIds.includes(groupId)) return false
    if (selectedPlatform.value && model.platform !== selectedPlatform.value) return false
    if (query && !model.name.toLowerCase().includes(query) && !model.platform.toLowerCase().includes(query)) return false
    return true
  })
})

const activeMultiplier = computed<EffectiveMultiplier | null>(() => {
  if (!selectedGroup.value) return null
  return resolveEffectiveMultiplier(
    selectedGroup.value,
    userGroupRates.value[selectedGroup.value.id],
    BILLING_MODE_TOKEN,
    { serverUTCOffset: appStore.cachedPublicSettings?.server_utc_offset },
  )
})

const currentRateLabel = computed(() => {
  const value = activeMultiplier.value?.value ?? 1
  return Number(value.toPrecision(8))
})

const emptyTitle = computed(() => {
  if (!loaded.value || channels.value.length === 0) return t('modelSquare.emptyUnavailable')
  return t('modelSquare.emptyFiltered')
})

const emptyDescription = computed(() => {
  if (!loaded.value || channels.value.length === 0) return t('modelSquare.emptyUnavailableHint')
  return t('modelSquare.emptyFilteredHint')
})

watch(selectedGroupId, () => {
  showMultiplier.value = false
})

watch(availableGroups, (value) => {
  if (selectedGroupId.value && !value.some((group) => String(group.id) === selectedGroupId.value)) {
    selectedGroupId.value = ''
  }
})

function groupRateLabel(group: Group): number {
  return userGroupRates.value[group.id] ?? group.rate_multiplier
}

function variantFor(model: ModelSquareModel): ModelPricingVariant | null {
  return resolveModelVariant(model, selectedGroup.value?.id)
}

function multiplierFor(model: ModelSquareModel): EffectiveMultiplier | null {
  const group = selectedGroup.value
  if (!group) return null
  const mode = variantFor(model)?.pricing?.billing_mode ?? BILLING_MODE_TOKEN
  return resolveEffectiveMultiplier(group, userGroupRates.value[group.id], mode, {
    serverUTCOffset: appStore.cachedPublicSettings?.server_utc_offset,
  })
}

async function loadData() {
  loading.value = true
  try {
    const [channelList, groupList, rateMap] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch(() => ({} as Record<number, number>)),
    ])
    channels.value = channelList
    groups.value = groupList
    userGroupRates.value = rateMap
    loaded.value = true
  } catch (error: unknown) {
    loaded.value = true
    appStore.showError(extractApiErrorMessage(error, t('modelSquare.loadFailed')))
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>

<style scoped>
.premium-filter-panel {
  position: relative;
  overflow: hidden;
  padding: 18px 20px;
  border: 1px solid #e9edf2;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.02), 0 7px 20px rgba(15, 23, 42, 0.035);
  font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.filter-platform-header {
  gap: 18px;
  margin-bottom: 16px;
  padding-bottom: 14px;
  border-color: #edf0f3;
}

.filter-section-label {
  color: #475569;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}

.platform-tabs {
  scrollbar-width: none;
}

.platform-tabs::-webkit-scrollbar {
  display: none;
}

.platform-tab {
  height: 32px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  box-shadow: 0 1px 1px rgba(15, 23, 42, 0.025);
}

.filter-control-grid {
  gap: 14px;
}

.filter-control-grid > label > span:first-child {
  color: #475569;
  font-size: 12px;
  font-weight: 500;
}

.premium-control {
  height: 40px;
  border-color: #e2e8f0;
  border-radius: 10px;
  background-color: #f8fafc;
  color: #0f172a;
  font-size: 13px;
  box-shadow: inset 0 1px 1px rgba(15, 23, 42, 0.015);
}

.premium-control:focus {
  border-color: #cbd5e1;
  background-color: #fff;
  box-shadow: 0 0 0 3px #f1f5f9;
}

.model-square-select {
  width: 100%;
}

.model-square-select :deep(.select-trigger) {
  height: 40px;
  min-height: 40px;
  padding: 0 12px;
  border-color: #e2e8f0;
  border-radius: 10px;
  background-color: #f8fafc;
  font-size: 13px;
  box-shadow: inset 0 1px 1px rgba(15, 23, 42, 0.015);
}

.model-select-content {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.model-select-option {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.model-select-check {
  flex-shrink: 0;
  color: #14b8a6;
}

.multiplier-control,
.filter-refresh-button {
  min-height: 40px;
  border-radius: 10px;
}

.multiplier-control {
  height: 40px;
  box-shadow: inset 0 1px 1px rgba(15, 23, 42, 0.015);
}

.filter-refresh-button {
  height: 40px;
}

.filter-panel-footer {
  margin-top: 16px;
  padding-top: 14px;
  border-color: #edf0f3;
}

.filter-status-pill {
  border-radius: 6px;
  line-height: 1.2;
}

:global(.dark) .premium-filter-panel {
  border-color: #334155;
  background: #0f172a;
  box-shadow: none;
}

:global(.dark) .filter-platform-header,
:global(.dark) .filter-panel-footer {
  border-color: #334155;
}

:global(.dark) .filter-section-label,
:global(.dark) .filter-control-grid > label > span:first-child {
  color: #cbd5e1;
}

:global(.dark) .premium-control {
  border-color: #334155;
  background-color: #1e293b;
  color: #f8fafc;
  box-shadow: none;
}
:global(.dark) .model-square-select :deep(.select-trigger) {
  border-color: #334155;
  background-color: #1e293b;
  color: #f8fafc;
  box-shadow: none;
}


:global(.dark) .premium-control:focus {
  border-color: #475569;
  background-color: #1e293b;
  box-shadow: 0 0 0 3px rgba(51, 65, 85, 0.45);
}

@media (min-width: 1280px) {
  .filter-refresh-button {
    width: 40px;
    min-width: 40px;
    padding-right: 0;
    padding-left: 0;
  }
}

@media (max-width: 640px) {
  .premium-filter-panel {
    padding: 16px;
  }

  .filter-platform-header {
    gap: 10px;
  }
}
</style>
