<template>
  <div class="status-page" :class="{ 'is-dark': isDark }">
    <header class="status-header">
      <nav class="status-nav" :aria-label="copy.primaryNavigation">
        <router-link to="/home" class="status-brand" :aria-label="siteName">
          <span class="brand-mark">
            <img v-if="siteLogo" :src="siteLogo" :alt="siteName" />
            <span v-else>{{ siteInitial }}</span>
          </span>
          <span class="brand-copy">
            <strong>{{ siteName }}</strong>
            <small>{{ copy.statusCenter }}</small>
          </span>
        </router-link>

        <div class="nav-actions">
          <router-link to="/home" class="nav-link">
            <Icon name="home" size="sm" />
            <span>{{ copy.home }}</span>
          </router-link>
          <LocaleSwitcher />
          <button
            type="button"
            class="icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
          <router-link :to="primaryActionPath" class="nav-cta">
            <span>{{ primaryActionLabel }}</span>
            <Icon name="arrowRight" size="xs" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="status-main">
      <section class="monitor-dashboard" aria-labelledby="monitor-list-title">
        <header class="dashboard-header">
          <div class="dashboard-title">
            <span class="dashboard-pulse"><Icon name="trendingUp" size="lg" :stroke-width="2.4" /></span>
            <h1 id="monitor-list-title">{{ copy.monitorTitle }}</h1>
            <span class="overall-badge" :class="'is-' + overallStatus">
              <Icon
                :name="overallStatus === 'operational' ? 'checkCircle' : 'exclamationCircle'"
                size="sm"
                :stroke-width="2.2"
              />
              {{ overallCompactLabel }}
            </span>
          </div>

          <div class="dashboard-meta">
            <button
              type="button"
              class="auto-refresh"
              :disabled="loading"
              :aria-label="copy.refresh"
              :title="copy.refresh"
              @click="loadMonitors"
            >
              <Icon name="refresh" size="sm" :class="{ 'is-spinning': loading }" />
              <span>{{ copy.nextRefresh }} {{ countdown }}s</span>
            </button>
            <span class="updated-meta">
              {{ copy.lastUpdated }}:
              <strong>{{ formattedLastUpdated }}</strong>
            </span>
          </div>
        </header>

        <div class="dashboard-body">
          <div v-if="loading && items.length === 0" class="loading-list" aria-live="polite">
            <div v-for="index in 3" :key="index" class="loading-row">
              <span></span><span></span><span></span>
            </div>
          </div>

          <div v-else-if="loadError" class="state-panel" role="alert">
            <span class="state-icon is-error"><Icon name="exclamationTriangle" size="lg" /></span>
            <h2>{{ copy.loadFailed }}</h2>
            <p>{{ loadError }}</p>
            <button type="button" class="state-action" @click="loadMonitors">
              <Icon name="refresh" size="sm" />
              {{ copy.retry }}
            </button>
          </div>

          <div v-else-if="items.length === 0" class="state-panel">
            <span class="state-icon"><Icon name="server" size="lg" /></span>
            <h2>{{ copy.emptyTitle }}</h2>
            <p>{{ copy.emptyDescription }}</p>
          </div>

          <div v-else class="monitor-list">
            <article v-for="item in items" :key="item.id" class="monitor-item">
              <div class="monitor-row">
                <div class="channel-overview">
                  <span class="provider-icon" :class="'is-' + item.provider">
                    <ProviderIcon :provider="item.provider" :size="28" />
                  </span>
                  <div class="monitor-identity">
                    <h2>{{ item.name }}</h2>
                    <p>
                      <span>{{ providerLabel(item.provider) }}</span>
                      <i></i>
                      <span class="monitored-models" :title="monitoredModelsLabel(item)">
                        {{ monitoredModelsLabel(item) }}
                      </span>
                    </p>
                  </div>
                  <span class="status-chip" :class="'is-' + normalizeStatus(item.primary_status)">
                    {{ channelStatusLabel(item.primary_status) }}
                  </span>
                </div>

                <div class="latency-summary">
                  <span>{{ copy.responseLatency }}</span>
                  <strong>{{ formatLatency(item.primary_latency_ms) }}<small>ms</small></strong>
                  <time>{{ formatRelativeTime(item.timeline[0]?.checked_at) }}</time>
                </div>

                <div class="timeline-section">
                  <span class="timeline-label">{{ copy.timelineStatus }}</span>
                  <div class="timeline-bars" role="img" :aria-label="copy.timelineAriaLabel">
                    <span
                      v-for="(bar, index) in timelineBars(item.timeline)"
                      :key="index"
                      :class="'is-' + bar.status"
                      :title="bar.title"
                    ></span>
                  </div>
                  <div class="timeline-range">
                    <span>{{ copy.past }}</span>
                    <span>{{ copy.now }}</span>
                  </div>
                </div>

                <div class="availability-summary">
                  <span>{{ copy.availability }}</span>
                  <strong>{{ formatAvailability(item.availability_7d) }}%</strong>
                  <small>{{ copy.availabilityPeriod }}</small>
                </div>

                <button
                  type="button"
                  class="details-toggle"
                  :aria-expanded="isExpanded(item.id)"
                  :aria-label="isExpanded(item.id) ? copy.collapse : copy.viewDetails"
                  :title="isExpanded(item.id) ? copy.collapse : copy.viewDetails"
                  @click="toggleExpanded(item.id)"
                >
                  <Icon :name="isExpanded(item.id) ? 'chevronUp' : 'chevronRight'" size="sm" :stroke-width="2.2" />
                </button>
              </div>

              <Transition name="history-slide">
                <div v-if="isExpanded(item.id)" class="monitor-details">
                  <dl class="detail-metrics">
                    <div>
                      <dt>{{ copy.networkLatency }}</dt>
                      <dd>{{ formatLatency(item.primary_ping_latency_ms) }}<small>ms</small></dd>
                    </div>
                    <div>
                      <dt>{{ copy.primaryModel }}</dt>
                      <dd>{{ item.primary_model }}</dd>
                    </div>
                    <div>
                      <dt>{{ copy.lastCheck }}</dt>
                      <dd>{{ formatDateTime(item.timeline[0]?.checked_at || '') }}</dd>
                    </div>
                    <div>
                      <dt>{{ copy.detectionRecords }}</dt>
                      <dd>{{ item.timeline.length }}</dd>
                    </div>
                  </dl>

                  <div v-if="item.extra_models.length > 0" class="extra-models">
                    <span>{{ copy.additionalModels }}</span>
                    <span
                      v-for="model in item.extra_models"
                      :key="model.model"
                      class="model-chip"
                      :class="'is-' + normalizeStatus(model.status)"
                    >
                      <i></i>{{ model.model }}
                    </span>
                  </div>

                  <div class="history-heading">
                    <span>{{ copy.detectionRecords }}</span>
                    <small>{{ item.timeline.length }}</small>
                  </div>

                  <div class="history-panel">
                    <table>
                      <thead>
                        <tr>
                          <th>{{ copy.checkTime }}</th>
                          <th>{{ copy.checkStatus }}</th>
                          <th>{{ copy.responseLatency }}</th>
                          <th>{{ copy.networkLatency }}</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="record in item.timeline" :key="record.checked_at">
                          <td :data-label="copy.checkTime">{{ formatDateTime(record.checked_at) }}</td>
                          <td :data-label="copy.checkStatus">
                            <span class="record-status" :class="'is-' + normalizeStatus(record.status)">
                              <i></i>{{ statusLabel(record.status) }}
                            </span>
                          </td>
                          <td :data-label="copy.responseLatency">
                            {{ formatLatency(record.latency_ms) }} ms
                          </td>
                          <td :data-label="copy.networkLatency">
                            {{ formatLatency(record.ping_latency_ms) }} ms
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </Transition>
            </article>
          </div>
        </div>
      </section>
    </main>

    <footer class="status-footer">
      <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
      <div>
        <span class="live-dot"></span>
        {{ copy.publicStatusPage }}
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import { publicList, type MonitorTimelinePoint, type MonitorStatus, type UserMonitorView } from '@/api/channelMonitor'
import { sanitizeUrl } from '@/utils/url'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'

