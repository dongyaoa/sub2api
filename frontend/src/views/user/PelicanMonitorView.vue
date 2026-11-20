<template>
  <AppLayout flush-content>
    <div class="pelican-design pelican-page px-4 py-5 md:px-6 md:py-6 lg:px-8 selection:bg-primary-100 selection:text-primary-900 dark:selection:bg-primary-800 dark:selection:text-primary-100" data-testid="pelican-monitor-page">
      <div class="min-w-0 w-full space-y-4">
        <header class="space-y-3.5 pb-1">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <div class="w-9 h-9 rounded-xl bg-primary-50 border border-primary-100 flex items-center justify-center text-primary-600 dark:bg-primary-500/10 dark:border-primary-500/20 dark:text-primary-300">
                <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M9 18h6"/><path d="M10 22h4"/>
                  <path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/>
                </svg>
              </div>
              <div>
                <div class="flex items-center gap-2.5">
                  <h1 class="text-xl font-semibold text-gray-900 tracking-tight dark:text-white">{{ settings.config.title || t('pelicanMonitor.title') }}</h1>
                  <span class="px-2 py-0.5 text-xs font-medium rounded-full bg-primary-50 text-primary-700 border border-primary-200/70 dark:bg-primary-500/10 dark:text-primary-300 dark:border-primary-500/20">{{ t('pelicanMonitor.aggregated') }}</span>
                </div>
                <p class="text-xs text-gray-500 mt-1 dark:text-dark-300">{{ settings.config.description || t('pelicanMonitor.description') }}</p>
              </div>
            </div>

            <div class="flex items-center gap-2.5 flex-wrap">
              <div v-if="settings.enabled" role="group" :aria-label="t('pelicanMonitor.globalLayout')" class="hidden sm:flex items-center gap-1 bg-white border border-gray-200 rounded-xl p-1 shadow-sm dark:bg-dark-800 dark:border-dark-700">
                <span class="text-xs text-gray-500 px-2 dark:text-dark-300">{{ t('pelicanMonitor.globalLayout') }}</span>
                <button
                  v-for="mode in modes"
                  :id="`global-${mode}`"
                  :key="mode"
                  type="button"
                  :aria-pressed="globalMode === mode"
                  class="px-2.5 py-1 text-xs rounded-lg transition"
                  :class="globalMode === mode ? 'font-semibold bg-primary-600 text-white shadow-sm dark:bg-primary-700' : 'font-medium text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'"
                  @click="setAllCardsMode(mode)"
                >
                  {{ t(`pelicanMonitor.card.${mode}`) }}
                </button>
              </div>

              <button type="button" :aria-busy="loading" data-testid="pelican-refresh" @click="manualRefresh" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5">
                <svg id="refreshSvg" :style="refreshing ? { transform: 'rotate(360deg)' } : undefined" class="w-3.5 h-3.5 text-gray-500 dark:text-dark-300 transition-transform duration-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M16 16h5v5"/>
                </svg>
                {{ t('pelicanMonitor.refresh') }}
              </button>
            </div>
          </div>

          <div v-if="settings.enabled" role="search" class="flex flex-wrap items-center justify-between gap-3 pt-0.5">
            <div class="flex items-center gap-2.5 min-w-0">
              <span class="w-1 h-4 rounded-full bg-primary-500 shrink-0"></span>
              <p class="text-xs font-normal leading-5 text-gray-500 dark:text-dark-400">
                {{ t('pelicanMonitor.drawingPrompt') }}
              </p>
            </div>

            <div class="flex items-center gap-3.5 flex-wrap">
              <div class="relative">
                <svg class="w-3.5 h-3.5 text-gray-400 dark:text-dark-400 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
                </svg>
                <input
                  id="searchBox"
                  v-model="search"
                  type="text"
                  :placeholder="t('pelicanMonitor.search')" :aria-label="t('pelicanMonitor.search')"
                  class="input w-52 max-w-full !pl-8 !pr-3 !py-1.5 !text-xs rounded-full"
                />
              </div>
              <div class="flex items-center gap-1.5 text-xs text-primary-700 dark:text-primary-300 font-medium">
                <span class="w-2 h-2 rounded-full bg-primary-500"></span>
                <span id="summaryCount">{{ t('pelicanMonitor.summary', { groups: visibleGroups.length, models: visibleModelCount }) }}</span>
              </div>
              <span id="updatedAgo" class="text-xs text-gray-500 dark:text-dark-400 tabular-nums">{{ refreshing ? t('pelicanMonitor.justUpdated') : t('pelicanMonitor.updated', { seconds: updatedSeconds }) }}</span>
            </div>
          </div>
        </header>
        <p v-if="settings.config.notice && settings.enabled" class="whitespace-pre-wrap rounded-xl border border-primary-100 bg-primary-50/70 px-3.5 py-2.5 text-xs leading-5 text-primary-800 dark:bg-primary-500/10 dark:border-primary-500/20 dark:text-primary-200">{{ settings.config.notice }}</p>
        <p v-if="error" role="alert" class="rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-600 dark:bg-rose-500/10 dark:text-rose-300">{{ error }}</p>
        <div v-if="!loaded && loading" class="space-y-3" role="status"><div v-for="n in 2" :key="n" class="h-[272px] animate-pulse rounded-2xl border border-gray-100 bg-white dark:border-dark-700 dark:bg-dark-800" /><span class="sr-only">{{ t('common.loading') }}</span></div>
        <div v-else-if="loaded && !settings.enabled" class="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-gray-200 bg-white p-8 text-center dark:border-dark-700 dark:bg-dark-800" data-testid="pelican-disabled"><Icon name="lightbulb" size="xl" class="mb-4 text-gray-300 dark:text-gray-500" /><h2 class="text-lg font-semibold text-gray-700 dark:text-gray-200">{{ t('pelicanMonitor.disabled') }}</h2><p class="mt-2 text-sm text-gray-400">{{ t('pelicanMonitor.disabledHint') }}</p></div>
        <section v-else-if="loaded && settings.enabled" id="groupsWrapper" class="space-y-4" :aria-label="t('pelicanMonitor.title')">
          <PelicanGroupCard
            v-for="group in groupedPlans"
            v-show="visibleKeys.has(group.key)"
            :key="group.key"
            :plans="group.plans"
            :mode="groupModes[group.key] ?? 'single'"
            :now="now + clockOffset"
            @mode="setGroupMode(group.key, $event)"
            @open="openArtwork"
          />
          <div v-if="!visibleGroups.length" class="rounded-2xl border border-gray-200 bg-white px-6 py-12 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300">
            {{ t(search ? 'pelicanMonitor.noMatches' : 'pelicanMonitor.empty') }}
          </div>
        </section>
      </div>
      <PelicanArtworkDialog
        v-if="selectedRun && selectedGroup && dialogPlans.length"
        :key="selectedRun.id"
        :run="selectedRun"
        :plan="selectedGroup"
        :plans="dialogPlans"
        :force-compare="selected?.compare"
        @close="selected = null"
      />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pelican/pelicanDesign.css'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { pelicanMonitorAPI, type PelicanMonitorGroup, type PelicanRun } from '@/api/pelicanMonitor'
