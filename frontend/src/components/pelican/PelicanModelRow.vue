<template>
  <div v-if="mode === 'single'" ref="track" class="gallery-scroll gallery-fade flex items-stretch gap-2 overflow-x-auto pb-2 pt-0.5" data-testid="pelican-gallery">
    <PelicanArtworkCard v-for="(run, index) in primary.works" :key="run.id" :run="run" :round="index" :dot="primary.dot" :autoplay="primary.auto.has(run.id)" @open="open(primary, run)" />
    <span v-if="!primary.works.length" class="p-2.5 text-xs text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.waiting') }}</span>
  </div>

  <div v-else-if="mode === 'paired'" ref="track" class="gallery-scroll gallery-fade flex items-stretch gap-3.5 overflow-x-auto pb-2 pt-0.5" data-testid="pelican-gallery">
    <div v-for="(round, index) in rounds" :key="round[0].run.id" class="shrink-0 space-y-1.5 rounded-2xl border border-primary-100 dark:border-primary-900/60 bg-primary-50/40 dark:bg-primary-500/[.04] p-2" :data-round="index">
      <div class="flex items-center justify-between px-1 text-xs">
        <div class="flex items-center gap-1.5">
          <span class="font-semibold" :class="index === 0 ? 'text-primary-700 dark:text-primary-300' : 'text-gray-600 dark:text-dark-300'">{{ index === 0 ? t('pelicanMonitor.card.latestRound') : t('pelicanMonitor.card.round', { count: index + 1 }) }}</span>
          <span class="tabular-nums text-gray-500 dark:text-dark-400">· {{ pelicanTime(round[0].run.started_at || round[0].run.created_at).slice(0, 5) }}</span>
        </div>
        <button type="button" class="cursor-pointer font-medium text-primary-600 dark:text-primary-400 hover:text-primary-800 dark:hover:text-primary-200" :data-testid="'compare-round-' + index" @click="open(round[0].row, round[0].run, true)">{{ t('pelicanMonitor.card.enlargeRound') }}</button>
      </div>
      <div class="flex items-stretch gap-2">
        <PelicanArtworkCard v-for="{ row, run } in round" :key="run.id" :run="run" :round="index" :dot="row.dot" compact tag :autoplay="row.auto.has(run.id)" @open="open(row, run)" />
      </div>
    </div>
    <span v-if="!rounds.length" class="p-2.5 text-xs text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.waiting') }}</span>
  </div>

  <div v-else ref="track" class="gallery-scroll gallery-fade space-y-2.5 overflow-x-auto pb-2 pt-0.5" data-testid="pelican-gallery">
    <div v-for="row in rows" :key="row.plan.id" class="flex w-max min-w-full items-center gap-3" :data-model-row="row.plan.id">
      <div class="sticky left-0 z-10 w-32 shrink-0 rounded-xl border border-gray-200/80 dark:border-dark-600 bg-white/95 dark:bg-dark-800/95 p-2.5 shadow-[0_1px_2px_rgb(0_0_0/.05)] backdrop-blur-[4px]">
        <div class="flex items-center gap-1.5"><span class="h-2 w-2 shrink-0 rounded-full" :style="{ background: row.dot }"></span><span class="truncate text-xs font-bold text-gray-900 dark:text-gray-100">{{ row.name }}</span></div>
        <div class="mt-1 text-xs text-primary-600 dark:text-primary-400">{{ row.badge }}</div>
        <div class="mt-0.5 tabular-nums text-xs text-gray-500 dark:text-dark-400">{{ row.completed }}/{{ PELICAN_ARTWORK_LIMIT }} {{ t('pelicanMonitor.card.finished') }}</div>
      </div>
      <div class="flex items-stretch gap-2">
        <PelicanArtworkCard v-for="(run, index) in row.works" :key="run.id" :run="run" :round="index" :dot="row.dot" compact :autoplay="row.auto.has(run.id)" @open="open(row, run)" />
        <span v-if="!row.works.length" class="p-2.5 text-xs text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.waiting') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PelicanRun, PelicanMonitorGroup } from '@/api/pelicanMonitor'
import { PELICAN_ARTWORK_LIMIT } from '@/utils/pelicanArtworks'
import PelicanArtworkCard from './PelicanArtworkCard.vue'
import { pelicanTime, type PelicanViewMode, type PelicanGalleryModel } from './pelicanFormat'

const props = defineProps<{ primary: PelicanGalleryModel; rows: PelicanGalleryModel[]; mode: PelicanViewMode }>()
const emit = defineEmits<{ open: [plan: PelicanMonitorGroup, run: PelicanRun, compare: boolean] }>()
const { t } = useI18n()
const track = ref<HTMLElement>()
const rounds = computed(() => {
  if (props.mode !== 'paired') return []
  const count = Math.max(0, ...props.rows.map(row => row.works.length))
  return Array.from({ length: count }, (_, index) => props.rows.flatMap(row => {
    const run = row.works[index]
    return run ? [{ row, run }] : []
  }))
})
function open(row: PelicanGalleryModel, run: PelicanRun, compare = props.mode !== 'single') {
  emit('open', row.plan, run, compare)
}
function scroll(delta: number) { track.value?.scrollBy({ left: delta, behavior: 'smooth' }) }
defineExpose({ scroll })
watch([() => props.mode, () => props.primary.plan.id, () => props.primary.works[0]?.id], async () => {
  await nextTick()
  if (track.value) track.value.scrollLeft = 0
})
</script>
