<template>
  <AppLayout>
    <div class="promotion-center space-y-5">
      <div class="center-header">
        <div class="flex min-w-0 items-center gap-4">
          <span class="center-symbol"><Icon name="gift" size="lg" /></span>
          <div class="min-w-0"><p class="center-eyebrow">{{ tp('rechargeCampaign') }}</p><h1 class="mt-1 text-xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('nav.promotionCenter') }}</h1><p class="mt-1.5 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ tp('description') }}</p></div>
        </div>
        <button v-if="tab !== 'settings'" type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loading" @click="loadReports">
          <Icon name="refresh" size="sm" :class="['mr-2', { 'animate-spin': loading }]" />{{ tp('refresh') }}
        </button>
      </div>

      <div class="center-tabs" role="tablist" :aria-label="t('nav.promotionCenter')">
        <button v-for="item in tabs" :id="`promotion-tab-${item}`" :key="item" type="button" role="tab" :aria-selected="tab === item" :aria-controls="`promotion-panel-${item}`" class="center-tab" :class="{ 'is-selected': tab === item }" @click="selectTab(item)"><Icon :name="item === 'overview' ? 'chart' : item === 'records' ? 'clipboard' : 'edit'" size="sm" class="hidden sm:block" />{{ tp(item) }}</button>
      </div>

      <section v-show="tab === 'settings'" id="promotion-panel-settings" role="tabpanel" aria-labelledby="promotion-tab-settings">
        <RechargePromotionSettings v-if="configVisited" @saved="loadConfig" />
      </section>

      <template v-if="tab !== 'settings'">
        <form class="center-filters flex flex-wrap items-end gap-3" @submit.prevent="applyFilters">
          <label class="block w-full sm:w-40"><span class="sr-only">{{ tp('dateHint') }}</span>
            <select v-model="range" class="input" data-testid="promotion-range" @change="applyPreset"><option value="all">{{ tp('allTime') }}</option><option value="7">{{ tp('days7') }}</option><option value="30">{{ tp('days30') }}</option><option value="custom">{{ tp('custom') }}</option></select>
          </label>
          <div v-if="range === 'custom'" class="grid min-w-0 flex-1 grid-cols-2 gap-3 sm:max-w-sm">
            <label class="block min-w-0"><span class="sr-only">{{ tp('start') }}</span><input v-model="startDate" :aria-label="tp('start')" type="date" class="input min-w-0" /></label>
            <label class="block min-w-0"><span class="sr-only">{{ tp('end') }}</span><input v-model="endDate" :aria-label="tp('end')" type="date" class="input min-w-0" /></label>
          </div>
          <label v-if="tab === 'records'" class="block w-full sm:w-40"><span class="sr-only">{{ tp('status') }}</span><select v-model="status" class="input" data-testid="promotion-status-filter"><option value="">{{ tp('allStatus') }}</option><option v-for="value in statuses" :key="value" :value="value">{{ tp(value) }}</option></select></label>
          <label v-if="tab === 'records'" class="block w-full min-w-0 sm:w-auto sm:flex-1"><span class="sr-only">{{ tp('search') }}</span><input v-model="keyword" type="search" class="input" :placeholder="tp('search')" data-testid="promotion-search" /></label>
          <button v-if="range === 'custom' || tab === 'records'" type="submit" class="btn btn-secondary" :disabled="loading">{{ tp('apply') }}</button>
          <button v-if="range !== 'all' || status || keyword" type="button" class="px-1 py-2 text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400" @click="resetFilters">{{ tp('reset') }}</button>
          <p v-if="tab === 'overview'" class="py-2 text-xs text-gray-400 sm:ml-auto">{{ tp('dateHint') }}</p>
        </form>
        <div v-if="error" class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/20 dark:text-red-300" role="alert"><span>{{ error }}</span><button type="button" class="font-medium underline" @click="loadReports">{{ tp('retry') }}</button></div>
        <div v-if="loading" class="flex min-h-64 items-center justify-center" role="status" :aria-label="tp('loading')"><LoadingSpinner /></div>

        <section v-else-if="tab === 'overview' && summary" id="promotion-panel-overview" role="tabpanel" aria-labelledby="promotion-tab-overview" class="space-y-5">
          <div class="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <div v-for="(metric, index) in metrics" :key="metric.label" class="center-metric" :class="{ 'center-metric-gold': index === 3 }">
              <div class="flex items-center justify-between gap-2"><p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p><span class="metric-icon"><Icon :name="(['clipboard', 'users', 'creditCard', 'gift'] as const)[index]!" size="sm" /></span></div>
              <p class="mt-3 break-words text-[28px] font-semibold tracking-tight tabular-nums text-gray-950 dark:text-white" :title="metric.value">{{ metric.value }}</p>
              <p class="mt-2 text-[11px] leading-5 text-gray-400 dark:text-gray-500">{{ metric.hint }}</p>
            </div>
          </div>

          <div class="grid items-start gap-5 xl:grid-cols-[minmax(0,1.65fr)_minmax(280px,1fr)]">
            <div class="space-y-5">
              <section class="bonus-ledger">
                <div class="flex items-center justify-between gap-2"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ tp('bonusLedger') }}</h2><span class="ledger-unit">USD</span></div>
                <div class="mt-5 flex items-center justify-between gap-5">
                  <div class="min-w-0"><p class="text-xs text-gray-500 dark:text-gray-400">{{ tp('netBonus') }}</p><p class="mt-2 break-words text-3xl font-semibold tracking-tight tabular-nums text-gray-950 dark:text-amber-50">{{ money(summary.net_bonus_amount) }}</p><p class="mt-2 text-xs text-gray-400">{{ tp('netEquation') }}</p></div>
                  <div class="bonus-ring" :style="{ '--net-degrees': `${netBonusPercent * 3.6}deg` }" aria-hidden="true"><span><Icon name="gift" size="lg" /></span></div>
                </div>
                <div class="mt-5 grid grid-cols-2 gap-4 border-t border-amber-900/10 pt-4 dark:border-amber-200/10">
                  <div><p class="text-xs text-gray-500 dark:text-gray-400"><span class="mr-2 inline-block h-1.5 w-1.5 rounded-full bg-amber-500" />{{ tp('issued') }}</p><p class="mt-1.5 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ money(summary.bonus_amount) }}</p></div>
                  <div><p class="text-xs text-gray-500 dark:text-gray-400"><span class="mr-2 inline-block h-1.5 w-1.5 rounded-full bg-stone-300 dark:bg-stone-600" />{{ tp('refunded') }}</p><p class="mt-1.5 text-lg font-semibold tabular-nums text-gray-500 dark:text-gray-300">{{ money(summary.refunded_bonus_amount) }}</p></div>
                </div>
                <p class="mt-4 text-[11px] leading-5 text-gray-400">{{ tp('ledgerHint') }}</p>
              </section>

              <section class="card overflow-hidden">
                <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ tp('cash') }}</h2><p class="mt-1 text-xs leading-5 text-gray-400">{{ tp('cashHint') }}</p></div>
                <div v-if="summary.cash_by_currency.length" class="overflow-x-auto">
                  <table class="w-full text-left text-sm"><thead class="text-xs text-gray-400"><tr><th class="px-5 py-3 font-medium">{{ tp('currency') }}</th><th class="px-3 py-3 text-right font-medium">{{ tp('paid') }}</th><th class="px-3 py-3 text-right font-medium">{{ tp('refund') }}</th><th class="px-5 py-3 text-right font-medium">{{ tp('netCash') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="item in summary.cash_by_currency" :key="item.currency"><td class="px-5 py-4 font-medium text-gray-900 dark:text-white">{{ item.currency }}</td><td class="px-3 py-4 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ decimal(item.paid_amount) }}</td><td class="px-3 py-4 text-right tabular-nums text-gray-400">{{ decimal(item.refunded_amount) }}</td><td class="px-5 py-4 text-right font-medium tabular-nums text-gray-900 dark:text-white">{{ decimal(item.net_amount) }}</td></tr></tbody></table>
                </div>
                <p v-else class="px-5 py-10 text-center text-sm text-gray-400">{{ tp('noCash') }}</p>
              </section>
            </div>

            <section class="card campaign-card overflow-hidden">
              <div class="flex items-center justify-between gap-2 border-b border-gray-100 px-5 py-4 dark:border-dark-700"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ tp('current') }}</h2><span v-if="!configLoading && !configError" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300" data-testid="campaign-status">{{ tp(campaignStatus) }}</span></div>
              <p v-if="configLoading" class="p-5 text-sm text-gray-400" role="status">{{ tp('loading') }}</p>
              <div v-else-if="configError" class="space-y-3 p-5"><p role="alert" class="text-sm text-red-600 dark:text-red-300">{{ configError }}</p><button type="button" class="btn btn-secondary btn-sm" @click="loadConfig">{{ tp('retry') }}</button></div>
              <div v-else class="space-y-4 p-5">
                <div class="flex items-start gap-3"><span class="campaign-symbol"><Icon name="sparkles" size="md" /></span><div class="min-w-0"><p class="break-words text-base font-semibold text-gray-900 dark:text-white">{{ campaign?.title || tp('unconfigured') }}</p><p v-if="campaign?.subtitle" class="mt-1.5 break-words text-xs leading-5 text-gray-400">{{ campaign.subtitle }}</p></div></div>
                <template v-if="campaign?.title"><p class="text-xs leading-6 text-gray-500 dark:text-gray-400">{{ dateTime(campaign.starts_at) }}<br />— {{ dateTime(campaign.ends_at) }}</p><div class="space-y-2"><div v-for="tier in campaign.tiers" :key="tier.min_amount" class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-800"><span class="text-gray-500 dark:text-gray-400">{{ formatPaymentAmount(tier.min_amount, campaign.currency, locale) }}</span><span class="font-medium tabular-nums text-gray-900 dark:text-white">+{{ tier.bonus_percent }}%</span></div></div></template>
                <div v-if="campaign?.title" class="campaign-schedule"><div class="mb-2 flex justify-between gap-2 text-[11px] text-gray-400"><span>{{ tp('scheduleProgress') }}</span><span>{{ tp(campaignStatus) }}</span></div><div class="h-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"><div class="h-full rounded-full bg-amber-500/80" :style="{ width: `${scheduleProgress}%` }" /></div></div>
                <button type="button" class="center-configure w-full" @click="selectTab('settings')"><Icon name="edit" size="sm" />{{ tp('configure') }}</button>
              </div>
            </section>
          </div>

          <section class="card overflow-hidden">
            <div class="flex items-center justify-between gap-2 border-b border-gray-100 px-5 py-4 dark:border-dark-700"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ tp('recent') }}</h2><button type="button" class="text-xs font-medium text-primary-600 dark:text-primary-400" @click="selectTab('records')">{{ tp('viewAll') }} →</button></div>
            <div v-if="orders.items.length" class="divide-y divide-gray-100 dark:divide-dark-700"><button v-for="order in orders.items.slice(0, 5)" :key="order.id" type="button" class="flex w-full items-center gap-3 px-5 py-4 text-left hover:bg-gray-50 dark:hover:bg-dark-800" @click="selectedOrder = order"><div class="min-w-0 flex-1"><p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ order.user_username || order.user_email || `#${order.user_id}` }}</p><p class="mt-1 truncate text-xs text-gray-400">#{{ order.id }} · {{ dateTime(order.created_at) }}</p></div><span class="hidden rounded-md px-2 py-1 text-xs sm:inline-flex" :class="statusBadgeClass(order.status)">{{ tp(order.status) }}</span><div class="text-right"><p class="text-sm font-medium tabular-nums text-gray-900 dark:text-white">+{{ money(order.bonus_amount || 0) }}</p><p class="mt-1 text-xs text-gray-400">{{ tp('bonusColumn') }}</p></div></button></div>
            <div v-else class="px-5 py-12 text-center"><p class="text-sm font-medium text-gray-600 dark:text-gray-300">{{ tp('noData') }}</p><p class="mt-2 text-xs text-gray-400">{{ tp('emptyHint') }}</p></div>
          </section>
          <p class="text-xs leading-5 text-gray-400">{{ tp('totalsHint') }}</p>
        </section>

        <section v-else-if="tab === 'records' && !error" id="promotion-panel-records" role="tabpanel" aria-labelledby="promotion-tab-records" class="space-y-3">
          <p class="text-xs leading-5 text-gray-400">{{ tp('recordsHint') }} {{ tp('dateHint') }}</p>
          <div class="card overflow-hidden"><div class="overflow-x-auto"><table class="w-full min-w-[1060px] text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"><tr><th class="px-5 py-3 font-medium">{{ tp('order') }}</th><th class="px-4 py-3 font-medium">{{ tp('user') }}</th><th class="px-4 py-3 text-right font-medium">{{ tp('actualPaid') }}</th><th class="px-4 py-3 text-right font-medium">{{ tp('baseColumn') }}</th><th class="px-4 py-3 text-right font-medium">{{ tp('bonusColumn') }}</th><th class="px-4 py-3 font-medium">{{ tp('status') }}</th><th class="px-4 py-3 font-medium">{{ tp('created') }}</th><th class="px-5 py-3"><span class="sr-only">{{ tp('detail') }}</span></th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="order in orders.items" :key="order.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/40"><td class="max-w-56 px-5 py-4"><button type="button" class="font-medium text-gray-900 hover:text-primary-600 dark:text-white" @click="selectedOrder = order">#{{ order.id }}</button><p class="mt-1 truncate text-xs text-gray-400" :title="order.promotion_snapshot?.title">{{ order.promotion_snapshot?.title }}</p></td><td class="max-w-56 px-4 py-4"><p class="truncate text-gray-700 dark:text-gray-200">{{ order.user_username || `#${order.user_id}` }}</p><p class="mt-1 truncate text-xs text-gray-400" :title="order.user_email">{{ order.user_email }}</p></td><td class="whitespace-nowrap px-4 py-4 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatPaymentAmount(order.pay_amount, order.currency, locale) }}</td><td class="px-4 py-4 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ money(order.base_amount || 0) }}</td><td class="px-4 py-4 text-right font-medium tabular-nums text-primary-600 dark:text-primary-400">+{{ money(order.bonus_amount || 0) }}</td><td class="px-4 py-4"><span class="whitespace-nowrap rounded-md px-2 py-1 text-xs" :class="statusBadgeClass(order.status)">{{ tp(order.status) }}</span></td><td class="whitespace-nowrap px-4 py-4 text-xs text-gray-500">{{ dateTime(order.created_at) }}</td><td class="px-5 py-4 text-right"><button type="button" class="whitespace-nowrap text-xs font-medium text-primary-600 dark:text-primary-400" @click="selectedOrder = order">{{ tp('detail') }}</button></td></tr><tr v-if="!orders.items.length"><td colspan="8" class="px-5 py-16 text-center text-sm text-gray-400">{{ tp('noData') }}</td></tr></tbody></table></div><Pagination v-if="orders.total" :total="orders.total" :page="orders.page" :page-size="orders.page_size" @update:page="changePage" @update:page-size="changePageSize" /></div>
        </section>
      </template>
    </div>

    <BaseDialog :show="!!selectedOrder" :title="tp('orderDetail')" width="wide" @close="selectedOrder = null">
      <div v-if="selectedOrder" class="space-y-5">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2"><div v-for="field in detailFields" :key="field.label" class="min-w-0"><p class="text-xs text-gray-400">{{ field.label }}</p><p class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white">{{ field.value }}</p></div></div>
        <div class="grid grid-cols-3 gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-800"><div v-for="field in detailAmounts" :key="field.label" class="min-w-0"><p class="text-xs text-gray-500">{{ field.label }}</p><p class="mt-2 break-words text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ field.value }}</p></div></div>
        <p class="text-xs leading-5 text-gray-400">{{ tp('detailHint') }}</p>
        <router-link to="/admin/orders" class="inline-flex text-sm font-medium text-primary-600 dark:text-primary-400">{{ tp('manageOrder') }} →</router-link>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import RechargePromotionSettings from '@/components/admin/payment/RechargePromotionSettings.vue'
