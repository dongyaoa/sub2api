<template>
  <Teleport to="body">
    <div id="previewModal" class="pelican-design pelican-demo-modal fixed inset-0 z-50 flex items-center justify-center bg-gray-900/45 dark:bg-black/65 backdrop-blur-xs p-4" role="dialog" aria-modal="true" aria-labelledby="modalTitle" @click.self="emit('close')">
      <div class="pelican-artwork-dialog flex max-h-full w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-gray-200 dark:border-dark-700 bg-white dark:bg-dark-800 shadow-2xl">
        <div class="flex shrink-0 items-center justify-between gap-3 border-b border-gray-100 dark:border-dark-700 px-5 py-3.5">
          <div class="flex min-w-0 items-center gap-2.5">
            <span id="modalGroupBadge" class="rounded-full bg-primary-50 dark:bg-primary-500/10 px-2.5 py-0.5 text-xs font-semibold text-primary-700 dark:text-primary-300">{{ t('pelicanMonitor.artworkModal.group', { name: plan.group_name, rate }) }}</span>
            <h3 id="modalTitle" class="text-sm font-bold text-gray-900 dark:text-gray-100">{{ title }}</h3>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <button id="modalCompareToggle" ref="toggleButton" type="button" class="cursor-pointer rounded-lg border border-primary-200 dark:border-primary-700/60 bg-primary-50 dark:bg-primary-500/10 px-3 py-1 text-xs font-medium text-primary-700 dark:text-primary-300 transition hover:bg-primary-100 dark:hover:bg-primary-500/20" :aria-pressed="comparing" @click="comparing = !comparing">
              {{ comparing ? t('pelicanMonitor.artworkModal.single') : t('pelicanMonitor.artworkModal.compare', { count: plans.length }) }}
            </button>
            <button ref="closeButton" type="button" class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg text-sm text-gray-500 dark:text-dark-400 hover:bg-gray-100 dark:hover:bg-dark-700 hover:text-gray-700 dark:hover:text-gray-100" :aria-label="t('common.close')" @click="emit('close')">✕</button>
          </div>
        </div>
        <div id="modalBody" class="min-h-0 overflow-y-auto overscroll-contain bg-gray-50 dark:bg-dark-900 p-5">
          <div v-if="comparing" class="grid grid-cols-1 gap-4" :class="plans.length === 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'">
            <div v-for="(item, index) in compared" :key="item.plan.id" class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700 bg-white dark:bg-dark-800 shadow-xs" :data-compare-plan="item.plan.id">
              <div class="flex items-center justify-between border-b border-gray-100 dark:border-dark-700 px-3.5 py-2.5">
                <div class="flex items-center gap-1.5">
                  <span class="h-2 w-2 shrink-0 rounded-full" :style="{ backgroundColor: pelicanModelDot(index) }" />
                  <span class="text-xs font-bold text-gray-900 dark:text-gray-100">{{ pelicanModelLabel(item.plan.model) }}</span>
                  <span class="rounded bg-primary-50 dark:bg-primary-500/10 px-1.5 py-0.5 text-xs text-primary-700 dark:text-primary-300">{{ effortBadge(item.run?.reasoning_effort || item.plan.reasoning_effort) }}</span>
                </div>
                <span class="tabular-nums text-xs text-gray-500 dark:text-dark-300">{{ t('pelicanMonitor.artworkModal.elapsed', { duration: item.run ? pelicanDuration(item.run) : '—' }) }}</span>
              </div>
              <div class="relative aspect-[240/168] bg-gray-50 dark:bg-dark-900">
                <PelicanArtworkPreview v-if="item.run" :key="item.run.id" :run="item.run" large presentation-only class="absolute inset-0 h-full w-full" />
                <div v-else class="absolute inset-0 grid place-items-center px-3 text-center text-xs text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.artworkModal.missing') }}</div>
              </div>
              <div class="flex justify-between px-3.5 py-2 tabular-nums text-xs text-gray-500 dark:text-dark-400">
                <span>{{ item.run ? runLabel(item.run) : '—' }}</span>
                <time v-if="item.run" :datetime="item.run.started_at || item.run.created_at">{{ stamp(item.run) }}</time>
              </div>
            </div>
          </div>
          <div v-else class="mx-auto max-w-xl overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700 bg-white dark:bg-dark-800 shadow-xs">
            <div class="relative aspect-[240/168]">
              <PelicanArtworkPreview :key="run.id" :run="run" large presentation-only class="absolute inset-0 h-full w-full" />
            </div>
            <div class="flex items-center justify-between border-t border-gray-100 dark:border-dark-700 px-4 py-3 text-xs text-gray-500 dark:text-dark-300">
              <span>{{ t('pelicanMonitor.artworkModal.model') }}<strong class="text-gray-900 dark:text-gray-100">{{ pelicanModelLabel(run.model) }} <span class="model-effort">{{ pelicanEffortLabel(run.reasoning_effort) }}</span></strong></span>
              <span class="tabular-nums">{{ t('pelicanMonitor.artworkModal.duration') }}<strong class="text-primary-600 dark:text-primary-400">{{ pelicanDuration(run) }}</strong></span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PelicanMonitorGroup, PelicanRun } from '@/api/pelicanMonitor'
