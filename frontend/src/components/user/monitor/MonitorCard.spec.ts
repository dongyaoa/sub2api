import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { UserMonitorView } from '@/api/channelMonitor'
import MonitorCard from './MonitorCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'channelStatus.groupRateValue') return `倍率 ${params?.value}x`
      if (key === 'channelStatus.groupRateTitle') return `${params?.group} 当前倍率：${params?.value}x`
      return key
    },
  }),
}))

function monitor(overrides: Partial<UserMonitorView> = {}): UserMonitorView {
  return {
    id: 1,
    name: 'OpenAI 主渠道',
    provider: 'openai',
    group_name: '',
    group_rate_multiplier: 0.35,
    primary_model: 'gpt-5',
    primary_status: 'operational',
    primary_latency_ms: 120,
    primary_ping_latency_ms: 20,
    availability_7d: 99.9,
    extra_models: [],
    timeline: [],
    ...overrides,
  }
}

function render(item: UserMonitorView) {
  return shallowMount(MonitorCard, {
    props: {
      item,
      window: '7d',
      availabilityValue: 99.9,
      countdownSeconds: 30,
    },
    global: {
      stubs: {
        ProviderIcon: true,
        MonitorMetricPair: true,
        MonitorAvailabilityRow: true,
        MonitorTimeline: true,
      },
    },
  })
}

describe('MonitorCard group rate', () => {
  it('shows the live group multiplier with adaptive precision', () => {
    const wrapper = render(monitor())

    expect(wrapper.get('[data-testid="monitor-group-rate"]').text()).toBe('0.35x')
    expect(wrapper.get('[data-testid="monitor-status-stack"]').text()).toContain('0.35x')
    expect(wrapper.get('[data-testid="monitor-group-rate"]').text()).not.toContain('倍率')
    expect(wrapper.get('[title="倍率 0.35x"]').exists()).toBe(true)
  })

  it('does not render a rate when no group matches', () => {
    const wrapper = render(monitor({ group_rate_multiplier: null }))

    expect(wrapper.find('[data-testid="monitor-group-rate"]').exists()).toBe(false)
  })
})
