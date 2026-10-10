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
import type { TFunction } from 'i18next'
import type { AIPDDTransitOrderStatus } from './types'

export const AIPDD_TRANSIT_ORDER_STATUS = {
  PENDING: 'PENDING',
  SETTLED: 'SETTLED',
  FAILED: 'FAILED',
  REFUNDED: 'REFUNDED',
} as const satisfies Record<string, AIPDDTransitOrderStatus>

export const AIPDD_TRANSIT_ORDER_STATUS_VALUES = [
  AIPDD_TRANSIT_ORDER_STATUS.PENDING,
  AIPDD_TRANSIT_ORDER_STATUS.SETTLED,
  AIPDD_TRANSIT_ORDER_STATUS.FAILED,
  AIPDD_TRANSIT_ORDER_STATUS.REFUNDED,
] as const

export function getAIPDDTransitOrderStatusLabelKey(
  status: string
): string {
  switch (status) {
    case AIPDD_TRANSIT_ORDER_STATUS.PENDING:
      return 'Pending Settlement'
    case AIPDD_TRANSIT_ORDER_STATUS.SETTLED:
      return 'Settled'
    case AIPDD_TRANSIT_ORDER_STATUS.FAILED:
      return 'Failed'
    case AIPDD_TRANSIT_ORDER_STATUS.REFUNDED:
      return 'Refunded'
    default:
      return status
  }
}

export function getAIPDDTransitOrderStatusOptions(t: TFunction) {
  return AIPDD_TRANSIT_ORDER_STATUS_VALUES.map((value) => ({
    value,
    label: t(getAIPDDTransitOrderStatusLabelKey(value)),
  }))
}
