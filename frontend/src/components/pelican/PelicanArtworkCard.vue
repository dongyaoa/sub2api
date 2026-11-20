<template>
  <article class="pelican-work group w-[204px] shrink-0 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700 bg-white dark:bg-dark-800 transition-all duration-200" :class="canOpen && 'cursor-pointer hover:border-primary-400 dark:hover:border-primary-500 hover:shadow-md'" :data-run-id="run.id" @mouseenter="hovered = true" @mouseleave="hovered = false" @focusin="focused = true" @focusout="leaveFocus">
    <div class="relative h-[148px] overflow-hidden border-b border-gray-100 dark:border-dark-700 bg-gray-50 dark:bg-dark-900">
      <PelicanArtworkPreview :run="run" :autoplay="autoplay || hovered || focused" presentation-only class="absolute inset-0 h-full w-full" @open="emit('open', run)" />
      <span v-if="tag" class="absolute right-1.5 top-1.5 flex items-center gap-1 rounded-md border border-gray-200/80 dark:border-dark-600 bg-white/90 dark:bg-dark-800/90 px-1.5 py-0.5 text-xs font-medium text-gray-700 dark:text-dark-200 shadow-[0_1px_2px_rgb(0_0_0/.05)] backdrop-blur-[4px]">
        <span class="h-1.5 w-1.5 rounded-full" :style="{ background: dot }"></span>{{ modelName }}
      </span>
      <button v-if="canOpen" type="button" class="absolute inset-0 z-10 flex items-center justify-center bg-transparent text-xs opacity-0 transition group-hover:bg-gray-900/10 group-hover:opacity-100 focus-visible:opacity-100" :aria-label="t('pelicanMonitor.card.open')" aria-haspopup="dialog" @click="emit('open', run)">
        <span class="rounded-full bg-white/95 dark:bg-dark-800/95 px-2.5 py-1 font-medium text-gray-900 dark:text-gray-100 shadow-sm">{{ t('pelicanMonitor.card.open') }}</span>
      </button>
    </div>
    <button v-if="compact" type="button" class="block w-full space-y-0.5 px-2.5 py-2 text-left text-[11px] leading-4" :disabled="!canOpen" @click="emit('open', run)" :aria-label="t('pelicanMonitor.open')">
      <span class="flex items-center justify-between">
        <span :class="round === 0 ? 'font-semibold text-primary-600 dark:text-primary-400' : 'tabular-nums text-gray-500 dark:text-dark-400'">{{ runLabel }}</span>
        <time class="tabular-nums text-gray-500 dark:text-dark-300" :datetime="stamp" :title="fullTime">{{ shortTime }}</time>
      </span>
      <span class="flex items-center justify-between text-gray-500 dark:text-dark-400">
        <span class="truncate">{{ modelName }}</span><span class="tabular-nums font-medium text-gray-600 dark:text-dark-300">{{ duration }}</span>
      </span>
    </button>
    <button v-else type="button" class="block w-full space-y-1 p-2.5 text-left text-[11px] leading-4" :disabled="!canOpen" @click="emit('open', run)" :aria-label="t('pelicanMonitor.open')">
      <span class="flex items-center justify-between">
        <span :class="round === 0 ? 'font-bold text-primary-600 dark:text-primary-400' : 'tabular-nums text-gray-500 dark:text-dark-400'">{{ runLabel }}</span>
        <span role="img" :aria-label="t('pelicanMonitor.status.' + run.status)" class="h-2 w-2 rounded-full" :class="run.status === 'succeeded' ? 'bg-emerald-500' : run.status === 'failed' ? 'bg-rose-500' : 'bg-amber-400'"></span>
      </span>
      <time :datetime="stamp" class="block tabular-nums tracking-tight text-gray-600 dark:text-dark-300">{{ fullTime }}</time>
      <span class="flex items-center justify-between pt-0.5">
        <span class="truncate pr-1 text-gray-500 dark:text-dark-400">{{ modelName }} {{ effort }}</span>
        <span class="shrink-0 tabular-nums font-medium text-gray-600 dark:text-dark-300">{{ duration }}</span>
      </span>
    </button>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PelicanRun } from '@/api/pelicanMonitor'
import { pelicanModelLabel } from '@/utils/pelicanModels'
import PelicanArtworkPreview from './PelicanArtworkPreview.vue'
import { pelicanDate, pelicanDuration, pelicanEffortLabel, pelicanTime } from './pelicanFormat'

const props = defineProps<{ run: PelicanRun; round: number; dot: string; compact?: boolean; tag?: boolean; autoplay?: boolean }>()
const emit = defineEmits<{ open: [run: PelicanRun] }>()
const { t, locale } = useI18n()
const hovered = ref(false), focused = ref(false)
function leaveFocus(event: FocusEvent) {
  focused.value = event.relatedTarget instanceof Node && (event.currentTarget as HTMLElement).contains(event.relatedTarget)
}
const stamp = computed(() => props.run.started_at || props.run.created_at)
const modelName = computed(() => pelicanModelLabel(props.run.model))
const effort = computed(() => pelicanEffortLabel(props.run.reasoning_effort))
const fullTime = computed(() => pelicanDate(stamp.value, locale.value, true))
const shortTime = computed(() => pelicanTime(stamp.value))
const runLabel = computed(() => props.round === 0 ? t('pelicanMonitor.latest') : '#' + props.run.id)
const duration = computed(() => pelicanDuration(props.run))
const canOpen = computed(() => props.run.status === 'succeeded')
</script>
