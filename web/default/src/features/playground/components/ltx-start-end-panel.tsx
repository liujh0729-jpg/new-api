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
  type PromptInputAttachmentRole,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import {
  LTX_23_FRAME_RATE,
  resolveLTXStartEndTimeline,
} from '../lib/ltx-start-end'
import { getLTXStartEndAttachmentState } from '../lib/ltx-start-end-attachments'

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
  const [preparing, setPreparing] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const uploadRoleRef = useRef<LTXReferenceRole>('first_frame')
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
    for (const file of attachmentState.extras) attachments.remove(file.id)
    const selected: Array<
      [LTXReferenceRole, (typeof attachments.files)[number] | undefined]
    > = [
      ['first_frame', attachmentState.firstFrame],
      ['last_frame', attachmentState.lastFrame],
      ['audio', attachmentState.audio],
    ]
    for (const [role, file] of selected) {
      if (!file || file.role === role) continue
      attachments.remove(file.id)
      attachments.upsertRemote(role, {
        url: file.url,
        mediaType: file.mediaType,
        filename: file.filename,
        sourceFile: file.sourceFile,
        role,
      })
    }
  }, [attachmentState, attachments])

  useEffect(() => {
    if (timelineResolution.error) setAdvancedOpen(true)
  }, [timelineResolution.error])

  const replaceReference = useCallback(
    (role: LTXReferenceRole, file: PromptInputPreparedFile) => {
      attachments.upsertRemote(role, { ...file, role })
    },
    [attachments]
  )

  const openUpload = (role: LTXReferenceRole) => {
    uploadRoleRef.current = role
    inputRef.current?.click()
  }

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    const role = uploadRoleRef.current
    if (
      (role === 'audio' && !fileMatchesAudio(file)) ||
      (role !== 'audio' && !fileMatchesImage(file))
    ) {
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
      if (prepared[0]) replaceReference(role, prepared[0])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to upload material')
      )
    } finally {
      setPreparing(false)
      onPreparingChange(false)
    }
  }

  const firstFrame = attachmentState.firstFrame
  const lastFrame = attachmentState.lastFrame
  const audio = attachmentState.audio

  const renderReference = (
    role: LTXReferenceRole,
    label: string,
    required: boolean,
    file: (typeof attachments.files)[number] | undefined
  ) => (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{label}</CardTitle>
        <CardAction>
          <Badge variant={file ? 'secondary' : 'outline'}>
            {file ? t('Added') : required ? t('Required') : t('Optional')}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {file ? (
          <div className='flex min-w-0 flex-col gap-2'>
            {role === 'audio' ? (
              <div className='bg-muted flex h-24 items-center justify-center rounded-lg px-3 text-center text-sm'>
                {file.filename || t('Audio')}
              </div>
            ) : (
              <div className='bg-muted flex aspect-video items-center justify-center overflow-hidden rounded-lg'>
                <img
                  alt={file.filename || label}
                  className='size-full object-contain'
                  src={file.url}
                />
              </div>
            )}
            <p className='text-muted-foreground truncate text-xs'>
              {file.filename || label}
            </p>
          </div>
        ) : (
          <Button
            className='h-24 w-full flex-col gap-2 whitespace-normal'
            disabled={controlsDisabled}
            onClick={() => openUpload(role)}
            type='button'
            variant='outline'
          >
            <HugeiconsIcon icon={Image01Icon} strokeWidth={1.5} />
            <span>
              {role === 'audio' ? t('Upload audio') : t('Upload image')}
            </span>
          </Button>
        )}
        <div className='flex flex-wrap gap-2'>
          <Button
            disabled={controlsDisabled}
            onClick={() => openUpload(role)}
            size='sm'
            type='button'
          >
            <HugeiconsIcon icon={FileUploadIcon} strokeWidth={2} />
            {file ? t('Replace') : t('Upload')}
          </Button>
          {file && (
            <Button
              aria-label={t('Remove {{label}}', { label })}
              disabled={controlsDisabled}
              onClick={() => attachments.remove(file.id)}
              size='icon-sm'
              type='button'
              variant='ghost'
            >
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )

  return (
    <>
      <div className='grid gap-3 px-4 pt-3 sm:grid-cols-2 lg:grid-cols-3'>
        {renderReference('first_frame', t('First frame'), true, firstFrame)}
        {renderReference('last_frame', t('Last frame'), false, lastFrame)}
        {renderReference('audio', t('Audio'), false, audio)}
      </div>

      <Card className='mx-4 mt-3 self-stretch' size='sm'>
        <CardHeader>
          <CardTitle>{t('Advanced timeline')}</CardTitle>
          <CardDescription>
            {t('Configure the 24 FPS timeline used by this LTX workflow.')}
          </CardDescription>
          <CardAction>
            <Badge variant='outline'>
              {duration}s · {timelineResolution.frameCount} {t('frames')}
            </Badge>
          </CardAction>
        </CardHeader>

        <CardContent className='flex flex-col gap-3'>
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

        <CardFooter>
          {!!timelineData.trim() && (
            <Button
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
        accept={uploadRoleRef.current === 'audio' ? 'audio/*' : 'image/*'}
        className='hidden'
        disabled={controlsDisabled}
        onChange={(event) => {
          void handleFile(event.currentTarget.files?.[0])
          event.currentTarget.value = ''
        }}
        tabIndex={-1}
        type='file'
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

function fileMatchesAudio(file: File): boolean {
  const mediaType = file.type.toLowerCase()
  return (
    mediaType.startsWith('audio/') ||
    (!mediaType && /\.(aac|flac|m4a|mp3|ogg|wav)$/i.test(file.name))
  )
}

type LTXReferenceRole = Extract<
  PromptInputAttachmentRole,
  'first_frame' | 'last_frame' | 'audio'
>
