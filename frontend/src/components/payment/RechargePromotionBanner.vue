<template>
  <section v-if="visible && promotion" class="promotion-tiers-panel min-w-0 rounded-xl border p-3 sm:p-3.5" data-testid="recharge-promotion">
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
      <div class="flex min-w-0 items-center gap-2">
        <Icon name="sparkles" size="sm" class="shrink-0 text-[#b18a42] dark:text-[#d0aa62]" aria-hidden="true" />
        <h3 class="min-w-0 break-words text-sm font-semibold text-[#785018] dark:text-[#ead7ae]">{{ promotion.title }}</h3>
      </div>
      <span class="text-xs font-medium text-[#926322] dark:text-[#d5b87c]">{{ tp('promotion.upTo', { percent: highestBonus }) }}</span>
    </div>

    <div class="mt-2.5 flex flex-wrap gap-2" data-testid="promotion-tiers">
      <component
        :is="selectable ? 'button' : 'span'"
        v-for="tier in tiers"
        :key="tier.min_amount"
        :type="selectable ? 'button' : undefined"
        :disabled="selectable && (currencyMismatch || disabledTiers.includes(tier.min_amount))"
        :aria-pressed="selectable ? selectedTier?.min_amount === tier.min_amount : undefined"
        :title="disabledTiers.includes(tier.min_amount) ? tp('promotion.unavailable') : tp('promotion.threshold', { amount: formatAmount(tier.min_amount) })"
        class="promotion-tier inline-flex max-w-full flex-wrap items-center gap-x-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-600 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-40 dark:focus-visible:ring-offset-dark-800"
        :class="selectedTier?.min_amount === tier.min_amount
          ? 'promotion-tier--selected'
          : ''"
        :data-tier="tier.min_amount"
        @click="selectTier(tier.min_amount)"
      >
        <span>{{ tp('promotion.threshold', { amount: formatAmount(tier.min_amount) }) }}</span>
        <span class="font-semibold">+{{ tier.bonus_percent }}%</span>
      </component>
    </div>

    <p v-if="currencyMismatch" class="mt-2.5 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
      {{ tp('promotion.currencyMismatch', { currency: selectedCurrency, promotionCurrency: promotion.currency }) }}
    </p>
    <details class="mt-2.5 text-xs text-gray-500 dark:text-gray-400">
      <summary class="w-fit cursor-pointer select-none rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500">{{ tp('promotion.details') }}</summary>
      <div class="mt-2 space-y-1 leading-relaxed">
        <p v-if="promotion.subtitle" class="break-words">{{ promotion.subtitle }}</p>
        <p>{{ tp('promotion.ends', { date: endDate }) }} · {{ tp('promotion.currencyOnly', { currency: promotion.currency }) }}</p>
        <p>{{ tp('promotion.rule') }}<span v-if="promotion.max_bonus > 0"> · {{ tp('promotion.cap', { amount: promotion.max_bonus.toFixed(2) }) }}</span></p>
      </div>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { RechargePromotion } from '@/types/payment'
import { formatPaymentAmount, normalizePaymentCurrency } from './currency'
import { rechargePromotionMessages } from './rechargePromotionMessages'
import { isRechargePromotionActive, sortedPromotionTiers, useRechargePromotionClock } from '@/utils/rechargePromotion'

const props = withDefaults(defineProps<{
  promotion?: RechargePromotion | null
  amount?: number
  selectable?: boolean
  selectedCurrency?: string
  disabledTiers?: number[]
}>(), { amount: 0, selectable: false, selectedCurrency: '', disabledTiers: () => [] })
const emit = defineEmits<{ select: [amount: number] }>()
const { t: tp, locale } = useI18n({ useScope: 'local', messages: rechargePromotionMessages })
const now = useRechargePromotionClock()
const visible = computed(() => isRechargePromotionActive(props.promotion, now.value))
const tiers = computed(() => props.promotion ? sortedPromotionTiers(props.promotion) : [])
const highestBonus = computed(() => Math.max(0, ...tiers.value.map(tier => tier.bonus_percent)))
const currencyMismatch = computed(() => !!props.selectedCurrency && normalizePaymentCurrency(props.selectedCurrency) !== normalizePaymentCurrency(props.promotion?.currency))
const selectedTier = computed(() => currencyMismatch.value ? undefined : tiers.value.filter(tier => props.amount >= tier.min_amount).slice(-1)[0])
const endDate = computed(() => new Intl.DateTimeFormat(locale.value || undefined, { timeZone: 'Asia/Shanghai', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(props.promotion?.ends_at || now.value)))
function formatAmount(value: number) { return formatPaymentAmount(value, props.promotion?.currency, locale.value) }
function selectTier(amount: number) {
  if (props.selectable && !currencyMismatch.value && !props.disabledTiers.includes(amount)) emit('select', amount)
}
</script>

<style scoped>
.promotion-tiers-panel { border-color: #ead8ad; background: linear-gradient(115deg, #fffdf8, #faf1dc 72%, #fcf7ea); box-shadow: inset 0 1px 0 #ffffffb3; }
.promotion-tier { border-color: #e7d8b7; background: #ffffffa3; color: #926322; }
button.promotion-tier:enabled:hover { border-color: #cba65c; background: #fff9eb; }
.promotion-tier--selected { border-color: #c49a46; background: linear-gradient(115deg, #f8e9c0, #f3dfad); color: #785018; box-shadow: inset 0 1px 0 #ffffff80; }
.dark .promotion-tiers-panel { border-color: #76603a66; background: linear-gradient(115deg, #302a21, #393022 72%, #2c2822); box-shadow: inset 0 1px 0 #f8df9f08; }
.dark .promotion-tier { border-color: #7d63364d; background: #d8b26708; color: #d5b87c; }
.dark button.promotion-tier:enabled:hover { border-color: #9c7b43; background: #d8b26714; }
.dark .promotion-tier--selected { border-color: #b58e46; background: linear-gradient(115deg, #5a4526, #68502b); color: #f0ddb2; box-shadow: inset 0 1px 0 #f8df9f12; }
</style>
