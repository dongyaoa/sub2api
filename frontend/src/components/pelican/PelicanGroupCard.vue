<template>
  <section v-if="primary" data-testid="pelican-group" class="flex min-w-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm lg:flex-row dark:border-dark-700 dark:bg-dark-800">
    <aside class="flex w-full shrink-0 flex-col gap-4 border-b border-gray-100 bg-gradient-to-br from-primary-50/70 via-white to-white p-4 lg:w-[236px] lg:border-b-0 lg:border-r dark:border-dark-700 dark:from-primary-500/5 dark:via-dark-800 dark:to-dark-800">
      <div class="space-y-3.5">
        <div class="flex items-center gap-2.5">
          <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-primary-100 bg-primary-50 text-primary-600 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-300">
            <svg aria-hidden="true" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18h6"/><path d="M10 22h4"/><path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/></svg>
          </div>
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold leading-5 text-gray-900 dark:text-gray-100">{{ primary.plan.group_name }}</h2>
            <span class="mt-1 inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium leading-[14px]" :class="statusBadge">
              <span class="h-1.5 w-1.5 rounded-full" :class="statusDot"></span>
              {{ t('pelicanMonitor.status.' + status) }}
            </span>
          </div>
        </div>
        <div class="grid grid-cols-2 items-start gap-2 rounded-xl border border-gray-100 bg-white/80 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/75">
          <div><div class="text-[11px] leading-4 text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.rate') }}</div><div class="mt-1 text-base font-semibold leading-5 tabular-nums text-primary-600 dark:text-primary-300" data-testid="pelican-rate">{{ rate }}</div></div>
          <div class="border-l border-gray-200/70 pl-3 dark:border-dark-600"><div class="text-[11px] leading-4 text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.next') }}</div><div class="mt-1 text-base font-semibold leading-5 tabular-nums text-gray-900 dark:text-gray-100">{{ countdown }}</div></div>
        </div>
        <div class="space-y-1.5">
          <button v-for="row in rows" :key="row.plan.id" type="button" class="flex w-full cursor-pointer items-center justify-between gap-1.5 rounded-xl border px-2.5 py-1.5 text-left transition" :class="isSelected(row) ? 'border-primary-200 bg-primary-50/80 text-gray-900 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-gray-100' : 'border-gray-100 bg-white/70 text-gray-500 hover:bg-gray-50 hover:text-gray-800 dark:border-dark-700 dark:bg-dark-800/80 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200'" :title="t(multi ? 'pelicanMonitor.card.includeModel' : 'pelicanMonitor.card.showModel', { name: row.name })" :aria-pressed="isSelected(row)" :data-testid="'choose-plan-' + row.plan.id" @click="choose(row.plan.id)">
            <span class="flex min-w-0 items-center gap-1.5"><span class="h-2 w-2 shrink-0 rounded-full bg-gray-300 dark:bg-dark-500" :style="isSelected(row) ? { backgroundColor: row.dot } : undefined"></span><span class="truncate text-[11px] font-semibold leading-4" :data-testid="primary.plan.id === row.plan.id ? 'pelican-plan-model' : undefined">{{ row.name }}</span></span>
            <span class="flex shrink-0 items-center gap-1"><span class="rounded-md bg-primary-500/10 px-1.5 py-0.5 text-[10px] font-medium leading-[14px] text-primary-700 dark:text-primary-300">{{ row.badge }}</span><span v-if="multi" class="text-xs font-bold" :class="isSelected(row) ? 'text-primary-600 dark:text-primary-300' : 'text-gray-400 dark:text-dark-400'">{{ isSelected(row) ? '✓' : '+' }}</span></span>
          </button>
        </div>
      </div>
      <div class="mt-auto grid gap-2.5 border-t border-gray-100 pt-3 text-[11px] leading-4 text-gray-500 dark:border-dark-700 dark:text-dark-400">
        <div class="flex items-center gap-2"><svg aria-hidden="true" class="h-3.5 w-3.5 shrink-0 text-primary-500/70 dark:text-primary-400/80" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><span>{{ interval }}</span></div>
        <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1"><span>{{ t('pelicanMonitor.lastRun') }}</span><time class="whitespace-nowrap tabular-nums text-gray-600 dark:text-gray-300" :datetime="primary.plan.latest_run?.started_at || primary.plan.last_run_at || undefined">{{ pelicanDate(primary.plan.latest_run?.started_at || primary.plan.last_run_at, locale) }}</time></div>
      </div>
    </aside>
    <div class="flex min-w-0 flex-1 flex-col gap-3 p-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2.5">
          <div class="flex items-baseline gap-1.5"><span class="text-xs font-semibold text-gray-800 dark:text-gray-100">{{ t('pelicanMonitor.recentWorks') }}</span><span class="text-xs text-gray-500 dark:text-dark-400">{{ t('pelicanMonitor.newestFirst') }}</span></div>
          <div class="inline-flex max-w-full flex-wrap items-center gap-0.5 rounded-xl border border-gray-200/70 bg-gray-100/80 dark:border-dark-600 dark:bg-dark-900/70 p-0.5">
            <button v-for="row in rows" :key="row.plan.id" type="button" class="flex cursor-pointer items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium transition" :class="isSelected(row) ? 'border border-primary-200/80 bg-white text-primary-700 shadow-sm dark:border-primary-500/30 dark:bg-dark-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'" :aria-pressed="isSelected(row)" :data-testid="'quick-model-' + row.plan.id" @click="choose(row.plan.id)">
              <span v-if="multi" class="h-1.5 w-1.5 rounded-full bg-gray-300 dark:bg-dark-500" :style="isSelected(row) ? { backgroundColor: row.dot } : undefined"></span><span>{{ row.name }}</span>
            </button>
          </div>
        </div>
        <div class="flex max-w-full flex-wrap items-center gap-2.5">
          <div class="inline-flex items-center rounded-xl border border-gray-200/70 bg-gray-100/80 dark:border-dark-600 dark:bg-dark-900/70 p-0.5">
            <button v-for="item in modes" :key="item" type="button" class="cursor-pointer rounded-lg px-2.5 py-1 text-xs font-medium transition" :class="mode === item ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'" :title="t('pelicanMonitor.card.' + item + 'Hint')" :aria-pressed="mode === item" :data-testid="'card-mode-' + item" @click="emit('mode', item)">{{ t('pelicanMonitor.card.' + item) }}</button>
          </div>
          <div class="flex items-center gap-1">
            <button v-for="item in directions" :key="item.label" type="button" class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-lg border border-gray-200 bg-white text-xs text-gray-500 hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-500/60 dark:hover:text-primary-300" :title="t('pelicanMonitor.card.' + item.label)" :aria-label="t('pelicanMonitor.card.' + item.label)" @click="gallery?.scroll(item.delta)">{{ item.icon }}</button>
          </div>
          <span class="text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ completedCount }}/{{ PELICAN_ARTWORK_LIMIT }}</span>
        </div>
      </div>
      <PelicanModelRow ref="gallery" :primary="primary" :rows="visibleRows" :mode="mode" @open="(plan, run, compare) => emit('open', plan, run, compare)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PelicanMonitorGroup, PelicanRun } from '@/api/pelicanMonitor'
