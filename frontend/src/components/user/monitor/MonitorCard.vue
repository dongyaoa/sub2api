<template>
  <button
    type="button"
    class="monitor-card group flex w-full min-w-0 flex-col rounded-[28px] border border-white/90 bg-white/75 text-left shadow-card backdrop-blur-xl transition-[border-color,box-shadow] duration-200 hover:border-emerald-200/80 hover:shadow-card-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/60 dark:border-dark-700/70 dark:bg-dark-800/65 dark:hover:border-emerald-500/30 motion-reduce:transition-none"
    @click="emit('click')"
  >
    <div class="monitor-card-header flex items-start gap-3">
      <span
        class="monitor-provider-icon grid h-[38px] w-[38px] flex-shrink-0 place-items-center rounded-xl ring-1 ring-black/5 dark:ring-white/10"
        :class="[providerGradient(item.provider), providerTintClass(item.provider)]"
        :title="providerLabel(item.provider)"
      >
        <ProviderIcon :provider="item.provider" :size="24" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="monitor-card-name truncate text-sm font-semibold leading-6 text-gray-900 dark:text-gray-100 sm:text-base" :title="item.name">
          {{ item.name }}
        </div>
        <div class="mt-1 flex min-w-0 items-center gap-1.5">
          <span
            class="monitor-model-badge min-w-0 truncate rounded-md px-1.5 py-0.5 text-[10.5px] leading-4 text-slate-500 dark:text-slate-300"
            :class="cardModelBadgeClass"
            :title="cardModelLabel"
          >
            {{ cardModelLabel }}
          </span>
          <span
            v-if="item.group_rate_multiplier != null"
            data-testid="monitor-group-rate"
            class="inline-flex flex-shrink-0 items-center whitespace-nowrap rounded bg-teal-50 px-1 text-[10px] font-medium leading-4 text-teal-700 dark:bg-teal-500/10 dark:text-teal-300"
            :title="groupRateTitle"
          >
            {{ formatMultiplier(item.group_rate_multiplier) }}x
          </span>
        </div>
      </div>
      <div data-testid="monitor-status-stack" class="flex flex-shrink-0 flex-col items-end gap-1 self-start">
        <span
          class="inline-flex items-center whitespace-nowrap rounded-md px-2 py-[3px] text-xs font-medium leading-[18px]"
          :class="statusBadgeClass(item.primary_status)"
        >
          {{ statusLabel(item.primary_status) }}
        </span>
        <span v-if="extraModelsCount" class="text-[10px] leading-4 text-gray-500 dark:text-gray-400">
          {{ t('channelStatus.cards.extraModels', { n: extraModelsCount }) }}
        </span>
      </div>
    </div>

    <div class="monitor-metrics mt-2 grid grid-cols-3 gap-2.5">
      <div class="monitor-metric">
        <div class="monitor-metric-label">
          <Icon name="bolt" aria-hidden="true" />
          <span>{{ t('channelStatus.cards.responseLatency') }}</span>
        </div>
        <div class="monitor-metric-value text-gray-900 dark:text-gray-100">
          {{ formatLatency(item.primary_latency_ms) }}<span class="monitor-metric-unit text-gray-400">ms</span>
        </div>
      </div>
      <div class="monitor-metric">
        <div class="monitor-metric-label">
          <Icon name="globe" aria-hidden="true" />
          <span>{{ t('channelStatus.cards.endpointLatency') }}</span>
        </div>
        <div class="monitor-metric-value text-gray-900 dark:text-gray-100">
          {{ formatLatency(item.primary_ping_latency_ms) }}<span class="monitor-metric-unit text-gray-400">ms</span>
        </div>
      </div>
      <div class="monitor-metric">
        <div class="monitor-metric-label">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
            <path d="M3 12h4l3-8 4 16 3-8h4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span>{{ t('channelStatus.cards.availability', { days: window.slice(0, -1) }) }}</span>
        </div>
        <div class="monitor-metric-value" :style="availabilityColorStyle">
          {{ availabilityDisplay }}<span class="monitor-metric-unit">%</span>
        </div>
      </div>
    </div>

    <!-- 配额开关仍由服务端剥离数据和前端可见性共同保护。 -->
    <MonitorQuotaView v-if="quotaVisible" :snapshot="item.latest_quota" />

    <MonitorTimeline
      class="mt-auto pt-1"
      :buckets="item.timeline"
      :last-checked-at="lastCheckedAt"
      :stale="freshness === 'stale'"
      :now="now"
    />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import { hslForPct, providerGradient, useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { formatMultiplier } from '@/utils/formatters'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import { getMonitorFreshness, getMonitorLastCheckedAt, providerTintClass } from '@/utils/channelMonitorDisplay'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from './ProviderIcon.vue'
import MonitorTimeline from './MonitorTimeline.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'

const props = withDefaults(defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  /** Kept for callers upgrading from the previous card; refresh is now page-wide. */
  countdownSeconds?: number
  now?: number
}>(), {
  now: () => Date.now(),
})

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
const { statusLabel, statusBadgeClass, providerLabel, formatLatency, formatMonitorModel } = useChannelMonitorFormat()

