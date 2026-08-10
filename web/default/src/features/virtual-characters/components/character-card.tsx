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
  MagicWand01Icon,
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
import { virtualCharacterAssetPreviewURL } from '../api'
import type { VirtualCharacter, VirtualCharacterAsset } from '../types'
import type { CharacterImagePreview } from './character-image-preview-dialog'
import { statusLabel, virtualCharacterFacetMeta } from './utils'

export function CharacterCard({
  item,
  onOpen,
  onPreview,
  onGenerate,
  onDelete,
}: {
  item: VirtualCharacter
  onOpen: () => void
  onPreview: (preview: CharacterImagePreview) => void
  onGenerate: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const primaryAsset =
    item.assets.find((asset) => asset.id === item.primary_asset_id) ??
    item.assets.find((asset) => asset.is_primary) ??
    item.assets[0]
  // The preview endpoint returns 404 for assets being deleted, which would render
  // as a broken image instead of the placeholder.
  const isPreviewable = (asset: VirtualCharacterAsset) =>
    asset.asset_type === 'Image' && asset.status !== 'Deleting'
  const imageAsset =
    primaryAsset && isPreviewable(primaryAsset)
      ? primaryAsset
      : item.assets.find(isPreviewable)
  const coverURL =
    item.cover_url ||
    imageAsset?.cover_url ||
    (imageAsset ? virtualCharacterAssetPreviewURL(item.id, imageAsset.id) : '')
  const providerAssetID = primaryAsset?.provider_asset_id?.trim()
  const assetReference = providerAssetID
    ? `asset://${providerAssetID.replace(/^asset:\/\//, '')}`
    : ''
  const isUploading = item.assets.some((asset) => asset.status === 'Processing')
  const canGenerate =
    item.status === 'active' &&
    !isUploading &&
    (item.scope === 'public' ||
      item.assets.some((asset) => asset.status === 'Active'))
  const facetMeta = virtualCharacterFacetMeta(item)
  return (
    <Card className='overflow-hidden'>
      <div className='bg-muted relative aspect-[4/3] overflow-hidden'>
        {coverURL ? (
          <button
            type='button'
            className='group focus-visible:ring-ring size-full cursor-zoom-in focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset'
            aria-label={`${t('Preview')}: ${item.name}`}
            onClick={() => onPreview({ name: item.name, url: coverURL })}
          >
            <img
              src={coverURL}
              alt={item.name}
              loading='lazy'
              decoding='async'
              className='size-full object-contain transition-transform duration-200 group-hover:scale-[1.02]'
            />
          </button>
        ) : (
          <div className='text-muted-foreground flex size-full items-center justify-center'>
            <HugeiconsIcon icon={AiUserIcon} className='size-10' />
          </div>
        )}
        {isUploading ? (
          <div className='absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/45 px-3 text-center text-white'>
            <Spinner className='size-6 text-white' />
            <p className='text-xs font-medium'>
              {t('Uploading to Volcengine asset library')}
            </p>
          </div>
        ) : null}
      </div>
      <CardHeader>
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='truncate'>{item.name}</CardTitle>
            <CardDescription className='line-clamp-2'>
              {item.description || t('No description')}
            </CardDescription>
            {assetReference ? (
              <div className='mt-1 flex min-w-0 items-center gap-1'>
                <p
                  className='text-muted-foreground truncate font-mono text-xs'
                  title={assetReference}
                >
                  {assetReference}
                </p>
                <Button
                  type='button'
                  size='icon-sm'
                  variant='ghost'
                  className='size-6 shrink-0'
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
          {isUploading ? (
            <Badge variant='secondary'>{t('Processing')}</Badge>
          ) : (
            <Badge variant={item.status === 'active' ? 'default' : 'secondary'}>
              {statusLabel(item.status, t)}
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {isUploading ? (
          <Progress value={null}>
            <ProgressLabel className='text-muted-foreground text-xs'>
              {t('Uploading to Volcengine asset library')}
            </ProgressLabel>
          </Progress>
        ) : (
          <div className='flex flex-col gap-2'>
            {facetMeta ? (
              <p className='text-muted-foreground text-sm'>{facetMeta}</p>
            ) : null}
            <div className='flex min-h-6 flex-wrap gap-1.5'>
              {item.tags.length > 0 ? (
                item.tags.map((tag) => (
                  <Badge key={tag} variant='outline'>
                    {tag}
                  </Badge>
                ))
              ) : (
                <span className='text-muted-foreground text-sm'>
                  {t('No tags')}
                </span>
              )}
            </div>
          </div>
        )}
        <div className='flex flex-wrap gap-2'>
          <Button size='sm' variant='outline' onClick={onOpen}>
            {item.scope === 'private' ? t('Manage assets') : t('View')}
          </Button>
          <Button size='sm' disabled={!canGenerate} onClick={onGenerate}>
            <HugeiconsIcon icon={MagicWand01Icon} data-icon='inline-start' />
            {t('Create video')}
          </Button>
          {item.scope === 'private' && (
            <Button size='icon-sm' variant='ghost' onClick={onDelete}>
              <HugeiconsIcon icon={Delete02Icon} />
              <span className='sr-only'>{t('Delete')}</span>
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