import { PELICAN_ARTWORK_LIMIT } from '@/utils/pelicanArtworks'
import { pelicanModelLabel } from '@/utils/pelicanModels'
import PelicanModelRow from './PelicanModelRow.vue'
import { pelicanActive, pelicanCountdown, pelicanDate, pelicanEffortLabel, pelicanInterval, pelicanModelDot, pelicanWorks, type PelicanViewMode, type PelicanGalleryModel } from './pelicanFormat'

const props = defineProps<{ plans: PelicanMonitorGroup[]; mode: PelicanViewMode; now: number; matchedPlanId?: number }>()
const emit = defineEmits<{ mode: [mode: PelicanViewMode]; open: [plan: PelicanMonitorGroup, run: PelicanRun, compare: boolean] }>()
const { t, locale } = useI18n()
const modes: readonly PelicanViewMode[] = ['single', 'paired', 'stacked']
const directions = [{ delta: -380, icon: '‹', label: 'left' }, { delta: 380, icon: '›', label: 'right' }] as const
const gallery = ref<InstanceType<typeof PelicanModelRow>>()
const activeId = ref<number>()
const excluded = ref(new Set<number>())
const rows = computed<PelicanGalleryModel[]>(() => props.plans.map((plan, index) => {
  const works = pelicanWorks(plan)
  const completed = works.filter(run => !pelicanActive(run))
  const effort = pelicanEffortLabel(plan.reasoning_effort)
  return { plan, works, completed: completed.length, auto: new Set(completed.slice(0, 8).map(run => run.id)),
    name: pelicanModelLabel(plan.model), badge: effort === 'Fast' ? effort : effort + ' ' + t('pelicanMonitor.reasoningShort'), dot: pelicanModelDot(index) }
}))
const multi = computed(() => props.mode !== 'single')
const primary = computed(() => rows.value.find(row => row.plan.id === activeId.value) ?? rows.value[0])
const visibleRows = computed(() => {
  if (!multi.value) return primary.value ? [primary.value] : []
  const checked = rows.value.filter(row => !excluded.value.has(row.plan.id))
  return checked.length ? checked : rows.value.slice(0, 1)
})
const completedCount = computed(() => Math.max(0, ...visibleRows.value.map(row => row.completed)))
function isSelected(row: PelicanGalleryModel) {
  return multi.value ? visibleRows.value.some(visible => visible.plan.id === row.plan.id) : primary.value?.plan.id === row.plan.id
}
function choose(id: number) {
  if (!multi.value) { activeId.value = id; return }
  const current = visibleRows.value
  const next = new Set(rows.value.filter(row => !current.includes(row)).map(row => row.plan.id))
  if (next.has(id)) next.delete(id)
  else if (current.length > 1) next.add(id)
  excluded.value = next
}
watch(() => props.matchedPlanId, id => {
  if (id === undefined) return
  activeId.value = id
  if (excluded.value.has(id)) { const next = new Set(excluded.value); next.delete(id); excluded.value = next }
}, { immediate: true })
const status = computed(() => primary.value?.plan.latest_run?.status ?? 'idle')
const statusBadge = computed(() => ({ succeeded: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300', failed: 'bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-300', pending: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300', running: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300', idle: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400' })[status.value])
const statusDot = computed(() => ({ succeeded: 'bg-emerald-500', failed: 'bg-rose-500', pending: 'bg-amber-400', running: 'bg-amber-400', idle: 'bg-gray-400' })[status.value])
const rate = computed(() => {
  const value = primary.value?.plan.group_rate_multiplier
  return typeof value === 'number' && Number.isFinite(value) ? new Intl.NumberFormat(locale.value, { maximumFractionDigits: 4 }).format(value) + 'x' : '—'
})
const countdown = computed(() => {
  const plan = primary.value?.plan
  if (!plan?.enabled) return t('pelicanMonitor.paused')
  if (pelicanActive(plan.latest_run)) return t('pelicanMonitor.afterCurrent')
  return pelicanCountdown(plan.next_run_at, props.now) || t('pelicanMonitor.waitingSchedule')
})
const interval = computed(() => {
  const plan = primary.value?.plan
  if (!plan?.enabled) return t('pelicanMonitor.paused')
  const value = pelicanInterval(plan.interval_seconds)
  return value.minutes === undefined ? t('pelicanMonitor.everySeconds', { count: value.seconds }) : t('pelicanMonitor.everyMinutes', { count: value.minutes })
})
</script>
