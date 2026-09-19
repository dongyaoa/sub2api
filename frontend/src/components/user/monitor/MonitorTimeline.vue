<template>
  <div class="w-full min-w-0">
    <div class="mb-[7px] flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-[10px] leading-[15px] text-gray-400">
      <span>{{ t('channelStatus.cards.timelineLabel', { n: length }) }}</span>
      <span class="flex items-center gap-1.5 tabular-nums" :title="lastCheckedTitle">
        <span>{{ latestCheckLabel }}</span>
        <span v-if="stale" class="rounded bg-amber-50 px-1 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300">
          {{ t('channelStatus.cards.stale') }}
        </span>
      </span>
    </div>

    <div
      v-if="maintenance"
      class="flex h-[21px] w-full items-center justify-center rounded border border-dashed border-gray-300 text-[10px] text-gray-400 dark:border-dark-600"
    >
      {{ t('monitorCommon.maintenancePaused') }}
    </div>
    <div
      v-else
      class="grid h-[21px] w-full items-end gap-[2px]"
      :style="{ gridTemplateColumns: `repeat(${length}, minmax(0, 1fr))` }"
      role="img"
      :aria-label="t('channelStatus.cards.timelineAria', { n: realPoints.length })"
    >
      <span
        v-for="(bar, idx) in displayBars"
        :key="idx"
        data-testid="monitor-timeline-bar"
        class="min-w-0 rounded-full"
        :class="bar.colorClass"
        :style="{ height: bar.heightPct + '%' }"
        :title="bar.title"
        :data-checked-at="bar.checkedAt"
      ></span>
    </div>

    <div class="mt-[5px] flex justify-between text-[9px] leading-3 tracking-[0.7px] text-gray-400" aria-hidden="true">
      <span>PAST</span>
      <span>NOW</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const props = withDefaults(defineProps<{
  buckets?: MonitorTimelinePoint[]
  countdownSeconds?: number
  length?: number
  maintenance?: boolean
  lastCheckedAt?: string | null
  stale?: boolean
  now?: number
}>(), {
  buckets: () => [],
  length: 45,
  maintenance: false,
  stale: false,
  now: () => Date.now(),
})

const { t, locale } = useI18n()
const { statusLabel, formatLatency } = useChannelMonitorFormat()

interface Bar {
  colorClass: string
  heightPct: number
  title: string
  checkedAt?: string
}

// Height supplements the colour so degraded/failed checks remain distinguishable.
const STATUS_HEIGHT: Record<string, number> = {
  operational: 100,
  degraded: 65,
  failed: 35,
  error: 35,
  empty: 15,
}

const STATUS_COLOR: Record<string, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  failed: 'bg-red-500',
  error: 'bg-red-500',
  empty: 'bg-gray-300 dark:bg-dark-600',
}

// The API returns real checks newest-first. Keep only the latest 45, then put
// the newest on the right; padding belongs on the older (left) side.
const realPoints = computed(() => props.buckets.slice(0, props.length).reverse())
const effectiveLastCheckedAt = computed(() => props.lastCheckedAt ?? props.buckets[0]?.checked_at ?? null)

function absoluteTime(value: string | null | undefined): string {
  if (!value || Number.isNaN(Date.parse(value))) return t('channelStatus.cards.timeUnknown')
  return new Date(value).toLocaleString(locale.value, { hour12: false })
}

const lastCheckedTitle = computed(() => effectiveLastCheckedAt.value ? absoluteTime(effectiveLastCheckedAt.value) : undefined)
const latestCheckLabel = computed(() => {
  const checkedAt = effectiveLastCheckedAt.value
  if (!checkedAt) return t('channelStatus.cards.noChecks')
  const timestamp = Date.parse(checkedAt)
  if (Number.isNaN(timestamp)) return t('channelStatus.cards.timeUnknown')
  const seconds = Math.max(0, Math.floor((props.now - timestamp) / 1000))
  const time = seconds < 60
    ? t('monitorCommon.relativeSecondsAgo', { n: seconds })
    : seconds < 3600
      ? t('monitorCommon.relativeMinutesAgo', { n: Math.floor(seconds / 60) })
      : seconds < 86400
        ? t('monitorCommon.relativeHoursAgo', { n: Math.floor(seconds / 3600) })
        : t('monitorCommon.relativeDaysAgo', { n: Math.floor(seconds / 86400) })
  return t('channelStatus.cards.latestCheck', { time })
})

const displayBars = computed<Bar[]>(() => {
  const padCount = Math.max(0, props.length - realPoints.value.length)
  const bars: Bar[] = Array.from({ length: padCount }, () => ({
    colorClass: STATUS_COLOR.empty,
    heightPct: STATUS_HEIGHT.empty,
    title: t('channelStatus.cards.noChecks'),
  }))

  for (const point of realPoints.value) {
    bars.push({
      colorClass: STATUS_COLOR[point.status] ?? STATUS_COLOR.empty,
      heightPct: STATUS_HEIGHT[point.status] ?? STATUS_HEIGHT.empty,
      title: `${absoluteTime(point.checked_at)} · ${statusLabel(point.status)} · ${formatLatency(point.latency_ms)}ms`,
      checkedAt: point.checked_at,
    })
  }
  return bars
})
</script>
