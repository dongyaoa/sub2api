import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { MonitorTimelinePoint } from '@/api/channelMonitor'
import MonitorTimeline from './MonitorTimeline.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: ref('en'),
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'channelStatus.cards.latestCheck') return `Last checked ${params?.time}`
        if (key === 'monitorCommon.relativeSecondsAgo') return `${params?.n} seconds ago`
        if (key === 'monitorCommon.relativeMinutesAgo') return `${params?.n} minutes ago`
        return key
      },
    }),
  }
})

const now = Date.parse('2026-09-19T06:32:18Z')

function points(count: number): MonitorTimelinePoint[] {
  return Array.from({ length: count }, (_, index) => ({
    status: 'operational',
    latency_ms: 100 + index,
    ping_latency_ms: 20,
    checked_at: new Date(now - index * 60000).toISOString(),
  }))
}

describe('MonitorTimeline', () => {
  it('keeps the latest 45 real checks, ordered oldest to newest, without mutating the response', () => {
    const buckets = points(60)
    const originalOrder = buckets.map(point => point.checked_at)
    const wrapper = mount(MonitorTimeline, { props: { buckets, now } })
    const bars = wrapper.findAll('[data-testid="monitor-timeline-bar"]')

    expect(bars).toHaveLength(45)
    expect(bars.map(bar => bar.attributes('data-checked-at'))).toEqual(originalOrder.slice(0, 45).reverse())
    expect(buckets.map(point => point.checked_at)).toEqual(originalOrder)
  })

  it('pads missing history on the left while keeping the newest check at NOW', () => {
    const buckets = points(2)
    const wrapper = mount(MonitorTimeline, { props: { buckets, now } })
    const bars = wrapper.findAll('[data-testid="monitor-timeline-bar"]')

    expect(bars).toHaveLength(45)
    expect(bars.slice(0, 43).every(bar => !bar.attributes('data-checked-at'))).toBe(true)
    expect(bars[43].attributes('data-checked-at')).toBe(buckets[1].checked_at)
    expect(bars[44].attributes('data-checked-at')).toBe(buckets[0].checked_at)
  })

  it('uses the detection timestamp and advances its age even when the page has just refreshed', async () => {
    const lastCheckedAt = new Date(now - 30000).toISOString()
    const wrapper = mount(MonitorTimeline, { props: { buckets: points(2), lastCheckedAt, now } })

    expect(wrapper.text()).toContain('Last checked 30 seconds ago')
    await wrapper.setProps({ now: now + 60000, stale: true })
    expect(wrapper.text()).toContain('Last checked 1 minutes ago')
    expect(wrapper.text()).toContain('channelStatus.cards.stale')
  })

  it('distinguishes no history from successful fresh checks', () => {
    const wrapper = mount(MonitorTimeline, { props: { now } })

    expect(wrapper.text()).toContain('channelStatus.cards.noChecks')
    expect(wrapper.findAll('[data-checked-at]')).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="monitor-timeline-bar"]')).toHaveLength(45)
  })
})
