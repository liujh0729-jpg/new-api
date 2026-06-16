import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { MATERIAL_TYPE } from '../constants'
import { formatFileSize, getMaterialPreviewUrl } from '../lib'
import { useMaterials } from './materials-provider'

export function MaterialsPreviewDialog() {
  const { t } = useTranslation()
  const { previewMaterial, setPreviewMaterial } = useMaterials()
  const [isLoading, setIsLoading] = useState(true)
  const [hasError, setHasError] = useState(false)

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setPreviewMaterial(null)
      setIsLoading(true)
      setHasError(false)
    }
  }

  const handleMediaLoad = () => {
    setIsLoading(false)
    setHasError(false)
  }

  const handleMediaError = () => {
    setIsLoading(false)
    setHasError(true)
  }

  if (!previewMaterial) return null
  const previewUrl = getMaterialPreviewUrl(previewMaterial.id)

  return (
    <Dialog open={!!previewMaterial} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{previewMaterial.name}</DialogTitle>
          <DialogDescription>
            {previewMaterial.type === MATERIAL_TYPE.IMAGE
              ? t('Image Preview')
              : previewMaterial.type === MATERIAL_TYPE.VIDEO
                ? t('Video Preview')
                : t('Audio Preview')}
            {' — '}
            {previewMaterial.file_name}
            {' · '}
            {formatFileSize(previewMaterial.file_size)}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className='max-h-[70vh]'>
          <div className='py-4'>
            <div className='bg-muted/50 relative flex min-h-[200px] items-center justify-center rounded-lg border'>
              {(isLoading || hasError) && (
                <Skeleton className='absolute inset-0 h-full w-full rounded-lg' />
              )}

              {previewMaterial.type === MATERIAL_TYPE.IMAGE && (
                <img
                  src={previewUrl}
                  alt={previewMaterial.name}
                  className={`max-h-[65vh] w-full rounded-lg object-contain ${
                    isLoading || hasError ? 'opacity-0' : 'opacity-100'
                  }`}
                  onLoad={handleMediaLoad}
                  onError={handleMediaError}
                />
              )}

              {previewMaterial.type === MATERIAL_TYPE.VIDEO && (
                <video
                  src={previewUrl}
                  controls
                  playsInline
                  className={`max-h-[65vh] w-full rounded-lg ${
                    isLoading || hasError ? 'opacity-0' : 'opacity-100'
                  }`}
                  onLoadedMetadata={handleMediaLoad}
                  onError={handleMediaError}
                />
              )}

              {previewMaterial.type === MATERIAL_TYPE.AUDIO && (
                <div className='w-full px-8 py-12'>
                  <audio
                    src={previewUrl}
                    controls
                    className='w-full'
                    onLoadedMetadata={handleMediaLoad}
                    onError={handleMediaError}
                  />
                </div>
              )}

              {hasError && (
                <div className='absolute inset-0 flex items-center justify-center'>
                  <p className='text-muted-foreground text-sm'>
                    {t('Failed to load media')}
                  </p>
                </div>
              )}
            </div>

            <div className='bg-muted mt-4 rounded-md p-3'>
              <p className='text-muted-foreground font-mono text-xs break-all'>
                {previewUrl}
              </p>
            </div>
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
