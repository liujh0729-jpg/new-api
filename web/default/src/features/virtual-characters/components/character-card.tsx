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
import {
  AiUserIcon,
  Copy01Icon,
  Delete02Icon,
  FileUploadIcon,
  MagicWand01Icon,
  RefreshIcon,
  Tick02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { virtualCharacterPreviewURL } from '../api'
import type { VirtualCharacter } from '../types'
import type { CharacterImagePreview } from './character-image-preview-dialog'
import { MaterialMedia } from './material-media'
import {
  effectiveVirtualCharacterAssetType,
  statusLabel,
  virtualCharacterFacetMeta,
} from './utils'

export function CharacterCard({
  item,
  onOpen,
  onPreview,
  onGenerate,
  onDelete,
  onSync,
  onUpload,
}: {
  item: VirtualCharacter
  onOpen: () => void
  onPreview: (preview: CharacterImagePreview) => void
  onGenerate: () => void
  onDelete: () => void
  onSync?: () => void
  onUpload?: () => void
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const coverURL =
    item.cover_url ||
    (item.scope === 'private' && item.provider_asset_id
      ? virtualCharacterPreviewURL(item.id)
      : '')
  const providerAssetID = item.provider_asset_id?.trim()
  const assetReference = providerAssetID
    ? `asset://${providerAssetID.replace(/^asset:\/\//, '')}`
    : ''
  const awaitingUpload = Boolean(item.asset_upload_required)
  const isUploading = item.status === 'creating' && !awaitingUpload
  const canGenerate =
    item.status === 'active' &&
    !isUploading &&
    Boolean(providerAssetID) &&
    (item.source_type !== 'volc_real_person' ||
      item.authorization?.status === 'active')
  const facetMeta = virtualCharacterFacetMeta(item)
  const assetType = effectiveVirtualCharacterAssetType(item)
  return (
    <Card className='gap-0 overflow-hidden py-0'>
      <div className='bg-muted relative aspect-[3/4] overflow-hidden'>
        {coverURL ? (
          <button
            type='button'
            className='group focus-visible:ring-ring size-full cursor-zoom-in focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset'
            aria-label={`${t('Preview')}: ${item.name}`}
            onClick={() =>
              onPreview({ name: item.name, url: coverURL, assetType })
            }
          >
            <MaterialMedia
              url={coverURL}
              name={item.name}
              assetType={assetType}
              className='size-full object-contain transition-transform duration-200 group-hover:scale-[1.02]'
            />
          </button>
        ) : (
          <div className='text-muted-foreground flex size-full items-center justify-center'>
            <HugeiconsIcon icon={AiUserIcon} className='size-8' />
          </div>
        )}
        {isUploading ? (
          <div className='absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/45 px-2 text-center text-white'>
            <Spinner className='size-5 text-white' />
            <p className='text-[11px] leading-tight font-medium'>
              {t('Processing material')}
            </p>
          </div>
        ) : null}
      </div>
      <CardHeader className='min-w-0 gap-1 px-2.5 pt-2.5 pb-0'>
        <div className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-1.5'>
          <div className='min-w-0'>
            <CardTitle className='truncate text-xs'>{item.name}</CardTitle>
            <CardDescription className='line-clamp-1 text-[11px]'>
              {item.description || t('No description')}
            </CardDescription>
            {assetReference ? (
              <div className='mt-0.5 flex min-w-0 items-center gap-0.5'>
                <p
                  className='text-muted-foreground min-w-0 truncate font-mono text-[10px]'
                  title={assetReference}
                >
                  {assetReference}
                </p>
                <Button
                  type='button'
                  size='icon-sm'
                  variant='ghost'
                  className='size-4 shrink-0'
                  onClick={(event) => {
                    event.stopPropagation()
                    void copyToClipboard(assetReference)
                  }}
                >
                  <HugeiconsIcon
                    icon={
                      copiedText === assetReference ? Tick02Icon : Copy01Icon
                    }
                  />
                  <span className='sr-only'>{t('Copy')}</span>
                </Button>
              </div>
            ) : null}
          </div>
          {awaitingUpload ? (
            <Badge
              variant='secondary'
              className='mr-1 shrink-0 px-1.5 text-[10px]'
            >
              {t('Waiting for portrait upload')}
            </Badge>
          ) : isUploading ? (
            <Badge
              variant='secondary'
              className='mr-1 shrink-0 px-1.5 text-[10px]'
            >
              {t('Processing')}
            </Badge>
          ) : (
            <div className='mr-1 flex shrink-0 flex-col items-end gap-1'>
              <Badge
                variant={item.status === 'active' ? 'default' : 'secondary'}
                className='px-1.5 text-[10px]'
              >
                {statusLabel(item.status, t)}
              </Badge>
              <Badge variant='outline' className='px-1.5 text-[10px]'>
                {t(assetType)}
              </Badge>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-1.5 px-2.5 pt-1.5 pb-2.5'>
        {awaitingUpload ? (
          <p className='text-muted-foreground text-[11px]'>
            {t("Upload the verified person's image to continue.")}
          </p>
        ) : isUploading ? (
          <Progress value={null}>
            <ProgressLabel className='text-muted-foreground text-[10px]'>
              {t('Processing material')}
            </ProgressLabel>
          </Progress>
        ) : (
          <div className='flex flex-col gap-1'>
            {facetMeta ? (
              <p className='text-muted-foreground text-[11px]'>{facetMeta}</p>
            ) : null}
            <div className='flex min-h-4 flex-wrap gap-1'>
              {item.tags.length > 0 ? (
                item.tags.map((tag) => (
                  <Badge
                    key={tag}
                    variant='outline'
                    className='px-1.5 py-0 text-[10px]'
                  >
                    {tag}
                  </Badge>
                ))
              ) : (
                <span className='text-muted-foreground text-[11px]'>
                  {t('No tags')}
                </span>
              )}
            </div>
          </div>
        )}
        <div className='flex flex-wrap gap-1'>
          {awaitingUpload && (
            <Button
              size='sm'
              className='h-6 px-1.5 text-[11px]'
              onClick={onUpload}
            >
              <HugeiconsIcon icon={FileUploadIcon} data-icon='inline-start' />
              {t('Upload portrait asset')}
            </Button>
          )}
          <Button
            size='sm'
            variant='outline'
            className='h-6 px-1.5 text-[11px]'
            onClick={onOpen}
          >
            {t('Details')}
          </Button>
          {item.source_type === 'volc_real_person' &&
            !awaitingUpload &&
            item.status !== 'deleting' &&
            item.authorization?.status !== 'expired' &&
            item.authorization?.status !== 'revoked' && (
              <Button
                size='icon-sm'
                variant='outline'
                className='size-6'
                onClick={onSync}
              >
                <HugeiconsIcon icon={RefreshIcon} />
                <span className='sr-only'>{t('Sync provider status')}</span>
              </Button>
            )}
          <Button
            size='sm'
            className='h-6 px-1.5 text-[11px]'
            disabled={!canGenerate}
            onClick={onGenerate}
          >
            <HugeiconsIcon icon={MagicWand01Icon} data-icon='inline-start' />
            {t('Create video')}
          </Button>
          {item.scope === 'private' && (
            <Button
              size='icon-sm'
              variant='ghost'
              className='size-6'
              onClick={onDelete}
            >
              <HugeiconsIcon icon={Delete02Icon} />
              <span className='sr-only'>{t('Delete')}</span>
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
