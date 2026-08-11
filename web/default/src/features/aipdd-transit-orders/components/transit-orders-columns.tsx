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
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { DataTableColumnHeader } from '@/components/data-table'
import { getAIPDDTransitOrderStatusLabelKey } from '../constants'
import { formatAWCoin, formatRMB } from '../lib/format'
import type { AIPDDTransitOrder } from '../types'

function statusVariant(
  status: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'SETTLED':
      return 'default'
    case 'PENDING':
      return 'secondary'
    case 'FAILED':
      return 'destructive'
    case 'REFUNDED':
      return 'outline'
    default:
      return 'secondary'
  }
}

export function useTransitOrdersColumns(): ColumnDef<AIPDDTransitOrder>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'platform_order_id',
      meta: { label: t('Platform Order ID'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Platform Order ID')}
        />
      ),
      cell: ({ row }) => (
        <div className='max-w-[220px] truncate font-mono text-xs'>
          {row.original.platform_order_id}
        </div>
      ),
    },
    {
      id: 'user',
      meta: { label: t('User') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: ({ row }) => {
        const name = row.original.username?.trim()
        return (
          <div className='text-sm'>
            <div>{name || t('Unknown')}</div>
            <div className='text-muted-foreground text-xs'>
              ID {row.original.user_id}
            </div>
          </div>
        )
      },
    },
    {
      id: 'token',
      meta: { label: t('Token'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Token')} />
      ),
      cell: ({ row }) => {
        const name = row.original.token_name?.trim()
        return (
          <div className='text-sm'>
            <div>{name || t('Unknown')}</div>
            <div className='text-muted-foreground text-xs'>
              ID {row.original.token_id}
            </div>
          </div>
        )
      },
    },
    {
      id: 'channel',
      meta: { label: t('AIPDD Channel'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('AIPDD Channel')}
        />
      ),
      cell: ({ row }) => {
        const name = row.original.channel_name?.trim()
        return (
          <div className='text-sm'>
            <div>{name || t('Unknown')}</div>
            <div className='text-muted-foreground text-xs'>
              ID {row.original.channel_id}
            </div>
          </div>
        )
      },
    },
    {
      accessorKey: 'channel_key_index',
      meta: { label: t('Key Slot'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Key Slot')} />
      ),
      cell: ({ row }) => (
        <div className='text-sm'>{row.original.channel_key_index}</div>
      ),
    },
    {
      accessorKey: 'model',
      meta: { label: t('Model') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[160px] truncate text-sm'>
          {row.original.model}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('Status') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => (
        <Badge variant={statusVariant(row.original.status)}>
          {t(getAIPDDTransitOrderStatusLabelKey(row.original.status))}
        </Badge>
      ),
    },
    {
      id: 'customer_charge',
      meta: { label: t('NewAPI Customer Charge') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('NewAPI Customer Charge')}
        />
      ),
      cell: ({ row }) => (
        <div className='text-sm'>
          <div>{formatQuota(row.original.customer_charge_quota)}</div>
          <div className='text-muted-foreground text-xs'>
            {formatRMB(row.original.customer_charge_rmb)}
          </div>
        </div>
      ),
    },
    {
      id: 'source_charge',
      meta: { label: t('AIPDD Source Cost') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('AIPDD Source Cost')}
        />
      ),
      cell: ({ row }) => {
        if (
          row.original.source_charge_awcoin == null ||
          row.original.source_charge_rmb == null
        ) {
          return (
            <div className='text-muted-foreground text-sm'>
              {t('Pending Settlement')}
            </div>
          )
        }
        return (
          <div className='text-sm'>
            <div>{formatAWCoin(row.original.source_charge_awcoin)}</div>
            <div className='text-muted-foreground text-xs'>
              {formatRMB(row.original.source_charge_rmb)}
            </div>
          </div>
        )
      },
    },
    {
      accessorKey: 'created_at',
      meta: { label: t('Created At'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Created At')}
        />
      ),
      cell: ({ row }) => (
        <div className='text-sm whitespace-nowrap'>
          {formatTimestampToDate(row.original.created_at)}
        </div>
      ),
    },
    {
      accessorKey: 'settled_at',
      meta: { label: t('Settled At'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Settled At')}
        />
      ),
      cell: ({ row }) => (
        <div className='text-sm whitespace-nowrap'>
          {row.original.settled_at
            ? formatTimestampToDate(row.original.settled_at)
            : '-'}
        </div>
      ),
    },
  ]
}