type DisplayStatus = 'operational' | 'degraded' | 'failed' | 'unknown'

interface TimelineBar {
  status: DisplayStatus | 'empty'
  title: string
}

const { t, locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const loadError = ref('')
const expandedIds = ref<Set<number>>(new Set())
const countdown = ref(60)
const lastUpdated = ref<Date | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))
let refreshTimer: number | null = null
let abortController: AbortController | null = null

const isZh = computed(() => locale.value.startsWith('zh'))
const siteName = computed(() => {
  const configured = (appStore.cachedPublicSettings?.site_name || appStore.siteName || '').trim()
  return !configured || configured.toLowerCase() === 'sub2api' ? 'Z-API' : configured
})
const siteInitial = computed(() => siteName.value.slice(0, 1).toUpperCase())
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const currentYear = new Date().getFullYear()
const primaryActionPath = computed(() => {
  if (!authStore.isAuthenticated) return '/login'
  return authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
})
const primaryActionLabel = computed(() => {
  if (authStore.isAuthenticated) return isZh.value ? '控制台' : 'Console'
  return isZh.value ? '立即开始' : 'Get Started'
})

const copy = computed(() =>
  isZh.value
    ? {
        primaryNavigation: '公共导航', statusCenter: '服务状态中心', home: '首页',
        liveMonitoring: '实时服务监控', title: '服务运行状态',
        description: '公开查看站内 AI 渠道的可用性、响应延迟与每次监测记录。',
        currentStatus: '当前运行状态', allOperational: '所有服务运行正常',
        degraded: '部分服务出现波动', unknown: '暂无可用监测数据',
        allOperationalDescription: '所有已监控渠道均可正常响应请求。',
        degradedDescription: '部分渠道响应异常或延迟升高，系统正在持续监测。',
        unknownDescription: '管理员尚未配置公开的渠道监测数据。',
        lastUpdated: '最近更新', refresh: '刷新状态', monitoredServices: '正常渠道',
        averageLatency: '平均响应延迟', averageAvailability: '7 天平均可用率', nextRefresh: '自动刷新',
        monitorTitle: '渠道状态监测', monitorDescription: '状态数据每分钟更新，可展开查看逐次监测明细。',
        searchPlaceholder: '搜索渠道、模型或供应商', loadFailed: '状态数据加载失败',
        retry: '重新加载', noMatches: '没有匹配的渠道', noMatchesDescription: '请尝试其他搜索关键词。',
        emptyTitle: '暂无监测数据', emptyDescription: '管理员尚未配置可公开显示的渠道监控。',
        responseLatency: '响应延迟', networkLatency: '网络延迟', availability7d: '7 天可用率',
        lastCheck: '最近检测', additionalModels: '附加模型', recentTimeline: '最近 60 次状态',
        past: '较早', now: '现在', timelineAriaLabel: '最近监测状态时间线', detectionRecords: '逐次监测记录',
        collapse: '收起', viewDetails: '查看详情', checkTime: '检测时间', checkStatus: '检测结果',
        allNormal: '全部正常', hasIssues: '部分异常', noStatus: '暂无状态',
        availability: '可用率', availabilityPeriod: '（7天）', primaryModel: '主模型', timelineStatus: '状态',
        publicStatusPage: '公开状态页', unavailable: '--',
      }
    : {
        primaryNavigation: 'Public navigation', statusCenter: 'Service Status', home: 'Home',
        liveMonitoring: 'Live service monitoring', title: 'Service status',
        description: 'Public availability, response latency, and individual checks for monitored AI channels.',
        currentStatus: 'Current status', allOperational: 'All systems operational',
        degraded: 'Some services are degraded', unknown: 'No monitoring data available',
        allOperationalDescription: 'Every monitored channel is responding normally.',
        degradedDescription: 'Some channels are failing or responding slowly. Monitoring remains active.',
        unknownDescription: 'No public channel monitoring data has been configured yet.',
        lastUpdated: 'Last updated', refresh: 'Refresh status', monitoredServices: 'Operational channels',
        averageLatency: 'Average response', averageAvailability: '7-day availability', nextRefresh: 'Auto refresh',
        monitorTitle: 'Channel status monitoring', monitorDescription: 'Status updates every minute. Expand a channel to inspect each check.',
        searchPlaceholder: 'Search channels, models, or providers', loadFailed: 'Unable to load status data',
        retry: 'Try again', noMatches: 'No matching channels', noMatchesDescription: 'Try a different search term.',
        emptyTitle: 'No monitoring data', emptyDescription: 'No public channel monitors have been configured yet.',
        responseLatency: 'Response latency', networkLatency: 'Network latency', availability7d: '7-day availability',
        lastCheck: 'Last check', additionalModels: 'Additional models', recentTimeline: 'Latest 60 checks',
        past: 'Past', now: 'Now', timelineAriaLabel: 'Recent monitoring status timeline', detectionRecords: 'Individual checks',
        collapse: 'Collapse', viewDetails: 'View details', checkTime: 'Checked at', checkStatus: 'Result',
        allNormal: 'All operational', hasIssues: 'Issues detected', noStatus: 'No status',
        availability: 'Availability', availabilityPeriod: '(7 days)', primaryModel: 'Primary model', timelineStatus: 'Status',
        publicStatusPage: 'Public status page', unavailable: '--',
      }
)