import { usePelicanMonitorStore } from '@/stores/pelicanMonitor'
import { useAuthStore } from '@/stores/auth'
import { useMonitorRefresh } from '@/composables/useMonitorRefresh'
import PelicanGroupCard from '@/components/pelican/PelicanGroupCard.vue'
import PelicanArtworkDialog from '@/components/pelican/PelicanArtworkDialog.vue'
import { pelicanDialogOpenKey, pelicanPageActiveKey, pelicanPreviewRefreshKey } from '@/components/pelican/pelicanContext'
import { groupPelicanPlans } from '@/utils/pelicanGroups'
import { pelicanModelLabel } from '@/utils/pelicanModels'
import { pelicanWorks, type PelicanViewMode } from '@/components/pelican/pelicanFormat'
import { resetPelicanArtworkAccess, retainPelicanArtworks } from '@/components/pelican/pelicanArtworkLoader'
import { reconcileMonitorData } from '@/components/admin/upstream/monitorReconcile'

const { t } = useI18n(), settings = usePelicanMonitorStore(), auth = useAuthStore()
const groups = ref<PelicanMonitorGroup[]>([]), loaded = ref(false), error = ref(''), search = ref('')
const selected = ref<{ planID: number; runID: number; compare: boolean } | null>(null)
const now = ref(Date.now()), clockOffset = ref(0), updatedAt = ref(Date.now()), polling = ref(true), previewRevision = ref(0)
const pageVisible = ref(!document.hidden)
const selectedGroup = computed(() => groups.value.find(group => group.id === selected.value?.planID))
const selectedRun = computed(() => selectedGroup.value ? pelicanWorks(selectedGroup.value).find(run => run.id === selected.value?.runID) : undefined)
const groupedPlans = computed(() => groupPelicanPlans(groups.value))
const modes: PelicanViewMode[] = ['single', 'paired', 'stacked']
const globalMode = ref<PelicanViewMode>('single')
const groupModes = ref<Record<string, PelicanViewMode>>({})
const visibleGroups = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return groupedPlans.value.filter(group => !query || group.plans.some(plan =>
    [plan.group_name, plan.model, pelicanModelLabel(plan.model)].some(label => label?.toLocaleLowerCase().includes(query))
  ))
})
const visibleKeys = computed(() => new Set(visibleGroups.value.map(group => group.key)))
const visibleModelCount = computed(() => visibleGroups.value.reduce((sum, group) => sum + group.plans.length, 0))
const dialogPlans = computed(() => groupedPlans.value.find(group => group.plans.some(plan => plan.id === selected.value?.planID))?.plans ?? [])

