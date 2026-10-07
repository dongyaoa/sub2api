<template>
  <div v-if="summary && (summary.cost_partial || summary.archived_key_count || summary.inactive_key_count || summary.conversion_applied)" class="space-y-1 text-[11px] leading-5">
    <p v-if="summary.cost_partial" class="text-amber-600 dark:text-amber-400" data-testid="finance-partial">{{ t('upstreamCenter.finance.partialHint', { known: summary.known_key_count ?? 0, missing: summary.missing_key_count ?? 0 }) }}</p>
    <p v-if="summary.inactive_key_count" class="text-gray-500 dark:text-dark-400" data-testid="finance-inactive">{{ t('upstreamCenter.finance.inactiveScope', { count: summary.inactive_key_count }) }}</p>
    <p v-if="summary.conversion_applied" class="text-gray-500 dark:text-dark-400" data-testid="finance-converted">{{ t('upstreamCenter.recharge.converted') }}<template v-if="summary.remote_raw_used != null"> · {{ t('upstreamCenter.recharge.rawUsage', { amount: amount(summary.remote_raw_used) }) }}</template></p>
    <p v-if="summary.archived_key_count" class="text-gray-400 dark:text-dark-400" data-testid="finance-archived">{{ t('upstreamCenter.finance.archivedScope', { count: summary.archived_key_count }) }}</p>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UpstreamFinanceSummary } from '@/api/admin/upstreamCenter'
import { amount } from './format'
defineProps<{ summary: UpstreamFinanceSummary | null | undefined }>()
const { t } = useI18n()
</script>