const overallStatus = computed<DisplayStatus>(() => {
  if (items.value.length === 0) return 'unknown'
  return items.value.every(item => item.primary_status === 'operational') ? 'operational' : 'degraded'
})
const overallCompactLabel = computed(() => {
  if (overallStatus.value === 'operational') return copy.value.allNormal
  if (overallStatus.value === 'degraded') return copy.value.hasIssues
  return copy.value.noStatus
})
const formattedLastUpdated = computed(() => {
  if (!lastUpdated.value) return copy.value.unavailable
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(lastUpdated.value)
})

function normalizeStatus(status: MonitorStatus | ''): DisplayStatus {
  if (status === 'operational') return 'operational'
  if (status === 'degraded') return 'degraded'
  if (status === 'failed' || status === 'error') return 'failed'
  return 'unknown'
}

function statusLabel(status: MonitorStatus | ''): string {
  const normalized = normalizeStatus(status)
  if (isZh.value) {
    return { operational: '正常', degraded: '波动', failed: '异常', unknown: '未知' }[normalized]
  }
  return { operational: 'Operational', degraded: 'Degraded', failed: 'Failed', unknown: 'Unknown' }[normalized]
}

function channelStatusLabel(status: MonitorStatus | ''): string {
  const normalized = normalizeStatus(status)
  if (normalized === 'operational') return isZh.value ? '可用' : 'Available'
  return statusLabel(status)
}

function monitoredModelsLabel(item: UserMonitorView): string {
  return [item.primary_model, ...item.extra_models.map(model => model.model)].filter(Boolean).join(' / ')
}

function providerLabel(provider: string): string {
  const labels: Record<string, string> = {
    openai: 'OpenAI', anthropic: 'Anthropic', gemini: 'Gemini', grok: 'Grok',
  }
  return labels[provider] || provider
}

function formatLatency(value: number | null | undefined): string {
  return value == null || !Number.isFinite(value) ? copy.value.unavailable : String(Math.round(value))
}

function formatAvailability(value: number | null | undefined): string {
  return value == null || !Number.isFinite(value) ? copy.value.unavailable : value.toFixed(2)
}

