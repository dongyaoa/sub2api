<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="inline-flex max-w-full overflow-x-auto rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button
            v-for="tab in tabs"
            :key="tab.value"
            type="button"
            class="whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-colors"
            :class="
              activeTab === tab.value
                ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'
            "
            @click="selectTab(tab.value)"
          >
            {{ tab.label }}
          </button>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="refreshing" @click="refreshCurrent">
          <Icon name="refresh" size="md" class="mr-2" :class="refreshing && 'animate-spin'" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div v-if="initialLoading" class="flex min-h-72 items-center justify-center">
        <LoadingSpinner />
      </div>

      <template v-else>
        <section v-if="activeTab === 'overview'" class="space-y-6">
          <div class="grid grid-cols-2 gap-4 xl:grid-cols-5">
            <div v-for="metric in overviewMetrics" :key="metric.label" class="card p-4">
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 flex-none items-center justify-center rounded-lg" :class="metric.iconClass">
                  <Icon :name="metric.icon" size="md" />
                </div>
                <div class="min-w-0">
                  <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
                  <p class="mt-1 truncate text-xl font-bold text-gray-950 dark:text-white" :title="metric.value">
                    {{ metric.value }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <div class="grid items-start gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(340px,0.85fr)]">
            <div class="space-y-4">
              <div class="card overflow-hidden">
                <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
                  <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('adminCheckin.rewardOverview') }}</h2>
                </div>
                <div class="grid grid-cols-3 divide-x divide-gray-100 dark:divide-dark-700">
                  <div v-for="item in overviewDetails" :key="item.label" class="min-w-0 px-2 py-4 text-center sm:px-5 sm:text-left">
                    <p class="min-h-8 text-xs leading-4 text-gray-500 dark:text-gray-400 sm:min-h-0">{{ item.label }}</p>
                    <p class="mt-1 break-all text-base font-semibold tabular-nums text-gray-950 dark:text-white sm:text-lg" :title="item.value">
                      {{ item.value }}
                    </p>
                  </div>
                </div>
              </div>

              <div class="card overflow-hidden">
                <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
                  <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('adminCheckin.recentCheckins') }}</h2>
                  <button type="button" class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="selectTab('records')">
                    {{ t('adminCheckin.viewAll') }}
                  </button>
                </div>
                <div v-if="recentRecords.length" class="divide-y divide-gray-100 dark:divide-dark-700">
                  <div v-for="item in recentRecords" :key="item.id" class="flex items-center gap-3 px-5 py-3">
                    <div class="flex h-9 w-9 flex-none items-center justify-center rounded-full bg-gray-100 text-sm font-semibold text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                      {{ (item.username || item.email || `#${item.user_id}`).charAt(0).toUpperCase() }}
                    </div>
                    <div class="min-w-0 flex-1">
                      <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.username || `#${item.user_id}` }}</p>
                      <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.email }}</p>
                    </div>
                    <div class="flex-none text-right">
                      <p class="text-sm font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">+{{ formatMoney(item.reward) }}</p>
                      <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ formatDateTime(item.claimed_at) }}</p>
                    </div>
                  </div>
                </div>
                <div v-else class="py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('adminCheckin.noRecentCheckins') }}</div>
              </div>
            </div>

            <div class="card overflow-hidden">
              <div class="border-b border-gray-100 px-4 py-4 dark:border-dark-700">
                <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('adminCheckin.dailyTrend') }}</h2>
              </div>
              <div v-if="recentDaily.length" class="divide-y divide-gray-100 dark:divide-dark-700">
                <div v-for="item in recentDaily" :key="item.date" class="grid grid-cols-[2.75rem_minmax(0,1fr)_5.5rem] items-center gap-2 px-4 py-2">
                  <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ item.date.slice(5) }}</span>
                  <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div class="h-full rounded-full bg-emerald-500" :style="{ width: `${dailyBarWidth(item.reward_total)}%` }"></div>
                  </div>
                  <div class="text-right leading-tight">
                    <p class="text-xs font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatMoney(item.reward_total) }}</p>
                    <p class="mt-0.5 text-[11px] text-gray-400 dark:text-gray-500">{{ t('adminCheckin.userCount', { count: item.user_count }) }}</p>
                  </div>
                </div>
              </div>
              <div v-else class="py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('adminCheckin.noData') }}</div>
            </div>
          </div>
        </section>

        <section v-else-if="activeTab === 'users'" class="space-y-4">
          <div class="flex flex-wrap items-center gap-3">
            <div class="relative min-w-60 flex-1 sm:max-w-sm">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="userSearch"
                type="search"
                class="input pl-9"
                :placeholder="t('adminCheckin.searchUsers')"
                @keyup.enter="searchUsers"
              />
            </div>
            <Select
              v-model="qualificationFilter"
              :options="qualificationOptions"
              class="w-40"
              @change="changeQualification"
            />
            <button type="button" class="btn btn-secondary" @click="searchUsers">{{ t('common.search') }}</button>
          </div>

          <div class="card overflow-hidden">
            <div v-if="usersLoading" class="flex min-h-64 items-center justify-center"><LoadingSpinner /></div>
            <div v-else class="overflow-x-auto">
              <table class="w-full min-w-[1120px]">
                <thead class="bg-gray-50/80 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3 font-medium">{{ t('adminCheckin.user') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.totalRecharge') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.effectiveRecharge') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.currentBalance') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.checkinDays') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.rewardTotal') }}</th>
                    <th class="px-5 py-3 font-medium">{{ t('adminCheckin.lastCheckin') }}</th>
                    <th class="px-5 py-3 text-center font-medium">{{ t('adminCheckin.qualification') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="item in users.items" :key="item.user_id">
                    <td class="px-5 py-4">
                      <p class="text-sm font-medium text-gray-900 dark:text-white">{{ item.username || `#${item.user_id}` }}</p>
                      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ item.email }}</p>
                    </td>
                    <td class="px-5 py-4 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatMoney(item.total_recharge) }}</td>
                    <td class="px-5 py-4 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatMoney(item.effective_recharge) }}</td>
                    <td class="px-5 py-4 text-right text-sm font-medium tabular-nums text-gray-900 dark:text-white">{{ formatMoney(item.current_balance) }}</td>
                    <td class="px-5 py-4 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ item.checkin_days }}</td>
                    <td class="px-5 py-4 text-right text-sm font-medium tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatMoney(item.reward_total) }}</td>
                    <td class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">{{ item.last_checkin_at ? formatDateTime(item.last_checkin_at) : '-' }}</td>
                    <td class="px-5 py-4 text-center">
                      <span
                        class="inline-flex rounded-full px-2 py-1 text-xs font-medium"
                        :class="item.eligible ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'"
                      >
                        {{ item.eligible ? t('adminCheckin.qualified') : t('adminCheckin.notQualified') }}
                      </span>
                    </td>
                  </tr>
                  <tr v-if="!users.items.length">
                    <td colspan="8" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('adminCheckin.noData') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Pagination
              v-if="users.total > 0"
              :total="users.total"
              :page="users.page"
              :page-size="users.page_size"
              @update:page="changeUserPage"
              @update:page-size="changeUserPageSize"
            />
          </div>
        </section>

        <section v-else-if="activeTab === 'records'" class="space-y-4">
          <div class="flex flex-wrap items-center gap-3">
            <div class="relative min-w-60 flex-1 sm:max-w-sm">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="recordSearch"
                type="search"
                class="input pl-9"
                :placeholder="t('adminCheckin.searchRecords')"
                @keyup.enter="searchRecords"
              />
            </div>
            <button type="button" class="btn btn-secondary" @click="searchRecords">{{ t('common.search') }}</button>
          </div>

          <div class="card overflow-hidden">
            <div v-if="recordsLoading" class="flex min-h-64 items-center justify-center"><LoadingSpinner /></div>
            <div v-else class="overflow-x-auto">
              <table class="w-full min-w-[820px]">
                <thead class="bg-gray-50/80 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3 font-medium">{{ t('adminCheckin.claimedAt') }}</th>
                    <th class="px-5 py-3 font-medium">{{ t('adminCheckin.user') }}</th>
                    <th class="px-5 py-3 font-medium">{{ t('adminCheckin.businessDate') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.reward') }}</th>
                    <th class="px-5 py-3 text-right font-medium">{{ t('adminCheckin.currentBalance') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="item in records.items" :key="item.id">
                    <td class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(item.claimed_at) }}</td>
                    <td class="px-5 py-4">
                      <p class="text-sm font-medium text-gray-900 dark:text-white">{{ item.username || `#${item.user_id}` }}</p>
                      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ item.email }}</p>
                    </td>
                    <td class="px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ item.business_date }}</td>
                    <td class="px-5 py-4 text-right text-sm font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">+{{ formatMoney(item.reward) }}</td>
                    <td class="px-5 py-4 text-right text-sm font-medium tabular-nums text-gray-900 dark:text-white">{{ formatMoney(item.current_balance) }}</td>
                  </tr>
                  <tr v-if="!records.items.length">
                    <td colspan="5" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('adminCheckin.noData') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Pagination
              v-if="records.total > 0"
              :total="records.total"
              :page="records.page"
              :page-size="records.page_size"
              @update:page="changeRecordPage"
              @update:page-size="changeRecordPageSize"
            />
          </div>
        </section>

        <section v-else-if="activeTab === 'settings' && config" class="card">
          <form class="p-5 sm:p-6" @submit.prevent="saveConfig">
            <div class="flex items-center justify-between gap-6 border-b border-gray-100 pb-5 dark:border-dark-700">
              <div>
                <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('adminCheckin.featureStatus') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('adminCheckin.featureStatusHint') }}</p>
              </div>
              <Toggle v-model="config.enabled" />
            </div>

            <div class="mt-6 grid gap-5 md:grid-cols-2 xl:grid-cols-3">
              <label class="block">
                <span class="input-label">{{ t('adminCheckin.minRecharge') }}</span>
                <input v-model.number="config.min_recharge_amount" class="input mt-1" type="number" min="0.01" step="0.01" required />
              </label>
              <label class="block">
                <span class="input-label">{{ t('adminCheckin.qualificationDays') }}</span>
                <input v-model.number="config.qualification_days" class="input mt-1" type="number" min="1" max="365" step="1" required />
              </label>
              <label class="block">
                <span class="input-label">{{ t('adminCheckin.minAccountAge') }}</span>
                <input v-model.number="config.min_account_age_hours" class="input mt-1" type="number" min="0" max="8760" step="1" required />
              </label>
              <label class="block">
                <span class="input-label">{{ t('adminCheckin.rewardMin') }}</span>
                <input v-model.number="config.reward_min" class="input mt-1" type="number" min="0.01" step="0.01" required />
              </label>
              <label class="block">
                <span class="input-label">{{ t('adminCheckin.rewardMax') }}</span>
                <input v-model.number="config.reward_max" class="input mt-1" type="number" min="0.01" step="0.01" required />
              </label>
              <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-800">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('adminCheckin.costGuard') }}</p>
                <p class="mt-1 text-sm font-semibold" :class="costGuardValid ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
                  {{ formatMoney(maxPeriodReward) }} / {{ formatMoney(rewardCostLimit) }}
                </p>
              </div>
            </div>

            <div class="mt-6 flex justify-end border-t border-gray-100 pt-5 dark:border-dark-700">
              <button type="submit" class="btn btn-primary" :disabled="saving || !costGuardValid">
                <Icon :name="saving ? 'refresh' : 'check'" size="md" class="mr-2" :class="saving && 'animate-spin'" />
                {{ saving ? t('adminCheckin.saving') : t('common.save') }}
              </button>
            </div>
          </form>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CheckinConfig,
  CheckinRecord,
  CheckinSummary,
  CheckinUserReport,
  Paginated
} from '@/api/admin/checkin'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'

