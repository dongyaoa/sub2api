import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { UserMonitorView } from '@/api/channelMonitor'
import MonitorCard from './MonitorCard.vue'

vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorQuotaVisible: () => false,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'channelStatus.groupRateValue') return `倍率 ${params?.value}x`
        if (key === 'channelStatus.groupRateTitle') return `${params?.group} 当前倍率：${params?.value}x`
        return key
      },
    }),
  }
})

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

describe('MonitorCard model labels', () => {
  it.each([
    ['gpt-5.6-sol', 'GPT-5.6 Sol'],
    ['gpt-6-astra', 'GPT-6 Astra'],
    ['grok-4.5', 'Grok 4.5'],
    ['claude-opus-4-6', 'Claude Opus 4.6'],
    ['gemini-3.7-flash-high', 'Gemini 3.7 Flash (high)'],
    ['claude-opus-4-7', 'Claude Opus 4.7'],
    ['claude-sonnet-5', 'Claude Sonnet 5'],
    ['custom-model-latest', 'custom-model-latest'],
    ['quota', 'monitorCommon.checkMode.quota'],
  ])('displays %s as %s without changing the monitor model', async (model, label) => {
    const item = Object.freeze(monitor({ primary_model: model }))
    const wrapper = render(item)

    expect(wrapper.get('.monitor-model-badge').text()).toBe(label)
    expect(wrapper.get('.monitor-model-badge').attributes('title')).toBe(label)
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
    expect(wrapper.props('item').primary_model).toBe(model)
  })

  it('updates the label when the configured model changes', async () => {
    const wrapper = render(monitor({ primary_model: 'gpt-5.6-sol' }))

    await wrapper.setProps({ item: monitor({ primary_model: 'gpt-6-astra' }) })

    expect(wrapper.get('.monitor-model-badge').text()).toBe('GPT-6 Astra')
    expect(wrapper.props('item').primary_model).toBe('gpt-6-astra')
  })
})

describe('MonitorCard group rate', () => {
  it('shows the live group multiplier with adaptive precision', () => {
    const wrapper = render(monitor())

    expect(wrapper.get('[data-testid="monitor-group-rate"]').text()).toBe('0.35x')
    expect(wrapper.get('[data-testid="monitor-group-rate"]').text()).not.toContain('倍率')
    expect(wrapper.get('[title="倍率 0.35x"]').exists()).toBe(true)
  })

  it('does not render a rate when no group matches', () => {
    const wrapper = render(monitor({ group_rate_multiplier: null }))

    expect(wrapper.find('[data-testid="monitor-group-rate"]').exists()).toBe(false)
  })

  it('retains the group name in the multiplier tooltip', () => {
    const wrapper = render(monitor({ group_name: '官方渠道', group_rate_multiplier: 1.025 }))

    expect(wrapper.get('[data-testid="monitor-group-rate"]').attributes('title')).toBe('官方渠道 当前倍率：1.025x')
  })

  it('opens the monitor detail when selected', async () => {
    const wrapper = render(monitor())

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('click')).toHaveLength(1)
  })
})
