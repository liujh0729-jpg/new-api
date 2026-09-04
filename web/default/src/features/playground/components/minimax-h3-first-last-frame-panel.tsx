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
import { useRef, useState } from 'react'
import { ImageIcon, UploadIcon, XIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  type PromptInputAttachmentRole,
  type PromptInputPreparedFile,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'

interface MinimaxH3FirstLastFramePanelProps {
  disabled?: boolean
  maxFileSize: number
  onPreparingChange: (preparing: boolean) => void
  prepareFiles: (files: File[]) => Promise<PromptInputPreparedFile[]>
}

type FrameRole = Extract<
  PromptInputAttachmentRole,
  'first_frame' | 'last_frame'
>

function isImageFile(file: File): boolean {
  return (
    file.type.startsWith('image/') ||
    /\.(avif|bmp|gif|heic|heif|jpe?g|png|webp)$/i.test(file.name)
  )
}

export function MinimaxH3FirstLastFramePanel(
  props: MinimaxH3FirstLastFramePanelProps
) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const inputRef = useRef<HTMLInputElement>(null)
  const uploadRoleRef = useRef<FrameRole>('first_frame')
  const [preparing, setPreparing] = useState(false)
  const controlsDisabled = props.disabled || preparing

  const replaceFrame = (role: FrameRole, file: PromptInputPreparedFile) => {
    attachments.upsertRemote(role, { ...file, role })
  }

  const openUpload = (role: FrameRole) => {
    uploadRoleRef.current = role
    inputRef.current?.click()
  }

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    if (!isImageFile(file)) {
      toast.error(t('No files match the accepted types.'))
      return
    }
    if (file.size > props.maxFileSize) {
      toast.error(t('All files exceed the maximum size.'))
      return
    }

    setPreparing(true)
    props.onPreparingChange(true)
    try {
      const prepared = await props.prepareFiles([file])
      if (prepared[0]) replaceFrame(uploadRoleRef.current, prepared[0])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to upload material')
      )
    } finally {
      setPreparing(false)
      props.onPreparingChange(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  const renderFrame = (role: FrameRole, label: string) => {
    const frame = attachments.files.find((file) => file.role === role)
    return (
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{label}</CardTitle>
          <CardAction>
            <Badge variant={frame ? 'secondary' : 'outline'}>
              {frame ? t('Added') : t('Required')}
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {frame ? (
            <div className='bg-muted relative flex aspect-video items-center justify-center overflow-hidden rounded-lg'>
              <img
                alt={frame.filename || label}
                className='size-full object-contain'
                src={frame.url}
              />
              <Button
                aria-label={t('Remove attachment')}
                className='absolute top-2 right-2'
                disabled={controlsDisabled}
                onClick={() => attachments.remove(frame.id)}
                size='icon-sm'
                type='button'
                variant='secondary'
              >
                <XIcon />
              </Button>
            </div>
          ) : (
            <button
              className='border-border text-muted-foreground hover:border-primary/50 hover:text-foreground flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50'
              disabled={controlsDisabled}
              onClick={() => openUpload(role)}
              type='button'
            >
              <ImageIcon size={24} />
              {t('Upload image')}
            </button>
          )}
          <div className='flex flex-wrap gap-2'>
            <Button
              disabled={controlsDisabled}
              onClick={() => openUpload(role)}
              size='sm'
              type='button'
              variant='outline'
            >
              <UploadIcon />
              {frame ? t('Replace') : t('Upload')}
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <div className='grid gap-3 px-4 pt-3 sm:grid-cols-2'>
        {renderFrame('first_frame', t('First frame'))}
        {renderFrame('last_frame', t('Last frame'))}
      </div>
      <input
        accept='image/*'
        className='hidden'
        disabled={controlsDisabled}
        onChange={(event) => void handleFile(event.target.files?.[0])}
        ref={inputRef}
        type='file'
      />
    </>
  )
}
