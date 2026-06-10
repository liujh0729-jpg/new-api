import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { Checkbox } from '@/components/ui/checkbox'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { type Material } from '../types'
import { formatFileSize } from '../lib'
import { DataTableRowActions } from './data-table-row-actions'
import { useMaterials } from './materials-provider'
import { Image, Video, Music } from 'lucide-react'

export function useMaterialsColumns(): ColumnDef<Material>[] {
  const { t } = useTranslation()
  const { setPreviewMaterial } = useMaterials()
  return [
    {
      id: 'select',
      meta: { label: t('Select') },
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('Select all')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('Select row')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      id: 'preview',
      meta: { label: t('Preview') },
      header: () => null,
      cell: ({ row }) => {
        const material = row.original
        const isImage = material.type === 'image'
        const isVideo = material.type === 'video'

        return (
          <div
            className='flex size-10 cursor-pointer items-center justify-center overflow-hidden rounded border border-border bg-muted transition-colors hover:ring-2 hover:ring-primary/50'
            onClick={(e) => {
              e.stopPropagation()
              setPreviewMaterial(material)
            }}
          >
            {isImage ? (
              <img
                src={material.url}
                alt={material.name}
                className='size-full object-cover'
                loading='lazy'
              />
            ) : isVideo ? (
              <video
                src={material.url}
                preload='metadata'
                muted
                playsInline
                className='size-full object-cover'
              />
            ) : null}
          </div>
        )
      },
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('ID')} />
      ),
      cell: ({ row }) => {
        return <div className='w-[60px]'>{row.getValue('id')}</div>
      },
    },
    {
      accessorKey: 'type',
      meta: { label: t('Type'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Type')} />
      ),
      cell: ({ row }) => {
        const type = row.getValue('type') as string
        const icon =
          type === 'image' ? (
            <Image className='h-4 w-4' />
          ) : type === 'video' ? (
            <Video className='h-4 w-4' />
          ) : (
            <Music className='h-4 w-4' />
          )
        const label =
          type === 'image'
            ? t('Image')
            : type === 'video'
              ? t('Video')
              : t('Audio')

        return (
          <span className='inline-flex items-center gap-1.5'>
            {icon}
            <StatusBadge label={label} variant='neutral' copyable={false} />
          </span>
        )
      },
      filterFn: (row, id, value) => {
        return value.length === 0 || value.includes(row.getValue(id) as string)
      },
    },
    {
      accessorKey: 'name',
      meta: { label: t('Name'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => {
        return (
          <div className='max-w-[200px] truncate font-medium'>
            {row.getValue('name')}
          </div>
        )
      },
    },
    {
      accessorKey: 'file_name',
      meta: { label: t('File Name'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('File Name')} />
      ),
      cell: ({ row }) => {
        return (
          <div className='max-w-[180px] truncate text-muted-foreground text-sm'>
            {row.getValue('file_name')}
          </div>
        )
      },
    },
    {
      accessorKey: 'file_size',
      meta: { label: t('Size') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Size')} />
      ),
      cell: ({ row }) => {
        const size = row.getValue('file_size') as number
        return (
          <StatusBadge
            label={formatFileSize(size)}
            variant='neutral'
            copyable={false}
          />
        )
      },
    },
    {
      accessorKey: 'created_time',
      meta: { label: t('Created'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => {
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {formatTimestampToDate(row.getValue('created_time'))}
          </div>
        )
      },
    },
    {
      id: 'actions',
      cell: ({ row }) => <DataTableRowActions row={row} />,
    },
  ]
}
