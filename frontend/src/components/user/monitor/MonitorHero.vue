<template>
  <section class="mb-6 rounded-2xl border border-white/80 bg-white/65 p-4 shadow-sm backdrop-blur-xl dark:border-dark-700/70 dark:bg-dark-800/60">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-3">
        <h1 class="text-lg font-semibold tracking-tight text-gray-900 dark:text-gray-100">{{ t('channelStatus.title') }}</h1>
        <span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium" :class="overallChipClass">
          <span class="h-1.5 w-1.5 rounded-full bg-current" />{{ t(`channelStatus.health.${overallStatus}`) }}
        </span>
      </div>
      <div class="text-[11px] tabular-nums text-gray-500 dark:text-gray-400" aria-live="polite">
        <template v-if="lastUpdated">{{ t('channelStatus.toolbar.updatedAt') }}<time :datetime="lastUpdated" class="ml-1 text-gray-700 dark:text-gray-300">{{ formattedUpdatedAt }}</time></template>
        <span v-else>{{ t('channelStatus.toolbar.notUpdated') }}</span>
      </div>
    </div>
    <div class="mt-3 flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2.5">
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('channelStatus.toolbar.statisticsPeriod') }}</span>
        <div class="relative isolate grid grid-cols-3 rounded-xl bg-gray-100/80 p-1 dark:bg-dark-900/50" role="group" :aria-label="t('channelStatus.toolbar.statisticsPeriod')">
          <span aria-hidden="true" class="absolute bottom-1 left-1 top-1 -z-10 rounded-lg bg-white shadow-sm transition-transform duration-200 motion-reduce:transition-none dark:bg-dark-700" :style="{ width: 'calc((100% - 8px) / 3)', transform: `translateX(${windowOptions.findIndex(opt => opt.value === window) * 100}%)` }" />
          <button v-for="opt in windowOptions" :key="opt.value" type="button" :aria-pressed="window === opt.value" class="min-w-12 rounded-lg px-3 py-1.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :class="window === opt.value ? 'font-medium text-primary-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'" @click="emit('update:window', opt.value)">{{ opt.label }}</button>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button type="button" class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-gray-200/80 bg-white/60 px-2.5 text-xs text-gray-600 transition-colors hover:bg-white disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700" :disabled="loading || refreshing" @click="emit('refresh')">
          <Icon name="refresh" size="sm" :class="loading || refreshing ? 'animate-spin' : ''" />{{ t('common.refresh') }}
        </button>
        <AutoRefreshButton v-if="autoRefresh" :enabled="autoRefresh.enabled.value" :interval-seconds="autoRefresh.intervalSeconds.value" :countdown="autoRefresh.countdown.value" :intervals="autoRefresh.intervals" @update:enabled="autoRefresh.setEnabled" @update:interval="autoRefresh.setInterval" />
      </div>
    </div>
    <p v-if="refreshError" role="status" class="mt-3 text-xs text-amber-700 dark:text-amber-300">{{ t('channelStatus.toolbar.refreshFailed') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
export type MonitorWindow = '7d' | '15d' | '30d'
export type OverallStatus = 'operational' | 'degraded' | 'unknown'
const props = defineProps<{
  overallStatus: OverallStatus
  intervalSeconds: number
  window: MonitorWindow
  loading: boolean
  refreshing?: boolean
  lastUpdated?: string | null
  refreshError?: boolean
  autoRefresh?: {
    enabled: { value: boolean }
    intervalSeconds: { value: number }
    countdown: { value: number }
    intervals: readonly number[]
    setEnabled: (v: boolean) => void
    setInterval: (v: number) => void
  }
}>()
const emit = defineEmits<{
  (e: 'update:window', value: MonitorWindow): void
  (e: 'refresh'): void
}>()
const { t, locale } = useI18n()
const windowOptions = computed(() => (['7d', '15d', '30d'] as const).map(value => ({ value, label: t(`channelStatus.windowTab.${value}`) })))
const formattedUpdatedAt = computed(() => {
  if (!props.lastUpdated) return ''
  return new Date(props.lastUpdated).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
})
const overallChipClass = computed(() => {
  if (props.overallStatus === 'operational') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
  if (props.overallStatus === 'degraded') return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
})
</script>
