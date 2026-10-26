import { reactive, readonly } from 'vue'
import { DEFAULT_WEB_ACCESS_MESSAGE, getWebAccessStatus, WebAccessLocationUnavailableError, type WebAccessLocationSource, type WebAccessStatus } from '@/api/webAccess'

export type WebAccessPhase = 'checking' | 'allowed' | 'blocked' | 'error'
export const WEB_ACCESS_REFRESH_MS = 60_000
export const WEB_ACCESS_LOCAL_REFRESH_MS = 5_000

export function createWebAccessGate(fetchStatus: () => Promise<WebAccessStatus> = getWebAccessStatus) {
  const state = reactive({
    phase: 'allowed' as WebAccessPhase,
    checking: false,
    message: DEFAULT_WEB_ACCESS_MESSAGE,
    enabled: false,
    setupRequired: false,
    locationSource: 'client_ip' as WebAccessLocationSource,
  })
  let lastCheckedAt = 0
  let generation = 0
  let pending: Promise<boolean> | null = null
  let interval: ReturnType<typeof setInterval> | null = null

  const isAllowed = () => !state.enabled || state.phase === 'allowed'
  const refreshIntervalMs = () => state.locationSource === 'local_network' ? WEB_ACCESS_LOCAL_REFRESH_MS : WEB_ACCESS_REFRESH_MS

  function schedulePolling() {
    interval = setInterval(() => { void check(true) }, refreshIntervalMs())
  }

  function check(force = false): Promise<boolean> {
    if (!state.enabled) return Promise.resolve(true)
    if (pending) return pending
    if (!force && lastCheckedAt && Date.now() - lastCheckedAt < refreshIntervalMs()) {
      return Promise.resolve(isAllowed())
    }
    const requestGeneration = generation
    state.checking = true
    const request = Promise.resolve().then(() => {
      if (requestGeneration !== generation || !state.enabled) return null
      return fetchStatus()
    }).then((result) => {
      if (result === null || requestGeneration !== generation || !state.enabled) return isAllowed()
      if (!result.enabled) {
        setEnabled(false)
        state.setupRequired = result.setup_required === true
        return true
      }
      state.phase = result.blocked ? 'blocked' : 'allowed'
      state.message = result.message || DEFAULT_WEB_ACCESS_MESSAGE
      state.setupRequired = result.setup_required === true
      const previousIntervalMs = refreshIntervalMs()
      state.locationSource = result.location_source === 'local_network' ? 'local_network' : 'client_ip'
      if (interval !== null && previousIntervalMs !== refreshIntervalMs()) {
        clearInterval(interval)
        schedulePolling()
      }
      lastCheckedAt = Date.now()
      return !result.blocked
    }).catch((error: unknown) => {
      if (requestGeneration !== generation || !state.enabled) return isAllowed()
      if (error instanceof WebAccessLocationUnavailableError && state.phase !== 'blocked') {
        state.phase = 'error'
        state.setupRequired = false
      } else if (state.phase !== 'blocked' && state.phase !== 'allowed') {
        state.phase = 'error'
      }
      lastCheckedAt = 0
      return isAllowed()
    }).finally(() => {
      if (pending === request && requestGeneration === generation) {
        pending = null
        state.checking = false
      }
    })
    pending = request
    return request
  }

  async function refreshAfterUpdate(): Promise<boolean> {
    if (!state.enabled) return true
    if (pending) await pending
    return check(true)
  }

  function onFocus() {
    if (state.enabled && document.visibilityState !== 'hidden') void check(true)
  }

  function start() {
    if (!state.enabled || interval !== null) return
    schedulePolling()
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onFocus)
  }

  function stop() {
    if (interval !== null) {
      clearInterval(interval)
      interval = null
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onFocus)
    }
  }

  function setEnabled(enabled: boolean) {
    if (enabled === state.enabled) {
      if (!enabled) state.setupRequired = false
      return
    }
    generation++
    pending = null
    lastCheckedAt = 0
    state.enabled = enabled
    state.checking = false
    state.setupRequired = false
    state.locationSource = 'client_ip'
    if (!enabled) {
      stop()
      state.phase = 'allowed'
      return
    }
    state.phase = 'checking'
    start()
    void check(true)
  }

  return { state: readonly(state), setEnabled, check, refreshAfterUpdate, start, stop }
}

export const webAccessGate = createWebAccessGate()