function setGroupMode(key: string, mode: PelicanViewMode) {
  groupModes.value[key] = mode
}
function setAllCardsMode(mode: PelicanViewMode) {
  globalMode.value = mode
  for (const group of groupedPlans.value) setGroupMode(group.key, mode)
}
watch(groupedPlans, list => {
  const keys = new Set(list.map(group => group.key))
  for (const key of Object.keys(groupModes.value)) if (!keys.has(key)) delete groupModes.value[key]
  for (const group of list) groupModes.value[group.key] ??= globalMode.value
})
const updatedSeconds = computed(() => Math.max(0, Math.floor((now.value - updatedAt.value) / 1000)))
provide(pelicanPageActiveKey, computed(() => auth.isAuthenticated && settings.enabled && pageVisible.value))
provide(pelicanPreviewRefreshKey, previewRevision)
provide(pelicanDialogOpenKey, computed(() => Boolean(selectedRun.value)))
function clearVisibleData() {
  groups.value = []
  selected.value = null
  groupModes.value = {}
  globalMode.value = 'single'
  resetPelicanArtworkAccess()
}
const { loading, refresh } = useMonitorRefresh({
  active: () => auth.isAuthenticated && polling.value,
  // One lightweight metadata poll for the entire page, including manual runs
  // started by an administrator while this user is already watching.
  intervalMs: () => 1500,
  request: async signal => {
    const token = auth.token, userID = auth.user?.id
    const result = await pelicanMonitorAPI.list(signal)
    if (signal.aborted || !auth.isAuthenticated || auth.token !== token || auth.user?.id !== userID) throw new DOMException('Session changed', 'AbortError')
    return result
  },
  apply: result => {
    settings.apply(result.config)
    loaded.value = true; error.value = ''; updatedAt.value = Date.now()
    const serverTime = Date.parse(result.server_time)
    if (Number.isFinite(serverTime)) clockOffset.value = serverTime - Date.now()
    if (!result.config.enabled) { clearVisibleData(); polling.value = false; return }
    groups.value = reconcileMonitorData(groups.value, result.items || [])
    retainPelicanArtworks(new Set(groups.value.flatMap(group => pelicanWorks(group).map(run => run.id))))
    if (selected.value && !selectedRun.value) selected.value = null
  },
  onError: cause => {
    if ((cause as Error)?.name === 'AbortError') return
    const status = (cause as { response?: { status?: number }; status?: number })?.response?.status ?? (cause as { status?: number })?.status
    if ([401, 403, 404].includes(status || 0)) { settings.disable(); clearVisibleData(); polling.value = false; loaded.value = true }
    else error.value = t('pelicanMonitor.loadFailed')
  },
})
const refreshing = ref(false)
let refreshAnimation: ReturnType<typeof setTimeout> | undefined
function manualRefresh() {
  previewRevision.value++
  refreshing.value = true
  clearTimeout(refreshAnimation)
  refreshAnimation = setTimeout(() => { refreshing.value = false }, 520)
  if (!polling.value) polling.value = true
  else void refresh()
}
function openArtwork(plan: PelicanMonitorGroup, run: PelicanRun, compare = false) {
  selected.value = { planID: plan.id, runID: run.id, compare }
}
let clock: ReturnType<typeof setInterval> | undefined
function syncClock() { clearInterval(clock); clock = undefined; pageVisible.value = !document.hidden; now.value = Date.now(); if (pageVisible.value) clock = setInterval(() => { now.value = Date.now() }, 1000) }
watch(() => settings.enabled, enabled => { if (!enabled) clearVisibleData() })
watch([() => auth.token, () => auth.user?.id], () => {
  const wasPolling = polling.value
  clearVisibleData(); loaded.value = false; polling.value = auth.isAuthenticated
  // Abort the old session immediately even when both accounts are logged in.
  if (auth.isAuthenticated && wasPolling) void refresh()
})
onMounted(() => { syncClock(); document.addEventListener('visibilitychange', syncClock) })
onBeforeUnmount(() => { clearTimeout(refreshAnimation); clearInterval(clock); document.removeEventListener('visibilitychange', syncClock); resetPelicanArtworkAccess() })
</script>
