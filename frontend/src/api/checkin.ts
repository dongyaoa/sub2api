import { apiClient } from './client'

export interface CheckinHistoryItem {
  id: number
  business_date: string
  reward: number
  claimed_at: string
}

export interface CheckinStatus {
  enabled: boolean
  eligible: boolean
  reason: string
  business_date: string
  claimed_today: boolean
  today_reward: number
  current_balance: number
  recharge_amount: number
  min_recharge_amount: number
  qualification_days: number
  min_account_age_hours: number
  reward_min: number
  reward_max: number
  next_reset_at: string
  history: CheckinHistoryItem[]
}

export interface CheckinClaimResult {
  business_date: string
  reward: number
  new_balance: number
  claimed_at: string
  already_claimed: boolean
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
