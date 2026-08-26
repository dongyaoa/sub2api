<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div v-if="loading" class="flex min-h-72 items-center justify-center">
        <LoadingSpinner />
      </div>

      <template v-else-if="status">
        <section class="card overflow-hidden">
          <div class="grid gap-6 p-5 sm:p-6 lg:grid-cols-[1.4fr_1fr] lg:p-8">
            <div class="min-w-0">
              <div class="grid gap-5 sm:grid-cols-2">
                <div class="flex items-start gap-4">
                  <div
                    class="flex h-12 w-12 flex-none items-center justify-center rounded-lg bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                  >
                    <Icon name="creditCard" size="lg" />
                  </div>
                  <div class="min-w-0">
                    <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                      {{ t('checkin.currentBalance') }}
                    </p>
                    <p class="mt-1 text-2xl font-bold text-gray-950 dark:text-white">
                      {{ formatMoney(status.current_balance) }}
                    </p>
                  </div>
                </div>
                <div class="flex items-start gap-4">
                  <div
                    class="flex h-12 w-12 flex-none items-center justify-center rounded-lg bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                  >
                    <Icon name="gift" size="lg" />
                  </div>
                  <div class="min-w-0">
                    <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                      {{ t('checkin.totalReward') }}
                    </p>
                    <p class="mt-1 text-2xl font-bold text-emerald-600 dark:text-emerald-400">
                      {{ formatMoney(status.total_reward) }}
                    </p>
                  </div>
                </div>
              </div>

              <div class="mt-7">
                <div class="mb-2 flex items-center justify-between gap-4 text-sm">
                  <span class="font-medium text-gray-700 dark:text-gray-300">
                    {{ t('checkin.qualificationProgress') }}
                  </span>
                  <span class="tabular-nums text-gray-500 dark:text-gray-400">
                    {{ formatMoney(status.recharge_amount) }} / {{ formatMoney(status.min_recharge_amount) }}
                  </span>
                </div>
                <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div
                    class="h-full rounded-full bg-emerald-500 transition-all duration-500"
                    :style="{ width: `${qualificationPercent}%` }"
                  ></div>
                </div>
                <div class="mt-2 flex items-center justify-between gap-4 text-xs">
                  <p class="text-gray-500 dark:text-gray-400">
                    {{ t('checkin.qualificationWindow', { days: status.qualification_days }) }}
                  </p>
                  <span
                    class="flex shrink-0 items-center gap-1 font-medium"
                    :class="rechargeQualified
                      ? 'text-emerald-600 dark:text-emerald-400'
                      : 'text-amber-600 dark:text-amber-400'"
                  >
                    <Icon v-if="rechargeQualified" name="check" size="xs" />
                    {{ rechargeQualified ? t('checkin.qualified') : t('checkin.notQualified') }}
                  </span>
                </div>
                <div v-if="status.next_tier" class="mt-5 border-t border-gray-100 pt-4 dark:border-dark-700">
                  <div class="mb-2 flex items-center justify-between gap-4 text-sm">
                    <span class="min-w-0 truncate font-medium text-gray-700 dark:text-gray-300">
                      {{ t('checkin.nextTierProgress', { name: status.next_tier.name }) }}
                    </span>
                    <span class="flex-none tabular-nums text-gray-500 dark:text-gray-400">
                      {{ formatMoney(status.recharge_amount) }} / {{ formatMoney(status.next_tier.min_recharge_amount) }}
                    </span>
                  </div>
                  <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div
                      class="h-full rounded-full bg-primary-500 transition-all duration-500"
                      :style="{ width: `${nextTierPercent}%` }"
                    ></div>
                  </div>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('checkin.nextTierRemaining', { amount: formatMoney(nextTierRemaining) }) }}
                  </p>
                </div>
              </div>
            </div>

            <div
              class="flex min-h-48 flex-col items-center justify-center border-t border-gray-100 pt-6 text-center dark:border-dark-700 lg:border-l lg:border-t-0 lg:pl-6 lg:pt-0"
              data-test="checkin-claim-panel"
              :data-custom-button="currentButtonTier ? 'true' : 'false'"
            >
              <template v-if="status.claimed_today">
                <div
                  class="flex h-12 w-12 items-center justify-center rounded-full bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400"
                >
                  <Icon name="checkCircle" size="lg" />
                </div>
                <p class="mt-3 text-sm font-medium text-gray-600 dark:text-gray-300">
                  {{ t('checkin.claimedToday') }}
                </p>
                <p class="mt-1 text-2xl font-bold text-emerald-600 dark:text-emerald-400">
                  +{{ formatMoney(status.today_reward) }}
                </p>
              </template>
              <template v-else>
                <span
                  v-if="currentTierBadge"
                  class="mb-2 inline-flex rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
                >
                  {{ currentTierBadge.name }}
                </span>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('checkin.rewardRange') }}
                </p>
                <p class="mt-1 text-xl font-bold text-gray-950 dark:text-white">
                  {{ formatMoney(status.reward_min) }} - {{ formatMoney(status.reward_max) }}
                </p>
                <button
                  type="button"
                  class="btn mt-5 min-w-40"
                  :class="claimButtonClass"
                  data-test="checkin-claim-button"
                  :disabled="!status.eligible || claiming"
                  @click="claimReward"
                >
                  <Icon
                    :name="claiming ? 'refresh' : 'gift'"
                    size="md"
                    class="mr-2"
                    :class="claiming && 'animate-spin'"
                  />
                  {{ claiming ? t('checkin.claiming') : t('checkin.claimButton') }}
                </button>
                <p class="mt-3 max-w-xs text-xs text-gray-500 dark:text-gray-400">
                  {{ reasonText }}
                </p>
              </template>
              <p class="mt-4 text-xs text-gray-400 dark:text-dark-400">
                {{ t('checkin.resetsAt', { time: formatDateTime(status.next_reset_at) }) }}
              </p>
            </div>
          </div>
        </section>

        <section v-if="currentTierOverview || nextTierOverview" class="card overflow-hidden">
          <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
            <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('checkin.rewardTiers') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('checkin.rewardTiersHint') }}</p>
          </div>
          <div class="grid divide-y divide-gray-100 dark:divide-dark-700 md:grid-cols-2 md:divide-x md:divide-y-0">
            <div v-if="currentTierOverview" class="flex min-w-0 items-center gap-3 px-5 py-4 sm:px-6">
              <div class="flex h-9 w-9 flex-none items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-400">
                <Icon name="check" size="sm" :stroke-width="2" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-[11px] font-medium text-emerald-600 dark:text-emerald-400">{{ t('checkin.currentTier') }}</p>
                <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ currentTierOverview.name }}</p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('checkin.tierRechargeRequirement', { amount: formatMoney(currentTierOverview.min_recharge_amount) }) }}
                </p>
              </div>
              <div class="flex-none text-right">
                <p class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
                  {{ formatMoney(currentTierOverview.reward_min) }} - {{ formatMoney(currentTierOverview.reward_max) }}
                </p>
                <p class="mt-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">{{ t('checkin.qualified') }}</p>
              </div>
            </div>

            <div v-if="nextTierOverview" class="flex min-w-0 items-center gap-3 px-5 py-4 sm:px-6">
              <div class="flex h-9 w-9 flex-none items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
                <Icon name="arrowRight" size="sm" :stroke-width="2" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-[11px] font-medium text-primary-600 dark:text-primary-400">{{ t('checkin.nextTier') }}</p>
                <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ nextTierOverview.name }}</p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('checkin.tierRechargeRequirement', { amount: formatMoney(nextTierOverview.min_recharge_amount) }) }}
                </p>
              </div>
              <div class="flex-none text-right">
                <p class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
                  {{ formatMoney(nextTierOverview.reward_min) }} - {{ formatMoney(nextTierOverview.reward_max) }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('checkin.tierRemainingShort', { amount: formatMoney(nextTierRemaining) }) }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <section class="card overflow-hidden">
          <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
            <h2 class="text-base font-semibold text-gray-950 dark:text-white">
              {{ t('checkin.recentHistory') }}
            </h2>
            <button
              type="button"
              class="btn btn-ghost btn-sm"
              :title="t('common.refresh')"
              @click="loadStatus"
            >
              <Icon name="refresh" size="md" :class="loading && 'animate-spin'" />
            </button>
          </div>

          <div v-if="status.history.length" class="overflow-x-auto">
            <table class="w-full min-w-[560px]">
              <thead class="bg-gray-50/80 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                <tr>
                  <th class="px-5 py-3 font-medium sm:px-6">{{ t('checkin.date') }}</th>
                  <th class="px-5 py-3 font-medium">{{ t('checkin.claimedAt') }}</th>
                  <th class="px-5 py-3 text-right font-medium sm:px-6">{{ t('checkin.reward') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in status.history" :key="item.id">
                  <td class="px-5 py-4 text-sm font-medium text-gray-900 dark:text-white sm:px-6">
                    {{ item.business_date }}
                  </td>
                  <td class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">
                    {{ formatDateTime(item.claimed_at) }}
                  </td>
                  <td class="px-5 py-4 text-right text-sm font-semibold text-emerald-600 dark:text-emerald-400 sm:px-6">
                    +{{ formatMoney(item.reward) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="flex flex-col items-center justify-center px-6 py-12 text-center">
            <Icon name="clock" size="xl" class="text-gray-300 dark:text-dark-500" />
            <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">
              {{ t('checkin.noHistory') }}
            </p>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { checkinAPI, type CheckinButtonColor, type CheckinStatus } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const status = ref<CheckinStatus | null>(null)
const loading = ref(false)
const claiming = ref(false)

const qualificationPercent = computed(() => {
  if (!status.value || status.value.min_recharge_amount <= 0) return 0
  return Math.min(100, Math.max(0, (status.value.recharge_amount / status.value.min_recharge_amount) * 100))
})

const rechargeQualified = computed(() => {
  if (!status.value) return false
  return status.value.recharge_amount + 0.0000001 >= status.value.min_recharge_amount
})

const nextTierPercent = computed(() => {
  const nextTier = status.value?.next_tier
  if (!nextTier) return 0
  const currentThreshold = status.value?.current_tier?.min_recharge_amount || status.value?.min_recharge_amount || 0
  const span = nextTier.min_recharge_amount - currentThreshold
  if (span <= 0) return 100
  const progress = ((status.value?.recharge_amount || 0) - currentThreshold) / span
  return Math.min(100, Math.max(0, progress * 100))
})

const nextTierRemaining = computed(() => {
  const nextTier = status.value?.next_tier
  if (!nextTier) return 0
  return Math.max(0, nextTier.min_recharge_amount - (status.value?.recharge_amount || 0))
})

const currentTierBadge = computed(() => {
  const tier = status.value?.current_tier
  if (!tier || tier.is_default || !tier.tier_badge_enabled) return null
  return tier
})

const currentTierOverview = computed(() => {
  const tier = status.value?.current_tier
  return tier?.visible ? tier : null
})

const nextTierOverview = computed(() => {
  const tier = status.value?.next_tier
  return tier?.visible ? tier : null
})

const currentButtonTier = computed(() => {
  const tier = status.value?.current_tier
  if (!tier || tier.is_default || !tier.custom_button_enabled) return null
  return tier
})

const tierButtonClassMap: Record<CheckinButtonColor, string> = {
  emerald: 'bg-gradient-to-r from-emerald-400 to-teal-500 text-white shadow-md shadow-emerald-500/20 hover:from-emerald-500 hover:to-teal-600 focus:ring-emerald-400/50',
  blue: 'bg-gradient-to-r from-sky-400 to-blue-500 text-white shadow-md shadow-blue-500/20 hover:from-sky-500 hover:to-blue-600 focus:ring-blue-400/50',
  amber: 'bg-gradient-to-r from-amber-400 to-orange-500 text-white shadow-md shadow-orange-500/20 hover:from-amber-500 hover:to-orange-600 focus:ring-orange-400/50',
  rose: 'bg-gradient-to-r from-rose-400 to-red-500 text-white shadow-md shadow-red-500/20 hover:from-rose-500 hover:to-red-600 focus:ring-red-400/50',
  violet: 'bg-gradient-to-r from-violet-400 to-indigo-500 text-white shadow-md shadow-indigo-500/20 hover:from-violet-500 hover:to-indigo-600 focus:ring-violet-400/50',
  slate: 'bg-gradient-to-r from-slate-400 to-slate-600 text-white shadow-md shadow-slate-500/20 hover:from-slate-500 hover:to-slate-700 focus:ring-slate-400/50'
}

const claimButtonClass = computed(() => {
  const tier = currentButtonTier.value
  return tier ? tierButtonClassMap[tier.button_color] : 'btn-primary'
})

const reasonText = computed(() => {
  const reason = status.value?.reason || 'disabled'
  return t(`checkin.reasons.${reason}`, {
    amount: formatMoney(status.value?.min_recharge_amount || 0),
    hours: status.value?.min_account_age_hours || 0
  })
})

const formatMoney = (value: number) => `$${Number(value || 0).toFixed(2)}`

const loadStatus = async () => {
  loading.value = true
  try {
    status.value = await checkinAPI.getStatus()
  } catch (error: any) {
    appStore.showError(error?.message || t('checkin.loadFailed'))
  } finally {
    loading.value = false
  }
}

const claimReward = async () => {
  if (!status.value?.eligible || claiming.value) return
  claiming.value = true
  try {
    const result = await checkinAPI.claim()
    appStore.showSuccess(t('checkin.claimSuccess', { amount: formatMoney(result.reward) }))
    await Promise.all([loadStatus(), authStore.refreshUser()])
  } catch (error: any) {
    appStore.showError(error?.message || t('checkin.claimFailed'))
    await loadStatus()
  } finally {
    claiming.value = false
  }
}

onMounted(loadStatus)
</script>
