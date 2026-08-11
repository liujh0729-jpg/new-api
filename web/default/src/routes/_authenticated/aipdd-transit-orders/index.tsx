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
import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { AIPDDTransitOrdersPage } from '@/features/aipdd-transit-orders'
import { AIPDD_TRANSIT_ORDER_STATUS_VALUES } from '@/features/aipdd-transit-orders/constants'

const transitOrdersSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(20),
  startTime: z.string().optional().catch(undefined),
  endTime: z.string().optional().catch(undefined),
  platformOrderId: z.string().optional().catch(undefined),
  userId: z.string().optional().catch(undefined),
  tokenId: z.string().optional().catch(undefined),
  channelId: z.string().optional().catch(undefined),
  model: z.string().optional().catch(undefined),
  status: z.enum(AIPDD_TRANSIT_ORDER_STATUS_VALUES).optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/aipdd-transit-orders/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: transitOrdersSearchSchema,
  component: AIPDDTransitOrdersPage,
})