function formatRelativeTime(value: string | undefined): string {
  if (!value) return copy.value.unavailable
  const timestamp = Date.parse(value)
  if (Number.isNaN(timestamp)) return copy.value.unavailable
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000))
  if (seconds < 60) return isZh.value ? `${seconds}秒前` : `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return isZh.value ? `${minutes}分钟前` : `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return isZh.value ? `${hours}小时前` : `${hours}h ago`
  const days = Math.floor(hours / 24)
  return isZh.value ? `${days}天前` : `${days}d ago`
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return copy.value.unavailable
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date)
}

function timelineBars(points: MonitorTimelinePoint[]): TimelineBar[] {
  const limit = 24
  const recent = [...points].slice(0, limit).reverse()
  const bars: TimelineBar[] = Array.from({ length: Math.max(0, limit - recent.length) }, () => ({
    status: 'empty', title: '',
  }))
  for (const point of recent) {
    bars.push({
      status: normalizeStatus(point.status),
      title: formatDateTime(point.checked_at) + ' · ' + statusLabel(point.status) + ' · ' + formatLatency(point.latency_ms) + ' ms',
    })
  }
  return bars
}

function isExpanded(id: number): boolean {
  return expandedIds.value.has(id)
}

function toggleExpanded(id: number) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIds.value = next
}

async function loadMonitors() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  loadError.value = ''
  try {
    const response = await publicList({ signal: controller.signal })
    if (controller.signal.aborted || abortController !== controller) return
    items.value = response.items || []
    lastUpdated.value = new Date()
    countdown.value = appStore.cachedPublicSettings?.channel_monitor_default_interval_seconds || 60
  } catch (error: unknown) {
    const candidate = error as { name?: string; code?: string; message?: string }
    if (candidate.name === 'AbortError' || candidate.code === 'ERR_CANCELED') return
    loadError.value = candidate.message || copy.value.loadFailed
  } finally {
    if (abortController === controller) {
      loading.value = false
      abortController = null
    }
  }
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(async () => {
  initTheme()
  if (!appStore.publicSettingsLoaded) {
    try { await appStore.fetchPublicSettings() } catch { /* The status feed still works with defaults. */ }
  }
  await loadMonitors()
  refreshTimer = window.setInterval(() => {
    if (document.hidden || loading.value) return
    countdown.value -= 1
    if (countdown.value <= 0) void loadMonitors()
  }, 1000)
})

onBeforeUnmount(() => {
  abortController?.abort()
  if (refreshTimer != null) window.clearInterval(refreshTimer)
})
</script>

<style scoped>
.status-page {
  --status-bg: #eff7fa;
  --status-panel: rgba(248, 252, 253, 0.46);
  --status-surface: rgba(255, 255, 255, 0.36);
  --status-surface-strong: #ffffff;
  --status-border: #e2e9ec;
  --status-border-strong: #d2dde2;
  --status-text: #10231f;
  --status-muted: #6c7f86;
  --status-accent: #22bd8b;
  --status-accent-strong: #07805e;
  min-height: 100vh;
  background:
    linear-gradient(rgba(18, 83, 75, 0.025) 1px, transparent 1px),
    linear-gradient(90deg, rgba(18, 83, 75, 0.025) 1px, transparent 1px),
    var(--status-bg);
  background-size: 56px 56px;
  color: var(--status-text);
  font-family: "Microsoft YaHei", sans-serif;
  font-weight: 400;
  letter-spacing: 0;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
  transition: background-color 200ms ease, color 200ms ease;
}

.status-page :deep(button),
.status-page :deep(input),
.status-page :deep(select),
.status-page :deep(textarea) {
  font-family: "Microsoft YaHei", sans-serif;
}

.status-page.is-dark {
  --status-bg: #06100f;
  --status-panel: rgba(6, 23, 20, 0.46);
  --status-surface: rgba(8, 29, 25, 0.44);
  --status-surface-strong: #0b211d;
  --status-border: rgba(123, 216, 202, 0.15);
  --status-border-strong: rgba(123, 216, 202, 0.27);
  --status-text: #f1fbf9;
  --status-muted: #90aaa5;
  --status-accent: #44d6ad;
  --status-accent-strong: #73e7ca;
  background:
    linear-gradient(rgba(91, 194, 179, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(91, 194, 179, 0.03) 1px, transparent 1px),
    var(--status-bg);
  background-size: 56px 56px;
}

.status-header {
  position: sticky;
  z-index: 50;
  top: 0;
  width: 100%;
  padding: 12px 16px 0;
}

.status-nav {
  display: flex;
  width: min(100%, 1240px);
  min-height: 68px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  overflow: visible;
  margin: 0 auto;
  padding: 0 20px;
  border: 1px solid color-mix(in srgb, var(--status-border) 62%, transparent);
  border-radius: 12px;
  background: color-mix(in srgb, var(--status-surface) 82%, transparent);
  box-shadow: 0 15px 42px rgba(38, 78, 91, 0.09), 0 1px 0 rgba(255, 255, 255, 0.82) inset;
  backdrop-filter: blur(28px) saturate(160%);
  -webkit-backdrop-filter: blur(28px) saturate(160%);
}

.is-dark .status-nav {
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.28), 0 1px 0 rgba(255, 255, 255, 0.06) inset;
}

