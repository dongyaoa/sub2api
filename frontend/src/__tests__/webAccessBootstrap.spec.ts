import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import type { WebAccessStatus } from '@/api/webAccess'
import type { PublicSettings } from '@/types'

const harness = vi.hoisted(() => ({
  loadStatus: vi.fn(),
  loadPublicSettings: vi.fn(),
  initI18n: vi.fn(),
  installRouter: vi.fn(),
  routerReady: vi.fn(),
  replaceRoute: vi.fn(),
  pauseBackgroundRefresh: vi.fn(),
}))

vi.mock('@/api/webAccess', async importOriginal => ({
  ...await importOriginal<typeof import('@/api/webAccess')>(),
  getWebAccessStatus: () => harness.loadStatus(),
}))
vi.mock('@/api/auth', () => ({ getPublicSettings: () => harness.loadPublicSettings() }))
vi.mock('@/i18n', () => ({
  default: { install() {} },
  i18n: { global: { t: (key: string) => key } },
  initI18n: () => harness.initI18n(),
}))
vi.mock('@/router', () => ({ default: {
  install: harness.installRouter,
  isReady: () => harness.routerReady(),
  replace: harness.replaceRoute,
  currentRoute: { value: { matched: [{}] } },
} }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ pauseBackgroundRefresh: harness.pauseBackgroundRefresh }) }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/utils/device', () => ({ isIOSDevice: () => false }))
vi.mock('@/App.vue', () => ({ default: defineComponent({ setup: () => () => h('div', { 'data-testid': 'regular-application' }, '网站内容') }) }))