type Tab = 'overview' | 'users' | 'records' | 'settings'

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<Tab>('overview')
const initialLoading = ref(true)
const refreshing = ref(false)
const saving = ref(false)
const usersLoading = ref(false)
const recordsLoading = ref(false)
const config = ref<CheckinConfig | null>(null)
const summary = ref<CheckinSummary | null>(null)
const recentRecords = ref<CheckinRecord[]>([])
const userSearch = ref('')
const qualificationFilter = ref('')
const recordSearch = ref('')

const emptyPage = <T,>(): Paginated<T> => ({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const users = ref<Paginated<CheckinUserReport>>(emptyPage())
const records = ref<Paginated<CheckinRecord>>(emptyPage())

const tabs = computed(() => [
  { value: 'overview' as const, label: t('adminCheckin.tabs.overview') },
  { value: 'users' as const, label: t('adminCheckin.tabs.users') },
  { value: 'records' as const, label: t('adminCheckin.tabs.records') },
  { value: 'settings' as const, label: t('adminCheckin.tabs.settings') }
])

const qualificationOptions = computed(() => [
  { value: '', label: t('adminCheckin.allQualifications') },
  { value: 'qualified', label: t('adminCheckin.qualified') },
  { value: 'unqualified', label: t('adminCheckin.notQualified') }
])

const formatMoney = (value: number) => `$${Number(value || 0).toFixed(2)}`

const overviewMetrics = computed(() => [
  { label: t('adminCheckin.todayUsers'), value: String(summary.value?.today_users || 0), icon: 'users' as const, iconClass: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: t('adminCheckin.todayReward'), value: formatMoney(summary.value?.today_reward || 0), icon: 'gift' as const, iconClass: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400' },
  { label: t('adminCheckin.todayAverage'), value: formatMoney(summary.value?.today_average || 0), icon: 'chart' as const, iconClass: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400' },
  { label: t('adminCheckin.sevenDayClaims'), value: String(summary.value?.users_7_days || 0), icon: 'clock' as const, iconClass: 'bg-cyan-100 text-cyan-600 dark:bg-cyan-900/30 dark:text-cyan-400' },
  { label: t('adminCheckin.eligibleUsers'), value: String(summary.value?.eligible_users || 0), icon: 'checkCircle' as const, iconClass: 'bg-violet-100 text-violet-600 dark:bg-violet-900/30 dark:text-violet-400' }
])

const recentDaily = computed(() => summary.value?.daily.slice(-14).reverse() || [])
const recentClaimCount = computed(() => recentDaily.value.reduce((total, item) => total + item.user_count, 0))
const recentRewardTotal = computed(() => recentDaily.value.reduce((total, item) => total + item.reward_total, 0))
const overviewDetails = computed(() => [
  { label: t('adminCheckin.reward30Days'), value: formatMoney(summary.value?.reward_30_days || 0) },
  { label: t('adminCheckin.fourteenDayClaims'), value: String(recentClaimCount.value) },
  { label: t('adminCheckin.fourteenDayAverage'), value: formatMoney(recentClaimCount.value ? recentRewardTotal.value / recentClaimCount.value : 0) }
])
const maxDailyReward = computed(() => Math.max(0, ...recentDaily.value.map((item) => item.reward_total)))
const dailyBarWidth = (value: number) => value > 0 && maxDailyReward.value > 0 ? Math.max(2, (value / maxDailyReward.value) * 100) : 0
const maxPeriodReward = computed(() => (config.value?.qualification_days || 0) * (config.value?.reward_max || 0))
const rewardCostLimit = computed(() => (config.value?.min_recharge_amount || 0) * 0.2)
const costGuardValid = computed(() => maxPeriodReward.value <= rewardCostLimit.value + 0.0000001)

const loadOverview = async () => {
  const [summaryResult, recordsResult] = await Promise.all([
    adminAPI.checkin.getSummary(),
    adminAPI.checkin.getRecords({ page: 1, page_size: 6, search: '' })
  ])
  summary.value = summaryResult
  recentRecords.value = recordsResult.items
}

const loadConfig = async () => {
  config.value = await adminAPI.checkin.getConfig()
}

const loadUsers = async () => {
  usersLoading.value = true
  try {
    users.value = await adminAPI.checkin.getUsers({
      page: users.value.page,
      page_size: users.value.page_size,
      search: userSearch.value.trim(),
      qualification: qualificationFilter.value
    })
  } finally {
    usersLoading.value = false
  }
}

const loadRecords = async () => {
  recordsLoading.value = true
  try {
    records.value = await adminAPI.checkin.getRecords({
      page: records.value.page,
      page_size: records.value.page_size,
      search: recordSearch.value.trim()
    })
  } finally {
    recordsLoading.value = false
  }
}

const selectTab = async (tab: Tab) => {
  activeTab.value = tab
  try {
    if (tab === 'users' && !users.value.items.length) await loadUsers()
    if (tab === 'records' && !records.value.items.length) await loadRecords()
    if (tab === 'settings' && !config.value) await loadConfig()
  } catch (error: any) {
    appStore.showError(error?.message || t('adminCheckin.loadFailed'))
  }
}

const refreshCurrent = async () => {
  refreshing.value = true
  try {
    if (activeTab.value === 'overview') await loadOverview()
    if (activeTab.value === 'users') await loadUsers()
    if (activeTab.value === 'records') await loadRecords()
    if (activeTab.value === 'settings') await loadConfig()
  } catch (error: any) {
    appStore.showError(error?.message || t('adminCheckin.loadFailed'))
  } finally {
    refreshing.value = false
  }
}

const searchUsers = () => { users.value.page = 1; void loadUsers() }
const changeQualification = () => { users.value.page = 1; void loadUsers() }
const searchRecords = () => { records.value.page = 1; void loadRecords() }
const changeUserPage = (page: number) => { users.value.page = page; void loadUsers() }
const changeUserPageSize = (size: number) => { users.value.page = 1; users.value.page_size = size; void loadUsers() }
const changeRecordPage = (page: number) => { records.value.page = page; void loadRecords() }
const changeRecordPageSize = (size: number) => { records.value.page = 1; records.value.page_size = size; void loadRecords() }

const saveConfig = async () => {
  if (!config.value || !costGuardValid.value) return
  saving.value = true
  try {
    config.value = await adminAPI.checkin.updateConfig({ ...config.value })
    appStore.showSuccess(t('adminCheckin.saveSuccess'))
    await loadOverview()
  } catch (error: any) {
    appStore.showError(error?.message || t('adminCheckin.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    await Promise.all([loadOverview(), loadConfig()])
  } catch (error: any) {
    appStore.showError(error?.message || t('adminCheckin.loadFailed'))
  } finally {
    initialLoading.value = false
  }
})
</script>
