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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowDown01Icon,
  ArrowUp01Icon,
  Delete02Icon,
  FileUploadIcon,
  FolderLibraryIcon,
  Image01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import {
  type PromptInputPreparedFile,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import type { Material } from '@/features/materials/types'
import {
  LTX_23_FRAME_RATE,
  resolveLTXStartEndTimeline,
} from '../lib/ltx-start-end'
import { getLTXStartEndAttachmentState } from '../lib/ltx-start-end-attachments'
import { MaterialSelectorDialog } from './material-selector-dialog'

interface LTXStartEndPanelProps {
  disabled?: boolean
  duration: number
  maxFileSize: number
  onPreparingChange: (preparing: boolean) => void
  onTimelineDataChange: (value: string) => void
  prepareFiles: (files: File[]) => Promise<PromptInputPreparedFile[]>
  prompt: string
  timelineData: string
}

export function LTXStartEndPanel({
  disabled,
  duration,
  maxFileSize,
  onPreparingChange,
  onTimelineDataChange,
  prepareFiles,
  prompt,
  timelineData,
}: LTXStartEndPanelProps) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [materialOpen, setMaterialOpen] = useState(false)
  const [preparing, setPreparing] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const attachmentState = useMemo(
    () => getLTXStartEndAttachmentState(attachments.files),
    [attachments.files]
  )
  const timelineResolution = useMemo(
    () => resolveLTXStartEndTimeline(prompt, duration, timelineData),
    [duration, prompt, timelineData]
  )
  const controlsDisabled = disabled || preparing

  useEffect(() => {
    const firstFrame = attachmentState.firstFrame
    for (const file of attachments.files) {
      if (file.id !== firstFrame?.id) attachments.remove(file.id)
    }
    if (!firstFrame || firstFrame.role === 'first_frame') return

    attachments.remove(firstFrame.id)
    attachments.upsertRemote('first_frame', {
      url: firstFrame.url,
      mediaType: firstFrame.mediaType,
      filename: firstFrame.filename,
      sourceFile: firstFrame.sourceFile,
      role: 'first_frame',
    })
  }, [attachmentState.firstFrame, attachments])

  useEffect(() => {
    if (timelineResolution.error) setAdvancedOpen(true)
  }, [timelineResolution.error])

  const replaceFirstFrame = useCallback(
    (file: PromptInputPreparedFile) => {
      attachments.clear()
      attachments.upsertRemote('first_frame', {
        ...file,
        role: 'first_frame',
      })
    },
    [attachments]
  )

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    if (!fileMatchesImage(file)) {
      toast.error(t('No files match the accepted types.'))
      return
    }
    if (file.size > maxFileSize) {
      toast.error(t('All files exceed the maximum size.'))
      return
    }

    setPreparing(true)
    onPreparingChange(true)
    try {
      const prepared = await prepareFiles([file])
      if (prepared[0]) replaceFirstFrame(prepared[0])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to upload material')
      )
    } finally {
      setPreparing(false)
      onPreparingChange(false)
    }
  }

  const handleMaterialSelect = (material: Material) => {
    replaceFirstFrame({
      url: material.url,
      mediaType: material.mime_type,
      filename: material.file_name || material.name,
    })
    setMaterialOpen(false)
  }

  const firstFrame = attachmentState.firstFrame

  return (
    <>
      <Card className='mx-4 mt-3 self-stretch' size='sm'>
        <CardHeader>
          <CardTitle>{t('First frame')}</CardTitle>
          <CardDescription>
            {t('Upload one image. It will be used as the first frame.')}
          </CardDescription>
          <CardAction>
            <Badge variant={firstFrame ? 'secondary' : 'outline'}>
              {firstFrame ? t('Added') : t('Required')}
            </Badge>
          </CardAction>
        </CardHeader>

        <CardContent className='flex flex-col gap-3'>
          {firstFrame ? (
            <div className='flex min-w-0 flex-col gap-2'>
              <div className='bg-muted flex aspect-video max-h-56 items-center justify-center overflow-hidden rounded-lg'>
                <img
                  alt={firstFrame.filename || t('First frame')}
                  className='size-full object-contain'
                  src={firstFrame.url}
                />
              </div>
              <p className='text-muted-foreground truncate text-xs'>
                {firstFrame.filename || t('First frame')}
              </p>
            </div>
          ) : (
            <Button
              className='h-36 w-full flex-col gap-2 whitespace-normal'
              disabled={controlsDisabled}
              onClick={() => inputRef.current?.click()}
              type='button'
              variant='outline'
            >
              <HugeiconsIcon
                data-icon='inline-start'
                icon={Image01Icon}
                strokeWidth={1.5}
              />
              <span>{t('Upload image')}</span>
              <span className='text-muted-foreground text-xs font-normal'>
                {t('Upload one image. It will be used as the first frame.')}
              </span>
            </Button>
          )}

          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <CollapsibleTrigger
                render={
                  <Button
                    disabled={controlsDisabled}
                    size='sm'
                    type='button'
                    variant='ghost'
                  />
                }
              >
                <HugeiconsIcon
                  data-icon='inline-start'
                  icon={advancedOpen ? ArrowUp01Icon : ArrowDown01Icon}
                  strokeWidth={2}
                />
                {t('Advanced timeline')}
                <Badge variant='outline'>
                  {timelineData.trim() ? t('Custom timeline') : t('Automatic')}
                </Badge>
              </CollapsibleTrigger>
              <span className='text-muted-foreground text-xs'>
                {duration}s ·{' '}
                {t('{{frames}} frames', {
                  frames: timelineResolution.frameCount,
                })}{' '}
                · {LTX_23_FRAME_RATE} FPS
              </span>
            </div>

            <CollapsibleContent className='pt-3'>
              <Field data-invalid={!!timelineResolution.error}>
                <FieldLabel htmlFor='ltx-timeline-data'>
                  {t('Custom timeline JSON')}
                </FieldLabel>
                <Textarea
                  aria-invalid={!!timelineResolution.error}
                  className='font-mono text-xs'
                  disabled={controlsDisabled}
                  id='ltx-timeline-data'
                  onChange={(event) => onTimelineDataChange(event.target.value)}
                  placeholder={`{"segments":[{"prompt":"...","length":${timelineResolution.frameCount}}]}`}
                  rows={5}
                  value={timelineData}
                />
                <FieldDescription>
                  {timelineData.trim()
                    ? t(
                        'Use a segments array. Segment lengths must add up to {{frames}} frames.',
                        { frames: timelineResolution.frameCount }
                      )
                    : t(
                        'A single timeline segment is generated from the prompt and duration.'
                      )}
                </FieldDescription>
                <FieldError>
                  {timelineResolution.error
                    ? t(timelineResolution.error, {
                        frames: timelineResolution.frameCount,
                      })
                    : null}
                </FieldError>
              </Field>
            </CollapsibleContent>
          </Collapsible>
        </CardContent>

        <CardFooter className='flex-wrap gap-2'>
          <Button
            disabled={controlsDisabled}
            onClick={() => inputRef.current?.click()}
            size='sm'
            type='button'
          >
            <HugeiconsIcon
              data-icon='inline-start'
              icon={FileUploadIcon}
              strokeWidth={2}
            />
            {firstFrame ? t('Replace') : t('Upload')}
          </Button>
          <Button
            disabled={controlsDisabled}
            onClick={() => setMaterialOpen(true)}
            size='sm'
            type='button'
            variant='outline'
          >
            <HugeiconsIcon
              data-icon='inline-start'
              icon={FolderLibraryIcon}
              strokeWidth={2}
            />
            {t('Material library')}
          </Button>
          {firstFrame && (
            <Button
              aria-label={t('Remove {{label}}', { label: t('First frame') })}
              disabled={controlsDisabled}
              onClick={() => attachments.remove(firstFrame.id)}
              size='icon-sm'
              type='button'
              variant='ghost'
            >
              <HugeiconsIcon
                data-icon='inline-start'
                icon={Delete02Icon}
                strokeWidth={2}
              />
            </Button>
          )}
          {!!timelineData.trim() && (
            <Button
              className='ml-auto'
              disabled={controlsDisabled}
              onClick={() => onTimelineDataChange('')}
              size='sm'
              type='button'
              variant='ghost'
            >
              {t('Reset to automatic')}
            </Button>
          )}
        </CardFooter>
      </Card>

      <input
        ref={inputRef}
        accept='image/*'
        className='hidden'
        disabled={controlsDisabled}
        onChange={(event) => {
          void handleFile(event.currentTarget.files?.[0])
          event.currentTarget.value = ''
        }}
        tabIndex={-1}
        type='file'
      />

      <MaterialSelectorDialog
        fixedType='image'
        mode='video'
        onOpenChange={setMaterialOpen}
        onSelect={handleMaterialSelect}
        open={materialOpen}
      />
    </>
  )
}

function fileMatchesImage(file: File): boolean {
  const mediaType = file.type.toLowerCase()
  return (
    mediaType.startsWith('image/') ||
    (!mediaType && /\.(avif|gif|jpe?g|png|webp)$/i.test(file.name))
  )
}
