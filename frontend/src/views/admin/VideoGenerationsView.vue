<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-gray-200 bg-gray-200 sm:grid-cols-3 xl:grid-cols-6 dark:border-dark-700 dark:bg-dark-700">
          <div v-for="metric in summaryMetrics" :key="metric.key" class="min-h-20 bg-white px-4 py-3 dark:bg-dark-900">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white" :class="metric.alert ? 'text-red-600 dark:text-red-400' : ''">
              {{ metric.value }}
            </p>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4 2xl:grid-cols-7">
            <label class="xl:col-span-2">
              <span class="input-label">{{ t('admin.videoGenerations.search') }}</span>
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="filters.search" class="input pl-9" :placeholder="t('admin.videoGenerations.searchPlaceholder')" @keyup.enter="search" />
              </div>
            </label>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.delivery') }}</span>
              <Select v-model="filters.delivery_status" :options="deliveryOptions" @change="search" />
            </label>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.billing') }}</span>
              <Select v-model="filters.billing_status" :options="billingOptions" @change="search" />
            </label>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.model') }}</span>
              <input v-model.trim="filters.model" class="input" placeholder="grok-imagine-video" @keyup.enter="search" />
            </label>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.account') }}</span>
              <input v-model="filters.account_id" type="number" min="1" class="input" @keyup.enter="search" />
            </label>
            <div class="flex items-end gap-2">
              <button type="button" class="btn btn-primary flex-1" :disabled="loading" @click="search">
                <Icon name="search" size="sm" class="mr-1.5" />
                {{ t('common.search') }}
              </button>
              <button type="button" class="btn btn-secondary" :disabled="loading" :title="t('admin.videoGenerations.refresh')" @click="load">
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              </button>
            </div>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.startTime') }}</span>
              <input v-model="filters.start_time" type="datetime-local" class="input" />
            </label>
            <label>
              <span class="input-label">{{ t('admin.videoGenerations.endTime') }}</span>
              <input v-model="filters.end_time" type="datetime-local" class="input" />
            </label>
            <div class="flex items-end gap-2 md:col-span-2 xl:col-span-2">
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">{{ t('common.reset') }}</button>
              <button type="button" class="btn btn-danger" :disabled="loading" @click="showAnomalies">
                <Icon name="exclamationCircle" size="sm" class="mr-1.5" />
                {{ t('admin.videoGenerations.anomalies') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="request_id" :sticky-first-column="false">
          <template #cell-created_at="{ value }">
            <span class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>
          <template #cell-user="{ row }">
            <div class="max-w-52">
              <p class="truncate font-medium text-gray-900 dark:text-white" :title="row.user_email">{{ row.user_email || `#${row.user_id}` }}</p>
              <p class="mt-0.5 truncate text-xs text-gray-400">#{{ row.user_id }} · {{ row.api_key_name || `Key #${row.api_key_id}` }}</p>
            </div>
          </template>
          <template #cell-request="{ row }">
            <div class="max-w-72">
              <p class="truncate font-mono text-xs text-gray-800 dark:text-gray-200" :title="row.request_id">{{ row.request_id }}</p>
              <p class="mt-1 truncate text-xs text-gray-400" :title="row.model">{{ row.model }} · {{ row.duration_seconds }}s</p>
            </div>
          </template>
          <template #cell-account="{ row }">
            <div class="max-w-44">
              <p class="truncate text-sm text-gray-800 dark:text-gray-200">{{ row.account_name || `#${row.account_id}` }}</p>
              <p class="mt-0.5 text-xs text-gray-400">#{{ row.account_id }}</p>
            </div>
          </template>
          <template #cell-delivery_status="{ value }">
            <span :class="deliveryBadgeClass(value)">{{ deliveryLabel(value) }}</span>
          </template>
          <template #cell-billing_status="{ value }">
            <span :class="billingBadgeClass(value)">{{ billingLabel(value) }}</span>
          </template>
          <template #cell-actual_cost="{ value }">
            <span class="font-mono text-sm tabular-nums text-gray-800 dark:text-gray-200">{{ formatCost(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button type="button" class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="openDetail(row)">
              <Icon name="eye" size="sm" />
              {{ t('admin.videoGenerations.columns.detail') }}
            </button>
          </template>
          <template #empty>
            <div class="flex flex-col items-center py-10 text-gray-400">
              <Icon name="play" size="xl" class="mb-3" />
              <p class="text-sm font-medium">{{ t('admin.videoGenerations.empty') }}</p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" @update:page="changePage" @update:pageSize="changePageSize" />
      </template>
    </TablePageLayout>

    <BaseDialog :show="Boolean(detail)" :title="t('admin.videoGenerations.detail.title')" width="extra-wide" :close-on-click-outside="true" @close="detail = null">
      <div v-if="detail" class="space-y-6 py-1 text-sm">
        <div class="flex flex-wrap items-center gap-2 border-b border-gray-200 pb-4 dark:border-dark-700">
          <span :class="deliveryBadgeClass(detail.delivery_status)">{{ deliveryLabel(detail.delivery_status) }}</span>
          <span :class="billingBadgeClass(detail.billing_status)">{{ billingLabel(detail.billing_status) }}</span>
          <span class="break-all font-mono text-xs text-gray-500">{{ detail.request_id }}</span>
        </div>

        <DetailSection :title="t('admin.videoGenerations.detail.identity')" :rows="identityRows" />
        <DetailSection :title="t('admin.videoGenerations.detail.generation')" :rows="generationRows" />

        <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoGenerations.detail.delivery') }}</h4>
          <DetailSection :rows="deliveryRows" />
          <a v-if="detail.video_url" :href="detail.video_url" target="_blank" rel="noopener noreferrer" class="mt-3 inline-flex items-center gap-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400">
            <Icon name="play" size="sm" />
            {{ t('admin.videoGenerations.detail.openVideo') }}
          </a>
        </section>

        <DetailSection :title="t('admin.videoGenerations.detail.billing')" :rows="billingRows" />

        <section v-if="errorRows.length" class="border-t border-gray-200 pt-5 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoGenerations.detail.errors') }}</h4>
          <dl class="space-y-3">
            <div v-for="row in errorRows" :key="row.label">
              <dt class="text-xs font-medium text-gray-500">{{ row.label }}</dt>
              <dd class="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-red-50 p-3 font-mono text-xs leading-5 text-red-700 dark:bg-red-950/20 dark:text-red-300">{{ row.value }}</dd>
            </div>
          </dl>
        </section>

        <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoGenerations.detail.timeline') }}</h4>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
            <div v-for="event in timeline" :key="event.label" class="rounded-md border border-gray-200 px-3 py-2 dark:border-dark-700">
              <p class="text-xs text-gray-500">{{ event.label }}</p>
              <p class="mt-1 text-xs font-medium text-gray-800 dark:text-gray-200">{{ event.value }}</p>
            </div>
          </div>
        </section>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listVideoGenerations } from '@/api/admin/videoGenerations'
import type { VideoGenerationLedgerItem, VideoGenerationLedgerQuery, VideoGenerationLedgerSummary } from '@/api/admin/videoGenerations'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import DetailSection from '@/components/admin/video-generations/DetailSection.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { useAppStore } from '@/stores/app'

const { t, locale } = useI18n()
const appStore = useAppStore()
const items = ref<VideoGenerationLedgerItem[]>([])
const summary = ref<VideoGenerationLedgerSummary>({ total: 0, processing: 0, delivered: 0, failed: 0, charged_without_output: 0, total_charged: 0 })
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const detail = ref<VideoGenerationLedgerItem | null>(null)
const filters = ref({ search: '', delivery_status: '', billing_status: '', model: '', account_id: '', start_time: '', end_time: '' })
let controller: AbortController | null = null

const columns = computed(() => [
  { key: 'created_at', label: t('admin.videoGenerations.columns.createdAt') },
  { key: 'user', label: t('admin.videoGenerations.columns.user') },
  { key: 'request', label: t('admin.videoGenerations.columns.request') },
  { key: 'account', label: t('admin.videoGenerations.columns.account') },
  { key: 'delivery_status', label: t('admin.videoGenerations.columns.delivery') },
  { key: 'billing_status', label: t('admin.videoGenerations.columns.billing') },
  { key: 'actual_cost', label: t('admin.videoGenerations.columns.cost') },
  { key: 'actions', label: t('admin.videoGenerations.columns.detail') },
])
const deliveryOptions = computed(() => [
  { value: '', label: t('admin.videoGenerations.all') },
  ...['processing', 'delivered', 'awaiting_storage', 'failed', 'charged_without_output'].map(value => ({ value, label: deliveryLabel(value) })),
])
const billingOptions = computed(() => [
  { value: '', label: t('admin.videoGenerations.all') },
  ...['pending', 'charged', 'not_charged', 'billing_failed'].map(value => ({ value, label: billingLabel(value) })),
])
const summaryMetrics = computed(() => [
  { key: 'total', label: t('admin.videoGenerations.summary.total'), value: summary.value.total },
  { key: 'processing', label: t('admin.videoGenerations.summary.processing'), value: summary.value.processing },
  { key: 'delivered', label: t('admin.videoGenerations.summary.delivered'), value: summary.value.delivered },
  { key: 'failed', label: t('admin.videoGenerations.summary.failed'), value: summary.value.failed },
  { key: 'charged_without_output', label: t('admin.videoGenerations.summary.chargedWithoutOutput'), value: summary.value.charged_without_output, alert: summary.value.charged_without_output > 0 },
  { key: 'total_charged', label: t('admin.videoGenerations.summary.totalCharged'), value: formatCost(summary.value.total_charged) },
])

function toISO(value: string): string | undefined {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString()
}
function query(): VideoGenerationLedgerQuery {
  return {
    page: page.value,
    page_size: pageSize.value,
    search: filters.value.search || undefined,
    delivery_status: filters.value.delivery_status || undefined,
    billing_status: filters.value.billing_status || undefined,
    model: filters.value.model || undefined,
    account_id: Number(filters.value.account_id) > 0 ? Number(filters.value.account_id) : undefined,
    start_time: toISO(filters.value.start_time),
    end_time: toISO(filters.value.end_time),
  }
}
async function load() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  try {
    const result = await listVideoGenerations(query(), { signal: controller.signal })
    items.value = result.items || []
    total.value = result.total || 0
    summary.value = result.summary || summary.value
  } catch (error: any) {
    if (error?.code !== 'ERR_CANCELED') appStore.showError(error?.message || 'Failed to load video generation ledger')
  } finally {
    loading.value = false
  }
}
function search() { page.value = 1; void load() }
function resetFilters() { filters.value = { search: '', delivery_status: '', billing_status: '', model: '', account_id: '', start_time: '', end_time: '' }; search() }
function showAnomalies() { filters.value.delivery_status = 'charged_without_output'; filters.value.billing_status = 'charged'; search() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function openDetail(row: VideoGenerationLedgerItem) { detail.value = row }
function formatTime(value?: string) { return value ? new Intl.DateTimeFormat(locale.value.startsWith('zh') ? 'zh-CN' : 'en-US', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value)) : '—' }
function formatCost(value: unknown) { return `$${Math.max(0, Number(value) || 0).toFixed(6)}` }
function formatBytes(value?: number) { if (!value) return '—'; if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`; return `${(value / 1024 / 1024).toFixed(1)} MB` }
function formatError(value: unknown) { if (!value) return ''; if (typeof value === 'string') return value; try { return JSON.stringify(value, null, 2) } catch { return String(value) } }
function deliveryLabel(value: string) { return t(`admin.videoGenerations.deliveryStatus.${value}`) }
function billingLabel(value: string) { return t(`admin.videoGenerations.billingStatus.${value}`) }
const badgeBase = 'inline-flex whitespace-nowrap rounded-md px-2 py-1 text-xs font-medium'
function deliveryBadgeClass(value: string) {
  if (value === 'delivered') return `${badgeBase} bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300`
  if (value === 'failed' || value === 'charged_without_output') return `${badgeBase} bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300`
  return `${badgeBase} bg-amber-50 text-amber-700 dark:bg-amber-950/30 dark:text-amber-300`
}
function billingBadgeClass(value: string) {
  if (value === 'charged') return `${badgeBase} bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300`
  if (value === 'not_charged') return `${badgeBase} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  if (value === 'billing_failed') return `${badgeBase} bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300`
  return `${badgeBase} bg-amber-50 text-amber-700 dark:bg-amber-950/30 dark:text-amber-300`
}
const identityRows = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.requestId'), value: detail.value.request_id, mono: true },
  { label: t('admin.videoGenerations.detail.user'), value: `${detail.value.user_email || '—'} (#${detail.value.user_id})` },
  { label: t('admin.videoGenerations.detail.apiKey'), value: `${detail.value.api_key_name || '—'} (#${detail.value.api_key_id})` },
  { label: t('admin.videoGenerations.detail.group'), value: `${detail.value.group_name || '—'}${detail.value.group_id ? ` (#${detail.value.group_id})` : ''}` },
  { label: t('admin.videoGenerations.detail.account'), value: `${detail.value.account_name || '—'} (#${detail.value.account_id})` },
] : [])
const generationRows = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.model'), value: detail.value.model, mono: true },
  { label: t('admin.videoGenerations.detail.upstreamModel'), value: detail.value.upstream_model || '—', mono: true },
  { label: t('admin.videoGenerations.detail.operation'), value: detail.value.operation },
  { label: t('admin.videoGenerations.detail.duration'), value: `${detail.value.duration_seconds}s` },
  { label: t('admin.videoGenerations.detail.resolution'), value: detail.value.resolution || '—' },
  { label: t('admin.videoGenerations.detail.aspectRatio'), value: detail.value.aspect_ratio || '—' },
  { label: t('admin.videoGenerations.detail.prompt'), value: detail.value.prompt || '—', wide: true },
] : [])
const deliveryRows = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.r2Url'), value: detail.value.video_url || '—', mono: true, wide: true },
  { label: t('admin.videoGenerations.detail.content'), value: `${detail.value.content_type || '—'} · ${formatBytes(detail.value.byte_size)}` },
  { label: 'Browser playable', value: detail.value.browser_playable ? 'Yes' : 'No' },
] : [])
const billingRows = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.actualCost'), value: formatCost(detail.value.actual_cost) },
  { label: t('admin.videoGenerations.detail.usageLog'), value: detail.value.usage_log_id ? `#${detail.value.usage_log_id}` : '—' },
  { label: t('admin.videoGenerations.billing'), value: billingLabel(detail.value.billing_status) },
] : [])
const errorRows = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.taskError'), value: formatError(detail.value.task_error) },
  { label: t('admin.videoGenerations.detail.upstreamError'), value: detail.value.last_upstream_error || '' },
  { label: t('admin.videoGenerations.detail.deliveryError'), value: detail.value.delivery_error || '' },
  { label: t('admin.videoGenerations.detail.billingError'), value: detail.value.billing_error || '' },
].filter(row => row.value) : [])
const timeline = computed(() => detail.value ? [
  { label: t('admin.videoGenerations.detail.submitted'), value: formatTime(detail.value.created_at) },
  { label: t('admin.videoGenerations.detail.checked'), value: formatTime(detail.value.last_checked_at) },
  { label: t('admin.videoGenerations.detail.completed'), value: formatTime(detail.value.completed_at) },
  { label: t('admin.videoGenerations.detail.delivered'), value: formatTime(detail.value.delivered_at) },
  { label: t('admin.videoGenerations.detail.billed'), value: formatTime(detail.value.billed_at) },
] : [])

onMounted(() => void load())
onBeforeUnmount(() => controller?.abort())
</script>
