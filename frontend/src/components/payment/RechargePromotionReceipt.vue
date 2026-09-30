<template>
  <div v-if="snapshot && snapshot.bonus_amount > 0" class="rounded-xl border border-amber-200/70 bg-amber-50/80 px-4 py-3 text-sm dark:border-amber-800/50 dark:bg-amber-950/20" data-testid="promotion-receipt">
    <p class="break-words font-semibold text-amber-900 dark:text-amber-200">{{ snapshot.title }}</p>
    <div class="mt-2 flex flex-wrap justify-between gap-x-4 gap-y-1 text-amber-800 dark:text-amber-300">
      <span>{{ tp('base') }} <strong class="tabular-nums">${{ snapshot.base_amount.toFixed(2) }}</strong></span>
      <span>{{ tp('bonus') }} <strong class="tabular-nums">+${{ snapshot.bonus_amount.toFixed(2) }}</strong></span>
    </div>
    <p class="mt-2 text-xs leading-relaxed text-amber-700 dark:text-amber-400">{{ tp('included') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RechargePromotionSnapshot } from '@/types/payment'

defineProps<{ snapshot?: RechargePromotionSnapshot | null }>()

const { locale } = useI18n()
const messages = {
  zh: { base: '基础额度', bonus: '活动加赠', included: '赠送已计入本单总到账额度，以订单记录为准。' },
  en: { base: 'Base credits', bonus: 'Promotion bonus', included: 'The bonus is included in this order’s total credits and follows its saved terms.' },
}
const tp = (key: keyof typeof messages.en) => messages[locale?.value?.startsWith('zh') ? 'zh' : 'en'][key]
</script>
