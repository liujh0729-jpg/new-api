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
import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Image,
  Video,
  Music,
  ChevronLeftIcon,
  ChevronRightIcon,
  SearchIcon,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { usePromptInputAttachments } from '@/components/ai-elements/prompt-input'
import { searchMaterials } from '@/features/materials/api'
import { formatFileSize, getMaterialPreviewUrl } from '@/features/materials/lib'
import type { Material } from '@/features/materials/types'

const PAGE_SIZE_OPTIONS = [12, 24, 48]

interface MaterialSelectorDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'image' | 'video'
}

export function MaterialSelectorDialog({
  open,
  onOpenChange,
  mode,
}: MaterialSelectorDialogProps) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const [keyword, setKeyword] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>(
    mode === 'image' ? 'image' : ''
  )
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(24)

  useEffect(() => {
    if (!open) {
      setKeyword('')
      setPage(1)
      setTypeFilter(mode === 'image' ? 'image' : '')
    }
  }, [open, mode])

  useEffect(() => {
    setPage(1)
  }, [keyword, typeFilter])

  const { data, isLoading } = useQuery({
    queryKey: ['material-selector', keyword, typeFilter, page, pageSize],
    queryFn: async () => {
      const result = await searchMaterials({
        keyword: keyword || undefined,
        type: typeFilter || undefined,
        p: page,
        page_size: pageSize,
      })
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    enabled: open,
    placeholderData: (previousData) => previousData,
  })

  const materials = data?.items || []
  const total = data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const handleSelect = useCallback(
    (material: Material) => {
      attachments.addRemote({
        url: material.url,
        mediaType: material.mime_type,
        filename: material.file_name,
      })
      onOpenChange(false)
    },
    [attachments, onOpenChange]
  )

  const renderIcon = (type: string) => {
    switch (type) {
      case 'image':
        return <Image className='h-5 w-5' />
      case 'video':
        return <Video className='h-5 w-5' />
      case 'audio':
        return <Music className='h-5 w-5' />
      default:
        return <Image className='h-5 w-5' />
    }
  }

  const renderCard = (material: Material) => {
    const isImage = material.type === 'image'
    const previewUrl = getMaterialPreviewUrl(material.id)

    return (
      <button
        key={material.id}
        className='group bg-card hover:border-primary/50 hover:bg-accent/50 focus-visible:ring-ring flex flex-col overflow-hidden rounded-lg border text-left transition-colors focus-visible:ring-2 focus-visible:outline-none'
        onClick={() => handleSelect(material)}
        type='button'
      >
        <div className='bg-muted relative aspect-video overflow-hidden'>
          {isImage && material.url ? (
            <img
              src={previewUrl}
              alt={material.name}
              className='h-full w-full object-cover transition-transform group-hover:scale-105'
              loading='lazy'
            />
          ) : (
            <div className='flex h-full w-full items-center justify-center'>
              {renderIcon(material.type)}
            </div>
          )}
          <div className='bg-background/80 absolute top-1.5 right-1.5 rounded px-1.5 py-0.5 text-xs font-medium backdrop-blur-sm'>
            {material.type}
          </div>
        </div>
        <div className='flex flex-col gap-0.5 p-2.5'>
          <div className='truncate text-sm font-medium'>{material.name}</div>
          <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
            {renderIcon(material.type)}
            <span>{formatFileSize(material.file_size)}</span>
          </div>
        </div>
      </button>
    )
  }

  const filterTypes =
    mode === 'video'
      ? [
          { value: '', label: t('All') },
          { value: 'image', label: t('Image') },
          { value: 'video', label: t('Video') },
          { value: 'audio', label: t('Audio') },
        ]
      : [{ value: 'image', label: t('Image') }]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-5xl'>
        <DialogHeader>
          <DialogTitle>{t('Select Material')}</DialogTitle>
        </DialogHeader>

        <div className='flex items-center gap-3'>
          <div className='relative flex-1'>
            <SearchIcon className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
            <Input
              placeholder={t('Search materials...')}
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              className='pl-9'
            />
            {keyword && (
              <Button
                variant='ghost'
                size='icon'
                className='absolute top-1/2 right-1 h-7 w-7 -translate-y-1/2'
                onClick={() => setKeyword('')}
              >
                <X className='h-4 w-4' />
              </Button>
            )}
          </div>

          {filterTypes.length > 1 && (
            <div className='flex gap-1'>
              {filterTypes.map((ft) => (
                <Button
                  key={ft.value}
                  variant={typeFilter === ft.value ? 'default' : 'outline'}
                  size='sm'
                  onClick={() => setTypeFilter(ft.value)}
                  className='h-9'
                >
                  {ft.label}
                </Button>
              ))}
            </div>
          )}
        </div>

        <ScrollArea className='h-[420px] rounded-md border'>
          {isLoading ? (
            <div className='grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5'>
              {Array.from({ length: pageSize }).map((_, i) => (
                <div key={i} className='flex flex-col gap-2'>
                  <div className='bg-muted aspect-video animate-pulse rounded-lg' />
                  <div className='bg-muted h-4 w-3/4 animate-pulse rounded' />
                  <div className='bg-muted h-3 w-1/2 animate-pulse rounded' />
                </div>
              ))}
            </div>
          ) : materials.length === 0 ? (
            <div className='flex h-full items-center justify-center py-24'>
              <p className='text-muted-foreground text-sm'>
                {t('No materials found')}
              </p>
            </div>
          ) : (
            <div className='grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5'>
              {materials.map(renderCard)}
            </div>
          )}
        </ScrollArea>

        {totalPages > 1 && (
          <div className='flex items-center justify-between'>
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <span>{t('Total {{count}} materials', { count: total })}</span>
              <Select
                items={PAGE_SIZE_OPTIONS.map((ps) => ({
                  value: `${ps}`,
                  label: ps,
                }))}
                value={`${pageSize}`}
                onValueChange={(v) => {
                  setPageSize(Number(v))
                  setPage(1)
                }}
              >
                <SelectTrigger className='h-8 w-[64px]'>
                  <SelectValue placeholder={pageSize} />
                </SelectTrigger>
                <SelectContent side='top' alignItemWithTrigger={false}>
                  <SelectGroup>
                    {PAGE_SIZE_OPTIONS.map((ps) => (
                      <SelectItem key={ps} value={`${ps}`}>
                        {ps}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            <div className='flex items-center gap-1.5'>
              <Button
                variant='outline'
                size='icon'
                className='h-8 w-8'
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
              >
                <ChevronLeftIcon className='h-4 w-4' />
              </Button>
              <span className='min-w-[60px] text-center text-sm'>
                {page} / {totalPages}
              </span>
              <Button
                variant='outline'
                size='icon'
                className='h-8 w-8'
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
              >
                <ChevronRightIcon className='h-4 w-4' />
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
