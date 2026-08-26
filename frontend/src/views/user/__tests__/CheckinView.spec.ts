import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CheckinView from '../CheckinView.vue'
import type { CheckinStatus } from '@/api/checkin'

const getStatus = vi.hoisted(() => vi.fn())
const claim = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/api', () => ({
  checkinAPI: { getStatus, claim }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser: vi.fn() })
}))

const statusFixture = (): CheckinStatus => ({
  enabled: true,
  eligible: true,
  reason: 'eligible',
  business_date: '2026-08-25',
  claimed_today: false,
  today_reward: 0,
  current_balance: 25.5,
  total_reward: 12.34,
  recharge_amount: 120,
  min_recharge_amount: 10,
  qualification_days: 30,
  min_account_age_hours: 24,
  reward_min: 0.3,
  reward_max: 0.5,
  current_tier: {
    id: 'gold',
    name: '黄金档',
    min_recharge_amount: 100,
    reward_min: 0.3,
    reward_max: 0.5,
    enabled: true,
    visible: true,
    qualified_only_visible: true,
    show_next_progress: true,
    custom_button_enabled: true,
    button_color: 'blue',
    tier_badge_enabled: true
  },
  next_tier: {
    id: 'platinum',
    name: '铂金档',
    min_recharge_amount: 200,
    reward_min: 0.6,
    reward_max: 0.8,
    enabled: true,
    visible: true,
    qualified_only_visible: true,
    show_next_progress: true,
    custom_button_enabled: false,
    button_color: 'emerald',
    tier_badge_enabled: false
  },
  reward_tiers: [
    {
      id: 'default',
      name: '默认档位',
      min_recharge_amount: 10,
      reward_min: 0.01,
      reward_max: 0.05,
      enabled: true,
      visible: true,
      qualified_only_visible: false,
      show_next_progress: false,
      custom_button_enabled: false,
      button_color: 'emerald',
      tier_badge_enabled: false,
      is_default: true
    }
  ],
  next_reset_at: '2026-08-26T00:00:00+08:00',
  history: []
})

describe('CheckinView reward tiers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getStatus.mockResolvedValue(statusFixture())
  })

  it('renders current and next tier summaries with a gradient claim button', async () => {
    const wrapper = shallowMount(CheckinView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          LoadingSpinner: true
        }
      }
    })

    await flushPromises()

    expect(getStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('checkin.totalReward')
    expect(wrapper.text()).toContain('$12.34')
    expect(wrapper.text()).toContain('黄金档')
    expect(wrapper.text()).toContain('铂金档')
    expect(wrapper.text()).toContain('$0.30 - $0.50')
    expect(wrapper.find('[data-test="checkin-claim-panel"]').attributes('data-custom-button')).toBe('true')
    expect(wrapper.find('[data-test="checkin-claim-button"]').classes()).toContain('bg-gradient-to-r')
    expect(wrapper.find('[data-test="checkin-claim-button"]').classes()).toContain('from-sky-400')
    expect(wrapper.text()).toContain('checkin.nextTierProgress')
    expect(wrapper.text()).toContain('$120.00 / $200.00')
  })

  it('keeps progress, badge, and button styling independent from tier-list visibility', async () => {
    const hiddenStatus = statusFixture()
    hiddenStatus.current_tier!.visible = false
    hiddenStatus.next_tier!.visible = false
    getStatus.mockResolvedValue(hiddenStatus)
    const wrapper = shallowMount(CheckinView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          LoadingSpinner: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('黄金档')
    expect(wrapper.text()).toContain('checkin.nextTierProgress')
    expect(wrapper.text()).not.toContain('checkin.rewardTiers')
    expect(wrapper.find('[data-test="checkin-claim-button"]').classes()).toContain('from-sky-400')
  })
})
