<template>
  <span class="inline-flex max-w-full items-center gap-1 whitespace-nowrap rounded-md border px-1.5 py-0.5 text-[11px] tabular-nums" :class="known ? outdated ? 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-500/10 dark:text-amber-400' : 'border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300' : 'border-gray-100 text-gray-400 dark:border-dark-700 dark:text-dark-400'" :title="detail">
    <span v-if="isNewapiBilling(billing)" class="mr-0.5 shrink-0 border-r border-current/20 pr-1.5 text-[10px] font-medium" data-testid="newapi-badge">New API</span>
    <span v-if="showGroupName && groupName" class="max-w-[12rem] truncate border-r border-current/20 pr-1.5 font-medium" data-testid="upstream-billing-group">{{ groupName }}</span>
    {{ t('upstreamCenter.billing.rate') }} <strong class="shrink-0 font-medium">{{ known ? `${billing!.effective_rate_multiplier}×` : automaticGroup ? t('upstreamCenter.newapi.dynamicRate') : '—' }}</strong><Icon v-if="outdated" name="clock" size="xs" />
  </span>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UpstreamBillingSnapshot } from '@/api/admin/upstreamCenter'
import { dateTime } from './format'
import { isNewapiBilling, upstreamSyncError } from './newapi'
const props = defineProps<{ billing?: UpstreamBillingSnapshot | null; showGroupName?: boolean }>()
const { t } = useI18n()
const groupName = computed(() => props.billing?.group_name?.trim())
const known = computed(() => props.billing?.effective_rate_multiplier != null)
const automaticGroup = computed(() => isNewapiBilling(props.billing) && props.billing?.error?.includes('newapi_auto_group'))
const outdated = computed(() => !!props.billing && (props.billing.stale || props.billing.status === 'error'))
const detail = computed(() => {
  const group = groupName.value ? `${t('upstreamCenter.newapi.remoteGroup')}: ${groupName.value}\n` : ''
  if (!props.billing || !known.value) return group + (props.billing?.error ? upstreamSyncError(props.billing.error, t) : t('upstreamCenter.billing.unavailable'))
  const value = (number: number | null) => number == null ? '—' : `${number}×`
  return `${group}${t('upstreamCenter.billing.effective')}: ${value(props.billing.effective_rate_multiplier)}\n${t('upstreamCenter.billing.group')}: ${value(props.billing.group_rate_multiplier)}\n${t('upstreamCenter.billing.user')}: ${value(props.billing.user_rate_multiplier)}\n${t('upstreamCenter.billing.base')}: ${value(props.billing.resolved_rate_multiplier)}\n${t('upstreamCenter.wallet.syncedAt', { time: dateTime(props.billing.synced_at) })}${outdated.value ? `\n${t('upstreamCenter.billing.stale')}` : ''}${props.billing.error ? `\n${upstreamSyncError(props.billing.error, t)}` : ''}`
})
</script>
