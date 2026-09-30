import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { PaymentOrder } from '@/types/payment'

export interface PromotionFilters {
  start_time?: string
  end_time?: string
  status?: string
  keyword?: string
  user_id?: number
}

export interface PromotionOrder extends PaymentOrder {
  user_email?: string
  user_username?: string
}

export interface PromotionSummary {
  order_count: number
  pending_order_count: number
  issued_order_count: number
  issued_user_count: number
  base_amount: number
  bonus_amount: number
  refunded_base_amount: number
  refunded_bonus_amount: number
  net_base_amount: number
  net_bonus_amount: number
  cash_by_currency: { currency: string; paid_amount: number; refunded_amount: number; net_amount: number }[]
}

export const adminPromotionsAPI = {
  summary: (params: PromotionFilters = {}) => apiClient.get<PromotionSummary>('/admin/promotions/summary', { params }),
  orders: (params: PromotionFilters & { page?: number; page_size?: number } = {}) =>
    apiClient.get<BasePaginationResponse<PromotionOrder>>('/admin/promotions/orders', { params }),
}