.status-brand,
.nav-actions,
.nav-link,
.nav-cta,
.icon-button,
.dashboard-title,
.dashboard-meta,
.auto-refresh,
.overall-badge,
.channel-overview,
.status-chip,
.details-toggle,
.extra-models,
.model-chip,
.record-status,
.status-footer,
.status-footer > div {
  display: flex;
  align-items: center;
}

.status-brand {
  min-width: 0;
  gap: 11px;
}

.brand-mark {
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid var(--status-border);
  border-radius: 7px;
  background: var(--status-surface-strong);
  color: var(--status-accent-strong);
  font-size: 13px;
  font-weight: 800;
}

.brand-mark img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  padding: 5px;
}

.brand-copy {
  display: block;
  min-width: 0;
}

.brand-copy strong,
.brand-copy small {
  display: block;
  white-space: nowrap;
}

.brand-copy strong {
  font-size: 15px;
  font-weight: 700;
}

.brand-copy small {
  margin-top: 1px;
  color: var(--status-muted);
  font-size: 10px;
  font-weight: 500;
}

.nav-actions {
  flex: 0 0 auto;
  gap: 7px;
}

.nav-link,
.nav-cta,
.icon-button,
.state-action {
  justify-content: center;
  border-radius: 7px;
  transition: background-color 160ms ease, border-color 160ms ease, color 160ms ease, transform 160ms ease;
}

.nav-link {
  height: 36px;
  gap: 7px;
  padding: 0 11px;
  color: var(--status-muted);
  font-size: 13px;
  font-weight: 600;
}

.nav-link:hover,
.icon-button:hover {
  background: var(--status-surface-strong);
  color: var(--status-text);
}

.icon-button {
  display: inline-flex;
  width: 36px;
  height: 36px;
  align-items: center;
  border: 1px solid transparent;
  color: var(--status-muted);
}

.nav-cta {
  height: 36px;
  gap: 7px;
  padding: 0 14px;
  background: var(--status-accent);
  color: #ffffff;
  box-shadow: 0 8px 22px rgba(34, 189, 139, 0.2);
  font-size: 13px;
  font-weight: 700;
}

.is-dark .nav-cta {
  color: #05241b;
}

.nav-cta:hover {
  transform: translateY(-1px);
}

.status-main {
  width: 100%;
}

.monitor-dashboard {
  width: min(calc(100% - 32px), 1240px);
  margin: 38px auto 72px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--status-border) 58%, transparent);
  border-radius: 14px;
  background: var(--status-panel);
  box-shadow: 0 20px 55px rgba(39, 81, 96, 0.09);
  backdrop-filter: blur(20px) saturate(135%);
  -webkit-backdrop-filter: blur(20px) saturate(135%);
}

.dashboard-header {
  display: flex;
  min-height: 92px;
  align-items: center;
  justify-content: space-between;
  gap: 28px;
  padding: 22px 28px;
}

.dashboard-title {
  min-width: 0;
  gap: 13px;
}

.dashboard-pulse {
  display: inline-flex;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  color: var(--status-accent);
}

.dashboard-title h1 {
  font-size: 25px;
  font-weight: 800;
  line-height: 1.2;
}

.overall-badge {
  height: 30px;
  gap: 6px;
  border-radius: 6px;
  background: rgba(34, 189, 139, 0.1);
  padding: 0 10px;
  color: var(--status-accent-strong);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.overall-badge.is-degraded,
.overall-badge.is-failed {
  background: rgba(226, 165, 45, 0.12);
  color: #a56a00;
}

.overall-badge.is-unknown {
  background: rgba(128, 144, 140, 0.12);
  color: var(--status-muted);
}

.dashboard-meta {
  flex: 0 0 auto;
  gap: 30px;
  color: var(--status-muted);
  font-size: 12px;
  font-weight: 600;
}

.auto-refresh {
  gap: 8px;
  color: inherit;
  font: inherit;
}

.auto-refresh:hover {
  color: var(--status-text);
}

.auto-refresh:disabled {
  cursor: wait;
  opacity: 0.72;
}

.updated-meta strong {
  color: inherit;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.dashboard-body {
  padding: 0 20px 20px;
}

.monitor-list {
  display: grid;
  gap: 8px;
}

.monitor-item {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 14px;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.48), rgba(238, 249, 248, 0.22));
  box-shadow:
    0 16px 42px rgba(37, 74, 86, 0.09),
    inset 0 1px 0 rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(24px) saturate(155%);
  -webkit-backdrop-filter: blur(24px) saturate(155%);
}

.is-dark .monitor-item {
  border-color: rgba(112, 198, 181, 0.14);
  background: rgba(8, 29, 25, 0.44);
  box-shadow:
    0 16px 38px rgba(0, 0, 0, 0.2),
    inset 0 1px 0 rgba(255, 255, 255, 0.05);
}

.monitor-row {
  display: grid;
  grid-template-columns: minmax(280px, 0.9fr) 118px minmax(430px, 1.75fr) 86px 44px;
  min-height: 122px;
  align-items: center;
  gap: 18px;
  padding: 20px 22px;
}

.channel-overview {
  min-width: 0;
  gap: 16px;
}

