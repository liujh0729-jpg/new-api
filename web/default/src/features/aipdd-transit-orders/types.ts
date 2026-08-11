/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type AIPDDTransitOrderStatus =
  | 'PENDING'
  | 'SETTLED'
  | 'FAILED'
  | 'REFUNDED'

export type AIPDDTransitOrder = {
  platform_order_id: string
  user_id: number
  username: string
  token_id: number
  token_name: string
  channel_id: number
  channel_name: string
  channel_key_index: number
  model: string
  status: AIPDDTransitOrderStatus | string
  customer_charge_quota: number
  customer_charge_rmb: number
  source_charge_awcoin: number | null
  source_charge_rmb: number | null
  created_at: number
  settled_at: number | null
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message: string
  data: T
}

export type GetAIPDDTransitOrdersParams = {
  p?: number
  page_size?: number
  start_timestamp?: number
  end_timestamp?: number
  platform_order_id?: string
  user_id?: number
  token_id?: number
  channel_id?: number
  model?: string
  status?: string
}

export type GetAIPDDTransitOrdersResponse = ApiResponse<{
  page: number
  page_size: number
  total: number
  items: AIPDDTransitOrder[]
}>

export type AIPDDTransitOrderFilters = {
  startTime?: Date
  endTime?: Date
  platformOrderId?: string
  userId?: string
  tokenId?: string
  channelId?: string
  model?: string
  status?: string
}
