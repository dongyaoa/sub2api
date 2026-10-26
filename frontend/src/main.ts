import { createApp, h, watch } from 'vue'
import { createPinia } from 'pinia'
import WebAccessRoot from './WebAccessRoot.vue'
import WebAccessPage from '@/components/common/WebAccessPage.vue'
import { webAccessGate } from '@/services/webAccess'
import { bindWebAccessTitle } from '@/services/webAccessTitle'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { updateFavicon } from '@/utils/branding'
import { isIOSDevice } from '@/utils/device'
import './style.css'

function initIOSViewportZoomFix() {
  // iOS Safari 在输入框字号小于 16px 时聚焦会自动放大页面，且失焦后不会恢复。
  // 限制 maximum-scale 可阻止该行为；iOS 10+ 用户仍可双指手动缩放，不影响可访问性。
  // 仅在 iOS 设备上注入，避免影响 Android Chrome 的手动缩放能力。
  if (!isIOSDevice()) return

  const viewport = document.querySelector('meta[name="viewport"]')
  if (!viewport) return

  const content = viewport.getAttribute('content') || ''
  if (/maximum-scale/i.test(content)) return
  viewport.setAttribute('content', `${content}, maximum-scale=1.0`)
}

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const path = window.location.pathname
  const isHomepage = path === '/' || path === '/home'
  const shouldUseDark = savedTheme ? savedTheme === 'dark' : isHomepage
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

async function loadPrimaryFont() {
  if (!document.fonts?.load) return
  try {
    await document.fonts.load('400 14px Geist')
  } catch {
    // Continue with the configured fallback if the local font cannot be loaded.
  }
}

let disposed = false
let app: ReturnType<typeof createApp> | null = null
let appMounted = false
let loadingApp: ReturnType<typeof createApp> | null = null
const disposers = new Set<() => void>()
const accessAllowed = () => !webAccessGate.state.enabled || webAccessGate.state.phase === 'allowed'

function unmountLoadingApp() {
  loadingApp?.unmount()
  loadingApp = null
}

function mountLoadingApp() {
  if (loadingApp || appMounted || disposed) return
  loadingApp = createApp({
    render: () => h(WebAccessPage, {
      phase: webAccessGate.state.phase,
      message: webAccessGate.state.message,
      checking: webAccessGate.state.checking,
    }),
  })
  loadingApp.mount('#app')
}

async function waitForWebAccess(): Promise<boolean> {
  while (!disposed && !accessAllowed()) {
    await new Promise<void>(resolve => {
      const finish = () => {
        stop()
        disposers.delete(finish)
        resolve()
      }
      const stop = watch(accessAllowed, allowed => {
        if (allowed) finish()
      }, { flush: 'sync' })
      disposers.add(finish)
    })
  }
  return !disposed
}

/** Release startup work as well as the mounted app during HMR and tests. */
export function cleanupBootstrap() {
  if (disposed) return
  disposed = true
  // Disabling also invalidates pending requests and restores the normal title.
  webAccessGate.setEnabled(false)
  for (const dispose of disposers) dispose()
  disposers.clear()
  unmountLoadingApp()
  if (appMounted) app?.unmount()
  appMounted = false
  app = null
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  initIOSViewportZoomFix()

  app = createApp(WebAccessRoot)
  const pinia = createPinia()
  app.use(pinia)

  // The public flag is authoritative. Static deployments use their existing
  // settings request; a missing/disabled flag never starts a region check.
  const appStore = useAppStore(pinia)
  if (!appStore.initFromInjectedConfig()) await appStore.fetchPublicSettings()
  if (disposed) {
    webAccessGate.setEnabled(false)
    return
  }

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteName} - AI API Gateway`
  }
  updateFavicon(appStore.siteLogo)

  // Keep this binding for a later admin enable. Its observer is connected only
  // while blocked, so normal pages retain their ordinary title behavior.
  disposers.add(bindWebAccessTitle())
  const stopLoadingWatch = watch(() => webAccessGate.state.enabled, enabled => {
    if (disposed || appMounted) return
    if (enabled) mountLoadingApp()
    else unmountLoadingApp()
  }, { immediate: true, flush: 'sync' })
  disposers.add(stopLoadingWatch)

  if (!await waitForWebAccess()) return
  await Promise.all([initI18n(), loadPrimaryFont()])

  // A location change during font/locale loading keeps the gate visible. When
  // access returns (or the switch is disabled), continue without a page reload.
  if (!await waitForWebAccess()) return

  disposers.add(watch(accessAllowed, allowed => {
    if (disposed) return
    if (!allowed) useAuthStore(pinia).pauseBackgroundRefresh()
    else if (!router.currentRoute.value.matched.length) {
      void router.replace(window.location.pathname + window.location.search + window.location.hash)
    }
  }))

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  try {
    await router.isReady()
  } catch (error) {
    if (disposed) return
    // A denied initial navigation can recover through the access watcher.
    if (accessAllowed()) throw error
    if (!await waitForWebAccess()) return
    await router.isReady()
  }
  if (!await waitForWebAccess()) return
  if (webAccessGate.state.setupRequired) await router.replace('/setup')
  if (!await waitForWebAccess()) return
  stopLoadingWatch()
  disposers.delete(stopLoadingWatch)
  unmountLoadingApp()
  app.mount('#app')
  appMounted = true
}

void bootstrap()
if (import.meta.hot) import.meta.hot.dispose(cleanupBootstrap)
