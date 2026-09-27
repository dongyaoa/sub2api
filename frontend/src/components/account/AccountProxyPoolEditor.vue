<template>
  <div class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600" data-testid="account-proxy-pool">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <label class="input-label mb-0">{{ t('admin.accounts.proxyPool.title') }}</label>
        <p class="input-hint mt-1">{{ t('admin.accounts.proxyPool.hint') }}</p>
      </div>
      <span class="shrink-0 rounded-md bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">
        {{ t('admin.accounts.proxyPool.selectedCount', { count: entries.length }) }}
      </span>
    </div>

    <div class="space-y-3 rounded-xl bg-gray-50 p-3 dark:bg-dark-700/50">
      <div class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem_auto] sm:items-end">
        <div class="min-w-0">
          <label class="input-label">{{ t('admin.accounts.proxyPool.selectProxy') }}</label>
          <ProxySelector
            v-model="selectedProxyId"
            :proxies="availableProxies"
            :allow-none="false"
            :show-availability="true"
            :placeholder="t('admin.accounts.proxyPool.selectProxy')"
            :aria-label="t('admin.accounts.proxyPool.selectProxy')"
            :disabled="availableProxies.length === 0 || entries.length >= MAX_POOL_SIZE"
          />
        </div>
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.accounts.proxyPool.newConcurrency') }}</span>
          <input
            v-model.number="newConcurrency"
            type="number"
            min="1"
            :max="MAX_TOTAL_CONCURRENCY"
            step="1"
            class="input"
            data-testid="proxy-pool-new-concurrency"
            @change="newConcurrency = normalizeValue(newConcurrency)"
          />
        </label>
        <button
          type="button"
          class="btn btn-primary justify-center whitespace-nowrap"
          :disabled="!canAddSelected"
          data-testid="proxy-pool-add"
          @click="addEntry"
        >
          <Icon name="plus" size="sm" />
          <span>{{ t('admin.accounts.proxyPool.add') }}</span>
        </button>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="input-hint mt-0">{{ t('admin.accounts.proxyPool.defaultConcurrencyHint', { count: DEFAULT_CONCURRENCY }) }}</p>
        <button
          type="button"
          class="text-xs font-medium text-primary-600 hover:text-primary-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:text-primary-300"
          :disabled="!canAddAll"
          data-testid="proxy-pool-add-all"
          @click="addAll"
        >
          {{ t('admin.accounts.proxyPool.addAll', { count: availableProxies.length }) }}
        </button>
      </div>
      <p v-if="exceedsPoolLimits" class="input-hint mt-0 text-amber-600 dark:text-amber-400" role="status">
        {{ t('admin.accounts.proxyPool.limitHint', { count: MAX_POOL_SIZE, concurrency: MAX_TOTAL_CONCURRENCY }) }}
      </p>
    </div>

    <div v-if="entries.length" class="space-y-3">
      <div
        v-for="(entry, index) in entries"
        :key="`${entry.proxy_id}-${index}`"
        class="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2 rounded-lg border border-gray-100 p-3 dark:border-dark-600 sm:grid-cols-[minmax(0,1fr)_8rem_auto]"
        data-testid="proxy-pool-entry"
      >
        <label class="col-span-2 min-w-0 sm:col-span-1">
          <span class="input-label">{{ t('admin.accounts.proxyPool.proxyNumber', { index: index + 1 }) }}</span>
          <select v-model.number="entry.proxy_id" class="input" @change="emitChange">
            <option v-if="!selectableProxies(index).some(proxy => proxy.id === entry.proxy_id)" :value="entry.proxy_id" disabled>
              {{ t('admin.accounts.proxyPool.unavailableProxy', { id: entry.proxy_id }) }}
            </option>
            <option
              v-for="proxy in selectableProxies(index)"
              :key="proxy.id"
              :value="proxy.id"
            >
              {{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})
            </option>
          </select>
        </label>
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.accounts.proxyPool.concurrency') }}</span>
          <input
            v-model.number="entry.concurrency"
            type="number"
            min="1"
            :max="maxEntryConcurrency(entry)"
            step="1"
            class="input"
            @input="normalizeConcurrency(entry)"
          />
        </label>
        <button
          type="button"
          class="btn btn-icon mb-0.5 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
          :title="t('admin.accounts.proxyPool.remove')"
          :aria-label="t('admin.accounts.proxyPool.remove')"
          @click="removeEntry(index)"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.proxyPool.empty') }}
    </p>

    <div class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 text-xs dark:border-dark-600">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.proxyPool.remainingCount', { count: availableProxies.length }) }}</span>
      <div class="flex items-center gap-2">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.proxyPool.total') }}</span>
        <span class="text-sm font-semibold text-gray-900 dark:text-gray-100" data-testid="proxy-pool-total">{{ totalConcurrency }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import type { AccountProxyPoolEntry, Proxy } from '@/types'

const { t } = useI18n()

const props = defineProps<{
  modelValue: AccountProxyPoolEntry[]
  proxies: Proxy[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: AccountProxyPoolEntry[]]
}>()

const localEntries = ref<AccountProxyPoolEntry[]>([])
const DEFAULT_CONCURRENCY = 20
const MAX_POOL_SIZE = 256
const MAX_TOTAL_CONCURRENCY = 100000
const selectedProxyId = ref<number | null>(null)
const newConcurrency = ref(DEFAULT_CONCURRENCY)
watch(
  () => props.modelValue,
  (value) => {
    localEntries.value = value.map((entry) => ({ ...entry }))
  },
  { immediate: true, deep: true }
)

const entries = computed(() => localEntries.value)
const totalConcurrency = computed(() =>
  entries.value.reduce((total, entry) => total + Math.max(0, Number(entry.concurrency) || 0), 0)
)
const isAvailable = (proxy: Proxy) =>
  proxy.status === 'active' && (!proxy.expires_at || new Date(proxy.expires_at).getTime() > Date.now())

const availableProxies = computed(() => {
  const used = new Set(entries.value.map((entry) => entry.proxy_id))
  return props.proxies.filter((proxy) => !used.has(proxy.id) && isAvailable(proxy))
})

watch(availableProxies, (proxies) => {
  if (!proxies.some((proxy) => proxy.id === selectedProxyId.value)) selectedProxyId.value = null
})

const normalizeValue = (value: number) =>
  Math.min(MAX_TOTAL_CONCURRENCY, Math.max(1, Math.trunc(Number(value) || 1)))

const fitsLimits = (count: number) =>
  entries.value.length + count <= MAX_POOL_SIZE &&
  totalConcurrency.value + count * normalizeValue(newConcurrency.value) <= MAX_TOTAL_CONCURRENCY

const canAddSelected = computed(() =>
  availableProxies.value.some((proxy) => proxy.id === selectedProxyId.value) && fitsLimits(1)
)
const canAddAll = computed(() => availableProxies.value.length > 0 && fitsLimits(availableProxies.value.length))
const exceedsPoolLimits = computed(() => availableProxies.value.length > 0 && !fitsLimits(availableProxies.value.length))

const emitChange = () => emit('update:modelValue', entries.value.map((entry) => ({ ...entry })))

const appendProxies = (proxies: Proxy[]) => {
  newConcurrency.value = normalizeValue(newConcurrency.value)
  localEntries.value = [
    ...entries.value,
    ...proxies.map((proxy) => ({ proxy_id: proxy.id, concurrency: newConcurrency.value }))
  ]
  selectedProxyId.value = null
  emitChange()
}

const addEntry = () => {
  if (!canAddSelected.value) return
  const proxy = availableProxies.value.find((proxy) => proxy.id === selectedProxyId.value)
  if (proxy && isAvailable(proxy)) appendProxies([proxy])
}

const addAll = () => {
  if (!canAddAll.value) return
  appendProxies(availableProxies.value.filter(isAvailable))
}

const removeEntry = (index: number) => {
  localEntries.value = entries.value.filter((_, current) => current !== index)
  emitChange()
}

const maxEntryConcurrency = (entry: AccountProxyPoolEntry) =>
  Math.max(1, MAX_TOTAL_CONCURRENCY - entries.value.reduce((total, current) =>
    total + (current === entry ? 0 : Math.max(1, Number(current.concurrency) || 1)), 0))

const normalizeConcurrency = (entry: AccountProxyPoolEntry) => {
  entry.concurrency = Math.min(maxEntryConcurrency(entry), normalizeValue(entry.concurrency))
  emitChange()
}

const selectableProxies = (index: number) => {
  const selectedElsewhere = new Set(
    entries.value.filter((_, current) => current !== index).map((entry) => entry.proxy_id)
  )
  const current = entries.value[index]
  const candidates = props.proxies.filter((proxy) =>
    !selectedElsewhere.has(proxy.id) && (proxy.id === current.proxy_id || isAvailable(proxy))
  )
  if (current.proxy?.id === current.proxy_id && !candidates.some((proxy) => proxy.id === current.proxy_id)) {
    candidates.unshift(current.proxy)
  }
  return candidates
}
</script>
