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
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Copy01Icon,
  Delete02Icon,
  FileUploadIcon,
  Image01Icon,
  Tick02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import {
  deleteVirtualCharacterAsset,
  getVirtualCharacter,
  setPrimaryVirtualCharacterAsset,
  virtualCharacterAssetPreviewURL,
  virtualCharacterQueryKeys,
} from '../api'
import type { VirtualCharacterAsset } from '../types'
import { AssetStatusBadge } from './ui-bits'
import { UploadAssetDialog } from './upload-asset-dialog'
import { errorMessage } from './utils'

export function CharacterDetailDialog({
  characterID,
  maxAssetsPerCharacter,
  onClose,
}: {
  characterID: number | null
  maxAssetsPerCharacter: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const queryClient = useQueryClient()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [deletingAsset, setDeletingAsset] = useState<number | null>(null)
  const [previewAsset, setPreviewAsset] =
    useState<VirtualCharacterAsset | null>(null)
  const [busy, setBusy] = useState(false)
  const query = useQuery({
    queryKey: virtualCharacterQueryKeys.detail(characterID ?? 0),
    queryFn: () => getVirtualCharacter(characterID ?? 0),
    enabled: Boolean(characterID),
    refetchInterval: (current) =>
      current.state.data?.data.assets.some(
        (asset) => asset.status === 'Processing'
      )
        ? 5000
        : false,
  })
  const character = query.data?.data
  const assetCount = character?.assets.length ?? 0
  const assetLimitReached = assetCount >= maxAssetsPerCharacter
  const refresh = async () => {
    await query.refetch()
    await queryClient.invalidateQueries({
      queryKey: virtualCharacterQueryKeys.all,
    })
  }
  const setPrimary = async (assetID: number) => {
    if (!character) return
    setBusy(true)
    try {
      await setPrimaryVirtualCharacterAsset(character.id, assetID)
      toast.success(t('Primary asset updated'))
      await refresh()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to update primary asset')))
    } finally {
      setBusy(false)
    }
  }
  const removeAsset = async () => {
    if (!character || deletingAsset == null) return
    setBusy(true)
    try {
      await deleteVirtualCharacterAsset(character.id, deletingAsset)
      toast.success(t('Asset deletion queued'))
      setDeletingAsset(null)
      await refresh()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to delete asset')))
    } finally {
      setBusy(false)
    }
  }
  // This dialog stays mounted while characterID toggles, so the nested dialogs must
  // be torn down explicitly; otherwise a preview stays open over the closed detail
  // view and reappears for the next character.
  const closeDetail = () => {
    setUploadOpen(false)
    setDeletingAsset(null)
    setPreviewAsset(null)
    onClose()
  }
  return (
    <>
      <Dialog
        open={characterID != null}
        onOpenChange={(open) => !open && closeDetail()}
      >
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>{character?.name ?? t('Actor group')}</DialogTitle>
            <DialogDescription>
              {t(
                'Manage the character and its provider-hosted related assets.'
              )}
            </DialogDescription>
          </DialogHeader>
          {!character ? (
            <div className='flex justify-center py-12'>
              <Spinner className='size-6' />
            </div>
          ) : (
            <div className='flex flex-col gap-4'>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge>{t('Validated')}</Badge>
                <Badge variant='outline'>
                  {t('{{count}} / {{limit}} assets', {
                    count: assetCount,
                    limit: maxAssetsPerCharacter,
                  })}
                </Badge>
                <Button
                  size='sm'
                  disabled={assetLimitReached}
                  onClick={() => !assetLimitReached && setUploadOpen(true)}
                >
                  <HugeiconsIcon
                    icon={FileUploadIcon}
                    data-icon='inline-start'
                  />
                  {t('Upload character-related asset')}
                </Button>
              </div>
              {assetLimitReached ? (
                <Alert>
                  <AlertTitle>{t('Character asset limit reached')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Each character can have up to {{limit}} related assets. Delete an existing asset before uploading another.',
                      { limit: maxAssetsPerCharacter }
                    )}
                  </AlertDescription>
                </Alert>
              ) : null}
              {character.assets.length === 0 ? (
                <Alert>
                  <AlertTitle>{t('No character-related assets')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Upload an image, video, or audio asset. Video generation unlocks after it becomes Active.'
                    )}
                  </AlertDescription>
                </Alert>
              ) : (
                <div className='grid gap-3 sm:grid-cols-2'>
                  {character.assets.map((asset) => {
                    const previewURL = virtualCharacterAssetPreviewURL(
                      character.id,
                      asset.id
                    )
                    const providerAssetID = asset.provider_asset_id?.trim()
                    const assetReference = providerAssetID
                      ? `asset://${providerAssetID.replace(/^asset:\/\//, '')}`
                      : ''
                    return (
                      <Card key={asset.id}>
                        <CardHeader>
                          <div className='flex items-start justify-between gap-2'>
                            <div className='min-w-0'>
                              <CardTitle className='text-base'>
                                {asset.name}
                              </CardTitle>
                              <CardDescription>
                                {t(asset.asset_type)}
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
                                    onClick={() => {
                                      void copyToClipboard(assetReference)
                                    }}
                                  >
                                    <HugeiconsIcon
                                      icon={
                                        copiedText === assetReference
                                          ? Tick02Icon
                                          : Copy01Icon
                                      }
                                    />
                                    <span className='sr-only'>{t('Copy')}</span>
                                  </Button>
                                </div>
                              ) : null}
                            </div>
                            <AssetStatusBadge status={asset.status} />
                          </div>
                        </CardHeader>
                        <CardContent className='flex flex-col gap-3'>
                          {asset.asset_type === 'Image' &&
                          asset.status !== 'Deleting' ? (
                            <button
                              type='button'
                              className='bg-muted aspect-[4/3] cursor-zoom-in overflow-hidden rounded-md'
                              onClick={() => setPreviewAsset(asset)}
                            >
                              <img
                                src={asset.cover_url || previewURL}
                                alt={asset.name}
                                className='size-full object-contain'
                              />
                            </button>
                          ) : null}
                          {asset.status === 'Processing' && (
                            <Progress value={null}>
                              <ProgressLabel>
                                {t('Uploading to Volcengine asset library')}
                              </ProgressLabel>
                            </Progress>
                          )}
                          {asset.last_error && (
                            <Alert variant='destructive'>
                              <AlertDescription>
                                {asset.last_error}
                              </AlertDescription>
                            </Alert>
                          )}
                          <div className='flex flex-wrap gap-2'>
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={asset.status === 'Deleting'}
                              onClick={() => setPreviewAsset(asset)}
                            >
                              <HugeiconsIcon
                                icon={Image01Icon}
                                data-icon='inline-start'
                              />
                              {t('Preview')}
                            </Button>
                            {!asset.is_primary && asset.status === 'Active' && (
                              <Button
                                size='sm'
                                variant='outline'
                                disabled={busy}
                                onClick={() => setPrimary(asset.id)}
                              >
                                {t('Set as primary')}
                              </Button>
                            )}
                            {asset.is_primary && (
                              <Badge variant='secondary'>{t('Primary')}</Badge>
                            )}
                            {!asset.is_primary && (
                              <Button
                                size='icon-sm'
                                variant='ghost'
                                disabled={busy || asset.status === 'Deleting'}
                                onClick={() => setDeletingAsset(asset.id)}
                              >
                                <HugeiconsIcon icon={Delete02Icon} />
                                <span className='sr-only'>{t('Delete')}</span>
                              </Button>
                            )}
                          </div>
                        </CardContent>
                      </Card>
                    )
                  })}
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant='outline' onClick={closeDetail}>
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {character && (
        <UploadAssetDialog
          character={character}
          open={uploadOpen}
          maxAssetsPerCharacter={maxAssetsPerCharacter}
          onClose={() => setUploadOpen(false)}
          onUploaded={refresh}
        />
      )}
      <Dialog
        open={previewAsset != null}
        onOpenChange={(open) => !open && setPreviewAsset(null)}
      >
        <DialogContent className='sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>{previewAsset?.name ?? t('Preview')}</DialogTitle>
            <DialogDescription>
              {previewAsset ? t(previewAsset.asset_type) : null}
            </DialogDescription>
          </DialogHeader>
          {character && previewAsset ? (
            <AssetPreviewMedia
              characterId={character.id}
              asset={previewAsset}
              unavailableLabel={t('Asset preview is unavailable')}
            />
          ) : null}
          <DialogFooter>
            <Button variant='outline' onClick={() => setPreviewAsset(null)}>
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AlertDialog
        open={deletingAsset != null}
        onOpenChange={(open) => !open && setDeletingAsset(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete this asset?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'New tasks are blocked immediately. Provider deletion continues in the background and retries on failure.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction disabled={busy} onClick={removeAsset}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function AssetPreviewMedia(props: {
  characterId: number
  asset: VirtualCharacterAsset
  unavailableLabel: string
}) {
  const previewURL = virtualCharacterAssetPreviewURL(
    props.characterId,
    props.asset.id
  )
  if (props.asset.asset_type === 'Image') {
    return (
      <img
        src={props.asset.cover_url || previewURL}
        alt={props.asset.name}
        className='bg-muted max-h-[70vh] w-full rounded-md object-contain'
      />
    )
  }
  if (props.asset.asset_type === 'Video') {
    return (
      <video
        src={previewURL}
        controls
        className='bg-muted max-h-[70vh] w-full rounded-md'
      >
        {props.unavailableLabel}
      </video>
    )
  }
  if (props.asset.asset_type === 'Audio') {
    return (
      <audio src={previewURL} controls className='w-full'>
        {props.unavailableLabel}
      </audio>
    )
  }
  return (
    <Alert>
      <AlertDescription>{props.unavailableLabel}</AlertDescription>
    </Alert>
  )
}