// These aliases belong only to the card; keep the model ID intact for probes and details.
const cardModelLabel = computed(() => {
  switch (props.item.primary_model) {
    case 'gpt-5.6-sol': return 'GPT-5.6 Sol'
    case 'gpt-6-astra': return 'GPT-6 Astra'
    case 'grok-4.5': return 'Grok 4.5'
    case 'claude-opus-4-6': return 'Claude Opus 4.6'
    case 'gemini-3.7-flash-high': return 'Gemini 3.7 Flash (high)'
    case 'claude-opus-4-7': return 'Claude Opus 4.7'
    case 'claude-sonnet-5': return 'Claude Sonnet 5'
    default: return formatMonitorModel(props.item.primary_model)
  }
})

const cardModelBadgeClass = computed(() => {
  switch (props.item.provider) {
    case 'anthropic': return 'bg-[#fff8f1] dark:bg-orange-950/25'
    case 'grok': return 'bg-[#f8f9fa] dark:bg-slate-700/25'
    case 'gemini': return 'bg-[#f5faff] dark:bg-sky-950/30'
    default: return 'bg-[#f4fcfa] dark:bg-teal-950/25'
  }
})

const quotaVisible = computed(() => isChannelMonitorQuotaVisible() && !!props.item.latest_quota)
const lastCheckedAt = computed(() => getMonitorLastCheckedAt(props.item))
const freshness = computed(() => getMonitorFreshness(props.item, props.now))
const extraModelsCount = computed(() => props.item.extra_models?.length ?? 0)
const availabilityDisplay = computed(() => {
  if (props.availabilityValue == null || Number.isNaN(props.availabilityValue)) return t('monitorCommon.latencyEmpty')
  return props.availabilityValue.toFixed(2)
})
const availabilityColorStyle = computed(() => ({
  color: hslForPct(props.availabilityValue) ?? 'rgb(156 163 175)',
}))

const groupRateTitle = computed(() => {
  if (props.item.group_rate_multiplier == null) return ''
  const value = formatMultiplier(props.item.group_rate_multiplier)
  if (!props.item.group_name) return t('channelStatus.groupRateValue', { value })
  return t('channelStatus.groupRateTitle', { group: props.item.group_name, value })
})
</script>

<style scoped>
.monitor-card {
  container-type: inline-size;
  aspect-ratio: 1.46;
  min-height: 248px;
  padding: clamp(14px, 1.25cqw, 24px);
  gap: 14px;
}

.monitor-metric {
  @apply min-w-0 rounded-2xl border border-gray-100 bg-gray-50/80 dark:border-dark-700/60 dark:bg-dark-900/35;
  min-height: clamp(64px, 20cqw, 88px);
  padding: clamp(8px, 3cqw, 14px);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.12);
}

.monitor-card-name {
  line-height: 1.2;
}

.monitor-metric-label {
  @apply mb-[5px] flex items-center gap-1 whitespace-nowrap text-[9px] font-normal leading-[14px] text-gray-400;
}

.monitor-metric-label svg {
  width: 10px;
  height: 10px;
  flex-shrink: 0;
}

.monitor-metric-value {
  font-size: 18px;
  line-height: 1.25;
  letter-spacing: -0.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.monitor-metric-unit {
  margin-left: 2px;
  font-size: 10px;
  font-weight: 400;
  letter-spacing: 0;
}

@container (max-width: 300px) {
  .monitor-card-header {
    gap: 8px;
  }

  .monitor-metrics {
    gap: 6px;
  }

  .monitor-metric {
    padding-inline: 4px;
  }

  .monitor-metric-label {
    gap: 2px;
  }

  .monitor-metric-label svg {
    width: 8px;
    height: 8px;
  }

  .monitor-metric-value {
    font-size: 15px;
  }
}
</style>
