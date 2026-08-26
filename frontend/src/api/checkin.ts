import { apiClient } from './client'

export interface CheckinHistoryItem {
  id: number
  business_date: string
  reward: number
  claimed_at: string
}

export interface CheckinRewardTier {
  id: string
  name: string
  min_recharge_amount: number
  reward_min: number
  reward_max: number
  enabled: boolean
  visible: boolean
  qualified_only_visible: boolean
  show_next_progress: boolean
  custom_button_enabled: boolean
  button_color: CheckinButtonColor
  tier_badge_enabled: boolean
  is_default?: boolean
}

export type CheckinButtonColor = 'emerald' | 'blue' | 'amber' | 'rose' | 'violet' | 'slate'

export interface CheckinStatus {
  enabled: boolean
  eligible: boolean
  reason: string
  business_date: string
  claimed_today: boolean
  today_reward: number
  current_balance: number
  total_reward: number
  recharge_amount: number
  min_recharge_amount: number
  qualification_days: number
  min_account_age_hours: number
  reward_min: number
  reward_max: number
  current_tier?: CheckinRewardTier
  next_tier?: CheckinRewardTier
  reward_tiers: CheckinRewardTier[]
  next_reset_at: string
  history: CheckinHistoryItem[]
}

export interface CheckinClaimResult {
  business_date: string
  reward: number
  new_balance: number
  claimed_at: string
  already_claimed: boolean
  tier_id: string
  tier_name: string
  tier_is_default: boolean
}

export const checkinAPI = {
  async getStatus(): Promise<CheckinStatus> {
    const { data } = await apiClient.get<CheckinStatus>('/checkin/status')
    return data
  },

  async claim(): Promise<CheckinClaimResult> {
    const { data } = await apiClient.post<CheckinClaimResult>('/checkin/claim')
    return data
  },

  async getHistory(): Promise<CheckinHistoryItem[]> {
    const { data } = await apiClient.get<CheckinHistoryItem[]>('/checkin/history')
    return data
  }
}

export default checkinAPI
