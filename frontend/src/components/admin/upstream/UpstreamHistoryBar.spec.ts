import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UpstreamHistoryRecord } from '@/api/admin/upstreamCenter'
import UpstreamHistoryBar from './UpstreamHistoryBar.vue'

const copy = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copy }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, values?: Record<string, number>) => values?.count != null ? `${key}:${values.count}` : key }) }))
const record: UpstreamHistoryRecord = { id: 1, target_id: 2, model: 'test-model', status: 'error', latency_ms: 1234, ping_latency_ms: null, http_status: 429, message: 'Rate limit exceeded · try later', checked_at: '2026-09-24T00:00:00Z', cost: null, cost_source: 'unknown' }
const views: VueWrapper[] = []
function render(lastCheckedAt: string | null = record.checked_at, enabled = false) {
  const wrapper = mount(UpstreamHistoryBar, { props: { records: [], lastCheckedAt, enabled }, global: { stubs: { HelpTooltip: true, Icon: true } } })
  views.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-24T00:00:05Z')); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); copy.mockClear() })
afterEach(() => { views.splice(0).forEach(view => view.unmount()); vi.restoreAllMocks(); vi.useRealTimers(); document.body.innerHTML = '' })

describe('upstream check history strip', () => {
  it('pads 45 compact bars on the left and exposes copyable error details using HelpTooltip', async () => {
    const wrapper = mount(UpstreamHistoryBar, { props: { records: [record] }, global: { stubs: { Icon: true } } })
    expect(wrapper.findAll('[data-status]')).toHaveLength(45)
    expect(wrapper.findAll('[data-status="unknown"]')).toHaveLength(44)
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull()
    const bar = wrapper.get('[data-status="error"]')
    expect(bar.classes()).toContain('history-bar--failure')
    await bar.trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([record])
    const trigger = wrapper.findComponent({ name: 'HelpTooltip' })
    await trigger.trigger('mouseenter')
    const tooltip = document.body.querySelector('[role="tooltip"]') as HTMLElement
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('HTTP 429')
    expect(tooltip.textContent).toContain('test-model')
    expect(tooltip.textContent).toContain('1,234 ms')
    expect(tooltip.textContent).toContain(record.message)
    tooltip.querySelector('button')?.click()
    expect(copy).toHaveBeenCalledWith(expect.stringContaining('HTTP 429'))
    expect(copy.mock.calls[0]?.[0]).toContain(new Date(record.checked_at).toLocaleString())
    expect(copy.mock.calls[0]?.[0]).toContain('test-model')
    expect(copy.mock.calls[0]?.[0]).toContain('1,234 ms')
    expect(copy.mock.calls[0]?.[0]).toContain(record.message)
    await trigger.trigger('mouseleave')
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull()
    wrapper.unmount()
  })

  it.each([
    [0, 'updatedJustNow', undefined],
    [59, 'updatedJustNow', undefined],
    [60, 'updatedMinutesAgo', 1],
    [3599, 'updatedMinutesAgo', 59],
    [3600, 'updatedHoursAgo', 1],
    [78_915, 'updatedHoursAgo', 21],
    [86_399, 'updatedHoursAgo', 23],
    [86_400, 'updatedDaysAgo', 1],
    [259_200, 'updatedDaysAgo', 3],
    [-60, 'updatedJustNow', undefined],
  ])('formats an age of %s seconds with a coarse unit', (seconds, key, count) => {
    const wrapper = render(new Date(Date.now() - seconds * 1000).toISOString())
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe(`upstreamCenter.history.${key}${count == null ? '' : `:${count}`}`)
  })

  it.each([false, true])('shows waiting for absent or invalid timestamps with enabled=%s', async enabled => {
    const wrapper = render(null, enabled)
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.waiting')
    await wrapper.setProps({ lastCheckedAt: 'invalid timestamp' })
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.waiting')
  })

  it.each([0, 59, 60, 78_915, -60])('shows the exact elapsed seconds for an enabled monitor aged %s seconds', seconds => {
    const wrapper = render(new Date(Date.now() - seconds * 1000).toISOString(), true)
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe(`upstreamCenter.history.updatedSecondsAgo:${Math.max(0, seconds)}`)
  })

  it('shares active seconds independently from paused minute updates and releases unused clocks', async () => {
    const first = render(record.checked_at, true)
    const second = render(record.checked_at, true)
    const paused = render(new Date(Date.now() - 59_000).toISOString())
    expect(vi.getTimerCount()).toBe(2)
    await vi.advanceTimersByTimeAsync(1000)
    expect(first.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedSecondsAgo:6')
    expect(second.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedSecondsAgo:6')
    expect(paused.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedJustNow')
    await first.setProps({ lastCheckedAt: new Date(Date.now()).toISOString() })
    expect(first.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedSecondsAgo:0')
    first.unmount()
    expect(vi.getTimerCount()).toBe(2)
    await second.setProps({ enabled: false })
    expect(second.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedJustNow')
    expect(vi.getTimerCount()).toBe(1)
    await second.setProps({ enabled: true })
    expect(second.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedSecondsAgo:6')
    expect(vi.getTimerCount()).toBe(2)
    paused.unmount()
    expect(vi.getTimerCount()).toBe(1)
    second.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('uses one minute timer across strips and immediately updates changed server timestamps', async () => {
    const first = render()
    const second = render()
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(59_000)
    expect(first.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedJustNow')
    await first.setProps({ lastCheckedAt: '2026-09-23T23:59:00Z' })
    expect(first.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedMinutesAgo:2')
    await vi.advanceTimersByTimeAsync(1000)
    expect(second.get('[data-testid="upstream-last-updated"]').text()).toBe('upstreamCenter.history.updatedMinutesAgo:1')
    first.unmount()
    expect(vi.getTimerCount()).toBe(1)
    second.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([false, true])('suspends the shared timer while hidden and catches up when visible with enabled=%s', async enabled => {
    const wrapper = render(record.checked_at, enabled)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(3_600_000)
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe(enabled ? 'upstreamCenter.history.updatedSecondsAgo:5' : 'upstreamCenter.history.updatedJustNow')
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="upstream-last-updated"]').text()).toBe(enabled ? 'upstreamCenter.history.updatedSecondsAgo:3605' : 'upstreamCenter.history.updatedHoursAgo:1')
    expect(vi.getTimerCount()).toBe(1)
  })
})
