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
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { DataTablePage } from '@/components/data-table'
import { getAIPDDTransitOrders } from '../api'
import { parseOptionalInt } from '../lib/format'
import { useTransitOrdersColumns } from './transit-orders-columns'
import { TransitOrdersFilterBar } from './transit-orders-filter-bar'

const route = getRouteApi('/_authenticated/aipdd-transit-orders/')

export function TransitOrdersTable() {
  const { t } = useTranslation()
  const columns = useTransitOrdersColumns()
  const searchParams = route.useSearch()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const { pagination, onPaginationChange, ensurePageInRange } =
    useTableUrlState({
      search: searchParams,
      navigate: route.useNavigate(),
      pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
      globalFilter: { enabled: false },
    })

  const startTimestamp = searchParams.startTime
    ? Math.floor(new Date(searchParams.startTime).getTime() / 1000)
    : undefined
  const endTimestamp = searchParams.endTime
    ? Math.floor(new Date(searchParams.endTime).getTime() / 1000)
    : undefined

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'aipdd-transit-orders',
      pagination.pageIndex + 1,
      pagination.pageSize,
      startTimestamp,
      endTimestamp,
      searchParams.platformOrderId,
      searchParams.userId,
      searchParams.tokenId,
      searchParams.channelId,
      searchParams.model,
      searchParams.status,
    ],
    queryFn: async () => {
      const result = await getAIPDDTransitOrders({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
        platform_order_id: searchParams.platformOrderId,
        user_id: parseOptionalInt(searchParams.userId),
        token_id: parseOptionalInt(searchParams.tokenId),
        channel_id: parseOptionalInt(searchParams.channelId),
        model: searchParams.model,
        status: searchParams.status,
      })
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const items = data?.items || []

  const table = useReactTable({
    data: items,
    columns,
    state: {
      sorting,
      columnVisibility,
      pagination,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onPaginationChange,
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <div className='flex flex-col gap-4'>
      <TransitOrdersFilterBar />
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No AIPDD Transit Orders Found')}
        emptyDescription={t(
          'No transit orders match the current filters.'
        )}
        skeletonKeyPrefix='aipdd-transit-orders-skeleton'
        toolbarProps={null}
      />
    </div>
  )
}
