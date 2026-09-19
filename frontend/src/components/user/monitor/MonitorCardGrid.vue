<template>
  <div class="monitor-layout space-y-6">
    <div v-if="loading && items.length === 0" class="monitor-grid" aria-busy="true">
      <div v-for="i in 6" :key="i" class="monitor-skeleton flex flex-col animate-pulse rounded-[28px] border border-white/80 bg-white/70 p-6 dark:border-dark-700 dark:bg-dark-800/60">
        <div class="flex gap-3"><div class="h-9 w-9 rounded-xl bg-gray-200 dark:bg-dark-700" /><div class="h-4 w-1/2 rounded bg-gray-200 dark:bg-dark-700" /></div>
        <div class="mt-7 grid grid-cols-3 gap-2.5"><div v-for="n in 3" :key="n" class="h-[88px] rounded-2xl bg-gray-100 dark:bg-dark-900/40" /></div>
        <div class="mt-auto h-[21px] rounded bg-gray-100 dark:bg-dark-900/40" />
      </div>
    </div>
    <EmptyState v-else-if="items.length === 0" :title="t('channelStatus.empty.title')" :description="t('channelStatus.empty.description')" />
    <template v-else>
      <section v-for="group in groups" :key="group.id" :aria-labelledby="`monitor-group-${group.id}`">
        <div class="mb-3 flex flex-wrap items-center gap-x-3 gap-y-2">
          <button :id="`monitor-group-${group.id}`" type="button" class="inline-flex shrink-0 items-center gap-3 rounded-lg text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :aria-expanded="!collapsed.has(group.id)" :aria-controls="`monitor-grid-${group.id}`" @click="toggleGroup(group.id)">
            <span class="grid h-8 w-8 shrink-0 place-items-center rounded-xl ring-1 ring-black/5 dark:ring-white/10" :class="groupIconClass(group.id)">
              <Icon v-if="group.id === 'other'" name="grid" size="sm" />
              <ProviderIcon v-else :provider="group.id" :size="19" />
            </span>
            <span class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t(`channelStatus.groups.${group.id}`) }}</span>
            <span class="rounded-md bg-white/60 px-1.5 py-0.5 text-[11px] tabular-nums text-gray-500 dark:bg-dark-800 dark:text-gray-400">{{ group.items.length }}</span>
            <Icon :name="collapsed.has(group.id) ? 'chevronRight' : 'chevronDown'" size="xs" class="text-gray-400" />
          </button>
          <span class="ml-auto flex flex-wrap items-center justify-end gap-x-2 text-[11px] text-gray-500 dark:text-gray-400">
            <span>{{ t('channelStatus.groups.healthSummary', { normal: group.normal, abnormal: group.abnormal }) }}</span>
            <span v-if="group.stale" class="text-amber-600 dark:text-amber-400">{{ t('channelStatus.groups.staleCount', { count: group.stale }) }}</span>
            <span v-if="group.unknown">{{ t('channelStatus.groups.unknownCount', { count: group.unknown }) }}</span>
          </span>
        </div>
        <div v-show="!collapsed.has(group.id)" :id="`monitor-grid-${group.id}`" class="monitor-grid">
          <MonitorCard v-for="item in group.items" :key="item.id" :item="item" :window="window" :availability-value="resolveAvailability(item)" :countdown-seconds="countdownSeconds" :now="now" @click="emit('cardClick', item)" />
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorDisplayGroup, MonitorDisplayOrder, UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { providerGradient } from '@/composables/useChannelMonitorFormat'
import { getMonitorFreshness, groupMonitorItems, providerTintClass } from '@/utils/channelMonitorDisplay'
import MonitorCard from './MonitorCard.vue'
import ProviderIcon from './ProviderIcon.vue'

const props = withDefaults(defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
  displayOrder?: MonitorDisplayOrder
  now?: number
}>(), { now: () => Date.now() })
const emit = defineEmits<{ (e: 'cardClick', item: UserMonitorView): void }>()
const { t } = useI18n()
const collapsed = ref(new Set<MonitorDisplayGroup>())
const groups = computed(() => groupMonitorItems(props.items, props.displayOrder).map(group => {
  let normal = 0, abnormal = 0, stale = 0, unknown = 0
  for (const item of group.items) {
    const freshness = getMonitorFreshness(item, props.now)
    if (freshness === 'stale') stale++
    else if (freshness === 'unknown') unknown++
    else if (item.primary_status === 'operational') normal++
    else abnormal++
  }
  return { ...group, normal, abnormal, stale, unknown }
}))
function toggleGroup(id: MonitorDisplayGroup) {
  const next = new Set(collapsed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsed.value = next
}
function groupIconClass(id: MonitorDisplayGroup) {
  return id === 'other'
    ? 'bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300'
    : `${providerGradient(id)} ${providerTintClass(id)}`
}
function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') return item.availability_7d ?? null
  const primary = props.detailCache[item.id]?.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>

<style scoped>
.monitor-layout {
  container: monitor-layout / inline-size;
}

.monitor-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: clamp(14px, 1.2cqw, 24px);
  align-items: start;
}

.monitor-skeleton {
  aspect-ratio: 1.46;
  min-height: 248px;
  padding: clamp(14px, 1.25cqw, 24px);
}

@container monitor-layout (min-width: 560px) {
  .monitor-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@container monitor-layout (min-width: 840px) {
  .monitor-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@container monitor-layout (min-width: 1080px) {
  .monitor-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
</style>