.provider-icon,
.state-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.provider-icon {
  width: 58px;
  height: 58px;
  flex: 0 0 auto;
  border-radius: 14px;
  background: #e7f8f2;
  color: #00a77d;
}

.provider-icon.is-anthropic {
  background: #fff3e5;
  color: #f06b16;
}

.provider-icon.is-gemini {
  background: #edf3ff;
  color: #4285f4;
}

.provider-icon.is-grok {
  background: #f0f1f3;
  color: #42474d;
}

.is-dark .provider-icon {
  background: rgba(68, 214, 173, 0.12);
  color: #56dfb8;
}

.is-dark .provider-icon.is-anthropic {
  background: rgba(240, 107, 22, 0.14);
  color: #ff9a5c;
}

.is-dark .provider-icon.is-gemini {
  background: rgba(66, 133, 244, 0.15);
  color: #78a7ff;
}

.is-dark .provider-icon.is-grok {
  background: rgba(212, 220, 224, 0.1);
  color: #cbd2d5;
}

.monitor-identity {
  min-width: 0;
  flex: 1;
}

.monitor-identity h2 {
  overflow: hidden;
  color: var(--status-text);
  font-size: 17px;
  font-weight: 700;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.monitor-identity p {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  color: var(--status-muted);
  font-size: 11px;
  font-weight: 500;
}

.monitor-identity p span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.monitored-models {
  min-width: 0;
  flex: 1;
}

.monitor-identity p i {
  width: 3px;
  height: 3px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--status-muted);
  opacity: 0.55;
}

.status-chip {
  min-width: 48px;
  height: 28px;
  flex: 0 0 auto;
  justify-content: center;
  border-radius: 5px;
  background: rgba(34, 189, 139, 0.12);
  color: var(--status-accent-strong);
  font-size: 11px;
  font-weight: 700;
}

.is-dark .status-chip.is-operational {
  color: #75e9c5;
}

.status-chip.is-degraded {
  background: rgba(226, 165, 45, 0.13);
  color: #9a6300;
}

.status-chip.is-failed {
  background: rgba(225, 83, 83, 0.12);
  color: #c53b3b;
}

.status-chip.is-unknown {
  background: rgba(128, 144, 140, 0.12);
  color: var(--status-muted);
}

.latency-summary,
.availability-summary {
  min-width: 0;
}

.latency-summary > span,
.availability-summary > span,
.timeline-label {
  display: block;
  color: var(--status-muted);
  font-size: 13px;
  font-weight: 400;
  line-height: 1.5;
}

.latency-summary strong,
.availability-summary strong {
  display: block;
  margin-top: 4px;
  color: var(--status-accent-strong);
  font-size: 19px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  line-height: 1.3;
}

.latency-summary strong small {
  margin-left: 4px;
  font-size: 10px;
  font-weight: 700;
}

.latency-summary time,
.availability-summary small {
  display: block;
  margin-top: 7px;
  color: var(--status-muted);
  font-size: 12px;
  font-weight: 400;
  font-variant-numeric: tabular-nums;
  line-height: 1.45;
  white-space: nowrap;
}

.timeline-section {
  min-width: 0;
  padding-left: 26px;
  border-left: 1px solid var(--status-border);
}

.timeline-bars {
  display: grid;
  grid-template-columns: repeat(24, minmax(6px, 1fr));
  gap: 5px;
  margin-top: 12px;
}

.timeline-bars span {
  height: 13px;
  border-radius: 5px;
  background: color-mix(in srgb, var(--status-muted) 17%, transparent);
}

.timeline-bars span.is-operational {
  background: #2dc994;
}

.timeline-bars span.is-degraded {
  background: #e2a52d;
}

.timeline-bars span.is-failed {
  background: #e15353;
}

.timeline-bars span.is-unknown,
.timeline-bars span.is-empty {
  background: color-mix(in srgb, var(--status-muted) 16%, transparent);
}

.timeline-range {
  display: flex;
  justify-content: space-between;
  margin-top: 7px;
  color: var(--status-muted);
  font-size: 11px;
  font-weight: 400;
  line-height: 1.4;
}

.details-toggle {
  width: 44px;
  min-width: 44px;
  height: 44px;
  justify-content: center;
  border: 0;
  border-radius: 12px;
  padding: 0;
  background: rgba(255, 255, 255, 0.38);
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.78),
    0 8px 20px rgba(37, 74, 86, 0.07);
  backdrop-filter: blur(14px) saturate(145%);
  -webkit-backdrop-filter: blur(14px) saturate(145%);
  color: var(--status-text);
  transition: background-color 160ms ease, color 160ms ease;
}

.details-toggle:hover {
  background: rgba(255, 255, 255, 0.58);
  color: var(--status-accent-strong);
}

.details-toggle:focus {
  outline: none;
}

.details-toggle:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--status-accent) 42%, transparent);
  outline-offset: 2px;
}

.is-dark .details-toggle {
  background: rgba(255, 255, 255, 0.045);
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.09),
    0 8px 20px rgba(0, 0, 0, 0.16);
}

.is-dark .details-toggle:hover {
  background: rgba(68, 214, 173, 0.09);
}

.monitor-details {
  padding: 0 22px 22px;
  border-top: 1px solid var(--status-border);
  background: color-mix(in srgb, var(--status-surface-strong) 42%, transparent);
}

