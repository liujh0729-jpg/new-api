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
import { api } from '@/lib/api'
import type {
  GetAIPDDTransitOrdersParams,
  GetAIPDDTransitOrdersResponse,
} from './types'

export async function getAIPDDTransitOrders(
  params: GetAIPDDTransitOrdersParams = {}
): Promise<GetAIPDDTransitOrdersResponse> {
  const search = new URLSearchParams()
  search.set('p', String(params.p ?? 1))
  search.set('page_size', String(params.page_size ?? 20))

  if (params.start_timestamp) {
    search.set('start_timestamp', String(params.start_timestamp))
  }
  if (params.end_timestamp) {
    search.set('end_timestamp', String(params.end_timestamp))
  }
  if (params.platform_order_id) {
    search.set('platform_order_id', params.platform_order_id)
  }
  if (params.user_id != null) {
    search.set('user_id', String(params.user_id))
  }
  if (params.token_id != null) {
    search.set('token_id', String(params.token_id))
  }
  if (params.channel_id != null) {
    search.set('channel_id', String(params.channel_id))
  }
  if (params.model) {
    search.set('model', params.model)
  }
  if (params.status) {
    search.set('status', params.status)
  }

  const res = await api.get(
    `/api/aipdd-transit-orders?${search.toString()}`
  )
  return res.data
}
