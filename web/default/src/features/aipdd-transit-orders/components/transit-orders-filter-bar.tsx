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
import { useQueryClient, useIsFetching } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { RefreshCw, RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  AIPDD_TRANSIT_ORDER_STATUS_VALUES,
  getAIPDDTransitOrderStatusOptions,
} from '../constants'
import type {
  AIPDDTransitOrderFilters,
  AIPDDTransitOrderStatus,
} from '../types'

const route = getRouteApi('/_authenticated/aipdd-transit-orders/')

const ALL_STATUS = '__all__'

function toStatus(
  value?: string
): AIPDDTransitOrderStatus | undefined {
  if (!value) return undefined
  return (
    AIPDD_TRANSIT_ORDER_STATUS_VALUES as readonly string[]
  ).includes(value)
    ? (value as AIPDDTransitOrderStatus)
    : undefined
}

export function TransitOrdersFilterBar() {
  const { t } = useTranslation()
  const navigate = route.useNavigate()
  const searchParams = route.useSearch()
  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: ['aipdd-transit-orders'] })
  const statusOptions = getAIPDDTransitOrderStatusOptions(t)

  const [filters, setFilters] = useState<AIPDDTransitOrderFilters>({})

  useEffect(() => {
    setFilters({
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : undefined,
      endTime: searchParams.endTime
        ? new Date(searchParams.endTime)
        : undefined,
      platformOrderId: searchParams.platformOrderId || '',
      userId: searchParams.userId || '',
      tokenId: searchParams.tokenId || '',
      channelId: searchParams.channelId || '',
      model: searchParams.model || '',
      status: searchParams.status || '',
    })
  }, [searchParams])

  const applyFilters = () => {
    void navigate({
      search: (prev) => ({
        ...prev,
        page: 1,
        startTime: filters.startTime?.toISOString(),
        endTime: filters.endTime?.toISOString(),
        platformOrderId: filters.platformOrderId?.trim() || undefined,
        userId: filters.userId?.trim() || undefined,
        tokenId: filters.tokenId?.trim() || undefined,
        channelId: filters.channelId?.trim() || undefined,
        model: filters.model?.trim() || undefined,
        status: toStatus(filters.status),
      }),
    })
  }

  const clearFilters = () => {
    setFilters({})
    void navigate({
      search: (prev) => ({
        page: prev.page,
        pageSize: prev.pageSize,
      }),
    })
  }

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['aipdd-transit-orders'] })
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <CompactDateTimeRangePicker
          start={filters.startTime}
          end={filters.endTime}
          onChange={(range) =>
            setFilters((prev) => ({
              ...prev,
              startTime: range.start,
              endTime: range.end,
            }))
          }
        />
        <Input
          className='h-8 w-[220px]'
          placeholder={t('Platform Order ID')}
          value={filters.platformOrderId || ''}
          onChange={(event) =>
            setFilters((prev) => ({
              ...prev,
              platformOrderId: event.target.value,
            }))
          }
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          className='h-8 w-[120px]'
          placeholder={t('User ID')}
          value={filters.userId || ''}
          onChange={(event) =>
            setFilters((prev) => ({ ...prev, userId: event.target.value }))
          }
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          className='h-8 w-[120px]'
          placeholder={t('Token ID')}
          value={filters.tokenId || ''}
          onChange={(event) =>
            setFilters((prev) => ({ ...prev, tokenId: event.target.value }))
          }
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          className='h-8 w-[120px]'
          placeholder={t('Channel ID')}
          value={filters.channelId || ''}
          onChange={(event) =>
            setFilters((prev) => ({
              ...prev,
              channelId: event.target.value,
            }))
          }
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          className='h-8 w-[160px]'
          placeholder={t('Model')}
          value={filters.model || ''}
          onChange={(event) =>
            setFilters((prev) => ({ ...prev, model: event.target.value }))
          }
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Select
          value={filters.status || ALL_STATUS}
          onValueChange={(value) =>
            setFilters((prev) => ({
              ...prev,
              status: value === ALL_STATUS ? '' : (value ?? ''),
            }))
          }
        >
          <SelectTrigger className='h-8 w-[160px]'>
            <SelectValue placeholder={t('Status')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_STATUS}>{t('All Statuses')}</SelectItem>
            {statusOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className='flex flex-wrap items-center gap-2'>
        <Button size='sm' onClick={applyFilters}>
          <Search className='size-4' />
          {t('Search')}
        </Button>
        <Button size='sm' variant='outline' onClick={clearFilters}>
          <RotateCcw className='size-4' />
          {t('Clear Filters')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={refresh}
          disabled={fetching > 0}
        >
          <RefreshCw
            className={`size-4 ${fetching > 0 ? 'animate-spin' : ''}`}
          />
          {t('Refresh')}
        </Button>
      </div>
    </div>
  )
}