.detail-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  padding: 18px 0;
  border-bottom: 1px solid var(--status-border);
}

.detail-metrics > div {
  min-width: 0;
  padding: 0 18px;
  border-right: 1px solid var(--status-border);
}

.detail-metrics > div:first-child {
  padding-left: 0;
}

.detail-metrics > div:last-child {
  padding-right: 0;
  border-right: 0;
}

.detail-metrics dt {
  color: var(--status-muted);
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
}

.detail-metrics dd {
  margin-top: 5px;
  overflow: hidden;
  font-size: 15px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-metrics dd small {
  margin-left: 3px;
  color: var(--status-muted);
  font-size: 9px;
}

.extra-models {
  flex-wrap: wrap;
  gap: 7px;
  padding-top: 16px;
}

.extra-models > span:first-child {
  margin-right: 3px;
  color: var(--status-muted);
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
}

.model-chip {
  gap: 5px;
  padding: 3px 6px;
  border: 1px solid var(--status-border);
  border-radius: 5px;
  color: var(--status-muted);
  font-family: inherit;
  font-size: 9px;
  font-weight: 600;
}

.model-chip i,
.record-status i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.model-chip.is-operational i {
  color: #20b985;
}

.model-chip.is-degraded i {
  color: #e2a52d;
}

.model-chip.is-failed i {
  color: #e15353;
}

.history-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 18px;
  color: var(--status-text);
  font-size: 11px;
  font-weight: 700;
}

.history-heading small {
  display: inline-flex;
  min-width: 20px;
  height: 18px;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  background: var(--status-surface-strong);
  color: var(--status-muted);
  font-size: 9px;
  font-weight: 600;
}

.history-panel {
  overflow-x: auto;
  margin-top: 10px;
  border: 1px solid var(--status-border);
  border-radius: 7px;
  background: color-mix(in srgb, var(--status-surface-strong) 58%, transparent);
}

.history-panel table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  font-weight: 500;
}

.history-panel th {
  padding: 11px 14px;
  color: var(--status-muted);
  font-size: 9px;
  font-weight: 700;
  text-align: left;
  text-transform: uppercase;
}

.history-panel td {
  padding: 11px 14px;
  border-top: 1px solid var(--status-border);
  color: var(--status-muted);
  font-variant-numeric: tabular-nums;
}

.record-status {
  width: max-content;
  gap: 6px;
  color: #168863;
  font-weight: 700;
}

.record-status.is-degraded {
  color: #b8790a;
}

.record-status.is-failed {
  color: #c53b3b;
}

.record-status.is-unknown {
  color: var(--status-muted);
}

.loading-list {
  display: grid;
  gap: 8px;
}

.loading-row {
  display: grid;
  grid-template-columns: 1.3fr 2fr 1fr;
  gap: 30px;
  padding: 34px 24px;
  border: 1px solid var(--status-border);
  border-radius: 8px;
  background: var(--status-surface);
}

.loading-row span {
  height: 18px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--status-muted) 13%, transparent);
  animation: pulse 1.3s ease-in-out infinite;
}

.state-panel {
  padding: 70px 24px;
  border: 1px solid var(--status-border);
  border-radius: 8px;
  background: var(--status-surface);
  text-align: center;
}

.state-icon {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  background: rgba(34, 189, 139, 0.09);
  color: var(--status-accent-strong);
}

.state-icon.is-error {
  background: rgba(225, 83, 83, 0.1);
  color: #c53b3b;
}

.state-panel h2 {
  margin-top: 16px;
  font-size: 18px;
  font-weight: 700;
}

.state-panel p {
  margin-top: 7px;
  color: var(--status-muted);
  font-size: 13px;
  font-weight: 500;
}

.state-action {
  display: inline-flex;
  height: 38px;
  align-items: center;
  gap: 7px;
  margin-top: 20px;
  padding: 0 15px;
  background: var(--status-accent);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
}

.status-footer {
  width: min(calc(100% - 32px), 1240px);
  justify-content: space-between;
  gap: 16px;
  margin: 0 auto;
  padding: 22px 0;
  border-top: 1px solid var(--status-border);
  color: var(--status-muted);
  font-size: 11px;
  font-weight: 500;
}

.status-footer > div {
  gap: 9px;
}

.live-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--status-accent);
}

.is-spinning {
  animation: spin 700ms linear infinite;
}

.history-slide-enter-active,
.history-slide-leave-active {
  transition: opacity 160ms ease, transform 160ms ease;
}

