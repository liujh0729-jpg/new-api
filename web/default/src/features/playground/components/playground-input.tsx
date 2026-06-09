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
import {
  PaperclipIcon,
  ChevronDownIcon,
  FileIcon,
  ImageIcon,
  MessageSquareIcon,
  ScreenShareIcon,
  CameraIcon,
  SendIcon,
  SquareIcon,
  Settings2Icon,
  VideoIcon,
  ClockIcon,
  AtSignIcon,
  RectangleHorizontalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  PromptInput,
  PromptInputActionAddAttachments,
  PromptInputAttachment,
  PromptInputButton,
  PromptInputFooter,
  PromptInputTextarea,
  PromptInputTools,
  type PromptInputMessage,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import { ModelGroupSelector } from '@/components/model-group-selector'
import {
  getImageSizeOptionsForModel,
  getVideoRatioOptionsForModel,
  IMAGE_REFERENCE_ACCEPT,
  IMAGE_REFERENCE_LIMITS,
  SEEDANCE_REFERENCE_ACCEPT,
  SEEDANCE_REFERENCE_LIMITS,
  VIDEO_DURATION_OPTIONS,
} from '../constants'
import type { ModelOption, GroupOption, PlaygroundMode } from '../types'

interface PlaygroundInputProps {
  onSubmit: (message: PromptInputMessage) => void | Promise<void>
  onStop?: () => void
  mode: PlaygroundMode
  onModeChange: (value: PlaygroundMode) => void
  disabled?: boolean
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  groups: GroupOption[]
  groupValue: string
  onGroupChange: (value: string) => void
  imageSize: string
  onImageSizeChange: (value: string) => void
  imageQuality: string
  onImageQualityChange: (value: string) => void
  imageCount: number
  onImageCountChange: (value: number) => void
  videoRatio: string
  onVideoRatioChange: (value: string) => void
  videoDuration: number
  onVideoDurationChange: (value: number) => void
}

const imageQualities = ['standard', 'hd', 'auto']
const imageCounts = [1, 2, 4]

function PlaygroundSubmitButton({
  disabled,
  isVideoMode,
  isImageMode,
  text,
}: {
  disabled?: boolean
  isVideoMode: boolean
  isImageMode: boolean
  text: string
}) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const hasText = text.trim().length > 0
  const hasReferences = attachments.files.length > 0
  const canSubmit = isVideoMode || isImageMode ? hasText || hasReferences : hasText

  return (
    <PromptInputButton
      className='text-foreground font-medium'
      disabled={disabled || !canSubmit}
      type='submit'
      variant='secondary'
    >
      <SendIcon size={16} />
      <span className='hidden sm:inline'>{t('Send')}</span>
      <span className='sr-only sm:hidden'>{t('Send')}</span>
    </PromptInputButton>
  )
}

function ReferenceAttachments() {
  const attachments = usePromptInputAttachments()
  if (attachments.files.length === 0) return null

  return (
    <div className='flex flex-wrap gap-2 px-4 pt-3'>
      {attachments.files.map((attachment) => (
        <PromptInputAttachment
          className='max-w-48'
          data={attachment}
          key={attachment.id}
        />
      ))}
    </div>
  )
}

