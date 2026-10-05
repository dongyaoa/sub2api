<template>
  <div class="space-y-5">
    <form class="flex flex-wrap items-end gap-3" @submit.prevent="load()">
      <div v-if="supplier && !target" class="min-w-0 flex-1 basis-40">
        <label for="finance-target" class="input-label">{{ t('upstreamCenter.finance.group') }}</label>
        <Select id="finance-target" v-model="targetId" :options="targetOptions" :searchable="false" :aria-label="t('upstreamCenter.finance.group')" @update:model-value="load(true)" />
      </div>
      <button type="submit" class="btn btn-primary" :disabled="loading">{{ t('upstreamCenter.finance.refresh') }}</button>
    </form>
    <p v-if="supplier?.recharge_ratio != null" class="text-xs text-gray-500 dark:text-dark-400" data-testid="supplier-recharge-ratio">{{ t('upstreamCenter.recharge.applied', { ratio: supplier.recharge_ratio }) }}</p>
    <p v-if="error" role="alert" class="rounded-xl bg-rose-50 p-3 text-sm text-rose-600 dark:bg-rose-500/10 dark:text-rose-400">{{ error }}</p>
    <template v-if="data">
      <section v-for="period in periods" :key="period.key" :data-period="period.key" class="space-y-3" :aria-busy="loading">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t(`upstreamCenter.finance.${period.key}`) }}</h3>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div v-for="metric in metrics(period.summary)" :key="metric.key" class="rounded-xl border border-gray-100 bg-gray-50/70 p-3.5 dark:border-dark-700 dark:bg-dark-900/40">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t(`upstreamCenter.finance.${metric.key}`) }}</p>
            <p class="mt-2 text-lg font-semibold tabular-nums" :class="metric.key === 'profit' ? metric.value == null ? 'text-amber-600 dark:text-amber-400' : metric.value < 0 ? 'text-rose-600 dark:text-rose-400' : 'text-primary-700 dark:text-primary-300' : 'text-gray-900 dark:text-gray-100'">{{ metric.value == null && metric.key === 'profit' ? t('upstreamCenter.finance.pending') : money(metric.value, period.summary.currency) }}</p>
          </div>
        </div>
        <div class="flex flex-wrap justify-between gap-2 text-[11px] text-gray-500 dark:text-dark-400">
          <span>{{ t('upstreamCenter.finance.accountingDate', { from: dateTime(period.summary.from), to: dateTime(period.summary.to) }) }}</span>
          <span><template v-if="period.summary.remote_synced_at">{{ t('upstreamCenter.wallet.syncedAt', { time: dateTime(period.summary.remote_synced_at) }) }} · </template>{{ t('upstreamCenter.finance.currency', { currency: period.summary.currency }) }}</span>
        </div>
        <p v-if="period.summary.remote_stale" class="text-xs text-amber-600 dark:text-amber-400">{{ t('upstreamCenter.finance.stale') }}</p>
        <UpstreamFinanceNotice :summary="period.summary" />
        <p v-if="actualUpstreamUsed(period.summary) == null" class="text-xs text-gray-500 dark:text-dark-400">{{ t('upstreamCenter.financeUnavailable') }}</p>
      </section>
      <div class="rounded-xl bg-primary-50/60 p-3.5 text-xs leading-5 text-primary-800 dark:bg-primary-500/10 dark:text-primary-200">
        <p>{{ t('upstreamCenter.finance.note') }}</p>
        <p class="mt-1 opacity-80">{{ t('upstreamCenter.finance.bindingHint') }}</p>
      </div>
    </template>
    <div v-else-if="loading" class="flex h-40 items-center justify-center"><Icon name="refresh" class="animate-spin text-primary-500" /></div>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { upstreamCenterAPI, type UpstreamFinancePeriods, type UpstreamFinanceSummary, type UpstreamSupplier, type UpstreamTarget } from '@/api/admin/upstreamCenter'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import UpstreamFinanceNotice from './UpstreamFinanceNotice.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { actualProfit, actualUpstreamUsed, dateTime, money } from './format'

const props = defineProps<{ supplier: UpstreamSupplier | null; target: UpstreamTarget | null }>()
const { t } = useI18n()
const data = ref<UpstreamFinancePeriods | null>(null), error = ref(''), loading = ref(false)
const targetId = ref('')
const targetOptions = computed(() => [
  { value: '', label: t('upstreamCenter.finance.allGroups') },
  ...(props.supplier?.targets || []).map(target => ({ value: String(target.id), label: target.name }))
])
const periods = computed(() => data.value ? [
  { key: 'today', summary: data.value.today },
  { key: 'last30Days', summary: data.value.last_30_days },
] : [])
function metrics(summary: UpstreamFinanceSummary) {
  return [
    { key: 'remoteUsedRange', value: actualUpstreamUsed(summary) },
    { key: 'revenue', value: summary.revenue },
    { key: 'profit', value: actualProfit(summary) },
  ]
}
let controller: AbortController | undefined
async function load(clear = false) {
  controller?.abort()
  const current = new AbortController()
  controller = current
  if (clear) data.value = null
  loading.value = true
  error.value = ''
  try {
    const result = await upstreamCenterAPI.financeSummary({ supplier_id: props.supplier?.id, target_id: props.target?.id || (targetId.value ? Number(targetId.value) : undefined) }, current.signal)
    if (!current.signal.aborted) data.value = result
  } catch (err) {
    if (!current.signal.aborted) error.value = extractApiErrorMessage(err, t('upstreamCenter.loadFailed'))
  } finally {
    if (!current.signal.aborted) loading.value = false
  }
}
watch([() => props.supplier?.id, () => props.target?.id], () => { targetId.value = ''; void load(true) }, { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>
