<template>
  <AppLayout>
    <div class="model-square mx-auto max-w-[1600px] space-y-5">
      <section class="premium-filter-panel" :aria-busy="loading">
        <div class="platform-section">
          <h2 id="model-platform-label" class="filter-section-label">
            <Icon name="chartBar" size="sm" />
            {{ t('modelSquare.platformLabel') }}
          </h2>

          <div class="platform-tabs" role="group" aria-labelledby="model-platform-label">
            <button
              type="button"
              class="platform-tab"
              :class="selectedPlatform === '' ? 'platform-tab--all-active' : 'platform-tab--all'"
              :aria-pressed="selectedPlatform === ''"
              data-platform=""
              @click="selectedPlatform = ''"
            >
              <Icon name="grid" size="sm" />
              <span>{{ t('modelSquare.allPlatforms') }}</span>
              <span class="platform-count">{{ models.length }}</span>
            </button>
            <button
              v-for="stat in platformStats"
              :key="stat.platform"
              type="button"
              class="platform-tab"
              :class="selectedPlatform === stat.platform
                ? ['border-transparent', platformButtonClass(stat.platform)]
                : platformBadgeClass(stat.platform)"
              :aria-pressed="selectedPlatform === stat.platform"
              :data-platform="stat.platform"
              @click="selectedPlatform = stat.platform"
            >
              <PlatformIcon :platform="stat.platform as GroupPlatform" size="sm" />
              <span>{{ platformLabel(stat.platform) }}</span>
              <span class="platform-count">{{ stat.count }}</span>
            </button>
          </div>

          <button
            type="button"
            class="filter-refresh-button"
            :disabled="loading"
            :aria-label="t('common.refresh')"
            :title="t('common.refresh')"
            @click="loadData"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('common.refresh') }}</span>
          </button>
        </div>

        <div class="group-section">
          <div class="filter-section-heading group-section-heading">
            <div class="group-heading-copy">
              <h2 id="model-group-label" class="filter-section-label">
                <Icon name="users" size="sm" />
                {{ t('modelSquare.groupLabel') }}
              </h2>
              <p class="filter-hint">{{ t('modelSquare.groupSelectionHint') }}</p>
            </div>
            <label class="multiplier-control" :title="multiplierHint" :class="{ 'multiplier-control--disabled': !selectedGroup || loading }">
              <span class="multiplier-label">
                <Icon name="bolt" size="sm" />
                {{ t('modelSquare.showMultiplier') }}
              </span>
              <span class="relative inline-flex h-5 w-9 flex-shrink-0">
                <input
                  v-model="showMultiplier"
                  type="checkbox"
                  role="switch"
                  :aria-description="multiplierHint"
                  class="peer sr-only"
                  :disabled="!selectedGroup || loading"
                />
                <span class="absolute inset-0 rounded-full bg-gray-300 transition-colors peer-checked:bg-emerald-600 peer-focus-visible:ring-2 peer-focus-visible:ring-emerald-500 peer-focus-visible:ring-offset-2 dark:bg-dark-600 dark:peer-focus-visible:ring-offset-dark-900"></span>
                <span class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform peer-checked:translate-x-4"></span>
              </span>
            </label>
          </div>

          <div v-if="visibleGroups.length" class="group-options" role="group" aria-labelledby="model-group-label">
            <button
              v-for="group in visibleGroups"
              :key="group.id"
              type="button"
              class="group-option"
              :class="{ 'group-option--selected': selectedGroupId === String(group.id) }"
              :aria-pressed="selectedGroupId === String(group.id)"
              :data-group-id="group.id"
              @click="selectedGroupId = String(group.id)"
            >
              <span class="group-option-heading">
                <span class="group-option-name">{{ group.name }}</span>
                <span class="group-check" aria-hidden="true">
                  <Icon v-if="selectedGroupId === String(group.id)" name="check" size="xs" :stroke-width="2.5" />
                </span>
              </span>
              <span class="group-option-details">
                <span>{{ t('modelSquare.modelCount', { count: groupModelCounts.get(group.id) ?? 0 }) }}</span>
                <span class="group-rate">×{{ groupRateLabel(group) }}</span>
              </span>
            </button>
          </div>
          <p v-else class="group-empty">{{ loading ? t('common.loading') : t('modelSquare.noGroups') }}</p>
        </div>

      </section>

      <div v-if="loading" class="model-card-grid">
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

      <section v-else class="model-card-grid">
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
import modelSquareMessages from '@/i18n/modelSquare'
import ModelPriceCard from '@/components/models/ModelPriceCard.vue'
import type { UserAvailableChannel } from '@/api/channels'
import modelSquareAPI from '@/api/modelSquare'
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
import { platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'

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
const selectedPlatform = ref('')
const selectedGroupId = ref('')
const showMultiplier = ref(false)

const models = computed(() => buildModelSquareModels(channels.value))
const availableGroups = computed(() => collectModelSquareGroups(channels.value, groups.value))
const visibleGroups = computed(() =>
  collectModelSquareGroups(channels.value, groups.value, selectedPlatform.value),
)
const selectedGroup = computed(() =>
  visibleGroups.value.find((group) => String(group.id) === selectedGroupId.value) ?? null,
)

// Platform choices are based on the catalog, never on the currently selected group.
const platformStats = computed(() => {
  const visibleGroupIds = new Set(availableGroups.value.map((group) => group.id))
  const counts = new Map<string, number>()
  for (const channel of channels.value) {
    for (const section of channel.platforms) {
      if (section.groups.some((group) => visibleGroupIds.has(group.id))) {
        counts.set(section.platform, 0)
      }
    }
  }
  for (const model of models.value) {
    if (counts.has(model.platform)) counts.set(model.platform, counts.get(model.platform)! + 1)
  }
  return [...counts.entries()]
    .map(([platform, count]) => ({ platform, count }))
    .sort((a, b) => a.platform.localeCompare(b.platform))
})

const platformModels = computed(() => models.value.filter(
  (model) => !selectedPlatform.value || model.platform === selectedPlatform.value,
))
const groupModelCounts = computed(() => {
  const counts = new Map<number, number>()
  for (const model of platformModels.value) {
    for (const groupId of model.groupIds) counts.set(groupId, (counts.get(groupId) ?? 0) + 1)
  }
  return counts
})
const filteredModels = computed(() => {
  const group = selectedGroup.value
  // An unselected/removed group must never fall back to another channel's price.
  if (!group) return []
  return platformModels.value.filter((model) => model.groupIds.includes(group.id))
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

const multiplierHint = computed(() => {
  if (!selectedGroup.value) return t('modelSquare.selectGroupHint')
  const notes = [showMultiplier.value
    ? t('modelSquare.multiplierEnabled', { rate: currentRateLabel.value })
    : t('modelSquare.originalPriceActive')]
  if (activeMultiplier.value?.source === 'user') notes.push(t('modelSquare.customRate'))
  if (activeMultiplier.value?.peakActive) {
    notes.push(t('modelSquare.peakActive', { rate: activeMultiplier.value.peakFactor }))
  }
  return notes.join(' · ')
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

watch(platformStats, (value) => {
  if (selectedPlatform.value && !value.some((stat) => stat.platform === selectedPlatform.value)) {
    selectedPlatform.value = ''
  }
})

watch(visibleGroups, (value) => {
  if (!value.some((group) => String(group.id) === selectedGroupId.value)) {
    const initialGroup = value.find((group) => (groupModelCounts.value.get(group.id) ?? 0) > 0) ?? value[0]
    selectedGroupId.value = initialGroup ? String(initialGroup.id) : ''
  }
}, { immediate: true })

function groupRateLabel(group: Group): number {
  return Number((userGroupRates.value[group.id] ?? group.rate_multiplier).toPrecision(8))
}

function variantFor(model: ModelSquareModel): ModelPricingVariant | null {
  return selectedGroup.value ? resolveModelVariant(model, selectedGroup.value.id) : null
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
      modelSquareAPI.getCatalog(),
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
.model-square {
  container-type: inline-size;
}

.model-card-grid {
  display: grid;
  /* Use the space left by the sidebar, including at browser zoom levels. */
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 336px), 1fr));
  gap: 12px;
}

.premium-filter-panel {
  --filter-text: #0f172a;
  --filter-muted: #64748b;
  --filter-line: #e9edf2;
  --filter-soft: #f8fafc;
  --filter-selected: #f0fdfa;
  --filter-accent: #0f766e;
  --filter-accent-line: #5eead4;
  overflow: hidden;
  border: 1px solid var(--filter-line);
  border-radius: 16px;
  background: #fff;
  color: var(--filter-text);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.02), 0 7px 20px rgba(15, 23, 42, 0.035);
}

.platform-section,
.group-section {
  padding: 12px 18px;
}

.platform-section {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
  border-bottom: 1px solid var(--filter-line);
}

.filter-section-heading,
.filter-section-label,
.platform-tab,
.group-option-heading,
.group-option-details,
.multiplier-control,
.multiplier-label,
.filter-refresh-button {
  display: flex;
  align-items: center;
}

.filter-section-heading {
  min-width: 0;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.filter-section-label {
  flex-shrink: 0;
  gap: 7px;
  font-size: 13px;
  font-weight: 600;
}

.filter-section-label > svg {
  color: var(--filter-muted);
}

.platform-tabs,
.group-options {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.platform-tab {
  min-height: 32px;
  min-width: 0;
  gap: 6px;
  padding: 5px 10px;
  border-width: 1px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 600;
  transition: background-color 0.2s, border-color 0.2s;
}

.platform-tab--all {
  border-color: var(--filter-line);
  background: var(--filter-soft);
  color: var(--filter-muted);
}

.platform-tab--all-active {
  @apply border-transparent bg-gradient-to-br from-primary-500 to-primary-600 text-white;
}

.platform-count {
  padding: 1px 5px;
  border-radius: 5px;
  background: rgba(127, 127, 127, 0.1);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.platform-tab[aria-pressed="true"] .platform-count {
  background: rgba(255, 255, 255, 0.18);
}

.filter-refresh-button {
  flex-shrink: 0;
  justify-content: center;
  gap: 6px;
  min-height: 30px;
  padding: 4px 9px;
  border: 1px solid var(--filter-line);
  border-radius: 8px;
  color: var(--filter-muted);
  font-size: 12px;
}

.filter-refresh-button:hover:not(:disabled) {
  background: var(--filter-soft);
  color: var(--filter-text);
}

.filter-refresh-button:disabled {
  opacity: 0.5;
  cursor: wait;
}

.group-heading-copy {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px 12px;
}

.filter-hint,
.group-empty {
  color: var(--filter-muted);
  font-size: 12px;
  line-height: 1.6;
}

.group-options {
  gap: 8px;
}

.group-option {
  flex: 0 1 168px;
  min-width: 0;
  max-width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--filter-line);
  border-radius: 12px;
  background: var(--filter-soft);
  text-align: left;
  transition: background-color 0.2s, border-color 0.2s, box-shadow 0.2s;
}

.group-option:hover {
  border-color: var(--filter-accent-line);
}

.group-option--selected {
  border-color: var(--filter-accent-line);
  background: var(--filter-selected);
  box-shadow: inset 0 0 0 1px var(--filter-accent-line);
}

.group-option-heading {
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.group-option-name {
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.group-check {
  display: inline-flex;
  width: 17px;
  height: 17px;
  margin-top: 1px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--filter-line);
  border-radius: 50%;
  background: #fff;
}

.group-option--selected .group-check {
  @apply border-transparent bg-gradient-to-br from-primary-500 to-primary-600 text-white;
}

.group-option-details {
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 4px 10px;
  margin-top: 4px;
  color: var(--filter-muted);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.group-rate {
  color: var(--filter-accent);
  font-weight: 600;
}

.group-empty {
  padding: 14px 0;
}

.multiplier-control {
  flex-shrink: 0;
  justify-content: space-between;
  gap: 10px;
  min-height: 32px;
  padding: 5px 8px;
  border: 1px solid var(--filter-line);
  border-radius: 10px;
  background: #fff;
  cursor: pointer;
  user-select: none;
}

.multiplier-label {
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
}

.multiplier-label > svg {
  color: var(--filter-accent);
}

.multiplier-control--disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.platform-tab:focus-visible,
.group-option:focus-visible,
.filter-refresh-button:focus-visible {
  outline: 2px solid var(--filter-accent);
  outline-offset: 3px;
}

:global(.dark .premium-filter-panel) {
  --filter-text: #f1f5f9;
  --filter-muted: #94a3b8;
  --filter-line: #334155;
  --filter-soft: #172235;
  --filter-selected: #143532;
  --filter-accent: #5eead4;
  --filter-accent-line: #2b7c72;
  background: #0f172a;
  box-shadow: none;
}

:global(.dark .premium-filter-panel .group-check),
:global(.dark .premium-filter-panel .multiplier-control) {
  background: #0f172a;
}

:global(.dark .premium-filter-panel .group-option--selected .group-check) {
  @apply border-transparent bg-gradient-to-br from-primary-500 to-primary-600;
}

@container (max-width: 680px) {
  .platform-section {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 10px;
  }

  .platform-tabs {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .filter-refresh-button {
    grid-column: 2;
    grid-row: 1;
  }

  .group-section-heading {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 6px 10px;
  }

  .group-heading-copy {
    display: contents;
  }

  .group-heading-copy .filter-section-label {
    grid-column: 1;
    grid-row: 1;
  }

  .group-heading-copy .filter-hint {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .multiplier-control {
    grid-column: 2;
    grid-row: 1;
  }
}

@container (max-width: 520px) {
  .platform-section,
  .group-section {
    padding: 12px 14px;
  }

  .group-options {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .multiplier-label > svg {
    display: none;
  }
}
</style>
