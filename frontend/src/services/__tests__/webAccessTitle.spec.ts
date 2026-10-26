import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createWebAccessGate } from '../webAccess'
import { bindWebAccessTitle, WEB_ACCESS_BLOCKED_TITLE } from '../webAccessTitle'

const allowed = { enabled: true, blocked: false, message: '' }
const blocked = { ...allowed, blocked: true }
let gate: ReturnType<typeof createWebAccessGate>
let dispose: (() => void) | undefined

beforeEach(() => { document.title = 'Dashboard - My Site' })
afterEach(() => {
  gate?.setEnabled(false)
  dispose?.()
  dispose = undefined
  vi.restoreAllMocks()
})

describe('real web access title policy', () => {
  it('uses exactly the English restriction title and restores the page title on allowance', async () => {
    const fetchStatus = vi.fn().mockResolvedValueOnce(blocked).mockResolvedValueOnce(allowed)
    gate = createWebAccessGate(fetchStatus)
    dispose = bindWebAccessTitle(gate)
    gate.setEnabled(true)
    await gate.check()
    expect(document.title).toBe('Exclusively for overseas users')
    await gate.check(true)
    expect(document.title).toBe('Dashboard - My Site')
  })

  it.each(['locale', 'router', 'settings', 'App'])('corrects a late %s title write and remembers it when disabled', async source => {
    gate = createWebAccessGate(vi.fn().mockResolvedValue(blocked))
    dispose = bindWebAccessTitle(gate)
    gate.setEnabled(true)
    await gate.check()
    await Promise.resolve().then(() => { document.title = `${source} - Updated Site` })
    await flushPromises()
    expect(document.title).toBe(WEB_ACCESS_BLOCKED_TITLE)
    gate.setEnabled(false)
    expect(document.title).toBe(`${source} - Updated Site`)
    document.title = 'Another route - Updated Site'
    await flushPromises()
    expect(document.title).toBe('Another route - Updated Site')
  })

  it('does not observe the document while disabled or allowed', async () => {
    const observe = vi.spyOn(MutationObserver.prototype, 'observe')
    const disconnect = vi.spyOn(MutationObserver.prototype, 'disconnect')
    const fetchStatus = vi.fn().mockResolvedValueOnce(allowed).mockResolvedValueOnce(blocked)
    gate = createWebAccessGate(fetchStatus)
    dispose = bindWebAccessTitle(gate)
    expect(observe).not.toHaveBeenCalled()
    gate.setEnabled(true)
    await gate.check()
    expect(observe).not.toHaveBeenCalled()
    await gate.check(true)
    expect(observe).toHaveBeenCalledOnce()
    gate.setEnabled(false)
    expect(disconnect).toHaveBeenCalledOnce()
  })
})