import { pelicanModelLabel } from '@/utils/pelicanModels'
import { pelicanDate, pelicanDuration, pelicanEffortLabel, pelicanModelDot, pelicanWorks } from './pelicanFormat'
import PelicanArtworkPreview from './PelicanArtworkPreview.vue'

const props = defineProps<{ run: PelicanRun; plan: PelicanMonitorGroup; plans: PelicanMonitorGroup[]; forceCompare?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t, locale } = useI18n()
const comparing = ref(Boolean(props.forceCompare))
const rate = computed(() => typeof props.plan.group_rate_multiplier === 'number' && Number.isFinite(props.plan.group_rate_multiplier) ? props.plan.group_rate_multiplier + 'x' : '—')
const roundIndex = computed(() => pelicanWorks(props.plan).findIndex(record => record.id === props.run.id))
const stamp = (run: PelicanRun) => pelicanDate(run.started_at || run.created_at, locale.value, true)
const runLabel = (run: PelicanRun) => roundIndex.value === 0 ? t('pelicanMonitor.latest') : '#' + run.id
const effortBadge = (effort: string) => t('pelicanMonitor.artworkModal.effort', { effort: pelicanEffortLabel(effort) })
const title = computed(() => comparing.value
  ? t('pelicanMonitor.artworkModal.title', { round: roundIndex.value + 1, count: props.plans.length })
  : pelicanModelLabel(props.run.model) + ' (' + effortBadge(props.run.reasoning_effort) + ') · ' + runLabel(props.run) + ' · ' + stamp(props.run))

// Each column uses the same newest-first position as the comparison gallery.
const compared = computed(() => props.plans.map(plan => ({
  plan,
  run: plan.id === props.plan.id ? props.run : pelicanWorks(plan)[roundIndex.value],
})))

const toggleButton = ref<HTMLButtonElement>()
const closeButton = ref<HTMLButtonElement>()
let returnFocus: HTMLElement | null = null
function handleKey(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
  } else if (event.key === 'Tab' && event.shiftKey && document.activeElement === toggleButton.value) {
    event.preventDefault()
    closeButton.value?.focus()
  } else if (event.key === 'Tab' && !event.shiftKey && document.activeElement === closeButton.value) {
    event.preventDefault()
    toggleButton.value?.focus()
  }
}
onMounted(() => {
  returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  toggleButton.value?.focus({ preventScroll: true })
  document.addEventListener('keydown', handleKey)
})
onUnmounted(() => {
  document.removeEventListener('keydown', handleKey)
  if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
})
</script>

<style scoped>
:global(body:has(#previewModal)) { overflow: hidden; }
</style>
