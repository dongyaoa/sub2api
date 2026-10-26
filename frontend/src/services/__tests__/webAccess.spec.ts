import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createWebAccessGate, WEB_ACCESS_REFRESH_MS, WEB_ACCESS_LOCAL_REFRESH_MS } from '../webAccess'
import { WebAccessLocationUnavailableError } from '@/api/webAccess'
import type { WebAccessStatus } from '@/api/webAccess'

const allowed = { enabled: true, blocked: false, message: '' }
const blocked = { enabled: true, blocked: true, message: '当前地区暂不支持访问' }
const gates: ReturnType<typeof createWebAccessGate>[] = []

function enabledGate(fetchStatus: () => Promise<WebAccessStatus>) {
  const gate = createWebAccessGate(fetchStatus)
  gates.push(gate)
  gate.setEnabled(true)
  return gate
}

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-25T00:00:00Z')) })
afterEach(() => {
  gates.splice(0).forEach(gate => gate.setEnabled(false))
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('web access gate', () => {
  it('does no requests, timers or focus work while disabled, including explicit checks', async () => {
    const fetchStatus = vi.fn()
    const addWindowListener = vi.spyOn(window, 'addEventListener')
    const addDocumentListener = vi.spyOn(document, 'addEventListener')
    const gate = createWebAccessGate(fetchStatus)
    gate.start()
    expect(await gate.check(true)).toBe(true)
    expect(await gate.refreshAfterUpdate()).toBe(true)
    window.dispatchEvent(new Event('focus'))
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS * 2)
    expect(fetchStatus).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
    expect(addWindowListener.mock.calls.some(([event]) => event === 'focus')).toBe(false)
    expect(addDocumentListener.mock.calls.some(([event]) => event === 'visibilitychange')).toBe(false)
    expect(gate.state.phase).toBe('allowed')
  })

  it('cancels a queued check when disabled before its request starts', async () => {
    const fetchStatus = vi.fn().mockResolvedValue(blocked)
    const gate = enabledGate(fetchStatus)
    gate.setEnabled(false)
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS * 2)
    expect(fetchStatus).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
    expect(gate.state.phase).toBe('allowed')
  })

  it.each(['blocked', 'error'])('ignores an in-flight %s result after disable', async outcome => {
    let resolve!: (value: WebAccessStatus) => void
    let reject!: (error: Error) => void
    const fetchStatus = vi.fn(() => new Promise<WebAccessStatus>((done, fail) => { resolve = done; reject = fail }))
    const gate = enabledGate(fetchStatus)
    const pending = gate.check()
    await Promise.resolve()
    gate.setEnabled(false)
    if (outcome === 'error') reject(new WebAccessLocationUnavailableError())
    else resolve(blocked)
    await pending
    expect(gate.state).toMatchObject({ enabled: false, phase: 'allowed', checking: false })
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps a newly enabled check authoritative when an older generation resolves later', async () => {
    let resolveOld!: (value: WebAccessStatus) => void
    let resolveNew!: (value: WebAccessStatus) => void
    const fetchStatus = vi.fn()
      .mockImplementationOnce(() => new Promise<WebAccessStatus>(done => { resolveOld = done }))
      .mockImplementationOnce(() => new Promise<WebAccessStatus>(done => { resolveNew = done }))
    const gate = enabledGate(fetchStatus)
    const oldRequest = gate.check()
    await Promise.resolve()
    gate.setEnabled(false)
    gate.setEnabled(true)
    const newRequest = gate.check()
    await Promise.resolve()
    resolveOld(blocked)
    await oldRequest
    expect(gate.state).toMatchObject({ phase: 'checking', checking: true })
    resolveNew(allowed)
    await newRequest
    expect(gate.state).toMatchObject({ phase: 'allowed', checking: false })
  })

  it('waits for an authoritative decision and deduplicates concurrent checks', async () => {
    let resolve!: (value: typeof blocked) => void
    const fetchStatus = vi.fn(() => new Promise<typeof blocked>(done => { resolve = done }))
    const gate = enabledGate(fetchStatus)
    const first = gate.check()
    const second = gate.check(true)
    expect(gate.state.phase).toBe('checking')
    await Promise.resolve()
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    resolve(blocked)
    expect(await first).toBe(false)
    expect(await second).toBe(false)
    expect(gate.state.phase).toBe('blocked')
  })

  it('never turns a known denial into permission after a failed retry', async () => {
    const fetchStatus = vi.fn().mockResolvedValueOnce(blocked).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(allowed)
    const gate = enabledGate(fetchStatus)
    await gate.check()
    await gate.check(true)
    expect(gate.state.phase).toBe('blocked')
    expect(gate.state.message).toBe(blocked.message)
    await gate.check(true)
    expect(gate.state.phase).toBe('allowed')
  })

  it('shows a retryable error instead of content when the first request fails', async () => {
    const gate = enabledGate(vi.fn().mockRejectedValue(new Error('offline')))
    expect(await gate.check()).toBe(false)
    expect(gate.state.phase).toBe('error')
  })

  it.each([true, false])('preserves an allowed page during a background failure (enabled=%s)', async enabled => {
    const fetchStatus = vi.fn().mockResolvedValueOnce({ ...allowed, enabled }).mockRejectedValueOnce(new Error('offline'))
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(await gate.check(true)).toBe(true)
    expect(gate.state.phase).toBe('allowed')
    expect(gate.state.enabled).toBe(enabled)
  })

  it('requires a retry when the server explicitly cannot complete the local check after access was allowed', async () => {
    const fetchStatus = vi.fn()
      .mockResolvedValueOnce(allowed)
      .mockRejectedValueOnce(new WebAccessLocationUnavailableError())
      .mockResolvedValueOnce(allowed)
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(await gate.refreshAfterUpdate()).toBe(false)
    expect(gate.state.phase).toBe('error')
    expect(await gate.check(true)).toBe(true)
    expect(gate.state.phase).toBe('allowed')
  })

  it('shows an error on an explicit local detection failure during the first check', async () => {
    const gate = enabledGate(vi.fn().mockRejectedValue(new WebAccessLocationUnavailableError()))
    expect(await gate.check()).toBe(false)
    expect(gate.state.phase).toBe('error')
  })

  it('keeps a previously blocked visitor blocked when local detection subsequently fails', async () => {
    const fetchStatus = vi.fn().mockResolvedValueOnce(blocked).mockRejectedValueOnce(new WebAccessLocationUnavailableError())
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(await gate.check(true)).toBe(false)
    expect(gate.state.phase).toBe('blocked')
    expect(gate.state.message).toBe(blocked.message)
  })

  it('issues a fresh check after a settings update even if an older request is still pending', async () => {
    let resolve!: (value: typeof allowed) => void
    const fetchStatus = vi.fn()
      .mockImplementationOnce(() => new Promise<typeof allowed>(done => { resolve = done }))
      .mockResolvedValueOnce(blocked)
    const gate = enabledGate(fetchStatus)
    void gate.check()
    const refresh = gate.refreshAfterUpdate()
    await Promise.resolve()
    resolve(allowed)
    expect(await refresh).toBe(false)
    expect(fetchStatus).toHaveBeenCalledTimes(2)
    expect(gate.state.phase).toBe('blocked')
  })

  it('refreshes on focus and within 60 seconds, and cleans up its listeners', async () => {
    const fetchStatus = vi.fn().mockResolvedValue({ ...allowed, location_source: 'client_ip' })
    const gate = enabledGate(fetchStatus)
    await gate.check()
    gate.start()
    await gate.check()
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_LOCAL_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS - WEB_ACCESS_LOCAL_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(2)
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchStatus).toHaveBeenCalledTimes(3)
    gate.stop()
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(3)
  })

  it('rechecks a server-confirmed local network every five seconds and applies changing decisions', async () => {
    const localAllowed = { ...allowed, location_source: 'local_network' }
    const fetchStatus = vi.fn().mockResolvedValue(localAllowed)
      .mockResolvedValueOnce(localAllowed)
      .mockResolvedValueOnce({ ...blocked, location_source: 'local_network' })
    const gate = enabledGate(fetchStatus)
    gate.start()
    await gate.check()
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_LOCAL_REFRESH_MS - 1)
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchStatus).toHaveBeenCalledTimes(2)
    expect(gate.state.phase).toBe('blocked')
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_LOCAL_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(3)
    expect(gate.state.phase).toBe('allowed')
    gate.stop()
  })

  it('reschedules an active timer when local mode is enabled and restores sixty seconds when disabled', async () => {
    const clientStatus = { ...allowed, location_source: 'client_ip' }
    const fetchStatus = vi.fn().mockResolvedValue(clientStatus)
      .mockResolvedValueOnce(clientStatus)
      .mockResolvedValueOnce({ ...allowed, location_source: 'local_network' })
    const gate = enabledGate(fetchStatus)
    await gate.check()
    gate.start()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetchStatus).toHaveBeenCalledTimes(1)

    await gate.refreshAfterUpdate()
    expect(gate.state.locationSource).toBe('local_network')
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_LOCAL_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(3)
    expect(gate.state.locationSource).toBe('client_ip')

    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS - 1)
    expect(fetchStatus).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchStatus).toHaveBeenCalledTimes(4)
    gate.stop()
  })

  it('stops all later checks when a status response disables the feature', async () => {
    const fetchStatus = vi.fn().mockResolvedValue({ ...allowed, enabled: false })
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(gate.state.enabled).toBe(false)
    await gate.refreshAfterUpdate()
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS * 2)
    expect(fetchStatus).toHaveBeenCalledTimes(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('preserves a confirmed setup response until the next public configuration is applied', async () => {
    const fetchStatus = vi.fn().mockResolvedValue({ ...allowed, enabled: false, setup_required: true })
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(gate.state).toMatchObject({ enabled: false, phase: 'allowed', setupRequired: true })
    expect(vi.getTimerCount()).toBe(0)
    gate.setEnabled(false)
    expect(gate.state.setupRequired).toBe(false)
  })

  it('automatically retries an initial error while enabled', async () => {
    const fetchStatus = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(allowed)
    const gate = enabledGate(fetchStatus)
    await gate.check()
    expect(gate.state.phase).toBe('error')
    await vi.advanceTimersByTimeAsync(WEB_ACCESS_REFRESH_MS)
    expect(fetchStatus).toHaveBeenCalledTimes(2)
    expect(gate.state.phase).toBe('allowed')
  })
})
