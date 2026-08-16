import { apiClient } from '../client'

export interface CheckinConfig {
  enabled: boolean
  min_recharge_amount: number
  qualification_days: number
  reward_min: number
  reward_max: number
  max_period_reward: number
  min_account_age_hours: number
  timezone: string
  version: number
}

export interface CheckinDailyStat {
  date: string
  user_count: number
  reward_total: number
  reward_average: number
  reward_min: number
  reward_max: number
}

export interface CheckinSummary {
  today_users: number
  today_reward: number
  today_average: number
  users_7_days: number
  reward_30_days: number
  eligible_users: number
  daily: CheckinDailyStat[]
}

export interface CheckinUserReport {
  user_id: number
  email: string
  username: string
  status: string
  current_balance: number
  total_recharge: number
  effective_recharge: number
  checkin_days: number
  reward_total: number
  last_checkin_at?: string
  eligible: boolean
}

export interface CheckinRecord {
  id: number
  user_id: number
  email: string
  username: string
  reward: number
  current_balance: number
  business_date: string
  claimed_at: string
}

export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

const checkinAdminAPI = {
  async getConfig(): Promise<CheckinConfig> {
    const { data } = await apiClient.get<CheckinConfig>('/admin/checkin/config')
    return data
  },

  async updateConfig(config: CheckinConfig): Promise<CheckinConfig> {
    const { data } = await apiClient.put<CheckinConfig>('/admin/checkin/config', config)
    return data
  },

  async getSummary(): Promise<CheckinSummary> {
    const { data } = await apiClient.get<CheckinSummary>('/admin/checkin/summary')
    return data
  },

  async getDaily(startDate: string, endDate: string): Promise<{ items: CheckinDailyStat[] }> {
    const { data } = await apiClient.get<{ items: CheckinDailyStat[] }>('/admin/checkin/daily', {
      params: { start_date: startDate, end_date: endDate }
    })
    return data
  },

  async getUsers(params: Record<string, string | number>): Promise<Paginated<CheckinUserReport>> {
    const { data } = await apiClient.get<Paginated<CheckinUserReport>>('/admin/checkin/users', { params })
    return data
  },

  async getRecords(params: Record<string, string | number>): Promise<Paginated<CheckinRecord>> {
    const { data } = await apiClient.get<Paginated<CheckinRecord>>('/admin/checkin/records', { params })
    return data
  }
}

export default checkinAdminAPI