export function PlaygroundInput({
  onSubmit,
  onStop,
  mode,
  onModeChange,
  disabled,
  isGenerating,
  models,
  modelValue,
  onModelChange,
  isModelLoading = false,
  groups,
  groupValue,
  onGroupChange,
  imageSize,
  onImageSizeChange,
  imageQuality,
  onImageQualityChange,
  imageCount,
  onImageCountChange,
  videoRatio,
  onVideoRatioChange,
  videoDuration,
  onVideoDurationChange,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const isImageMode = mode === 'image'
  const isVideoMode = mode === 'video'
  const imageSizeOptions = getImageSizeOptionsForModel(modelValue)
  const ModeIcon = isVideoMode
    ? VideoIcon
    : isImageMode
      ? ImageIcon
      : MessageSquareIcon

  const isModelSelectDisabled =
    disabled || isModelLoading || models.length === 0
  const isGroupSelectDisabled = disabled || groups.length === 0
  const isSubmitDisabled =
    disabled || isGenerating || isModelLoading || !modelValue

  const handleSubmit = (message: PromptInputMessage) => {
    const hasText = !!message.text?.trim()
    const hasReferences = !!message.files?.length
    if (isSubmitDisabled || (!hasText && (!isVideoMode && !isImageMode || !hasReferences))) {
      return
    }
    const result = onSubmit({
      ...message,
      text: message.text?.trim() || '',
    })
    if (result instanceof Promise) {
      return result.then(() => setText(''))
    }
    setText('')
    return result
  }

  const handleInsertReferenceMarker = () => {
    setText((prev) => {
      if (!prev.trim()) return '@'
      return prev.endsWith(' ') ? `${prev}@` : `${prev} @`
    })
  }

  return (
    <div className='grid shrink-0 gap-4 px-1 md:pb-4'>
      <PromptInput
        accept={
          isVideoMode
            ? SEEDANCE_REFERENCE_ACCEPT
            : isImageMode
              ? IMAGE_REFERENCE_ACCEPT
              : undefined
        }
        groupClassName='rounded-xl'
        maxFileSize={
          isVideoMode
            ? SEEDANCE_REFERENCE_LIMITS.maxFileSize
            : isImageMode
              ? IMAGE_REFERENCE_LIMITS.maxFileSize
              : undefined
        }
        maxFiles={
          isVideoMode
            ? SEEDANCE_REFERENCE_LIMITS.total
            : isImageMode
              ? IMAGE_REFERENCE_LIMITS.maxFiles
              : undefined
        }
        multiple={isVideoMode}
        onError={(error) => toast.error(error.message)}
        onSubmit={handleSubmit}
      >
        {(isVideoMode || isImageMode) && <ReferenceAttachments />}

        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='px-5 md:text-base'
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          placeholder={
            isVideoMode
              ? t('Upload references or describe the video')
              : isImageMode
                ? t('Describe the image to generate')
                : t('Ask anything')
          }
          value={text}
        />

        <PromptInputFooter className='p-2.5'>
          <PromptInputTools>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <PromptInputButton
                    className='min-w-24 justify-between border font-medium'
                    disabled={disabled}
                    type='button'
                    variant='outline'
                  />
                }
              >
                <ModeIcon size={16} />
                <span>
                  {isVideoMode
                    ? t('Video')
                    : isImageMode
                      ? t('Image')
                      : t('Chat')}
                </span>
                <ChevronDownIcon className='opacity-70' size={14} />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='start' className='min-w-36'>
                <DropdownMenuRadioGroup
                  value={mode}
                  onValueChange={(value) =>
                    onModeChange(value as PlaygroundMode)
                  }
                >
                  <DropdownMenuRadioItem value='chat'>
                    <MessageSquareIcon className='mr-2' size={16} />
                    {t('Chat')}
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value='image'>
                    <ImageIcon className='mr-2' size={16} />
                    {t('Image')}
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value='video'>
                    <VideoIcon className='mr-2' size={16} />
                    {t('Video')}
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>

            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <PromptInputButton
                    className='border font-medium'
                    disabled={disabled}
                    variant='outline'
                  />
                }
              >
                <PaperclipIcon size={16} />
                <span className='hidden sm:inline'>{t('Attach')}</span>
                <span className='sr-only sm:hidden'>{t('Attach')}</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='start'>
                {isVideoMode || isImageMode ? (
                  <PromptInputActionAddAttachments
                    label={
                      isVideoMode
                        ? t('Add reference media')
                        : t('Add reference image')
                    }
                  />
                ) : (
                  <>
                    <DropdownMenuItem
                      onClick={() =>
                        toast.info(t('Feature in development'))
                      }
                    >
                      <FileIcon className='mr-2' size={16} />
                      {t('Upload file')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() =>
                        toast.info(t('Feature in development'))
                      }
                    >
                      <ImageIcon className='mr-2' size={16} />
                      {t('Upload photo')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() =>
                        toast.info(t('Feature in development'))
                      }
                    >
                      <ScreenShareIcon className='mr-2' size={16} />
                      {t('Take screenshot')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() =>
                        toast.info(t('Feature in development'))
                      }
                    >
                      <CameraIcon className='mr-2' size={16} />
                      {t('Take photo')}
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>

            {isImageMode && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <PromptInputButton
                      className='border font-medium'
                      disabled={disabled}
                      type='button'
                      variant='outline'
                    />
                  }
                >
                  <Settings2Icon size={16} />
                  <span className='hidden sm:inline'>
                    {t('Image settings')}
                  </span>
                  <span className='sr-only sm:hidden'>
                    {t('Image settings')}
                  </span>
                </DropdownMenuTrigger>
                <DropdownMenuContent align='start' className='min-w-44'>
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>{t('Size')}</DropdownMenuLabel>
                    <DropdownMenuRadioGroup
                      value={imageSize}
                      onValueChange={onImageSizeChange}
                    >
                      {imageSizeOptions.map((size) => (
                        <DropdownMenuRadioItem key={size} value={size}>
                          {size}
                        </DropdownMenuRadioItem>
                      ))}
                    </DropdownMenuRadioGroup>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>{t('Quality')}</DropdownMenuLabel>
                    <DropdownMenuRadioGroup
                      value={imageQuality}
                      onValueChange={onImageQualityChange}
                    >
                      {imageQualities.map((quality) => (
                        <DropdownMenuRadioItem key={quality} value={quality}>
                          {t(quality)}
                        </DropdownMenuRadioItem>
                      ))}
                    </DropdownMenuRadioGroup>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>{t('Count')}</DropdownMenuLabel>
                    <DropdownMenuRadioGroup
                      value={String(imageCount)}
                      onValueChange={(value) =>
                        onImageCountChange(Number.parseInt(value, 10))
                      }
                    >
                      {imageCounts.map((count) => (
                        <DropdownMenuRadioItem
                          key={count}
                          value={String(count)}
                        >
                          {count}
                        </DropdownMenuRadioItem>
                      ))}
                    </DropdownMenuRadioGroup>
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}

            {isVideoMode && (
              <>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <PromptInputButton
                        className='border font-medium'
                        disabled={disabled}
                        type='button'
                        variant='outline'
                      />
                    }
                  >
                    <RectangleHorizontalIcon size={16} />
                    <span>{videoRatio}</span>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align='start' className='min-w-36'>
                    <DropdownMenuGroup>
                      <DropdownMenuLabel>{t('Aspect ratio')}</DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        value={videoRatio}
                        onValueChange={onVideoRatioChange}
                      >
                        {getVideoRatioOptionsForModel(modelValue).map((ratio) => (
                          <DropdownMenuRadioItem key={ratio} value={ratio}>
                            {ratio}
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>

                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <PromptInputButton
                        className='border font-medium'
                        disabled={disabled}
                        type='button'
                        variant='outline'
                      />
                    }
                  >
                    <ClockIcon size={16} />
                    <span>{videoDuration}s</span>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align='start' className='min-w-32'>
                    <DropdownMenuGroup>
                      <DropdownMenuLabel>{t('Duration')}</DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        value={String(videoDuration)}
                        onValueChange={(value) =>
                          onVideoDurationChange(Number.parseInt(value, 10))
                        }
                      >
                        {VIDEO_DURATION_OPTIONS.map((duration) => (
                          <DropdownMenuRadioItem
                            key={duration}
                            value={String(duration)}
                          >
                            {duration}s
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>

                <PromptInputButton
                  className='border font-medium'
                  disabled={disabled}
                  onClick={handleInsertReferenceMarker}
                  type='button'
                  variant='outline'
                >
                  <AtSignIcon size={16} />
                  <span className='sr-only'>
                    {t('Insert reference marker')}
                  </span>
                </PromptInputButton>
              </>
            )}
          </PromptInputTools>

          <div className='flex items-center gap-1.5 md:gap-2'>
            <ModelGroupSelector
              selectedModel={modelValue}
              models={models}
              onModelChange={onModelChange}
              selectedGroup={groupValue}
              groups={groups}
              onGroupChange={onGroupChange}
              disabled={isModelSelectDisabled || isGroupSelectDisabled}
            />

            {isGenerating && onStop ? (
              <PromptInputButton
                className='text-foreground font-medium'
                onClick={onStop}
                variant='secondary'
              >
                <SquareIcon className='fill-current' size={16} />
                <span className='hidden sm:inline'>{t('Stop')}</span>
                <span className='sr-only sm:hidden'>{t('Stop')}</span>
              </PromptInputButton>
            ) : (
              <PlaygroundSubmitButton
                disabled={isSubmitDisabled}
                isVideoMode={isVideoMode}
                isImageMode={isImageMode}
                text={text}
              />
            )}
          </div>
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}
