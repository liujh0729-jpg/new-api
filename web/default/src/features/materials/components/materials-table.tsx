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
import { DataTablePage } from '@/components/data-table'
import { TableRow, TableCell } from '@/components/ui/table'
import { getMaterials, searchMaterials } from '../api'
import { useMaterialsColumns } from './materials-columns'
import { useMaterials } from './materials-provider'
import type { Material } from '../types'

const route = getRouteApi('/_authenticated/materials/')

export function MaterialsTable() {
  const { t } = useTranslation()
  const columns = useMaterialsColumns()
  const { refreshTrigger, setPreviewMaterial } = useMaterials()
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
    columnFilters: [{ columnId: 'type', searchKey: 'type', type: 'array' }],
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'materials',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result = hasFilter
        ? await searchMaterials({ ...params, keyword: globalFilter })
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
    manualPagination: !globalFilter,
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

  const renderRow = (row: Row<Material>) => {
    return (
      <TableRow
        key={row.id}
        data-state={row.getIsSelected() && 'selected'}
        className='cursor-pointer transition-colors hover:bg-muted/50'
        onClick={() => setPreviewMaterial(row.original)}
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
            columnId: 'type',
            title: t('Type'),
            options: typeOptions,
          },
        ],
      }}
    />
  )
}
