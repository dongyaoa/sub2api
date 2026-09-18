import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheTotal': 'Cache',
  'usage.cacheRate': 'Cache Hit Rate',
  'usage.cacheRateHint': 'Cache read tokens / (input tokens + cache creation tokens + cache read tokens) × 100%',
  'usage.cacheRateWeightedHint': 'Token-weighted across current filters',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const stats = {
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('only shows the cache rate when enabled', () => {
    const wrapper = mount(UsageStatsCards, { props: { stats } })

    expect(wrapper.find('[data-testid="usage-cache-rate"]').exists()).toBe(false)
    expect(wrapper.findAll('.card')).toHaveLength(4)
  })

  it('calculates the cache read share of all prompt tokens and updates with stats', async () => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats, showCacheRate: true },
      global: { stubs: { HelpTooltip: true } },
    })
    const card = wrapper.get('[data-testid="usage-cache-rate"]')

    expect(wrapper.findAll('.card')).toHaveLength(5)
    expect(card.text()).toContain('Cache Hit Rate')
    expect(card.text()).toContain('Token-weighted across current filters')
    expect(card.text()).toContain('16.4%')
    expect(card.get('help-tooltip-stub').attributes('content')).toBe(messages['usage.cacheRateHint'])
    const progress = card.get('[role="progressbar"]')
    expect(progress.attributes('aria-label')).toBe('Cache Hit Rate')
    expect(progress.attributes('aria-valuenow')).toBe('16.4')
    expect(progress.attributes('aria-valuetext')).toBe('16.4%')
    expect(parseFloat((progress.element.firstElementChild as HTMLElement).style.width)).toBeCloseTo(22 / 134 * 100)

    await wrapper.setProps({ stats: { ...stats, total_output_tokens: 10000 } })
    expect(card.text()).toContain('16.4%')
    expect(progress.attributes('aria-valuenow')).toBe('16.4')

    await wrapper.setProps({ stats: { ...stats, total_input_tokens: 0, total_cache_creation_tokens: 0 } })
    expect(card.text()).toContain('100.0%')
    expect(progress.attributes('aria-valuenow')).toBe('100')
    expect((progress.element.firstElementChild as HTMLElement).style.width).toBe('100%')

    await wrapper.setProps({ stats: { ...stats, total_cache_read_tokens: 0 } })
    expect(card.text()).toContain('0.0%')
    expect(progress.attributes('aria-valuenow')).toBe('0')
    expect((progress.element.firstElementChild as HTMLElement).style.width).toBe('0%')

    await wrapper.setProps({ stats: null })
    expect(card.text()).toContain('0.0%')
    expect(progress.attributes('aria-valuenow')).toBe('0')
  })

  it('weights the overall hit rate by prompt tokens across the filtered requests', () => {
    // A 100-token prompt with 10 cache reads plus a 900-token fully cached
    // prompt gives 91%, rather than the 55% mean of their individual rates.
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats: {
          ...stats,
          total_requests: 2,
          total_input_tokens: 80,
          total_cache_creation_tokens: 10,
          total_cache_read_tokens: 910,
          total_cache_tokens: 920,
          total_output_tokens: 10000,
          total_tokens: 11000,
        },
        showCacheRate: true,
      },
      global: { stubs: { HelpTooltip: true } },
    })
    const card = wrapper.get('[data-testid="usage-cache-rate"]')

    expect(card.text()).toContain('91.0%')
    expect(card.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('91')
  })

  it('shows cache token breakdown values', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache: 34')
    expect(text).toContain('Cache Token Breakdown')
    expect(text).toContain('Cache Creation')
    expect(text).toContain('12')
    expect(text).toContain('Cache Read')
    expect(text).toContain('22')
  })

  it('keeps the cache tooltip out of the layout while it is hidden', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const tooltip = wrapper.findAll('span').find((el) => el.classes().includes('group-hover:block'))

    expect(tooltip).toBeDefined()
    // `opacity-0` hides the tooltip visually but keeps it in the layout, so its
    // fixed width still widens the document and causes horizontal scrolling on
    // narrow screens. `hidden` (display: none) takes it out of the flow.
    expect(tooltip?.classes()).toContain('hidden')
    expect(tooltip?.classes()).not.toContain('opacity-0')
  })
})
