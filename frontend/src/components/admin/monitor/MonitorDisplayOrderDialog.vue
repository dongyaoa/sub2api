<template>
  <BaseDialog
    :show="show"
    :title="t('channelStatus.order.title')"
    width="wide"
    :close-on-escape="!saving"
    :show-close-button="!saving"
    @close="close"
  >
    <p class="text-sm leading-relaxed text-gray-600 dark:text-gray-300">
      {{ t('channelStatus.order.description') }}
    </p>
    <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
      {{ t('channelStatus.order.newItemsHint') }}
    </p>

    <div v-if="loading" role="status" class="flex items-center justify-center gap-2 py-12 text-sm text-gray-500">
      <Icon name="refresh" size="sm" class="animate-spin" />
      {{ t('common.loading') }}
    </div>
    <div v-else-if="loadError" role="alert" class="mt-5 rounded-2xl bg-red-50 p-4 dark:bg-red-500/10">
      <p class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
      <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadOrder">
        <Icon name="refresh" size="sm" class="mr-2" />
        {{ t('common.refresh') }}
      </button>
    </div>
    <VueDraggable
      v-else-if="loaded"
      v-model="groups"
      class="mt-5 space-y-4"
      handle=".monitor-group-handle"
      draggable=".monitor-order-group"
      :animation="180"
      :disabled="saving"
      ghost-class="monitor-order-ghost"
      :group="{ name: 'monitor-platform-groups', pull: false, put: false }"
    >
      <section
        v-for="(group, groupIndex) in groups"
        :key="group.key"
        :data-order-group="group.key"
        class="monitor-order-group overflow-hidden rounded-2xl border border-gray-200/80 bg-gray-50/60 dark:border-dark-600 dark:bg-dark-800/60"
      >
        <div class="flex items-center gap-2 border-b border-gray-200/70 px-3 py-2.5 dark:border-dark-600">
          <span
            class="monitor-group-handle drag-handle"
            :title="t('channelStatus.order.dragGroup')"
            aria-hidden="true"
          ><Icon name="menu" size="sm" /></span>
          <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-white shadow-sm dark:bg-dark-700">
            <Icon v-if="group.key === 'other'" name="grid" size="sm" class="text-violet-500 dark:text-violet-400" />
            <ProviderIcon v-else :provider="group.key" :size="19" :class="providerTintClass(group.key)" />
          </span>
          <h4 class="min-w-0 flex-1 text-sm font-semibold text-gray-800 dark:text-gray-100">
            {{ groupLabel(group.key) }}
            <span class="ml-1 text-xs font-normal text-gray-400">{{ group.items.length }}</span>
          </h4>
          <button
            type="button"
            class="order-button"
            data-move-group="up"
            :disabled="saving || groupIndex === 0"
            :aria-label="`${t('channelStatus.order.moveUp')}: ${groupLabel(group.key)}`"
            :title="t('channelStatus.order.moveUp')"
            @click="move(groups, groupIndex, -1)"
          ><Icon name="chevronUp" size="sm" /></button>
          <button
            type="button"
            class="order-button"
            data-move-group="down"
            :disabled="saving || groupIndex === groups.length - 1"
            :aria-label="`${t('channelStatus.order.moveDown')}: ${groupLabel(group.key)}`"
            :title="t('channelStatus.order.moveDown')"
            @click="move(groups, groupIndex, 1)"
          ><Icon name="chevronDown" size="sm" /></button>
        </div>
        <VueDraggable
          v-model="group.items"
          class="space-y-1.5 p-2"
          handle=".monitor-item-handle"
          draggable=".monitor-order-item"
          :animation="180"
          :disabled="saving"
          ghost-class="monitor-order-ghost"
          :group="{ name: `monitor-items-${group.key}`, pull: false, put: false }"
        >
          <div
            v-for="(monitor, monitorIndex) in group.items"
            :key="monitor.id"
            :data-order-monitor="monitor.id"
            class="monitor-order-item flex items-center gap-2 rounded-xl border border-gray-100 bg-white/90 px-2 py-2 dark:border-dark-700 dark:bg-dark-900/70"
          >
            <span
              class="monitor-item-handle drag-handle"
              :title="t('channelStatus.order.dragMonitor')"
              aria-hidden="true"
            ><Icon name="menu" size="sm" /></span>
            <ProviderIcon :provider="monitor.provider" :size="17" :class="['shrink-0', providerTintClass(monitor.provider)]" />
            <div class="min-w-0 flex-1">
              <div class="flex min-w-0 items-center gap-2">
                <span class="truncate text-sm font-medium text-gray-800 dark:text-gray-100" :title="monitor.name">{{ monitor.name }}</span>
                <span v-if="!monitor.enabled" class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                  {{ t('channelStatus.order.disabled') }}
                </span>
              </div>
              <p class="truncate text-xs text-gray-400" :title="monitor.primary_model">{{ monitor.primary_model }}</p>
            </div>
            <button
              type="button"
              class="order-button"
              data-move-monitor="up"
              :disabled="saving || monitorIndex === 0"
              :aria-label="`${t('channelStatus.order.moveUp')}: ${monitor.name}`"
              :title="t('channelStatus.order.moveUp')"
              @click="move(group.items, monitorIndex, -1)"
            ><Icon name="chevronUp" size="sm" /></button>
            <button
              type="button"
              class="order-button"
              data-move-monitor="down"
              :disabled="saving || monitorIndex === group.items.length - 1"
              :aria-label="`${t('channelStatus.order.moveDown')}: ${monitor.name}`"
              :title="t('channelStatus.order.moveDown')"
              @click="move(group.items, monitorIndex, 1)"
            ><Icon name="chevronDown" size="sm" /></button>
          </div>
        </VueDraggable>
        <p v-if="group.items.length === 0" class="px-5 pb-4 text-xs text-gray-400">{{ t('common.noData') }}</p>
      </section>
    </VueDraggable>

    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">
        {{ t('common.cancel') }}
      </button>
      <button type="button" data-save-order class="btn btn-primary" :disabled="!loaded || loading || saving" @click="save">
        {{ t(saving ? 'channelStatus.order.saving' : 'channelStatus.order.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import { adminAPI } from '@/api/admin'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { MonitorDisplayGroup, MonitorDisplayOrder } from '@/api/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getMonitorDisplayGroup, normalizeMonitorGroupOrder, providerTintClass, sortMonitorItems } from '@/utils/channelMonitorDisplay'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; saved: [order: MonitorDisplayOrder] }>()
const { t } = useI18n()
const appStore = useAppStore()

interface DisplayGroup {
  key: MonitorDisplayGroup
  items: ChannelMonitor[]
}

const groups = ref<DisplayGroup[]>([])
const loading = ref(false)
const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
let loadController: AbortController | null = null

function groupLabel(group: MonitorDisplayGroup): string {
  return t(group === 'other' ? 'channelStatus.groups.other' : `monitorCommon.providers.${group}`)
}

async function allMonitors(signal: AbortSignal): Promise<ChannelMonitor[]> {
  const byID = new Map<number, ChannelMonitor>()
  let page = 1
  let pages = 1
  let total = 0
  do {
    const result = await adminAPI.channelMonitor.list({ page, page_size: 100 }, { signal })
    if (signal.aborted) return []
    pages = result.pages
    total = result.total
    for (const item of result.items) byID.set(item.id, item)
    // Never allow a partial list to overwrite the shared ordering.
    if (!result.items.length && byID.size < total) throw new Error(t('channelStatus.order.loadError'))
    page++
  } while (page <= pages)
  if (byID.size < total) throw new Error(t('channelStatus.order.loadError'))
  return [...byID.values()]
}

async function loadOrder() {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  loading.value = true
  loaded.value = false
  loadError.value = ''
  groups.value = []
  try {
    const [items, order] = await Promise.all([
      allMonitors(controller.signal),
      adminAPI.channelMonitor.getDisplayOrder({ signal: controller.signal }),
    ])
    if (controller.signal.aborted || !props.show) return
    const sorted = sortMonitorItems(items, order)
    groups.value = normalizeMonitorGroupOrder(order.group_order).map(key => ({
      key,
      items: sorted.filter(item => getMonitorDisplayGroup(item.provider) === key),
    }))
    loaded.value = true
  } catch (error) {
    if (!controller.signal.aborted) {
      loadError.value = extractApiErrorMessage(error, t('channelStatus.order.loadError'))
      controller.abort()
    }
  } finally {
    if (loadController === controller) {
      loading.value = false
      loadController = null
    }
  }
}

function move<T>(items: T[], index: number, offset: number) {
  const target = index + offset
  if (saving.value || index < 0 || index >= items.length || target < 0 || target >= items.length) return
  const [item] = items.splice(index, 1)
  items.splice(target, 0, item)
}

async function save() {
  if (!loaded.value || loading.value || saving.value) return
  const order: MonitorDisplayOrder = {
    group_order: groups.value.map(group => group.key),
    monitor_order: groups.value.flatMap(group => group.items.map(item => item.id)),
  }
  saving.value = true
  try {
    const saved = await adminAPI.channelMonitor.saveDisplayOrder(order)
    appStore.showSuccess(t('channelStatus.order.saved'))
    emit('saved', saved)
    emit('close')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('channelStatus.order.saveError')))
  } finally {
    saving.value = false
  }
}

function close() {
  if (saving.value) return
  loadController?.abort()
  loaded.value = false
  groups.value = []
  emit('close')
}

watch(() => props.show, show => {
  if (show) void loadOrder()
  else {
    loadController?.abort()
    loaded.value = false
    groups.value = []
  }
}, { immediate: true })

onUnmounted(() => loadController?.abort())
</script>

<style scoped>
.order-button {
  @apply flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 disabled:cursor-not-allowed disabled:opacity-25 dark:text-gray-400 dark:hover:bg-dark-600;
}
.drag-handle {
  @apply flex h-8 w-6 shrink-0 cursor-grab touch-none items-center justify-center text-gray-400 active:cursor-grabbing;
}
.monitor-order-ghost {
  @apply opacity-30;
}
</style>