.history-slide-enter-from,
.history-slide-leave-to {
  opacity: 0;
  transform: translateY(-5px);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes pulse {
  50% {
    opacity: 0.45;
  }
}

@media (max-width: 1180px) {
  .monitor-row {
    grid-template-columns: minmax(300px, 1fr) 124px 90px;
    gap: 18px 24px;
  }

  .channel-overview {
    grid-column: 1;
    grid-row: 1;
  }

  .latency-summary {
    grid-column: 2;
    grid-row: 1;
  }

  .availability-summary {
    grid-column: 3;
    grid-row: 1;
  }

  .timeline-section {
    grid-column: 1 / 3;
    grid-row: 2;
    padding-left: 0;
    border-left: 0;
  }

  .details-toggle {
    grid-column: 3;
    grid-row: 2;
    justify-self: end;
  }
}

@media (max-width: 800px) {
  .status-header {
    padding: 8px 8px 0;
  }

  .status-nav {
    width: 100%;
    min-height: 64px;
    padding: 0 11px;
  }

  .brand-copy small,
  .nav-link span,
  .nav-cta span {
    display: none;
  }

  .nav-link,
  .nav-cta {
    width: 36px;
    padding: 0;
  }

  .monitor-dashboard {
    margin-top: 24px;
  }

  .dashboard-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 16px;
    padding: 22px;
  }

  .dashboard-meta {
    width: 100%;
    justify-content: space-between;
    gap: 16px;
  }

  .monitor-row {
    grid-template-columns: minmax(0, 1fr) minmax(92px, auto);
    gap: 20px 16px;
    padding: 20px;
  }

  .channel-overview {
    grid-column: 1 / -1;
    grid-row: 1;
  }

  .latency-summary {
    grid-column: 1;
    grid-row: 2;
    text-align: left;
  }

  .availability-summary {
    grid-column: 2;
    grid-row: 2;
    text-align: right;
  }

  .availability-summary > span,
  .availability-summary small {
    text-align: right;
  }

  .timeline-section {
    grid-column: 1 / -1;
    grid-row: 3;
    padding-left: 0;
    border-left: 0;
  }

  .details-toggle {
    grid-column: 1 / -1;
    grid-row: 4;
    width: 44px;
    justify-self: end;
  }

  .detail-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .detail-metrics > div:nth-child(2) {
    padding-right: 0;
    border-right: 0;
  }

  .detail-metrics > div:nth-child(3) {
    padding-left: 0;
  }
}

@media (max-width: 560px) {
  .status-page {
    background-size: 42px 42px;
  }

  .status-nav {
    min-height: 58px;
    padding: 0 10px;
  }

  .brand-mark {
    width: 34px;
    height: 34px;
  }

  .nav-actions {
    gap: 3px;
  }

  .nav-link,
  .nav-cta,
  .icon-button {
    width: 34px;
    height: 34px;
  }

  .monitor-dashboard {
    width: calc(100% - 16px);
    margin: 16px auto 48px;
  }

  .dashboard-header {
    gap: 18px;
    padding: 20px 16px 18px;
  }

  .dashboard-title {
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr);
    gap: 10px;
  }

  .dashboard-title h1 {
    font-size: 21px;
  }

  .overall-badge {
    grid-column: 2;
    width: max-content;
    margin-top: -2px;
  }

  .dashboard-meta {
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
    font-size: 11px;
  }

  .dashboard-body {
    padding: 0 8px 8px;
  }

  .monitor-row {
    gap: 18px 12px;
    min-height: 0;
    padding: 16px;
  }

  .channel-overview {
    display: grid;
    grid-template-columns: 48px minmax(0, 1fr) auto;
    gap: 12px;
  }

  .provider-icon {
    width: 48px;
    height: 48px;
  }

  .monitor-identity h2 {
    font-size: 16px;
  }

  .monitor-identity p {
    gap: 5px;
    margin-top: 5px;
    font-size: 10px;
  }

  .status-chip {
    min-width: 44px;
    height: 26px;
    font-size: 10px;
  }

  .latency-summary strong,
  .availability-summary strong {
    font-size: 16px;
  }

  .timeline-bars {
    grid-template-columns: repeat(24, minmax(4px, 1fr));
    gap: 3px;
  }

  .timeline-bars span {
    height: 11px;
    border-radius: 4px;
  }

  .details-toggle {
    width: 40px;
    min-width: 40px;
    height: 40px;
  }

  .monitor-details {
    padding: 0 16px 16px;
  }

  .detail-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0;
  }

  .detail-metrics > div {
    padding: 13px 12px;
    border-bottom: 1px solid var(--status-border);
  }

  .detail-metrics > div:first-child,
  .detail-metrics > div:nth-child(3) {
    padding-left: 0;
  }

  .detail-metrics > div:nth-child(2),
  .detail-metrics > div:last-child {
    padding-right: 0;
    border-right: 0;
  }

  .detail-metrics > div:nth-child(n + 3) {
    border-bottom: 0;
  }

  .history-panel {
    overflow: visible;
    border: 0;
    background: transparent;
  }

  .history-panel thead {
    display: none;
  }

  .history-panel,
  .history-panel table,
  .history-panel tbody,
  .history-panel tr,
  .history-panel td {
    display: block;
    width: 100%;
  }

  .history-panel tr {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    padding: 14px 0;
    border-top: 1px solid var(--status-border);
  }

  .history-panel td {
    padding: 0;
    border: 0;
  }

  .history-panel td::before {
    display: block;
    margin-bottom: 4px;
    color: var(--status-muted);
    content: attr(data-label);
    font-size: 8px;
    font-weight: 700;
    text-transform: uppercase;
  }

  .status-footer {
    width: calc(100% - 24px);
    align-items: center;
    flex-direction: column;
    text-align: center;
  }
}
@media (prefers-reduced-motion: reduce) {
  .is-spinning,
  .loading-row span { animation: none; }
}
</style>