import { adminPromotionsAPI, type PromotionFilters, type PromotionOrder, type PromotionSummary } from '@/api/admin/promotions'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { RechargePromotion } from '@/types/payment'
import type { BasePaginationResponse } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import { statusBadgeClass } from '@/components/payment/orderUtils'
import { promotionCenterMessages } from './promotionCenterMessages'

const { t, locale } = useI18n()
const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
function tp(key: string, params: Record<string, string | number> = {}) {
  const messages: Record<string, string> = locale.value.startsWith('zh') ? promotionCenterMessages.zh : promotionCenterMessages.en
  return (messages[key] || key).replace(/\{(\w+)\}/g, (match, name: string) => String(params[name] ?? match))
}
const tabs = ['overview', 'records', 'settings'] as const
type Tab = typeof tabs[number]
const tab = ref<Tab>(tabs.includes(route.query.tab as Tab) ? route.query.tab as Tab : 'overview')
const configVisited = ref(tab.value === 'settings')
const range = ref('all')
const startDate = ref('')
const endDate = ref('')
const status = ref('')
const keyword = ref('')
const statuses = ['PENDING', 'PAID', 'RECHARGING', 'COMPLETED', 'FAILED', 'EXPIRED', 'CANCELLED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUND_PENDING', 'REFUND_FAILED', 'REFUNDED', 'PARTIALLY_REFUNDED']
const summary = ref<PromotionSummary | null>(null)
const orders = ref<BasePaginationResponse<PromotionOrder>>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const campaign = ref<RechargePromotion | null>(null)
const configLoading = ref(true)
const configError = ref('')
const selectedOrder = ref<PromotionOrder | null>(null)
const loading = ref(false)
const error = ref('')
const applied = ref<PromotionFilters>({})
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
let requestVersion = 0
let configRequestVersion = 0
const decimal = (value: number) => new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value || 0)
const money = (value: number) => `$${decimal(value)}`
const dateTime = (value?: string) => value ? new Intl.DateTimeFormat(locale.value, { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—'
const campaignStatus = computed(() => !campaign.value?.enabled ? 'disabled' : now.value < Date.parse(campaign.value.starts_at) ? 'scheduled' : now.value >= Date.parse(campaign.value.ends_at) ? 'ended' : 'active')
const metrics = computed(() => summary.value ? [
  { label: tp('issuedOrders'), value: String(summary.value.issued_order_count), hint: `${tp('totalOrders')} ${summary.value.order_count} · ${tp('pending', { count: summary.value.pending_order_count })}` },
  { label: tp('users'), value: String(summary.value.issued_user_count), hint: tp('usersHint') },
  { label: tp('base'), value: money(summary.value.base_amount), hint: tp('creditUnit') },
  { label: tp('bonus'), value: money(summary.value.bonus_amount), hint: tp('bonusHint') },
] : [])
const netBonusPercent = computed(() => summary.value?.bonus_amount ? Math.max(0, Math.min(100, summary.value.net_bonus_amount / summary.value.bonus_amount * 100)) : 0)
const scheduleProgress = computed(() => {
  if (!campaign.value?.enabled) return 0
  const start = Date.parse(campaign.value.starts_at)
  const duration = Date.parse(campaign.value.ends_at) - start
  return duration > 0 ? Math.max(0, Math.min(100, (now.value - start) / duration * 100)) : 0
})
const detailFields = computed(() => selectedOrder.value ? [
  { label: tp('order'), value: `#${selectedOrder.value.id}` },
  { label: tp('status'), value: tp(selectedOrder.value.status) },
  { label: tp('tradeNo'), value: selectedOrder.value.out_trade_no },
  { label: tp('user'), value: selectedOrder.value.user_email || `#${selectedOrder.value.user_id}` },
  { label: tp('campaign'), value: selectedOrder.value.promotion_snapshot?.title || '—' },
  { label: tp('actualPaid'), value: formatPaymentAmount(selectedOrder.value.pay_amount, selectedOrder.value.currency, locale.value) },
  { label: tp('created'), value: dateTime(selectedOrder.value.created_at) },
  { label: tp('completed'), value: dateTime(selectedOrder.value.completed_at) },
] : [])
const detailAmounts = computed(() => selectedOrder.value ? [
  { label: tp('baseColumn'), value: money(selectedOrder.value.base_amount || 0) },
  { label: tp('bonusColumn'), value: money(selectedOrder.value.bonus_amount || 0) },
  { label: tp('total'), value: money(selectedOrder.value.amount) },
] : [])

function selectTab(value: Tab) {
  tab.value = value
  if (value === 'settings') configVisited.value = true
  void router.replace({ query: { ...route.query, tab: value } })
  if (value !== 'settings') { status.value = ''; keyword.value = ''; void applyFilters() }
}
function beijingDate(time: number) { return new Date(time + 8 * 3600_000).toISOString().slice(0, 10) }
function applyPreset() {
  if (range.value === 'custom') return
  if (range.value === 'all') { startDate.value = ''; endDate.value = '' }
  else { endDate.value = beijingDate(Date.now()); startDate.value = beijingDate(Date.now() - (Number(range.value) - 1) * 86400_000) }
  void applyFilters()
}
function applyFilters() {
  const filters: PromotionFilters = {}
  if (range.value !== 'all') {
    const start = Date.parse(`${startDate.value}T00:00:00+08:00`)
    const end = Date.parse(`${endDate.value}T00:00:00+08:00`)
    if (!Number.isFinite(start) || !Number.isFinite(end) || start > end || beijingDate(start) !== startDate.value || beijingDate(end) !== endDate.value) { error.value = tp('invalidDates'); return }
    filters.start_time = new Date(start).toISOString()
    filters.end_time = new Date(end + 86400_000).toISOString()
  }
  if (tab.value === 'records') { if (status.value) filters.status = status.value; if (keyword.value.trim()) filters.keyword = keyword.value.trim() }
  applied.value = filters
  orders.value.page = 1
  return loadReports()
}
function resetFilters() { range.value = 'all'; status.value = ''; keyword.value = ''; applyPreset() }
async function loadConfig() {
  if (appStore.cachedPublicSettings?.recharge_promotion_enabled !== true) return
  const version = ++configRequestVersion
  configLoading.value = true
  configError.value = ''
  try {
    const response = await adminPaymentAPI.getConfig()
    if (version === configRequestVersion) campaign.value = response.data.recharge_promotion || null
  } catch (err) {
    if (version === configRequestVersion) configError.value = extractApiErrorMessage(err, tp('failed'))
  } finally {
    if (version === configRequestVersion) configLoading.value = false
  }
}
async function loadReports() {
  if (appStore.cachedPublicSettings?.recharge_promotion_enabled !== true) return
  const version = ++requestVersion
  loading.value = true
  error.value = ''
  try {
    const [stats, records] = await Promise.all([
      adminPromotionsAPI.summary(applied.value),
      adminPromotionsAPI.orders({ ...applied.value, page: orders.value.page, page_size: orders.value.page_size }),
    ])
    if (version !== requestVersion) return
    summary.value = stats.data
    orders.value = records.data
  } catch (err) {
    if (version !== requestVersion) return
    summary.value = null
    orders.value = { ...orders.value, items: [], total: 0 }
    error.value = extractApiErrorMessage(err, tp('failed'))
  } finally { if (version === requestVersion) loading.value = false }
}
function changePage(page: number) { orders.value.page = page; void loadReports() }
function changePageSize(size: number) { orders.value.page_size = size; orders.value.page = 1; void loadReports() }
watch(() => appStore.cachedPublicSettings?.recharge_promotion_enabled, enabled => {
  if (enabled !== true) { ++requestVersion; ++configRequestVersion; void router.replace('/admin/settings?tab=features') }
}, { immediate: true })
onMounted(() => { void loadConfig(); if (tab.value !== 'settings') void loadReports(); timer = setInterval(() => { now.value = Date.now() }, 1000) })
onBeforeUnmount(() => { ++requestVersion; ++configRequestVersion; if (timer) clearInterval(timer) })
</script>

<style scoped>
.promotion-center { min-width: 0; position: relative; }
.promotion-center .overflow-x-auto { position: relative; }
.promotion-center input[type='date'] { max-width: 100%; }
.center-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 4px 0 8px; }
.center-symbol { display: flex; flex: none; align-items: center; justify-content: center; width: 52px; height: 52px; color: #ae7519; border: 1px solid #e8d9ba; border-radius: 16px; background: linear-gradient(135deg,#fffcf5,#f6e5b9); box-shadow: inset 0 1px 0 #fff, 0 4px 12px #ac791015; }
.center-eyebrow { font-size: 10px; line-height: 14px; font-weight: 600; color: #987749; letter-spacing: .08em; }
.center-tabs { display: flex; gap: 5px; width: fit-content; max-width: 100%; border: 1px solid #e5e7eb; border-radius: 12px; padding: 5px; background: #f4f5f7; }
.center-tab { display: inline-flex; align-items: center; justify-content: center; gap: 8px; border-radius: 8px; padding: 9px 18px; color: #6b7280; font-size: 13px; font-weight: 500; white-space: nowrap; transition: background .15s,color .15s; }
.center-tab:hover { color: #111827; }
.center-tab.is-selected { color: #8c5c16; background: #fff; box-shadow: 0 1px 4px #11182712; }
.center-filters { border-radius: 12px; padding: 2px 0; }
.center-metric { min-width: 0; border: 1px solid #e7e9ee; border-radius: 16px; background: #fff; padding: 18px 20px; box-shadow: 0 2px 5px #11182703; }
.center-metric-gold { border-color: #e9dcbf; background: linear-gradient(125deg,#fffdf9,#fcf5e7); }
.metric-icon { display: inline-flex; align-items: center; justify-content: center; width: 30px; height: 30px; flex: none; color: #7b8495; background: #f4f5f7; border-radius: 9px; }
.center-metric-gold .metric-icon { color: #a06b15; background: #f4e6c5; }
.bonus-ledger { overflow: hidden; padding: 20px 24px; border: 1px solid #e9dcc3; border-radius: 16px; background: linear-gradient(115deg,#fffdf8 20%,#fbf3e2 100%); }
.ledger-unit { font-size: 10px; letter-spacing: .06em; font-weight: 600; color: #957442; border: 1px solid #e8dac0; border-radius: 5px; padding: 3px 7px; }
.bonus-ring { width: 98px; height: 98px; flex: none; border-radius: 50%; padding: 9px; background: conic-gradient(from -90deg,#c4933b 0deg,#eacb7d var(--net-degrees),#eee7da var(--net-degrees)); }
.bonus-ring > span { display: flex; width: 100%; height: 100%; align-items: center; justify-content: center; color: #ac802d; background: #fdf8ed; border-radius: 50%; }
.campaign-card { border: 1px solid #e7e9ee; box-shadow: 0 2px 5px #11182703; }
.campaign-symbol { display: inline-flex; align-items: center; justify-content: center; flex: none; width: 36px; height: 36px; border: 1px solid #ecdfc2; border-radius: 11px; background: #fcf5e5; color: #af7b20; }
.center-configure { display: inline-flex; align-items: center; justify-content: center; gap: 8px; border: 1px solid #e5d4ad; border-radius: 9px; padding: 10px 14px; color: #80581d; background: linear-gradient(100deg,#fcf7ea,#f6e9c9); font-size: 12px; font-weight: 600; transition: filter .15s; }
.center-configure:hover { filter: brightness(.97); }
.center-tab:focus-visible,.center-configure:focus-visible { outline: 2px solid #c59d50; outline-offset: 3px; }
.dark .center-symbol { color: #e7c47a; background: linear-gradient(130deg,#342c20,#47371f); border-color: #605031; box-shadow: none; }
.dark .center-eyebrow { color: #c2a877; }
.dark .center-tabs { border-color: #303747; background: #151c28; }
.dark .center-tab { color: #98a2b3; }
.dark .center-tab.is-selected { color: #e7cc91; background: #30333b; box-shadow: 0 1px 3px #0003; }
.dark .center-tab:hover { color: #fff; }
.dark .center-metric { background: #1e2532; border-color: #313947; }
.dark .center-metric-gold { background: linear-gradient(130deg,#292a2e,#332d22); border-color: #534735; }
.dark .metric-icon { background: #2b3443; color: #a1acbc; }
.dark .center-metric-gold .metric-icon { background: #483b25; color: #e0bc6e; }
.dark .bonus-ledger { background: linear-gradient(120deg,#28292d,#362e20); border-color: #564834; }
.dark .ledger-unit { color: #cbb17f; border-color: #5c4c31; }
.dark .bonus-ring { background: conic-gradient(from -90deg,#bc903e 0deg,#e5c884 var(--net-degrees),#494334 var(--net-degrees)); }
.dark .bonus-ring > span { background: #302c24; color: #d2b372; }
.dark .campaign-card { border-color: #313947; }
.dark .campaign-symbol { background: #373022; border-color: #544831; color: #dcb97a; }
.dark .center-configure { color: #e7cc91; background: linear-gradient(100deg,#373124,#493b24); border-color: #645132; }
@media (max-width: 640px) {
  .center-header { flex-wrap: wrap; }
  .center-header > button { margin-left: auto; }
  .center-tabs { width: 100%; }
  .center-tab { flex: 1; padding: 9px 6px; font-size: 12px; }
  .center-metric { padding: 14px; }
  .metric-icon { width: 25px; height: 25px; }
  .bonus-ledger { padding: 18px; }
  .bonus-ring { width: 80px; height: 80px; padding: 7px; }
}
</style>
