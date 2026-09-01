<template>
  <div class="space-y-2 rounded-md border border-gray-200 p-3 dark:border-dark-600">
    <div class="flex items-center justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.proxyPool.title') }}</label>
        <p class="input-hint mt-1">{{ t('admin.accounts.proxyPool.hint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary flex-shrink-0" @click="addEntry">
        <Icon name="plus" size="sm" />
        <span>{{ t('admin.accounts.proxyPool.add') }}</span>
      </button>
    </div>

    <div v-if="entries.length" class="space-y-2">
      <div
        v-for="(entry, index) in entries"
        :key="`${entry.proxy_id}-${index}`"
        class="grid grid-cols-[minmax(0,1fr)_7rem_auto] items-end gap-2"
      >
        <div>
          <label class="sr-only">{{ t('admin.accounts.proxyPool.proxy') }}</label>
          <select v-model.number="entry.proxy_id" class="input" @change="emitChange">
            <option :value="0">{{ t('admin.accounts.proxyPool.selectProxy') }}</option>
            <option
              v-for="proxy in selectableProxies(index)"
              :key="proxy.id"
              :value="proxy.id"
            >
              {{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})
            </option>
          </select>
        </div>
        <div>
          <label class="sr-only">{{ t('admin.accounts.proxyPool.concurrency') }}</label>
          <input
            v-model.number="entry.concurrency"
            type="number"
            min="1"
            class="input"
            @input="normalizeConcurrency(entry)"
          />
        </div>
        <button
          type="button"
          class="btn btn-icon text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
          :title="t('admin.accounts.proxyPool.remove')"
          @click="removeEntry(index)"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.proxyPool.empty') }}
    </p>

    <div v-if="entries.length" class="flex items-center justify-between border-t border-gray-100 pt-2 text-xs dark:border-dark-600">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.proxyPool.total') }}</span>
      <span class="font-semibold text-gray-900 dark:text-gray-100">{{ totalConcurrency }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
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

const emitChange = () => emit('update:modelValue', entries.value.map((entry) => ({ ...entry })))

const addEntry = () => {
  const used = new Set(entries.value.map((entry) => entry.proxy_id))
  const first = props.proxies.find((proxy) => !used.has(proxy.id))
  if (!first) return
  emit('update:modelValue', [
    ...entries.value.map((entry) => ({ ...entry })),
    { proxy_id: first.id, concurrency: 1 }
  ])
}

const removeEntry = (index: number) => {
  emit('update:modelValue', entries.value.filter((_, current) => current !== index).map((entry) => ({ ...entry })))
}

const normalizeConcurrency = (entry: AccountProxyPoolEntry) => {
  entry.concurrency = Math.max(1, Math.trunc(Number(entry.concurrency) || 1))
  emitChange()
}

const selectableProxies = (index: number) => {
  const selectedElsewhere = new Set(
    entries.value.filter((_, current) => current !== index).map((entry) => entry.proxy_id)
  )
  return props.proxies.filter((proxy) => !selectedElsewhere.has(proxy.id))
}
</script>
