import { effectScope, ref, type EffectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAccountListRefresh } from '../useAccountListRefresh'

const scopes: EffectScope[] = []

function startRefresh(overrides: { autoEnabled?: boolean; interval?: number } = {}) {
  const blocked = ref(false)
  const state = {
    autoEnabled: ref(overrides.autoEnabled ?? false),
    autoIntervalSeconds: ref(overrides.interval ?? 30),
    autoCountdown: ref(overrides.autoEnabled ? (overrides.interval ?? 30) : 0),
    silentUntil: ref(0),
    lastRefreshAt: ref(Date.now()),
    isBlocked: () => blocked.value,
    refresh: vi.fn(async () => {})
  }
  const scope = effectScope()
  scopes.push(scope)
  scope.run(() => useAccountListRefresh(state))
  return { ...state, blocked, scope }
}

describe('account list automatic refresh', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-18T00:00:00Z'))
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  })
  afterEach(() => {
    scopes.splice(0).forEach((scope) => scope.stop())
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('does not poll when automatic refresh is disabled', async () => {
    const state = startRefresh()
    await vi.advanceTimersByTimeAsync(10000)
    expect(state.refresh).not.toHaveBeenCalled()
  })

  it('pauses on hidden pages, open editors, loading, and the post-edit silent window', async () => {
    const state = startRefresh({ autoEnabled: true, interval: 1 })
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await vi.advanceTimersByTimeAsync(10000)
    expect(state.refresh).not.toHaveBeenCalled()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    state.blocked.value = true
    await vi.advanceTimersByTimeAsync(10000)
    expect(state.refresh).not.toHaveBeenCalled()
    state.blocked.value = false
    state.silentUntil.value = Date.now() + 5000
    await vi.advanceTimersByTimeAsync(4999)
    expect(state.refresh).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(state.refresh).toHaveBeenCalledTimes(1)
  })

  it('refreshes usage statistics on automatic refresh', async () => {
    const state = startRefresh({ autoEnabled: true, interval: 10 })
    await vi.advanceTimersByTimeAsync(10000)
    expect(state.refresh.mock.calls).toEqual([
      [{ refreshTodayStats: true }]
    ])
    expect(state.autoCountdown.value).toBe(10)
  })

  it('clears the timer on disposal', async () => {
    const state = startRefresh({ autoEnabled: true, interval: 5 })
    await vi.advanceTimersByTimeAsync(5000)
    expect(state.refresh).toHaveBeenCalledTimes(1)
    expect(state.refresh).toHaveBeenCalledWith({ refreshTodayStats: true })
    state.scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })
})
