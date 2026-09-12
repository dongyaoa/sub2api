<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import type { AccountRecentRequest } from '@/api/admin/accounts'

const props = withDefaults(defineProps<{
  requests?: AccountRecentRequest[] | null
  loading?: boolean
  error?: boolean
  maxItems?: number
}>(), {
  requests: () => [],
  loading: false,
  error: false,
  maxItems: 10
})

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const entries = computed(() => (props.requests ?? []).slice(0, props.maxItems))
// The API is newest first; the strip reads oldest to newest, with unfilled
// slots on the left so newly completed requests always enter at the right.
const displayedEntries = computed(() => [...entries.value].reverse())
const emptySlots = computed(() => Math.max(0, props.maxItems - entries.value.length))

const timeOf = (request: AccountRecentRequest | undefined): Date | null => {
  if (!request) return null
  const raw = request.created_at ?? request.timestamp
  if (!raw) return null
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? null : date
}

const latestTime = computed(() => timeOf(entries.value[0]))

const formatCompactTime = (date: Date | null): string => {
  if (!date) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const statusCodeOf = (request: AccountRecentRequest): string => {
  const value = request.status_code ?? request.status
  return value === null || value === undefined || value === '' || value === 0 ? '' : String(value)
}

const isSuccess = (request: AccountRecentRequest): boolean | null => {
  if (typeof request.success === 'boolean') return request.success
  if (typeof request.ok === 'boolean') return request.ok
  const status = Number(statusCodeOf(request))
  if (Number.isFinite(status) && status > 0) return status >= 200 && status < 400
  return null
}

const barClass = (request: AccountRecentRequest): string => {
  const result = isSuccess(request)
  if (result === true) return 'recent-request-bar--success'
  if (result === false) return 'recent-request-bar--failure'
  return 'recent-request-bar--unknown'
}

const errorMessageOf = (request: AccountRecentRequest): string =>
  String(request.error_message ?? request.error ?? '').trim()

const detailTime = (request: AccountRecentRequest): string => {
  const date = timeOf(request)
  return date ? formatDateTime(date) : t('admin.accounts.recentRequests.unknownTime')
}

const proxyLabelOf = (request: AccountRecentRequest): string => {
  const name = request.proxy_name?.trim() ?? ''
  if (!request.proxy_id) {
    if (name === 'direct' || name === 'no_proxy') return t('admin.accounts.testProxyRoute.direct')
    if (name === 'unknown') return t('admin.accounts.testProxyRoute.unknown')
  }
  return name || (request.proxy_id ? `#${request.proxy_id}` : '')
}

const copyDetails = (request: AccountRecentRequest) => {
  const lines = [detailTime(request)]
  const statusCode = statusCodeOf(request)
  if (statusCode) lines.push(`HTTP ${statusCode}`)
  if (request.account_name) lines.push(`${t('admin.accounts.recentRequests.account')}: ${request.account_name}`)
  if (request.model) lines.push(`${t('admin.accounts.recentRequests.model')}: ${request.model}`)
  if (request.proxy_name || request.proxy_id) {
    lines.push(`${t('admin.accounts.recentRequests.proxy')}: ${proxyLabelOf(request)}`)
  }
  const message = errorMessageOf(request)
  if (message) lines.push(message)
  void copyToClipboard(lines.join('\n'))
}
</script>

<template>
  <div class="recent-requests-cell" :aria-busy="loading || undefined">
    <div v-if="latestTime" class="recent-requests-time-row">
      <span
        class="recent-requests-time text-gray-500 dark:text-gray-400"
        data-testid="recent-request-time"
      >{{ formatCompactTime(latestTime) }}</span>
    </div>
      <div class="recent-requests-strip" :aria-label="t('admin.accounts.recentRequests.ariaLabel')" data-testid="recent-request-strip">
      <HelpTooltip v-if="error" class="recent-requests-error !ml-0" :content="t('admin.accounts.recentRequests.loadFailed')">
        <template #trigger>
          <button
            type="button"
            class="inline-flex text-amber-500 dark:text-amber-400"
            :aria-label="t('admin.accounts.recentRequests.loadFailed')"
            data-testid="recent-request-load-error"
          ><Icon name="exclamationCircle" size="sm" /></button>
        </template>
      </HelpTooltip>
        <span
          v-for="index in emptySlots"
          :key="`empty-${index}`"
          class="recent-request-bar recent-request-bar--empty"
          data-testid="recent-request-placeholder"
          aria-hidden="true"
        />
        <HelpTooltip
          v-for="(request, index) in displayedEntries"
          :key="request.id ?? `${request.created_at ?? request.timestamp ?? 'request'}-${index}`"
          class="!ml-0"
          width-class="w-72 max-w-[calc(100vw-2rem)]"
        >
          <template #trigger>
            <span
              class="recent-request-bar cursor-help transition-[transform,filter] duration-150 hover:scale-y-110 hover:brightness-105"
              :class="barClass(request)"
              data-testid="recent-request-bar"
              :title="statusCodeOf(request) || t('admin.accounts.recentRequests.unknownStatus')"
            />
          </template>
          <div class="space-y-1">
            <div class="flex items-start justify-between gap-3">
              <span class="font-medium text-white">{{ detailTime(request) }}</span>
              <button
                type="button"
                class="rounded p-0.5 text-gray-300 hover:bg-white/10 hover:text-white"
                :aria-label="t('admin.accounts.recentRequests.copyDetails')"
                @click.stop="copyDetails(request)"
              >
                <Icon name="copy" size="sm" />
              </button>
            </div>
            <div
              v-if="statusCodeOf(request)"
              class="font-semibold"
              :class="isSuccess(request) === false ? 'text-rose-400' : isSuccess(request) === true ? 'text-emerald-400' : 'text-amber-300'"
            >
              HTTP {{ statusCodeOf(request) }}
            </div>
            <div v-if="request.model">
              {{ t('admin.accounts.recentRequests.model') }}: {{ request.model }}
            </div>
            <div v-if="request.account_name" class="break-words">
              {{ t('admin.accounts.recentRequests.account') }}: {{ request.account_name }}
            </div>
            <div v-if="request.proxy_name || request.proxy_id">
              {{ t('admin.accounts.recentRequests.proxy') }}: {{ proxyLabelOf(request) }}
            </div>
            <div v-if="errorMessageOf(request)" class="break-words text-rose-200">
              {{ errorMessageOf(request) }}
            </div>
            <div v-if="request.attempt_count && request.attempt_count > 1">
              {{ t('admin.accounts.recentRequests.attempts', { count: request.attempt_count }) }}
            </div>
            <div v-if="!statusCodeOf(request) && !errorMessageOf(request)" class="text-gray-300">
              {{ t('admin.accounts.recentRequests.noDetails') }}
            </div>
          </div>
        </HelpTooltip>
      </div>
  </div>
</template>

<style scoped>
.recent-requests-cell {
  display: flex;
  min-width: 108px;
  height: 38px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 5px;
}

.recent-requests-time-row {
  display: flex;
  align-items: center;
  height: 16px;
}

.recent-requests-time {
  font-size: 12px;
  line-height: 16px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  white-space: nowrap;
}

.recent-requests-strip {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 16px;
  gap: 4px;
}

.recent-requests-error {
  position: absolute;
  left: calc(100% + 5px);
}

.recent-request-bar {
  display: inline-block;
  width: 5px;
  height: 16px;
  flex-shrink: 0;
  border-radius: 2.5px;
}

.recent-request-bar--empty {
  background: linear-gradient(180deg, #e8eaf0, #e3e6ed);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 35%);
}

.recent-request-bar--success {
  background: linear-gradient(180deg, #27d96c, #19c55d);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 20%), 0 1px 2px rgb(22 163 74 / 12%);
}

.recent-request-bar--failure {
  background: linear-gradient(180deg, #fb595e, #ef3d45);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 20%), 0 1px 2px rgb(220 38 38 / 12%);
}

.recent-request-bar--unknown {
  background: linear-gradient(180deg, #fbc94c, #f2b82b);
}

.dark .recent-request-bar--empty {
  background: linear-gradient(180deg, #465367, #3b475a);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 5%);
}

@media (prefers-reduced-motion: reduce) {
  .recent-request-bar {
    animation: none;
    transition: none;
  }
}
</style>