function publicSettings(enabled?: boolean): PublicSettings {
  return {
    site_name: 'Example Site',
    site_logo: '',
    ...(enabled === undefined ? {} : { web_access_cn_block_enabled: enabled }),
  } as PublicSettings
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

const allowedStatus: WebAccessStatus = { enabled: true, blocked: false, message: '本网站暂不向中国大陆地区提供网页访问服务。' }
const blockedStatus: WebAccessStatus = { enabled: true, blocked: true, message: '指定的地区访问提示' }
const normalTitle = 'Example Site - AI API Gateway'
let cleanup: (() => void) | undefined

async function startApplication() {
  const main = await import('../main')
  cleanup = main.cleanupBootstrap
}

function expectOnlyLoading() {
  expect(document.querySelector('[data-testid="web-access-loading"]')).not.toBeNull()
  expect(document.querySelector('[data-testid="web-access-loading"]')?.textContent).toContain('正在檢查訪問狀態')
  expect(document.querySelector('[data-testid="web-access-loading"]')?.textContent).toContain('Checking availability')
  expect(document.querySelector('[data-testid="web-access-notice"]')).toBeNull()
  expect(document.querySelector('[data-testid="regular-application"]')).toBeNull()
  expect(document.querySelector('h1')).toBeNull()
  expect(document.querySelector('button')).toBeNull()
  expect(document.body.textContent).not.toMatch(/当前地区|访问受限|中国大陆|當前地區|訪問受限|中國大陸|本站[仅僅]面向海外|Exclusively for overseas users/)
}

function expectRegularApplication() {
  expect(document.querySelector('[data-testid="regular-application"]')).not.toBeNull()
  expect(document.querySelector('[data-testid="web-access-loading"]')).toBeNull()
  expect(document.querySelector('[data-testid="web-access-notice"]')).toBeNull()
}

function expectNoRegionActivity() {
  expect(harness.loadStatus).not.toHaveBeenCalled()
  expect(vi.getTimerCount()).toBe(0)
  expect(vi.mocked(window.addEventListener).mock.calls.some(([event]) => event === 'focus')).toBe(false)
  expect(vi.mocked(document.addEventListener).mock.calls.some(([event]) => event === 'visibilitychange')).toBe(false)
  expect(MutationObserver.prototype.observe).not.toHaveBeenCalled()
  expect(harness.pauseBackgroundRefresh).not.toHaveBeenCalled()
}

beforeEach(() => {
  vi.resetModules()
  vi.resetAllMocks()
  vi.useFakeTimers()
  localStorage.clear()
  delete window.__APP_CONFIG__
  cleanup = undefined
  document.title = 'Original title'
  document.body.innerHTML = '<div id="app"></div>'
  harness.loadStatus.mockResolvedValue(allowedStatus)
  harness.loadPublicSettings.mockResolvedValue(publicSettings(false))
  harness.initI18n.mockResolvedValue(undefined)
  harness.routerReady.mockResolvedValue(undefined)
  harness.replaceRoute.mockResolvedValue(undefined)
  vi.spyOn(window, 'addEventListener')
  vi.spyOn(document, 'addEventListener')
  vi.spyOn(MutationObserver.prototype, 'observe')
})

afterEach(async () => {
  cleanup?.()
  await flushPromises()
  document.body.innerHTML = ''
  delete window.__APP_CONFIG__
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('actual first-page bootstrap', () => {
  it('starts normally with injected false without HTTP, region UI, timers or event listeners', async () => {
    window.__APP_CONFIG__ = publicSettings(false)
    await startApplication()
    await flushPromises()

    expectRegularApplication()
    expect(harness.loadPublicSettings).not.toHaveBeenCalled()
    expectNoRegionActivity()
    expect(harness.installRouter).toHaveBeenCalledOnce()
    document.title = 'Normal route title'
    await flushPromises()
    expect(document.title).toBe('Normal route title')
  })

  it('uses only the existing public-settings request when there is no injection and the flag is false', async () => {
    const settings = deferred<PublicSettings>()
    harness.loadPublicSettings.mockReturnValue(settings.promise)
    await startApplication()
    expect(harness.loadPublicSettings).toHaveBeenCalledOnce()
    expectNoRegionActivity()
    expect(document.querySelector('[data-testid="web-access-loading"]')).toBeNull()
    expect(harness.installRouter).not.toHaveBeenCalled()

    settings.resolve(publicSettings(false))
    await flushPromises()
    expectRegularApplication()
    expect(harness.loadPublicSettings).toHaveBeenCalledOnce()
    expectNoRegionActivity()
  })

  it('treats the missing legacy public field as disabled', async () => {
    window.__APP_CONFIG__ = publicSettings()
    await startApplication()
    await flushPromises()
    expectRegularApplication()
    expect(harness.loadPublicSettings).not.toHaveBeenCalled()
    expectNoRegionActivity()
  })

  it('preserves normal/setup startup when the public-settings request is unavailable', async () => {
    harness.loadPublicSettings.mockRejectedValue(new Error('Not configured yet'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await startApplication()
    await flushPromises()
    expectRegularApplication()
    expect(harness.loadPublicSettings).toHaveBeenCalledOnce()
    expectNoRegionActivity()
  })

  it('does not flash website content or restriction copy while enabled status and locale loading are pending', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    const status = deferred<WebAccessStatus>()
    const locale = deferred<void>()
    harness.loadStatus.mockReturnValue(status.promise)
    harness.initI18n.mockReturnValue(locale.promise)

    await startApplication()
    expectOnlyLoading()
    expect(harness.installRouter).not.toHaveBeenCalled()
    expect(harness.initI18n).not.toHaveBeenCalled()
    expect(harness.loadPublicSettings).not.toHaveBeenCalled()

    status.resolve(allowedStatus)
    await flushPromises()
    expectOnlyLoading()
    expect(harness.installRouter).not.toHaveBeenCalled()

    locale.resolve()
    await flushPromises()
    expectRegularApplication()
    expect(harness.loadStatus).toHaveBeenCalledOnce()
  })

  it('automatically starts the app when a blocked visitor becomes allowed on the next poll', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    harness.loadStatus.mockResolvedValueOnce(blockedStatus).mockResolvedValue(allowedStatus)
    await startApplication()
    await flushPromises()

    expect(document.querySelector('[data-testid="regular-application"]')).toBeNull()
    expect(document.querySelector('[data-testid="web-access-notice"]')).not.toBeNull()
    expect(document.body.textContent).toContain('指定的地区访问提示')
    expect(document.body.textContent).toContain('本站僅面向海外用戶提供服務')
    expect(document.body.textContent).toContain('Exclusively for overseas users')
    expect(document.querySelector('button')).toBeNull()
    expect(harness.installRouter).not.toHaveBeenCalled()
    expect(harness.initI18n).not.toHaveBeenCalled()

    const { WEB_ACCESS_REFRESH_MS } = await import('@/services/webAccess')
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    await flushPromises()
    expectRegularApplication()
    expect(harness.installRouter).toHaveBeenCalledOnce()
    expect(harness.initI18n).toHaveBeenCalledOnce()
    expect(document.title).toBe(normalTitle)
  })

  it('restores the normal title and starts automatically when the switch is disabled while blocked', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    harness.loadStatus.mockResolvedValue(blockedStatus)
    await startApplication()
    await flushPromises()
    expect(document.title).toBe('Exclusively for overseas users')

    const { webAccessGate, WEB_ACCESS_REFRESH_MS } = await import('@/services/webAccess')
    webAccessGate.setEnabled(false)
    await flushPromises()
    expectRegularApplication()
    expect(document.title).toBe(normalTitle)
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    expect(harness.loadStatus).toHaveBeenCalledOnce()
    expect(harness.pauseBackgroundRefresh).not.toHaveBeenCalled()
  })

  it('enforces the exact blocked title if the admin enables the switch after a normal disabled startup', async () => {
    window.__APP_CONFIG__ = publicSettings(false)
    await startApplication()
    await flushPromises()
    expectRegularApplication()
    expectNoRegionActivity()

    harness.loadStatus.mockResolvedValue(blockedStatus)
    const { webAccessGate } = await import('@/services/webAccess')
    webAccessGate.setEnabled(true)
    await flushPromises()
    expect(document.querySelector('[data-testid="regular-application"]')).toBeNull()
    expect(document.querySelector('[data-testid="web-access-notice"]')).not.toBeNull()
    expect(document.title).toBe('Exclusively for overseas users')
    expect(harness.pauseBackgroundRefresh).toHaveBeenCalledOnce()

    document.title = 'A late normal route title'
    await flushPromises()
    expect(document.title).toBe('Exclusively for overseas users')
    webAccessGate.setEnabled(false)
    await flushPromises()
    expectRegularApplication()
    expect(document.title).toBe('A late normal route title')
  })

  it('keeps the router uninstalled if access becomes blocked during locale loading, then resumes automatically', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    const locale = deferred<void>()
    harness.initI18n.mockReturnValue(locale.promise)
    harness.loadStatus.mockResolvedValueOnce(allowedStatus).mockResolvedValueOnce(blockedStatus).mockResolvedValue(allowedStatus)
    await startApplication()
    await flushPromises()
    expectOnlyLoading()

    const { WEB_ACCESS_REFRESH_MS } = await import('@/services/webAccess')
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    locale.resolve()
    await flushPromises()
    expect(harness.installRouter).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="regular-application"]')).toBeNull()
    expect(document.querySelector('[data-testid="web-access-notice"]')).not.toBeNull()

    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    await flushPromises()
    expectRegularApplication()
    expect(harness.installRouter).toHaveBeenCalledOnce()
  })

  it('preserves an explicit setup signal returned after an old enabled injection', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    harness.loadStatus.mockResolvedValue({ enabled: false, blocked: false, message: '', setup_required: true })
    await startApplication()
    await flushPromises()
    expectRegularApplication()
    expect(harness.replaceRoute).toHaveBeenCalledWith('/setup')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('invalidates a pending region response and releases startup when disposed', async () => {
    window.__APP_CONFIG__ = publicSettings(true)
    const status = deferred<WebAccessStatus>()
    harness.loadStatus.mockReturnValue(status.promise)
    await startApplication()
    expectOnlyLoading()

    cleanup?.()
    status.resolve(blockedStatus)
    await flushPromises()
    expect(document.getElementById('app')?.textContent).toBe('')
    expect(harness.installRouter).not.toHaveBeenCalled()
    expect(harness.initI18n).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
    const { webAccessGate } = await import('@/services/webAccess')
    expect(webAccessGate.state.enabled).toBe(false)
    expect(document.title).toBe(normalTitle)
  })

  it('does not mount after disposal while locale loading is pending', async () => {
    window.__APP_CONFIG__ = publicSettings(false)
    const locale = deferred<void>()
    harness.initI18n.mockReturnValue(locale.promise)
    await startApplication()
    await flushPromises()
    cleanup?.()
    locale.resolve()
    await flushPromises()
    expect(document.getElementById('app')?.textContent).toBe('')
    expect(harness.installRouter).not.toHaveBeenCalled()
    expectNoRegionActivity()
  })
})
