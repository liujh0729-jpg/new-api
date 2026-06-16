import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  flexRender,
  type Row,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { TableRow, TableCell } from '@/components/ui/table'
import { DataTablePage } from '@/components/data-table'
import { getMaterials, searchMaterials } from '../api'
import { MATERIAL_SOURCE_TYPE, MATERIAL_TIME_FILTER } from '../constants'
import { getMaterialTimeRange } from '../lib'
import type { Material } from '../types'
import { useMaterialsColumns } from './materials-columns'
import { useMaterials } from './materials-provider'

const route = getRouteApi('/_authenticated/materials/')

export function MaterialsTable() {
  const { t } = useTranslation()
  const columns = useMaterialsColumns()
  const { refreshTrigger } = useMaterials()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'source_type', searchKey: 'source', type: 'array' },
      { columnId: 'created_time', searchKey: 'time', type: 'array' },
    ],
  })
  const typeFilters = useMemo(() => {
    const value = columnFilters.find((filter) => filter.id === 'type')?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])
  const sourceTypeFilters = useMemo(() => {
    const value = columnFilters.find(
      (filter) => filter.id === 'source_type'
    )?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])
  const timeFilters = useMemo(() => {
    const value = columnFilters.find(
      (filter) => filter.id === 'created_time'
    )?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])
  const timeRange = useMemo(
    () => getMaterialTimeRange(timeFilters[0]),
    [timeFilters]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'materials',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      typeFilters.join(','),
      sourceTypeFilters.join(','),
      timeFilters.join(','),
      timeRange.created_after,
      timeRange.created_before,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = Boolean(
        globalFilter?.trim() ||
        typeFilters.length ||
        sourceTypeFilters.length ||
        timeRange.created_after ||
        timeRange.created_before
      )
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        type: typeFilters,
        source_type: sourceTypeFilters,
        ...timeRange,
      }

      const result = hasFilter
        ? await searchMaterials({
            ...params,
            keyword: globalFilter,
          })
        : await getMaterials(params)

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const materials = data?.items || []

  const table = useReactTable({
    data: materials,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      globalFilter,
      pagination,
    },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    globalFilterFn: (row, _columnId, filterValue) => {
      const name = String(row.getValue('name')).toLowerCase()
      const fileName = String(row.getValue('file_name')).toLowerCase()
      const id = String(row.getValue('id'))
      const searchValue = String(filterValue).toLowerCase()

      return (
        name.includes(searchValue) ||
        fileName.includes(searchValue) ||
        id.includes(searchValue)
      )
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const typeOptions = useMemo(
    () => [
      { label: t('Image'), value: 'image' },
      { label: t('Video'), value: 'video' },
      { label: t('Audio'), value: 'audio' },
    ],
    [t]
  )
  const sourceTypeOptions = useMemo(
    () => [
      { label: t('Material'), value: MATERIAL_SOURCE_TYPE.MATERIAL },
      { label: t('AI Output'), value: MATERIAL_SOURCE_TYPE.AI_OUTPUT },
    ],
    [t]
  )
  const timeOptions = useMemo(
    () => [
      { label: t('Today'), value: MATERIAL_TIME_FILTER.TODAY },
      { label: t('Last 7 days'), value: MATERIAL_TIME_FILTER.LAST_7_DAYS },
      { label: t('Last 30 days'), value: MATERIAL_TIME_FILTER.LAST_30_DAYS },
      { label: t('Last 90 days'), value: MATERIAL_TIME_FILTER.LAST_90_DAYS },
    ],
    [t]
  )

  const renderRow = (row: Row<Material>) => {
    return (
      <TableRow
        key={row.id}
        data-state={row.getIsSelected() && 'selected'}
        className='hover:bg-muted/50 transition-colors'
      >
        {row.getVisibleCells().map((cell) => (
          <TableCell key={cell.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </TableCell>
        ))}
      </TableRow>
    )
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Materials Found')}
      emptyDescription={t(
        'No materials available. Upload your first material to get started.'
      )}
      skeletonKeyPrefix='materials-skeleton'
      renderRow={renderRow}
      toolbarProps={{
        searchPlaceholder: t('Filter by name, filename or ID...'),
        filters: [
          {
            columnId: 'source_type',
            title: t('Type'),
            options: sourceTypeOptions,
            singleSelect: true,
          },
          {
            columnId: 'type',
            title: t('Media Type'),
            options: typeOptions,
          },
          {
            columnId: 'created_time',
            title: t('Time'),
            options: timeOptions,
            singleSelect: true,
          },
        ],
      }}
    />
  )
}
