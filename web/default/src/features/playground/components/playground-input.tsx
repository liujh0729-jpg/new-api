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
import { useCallback, useState } from 'react'
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
  MonitorIcon,
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
  PromptInputAttachment,
  PromptInputButton,
  PromptInputFooter,
  PromptInputTextarea,
  PromptInputTools,
  type PromptInputMessage,
  type PromptInputPreparedFile,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { uploadReferenceMedia } from '../api'
import {
  getImageSizeOptionsForModel,
  getLTXVideoSizeOptionsForModel,
  getVideoDurationRangeForModel,
  getVideoRatioOptionsForModel,
  getVideoResolutionOptionsForModel,
  IMAGE_REFERENCE_ACCEPT,
  IMAGE_REFERENCE_LIMITS,
  normalizeVideoDurationForModel,
  SEEDANCE_REFERENCE_ACCEPT,
  SEEDANCE_REFERENCE_LIMITS,
} from '../constants'
import { normalizePlaygroundError } from '../lib'
import type { ModelOption, GroupOption, PlaygroundMode } from '../types'
import { MaterialSelectorDialog } from './material-selector-dialog'

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
  videoResolution: string
  onVideoResolutionChange: (value: string) => void
  videoSize: string
  onVideoSizeChange: (value: string) => void
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
  const canSubmit =
    isVideoMode || isImageMode ? hasText || hasReferences : hasText

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

function DirectAttachmentButton({ disabled }: { disabled?: boolean }) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()

  return (
    <PromptInputButton
      aria-label={t('Attach')}
      className='border font-medium'
      disabled={disabled}
      onClick={attachments.openFileDialog}
      type='button'
      variant='outline'
    >
      <PaperclipIcon size={16} />
      <span className='hidden sm:inline'>{t('Attach')}</span>
      <span className='sr-only sm:hidden'>{t('Attach')}</span>
    </PromptInputButton>
  )
}

function ReferenceAttachments() {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  if (attachments.files.length === 0) return null

  return (
    <div className='flex flex-wrap gap-2 px-4 pt-3'>
      {attachments.files.map((attachment, index) => (
        <div className='relative' key={attachment.id}>
          <span
            aria-label={t('Reference {{n}}', { n: index + 1 })}
            className='bg-primary text-primary-foreground pointer-events-none absolute -top-1.5 -left-1.5 z-10 flex size-4 items-center justify-center rounded-full text-[9px] font-semibold shadow-sm'
          >
            {index + 1}
          </span>
          <PromptInputAttachment className='max-w-48' data={attachment} />
        </div>
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
  videoResolution,
  onVideoResolutionChange,
  videoSize,
  onVideoSizeChange,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [isMaterialSelectorOpen, setIsMaterialSelectorOpen] = useState(false)
  const [isPreparingReferences, setIsPreparingReferences] = useState(false)
  const isImageMode = mode === 'image'
  const isVideoMode = mode === 'video'
  const controlsDisabled = disabled || isPreparingReferences
  const imageSizeOptions = getImageSizeOptionsForModel(modelValue)
  const videoDurationRange = getVideoDurationRangeForModel(modelValue)
  const videoSizeOptions = getLTXVideoSizeOptionsForModel(modelValue)
  const videoResolutionOptions = getVideoResolutionOptionsForModel(modelValue)
  const usesVideoSizeOptions = videoSizeOptions.length > 0
  const normalizedVideoDuration = normalizeVideoDurationForModel(
    modelValue,
    videoDuration
  )
  const videoDurationPercent =
    videoDurationRange.max === videoDurationRange.min
      ? 100
      : ((normalizedVideoDuration - videoDurationRange.min) /
          (videoDurationRange.max - videoDurationRange.min)) *
        100
  const ModeIcon = isVideoMode
    ? VideoIcon
    : isImageMode
      ? ImageIcon
      : MessageSquareIcon

  const isModelSelectDisabled =
    controlsDisabled || isModelLoading || models.length === 0
  const isGroupSelectDisabled = controlsDisabled || groups.length === 0
  const isSubmitDisabled =
    controlsDisabled || isGenerating || isModelLoading || !modelValue

  const prepareReferenceFiles = useCallback(
    async (files: File[]): Promise<PromptInputPreparedFile[]> => {
      try {
        return await Promise.all(
          files.map(async (file) => {
            const uploaded = await uploadReferenceMedia(file)
            return {
              url: uploaded.url,
              mediaType: uploaded.media_type || file.type,
              filename: uploaded.filename || file.name,
              sourceFile: isVideoMode ? file : undefined,
            }
          })
        )
      } catch (error) {
        const uploadError = normalizePlaygroundError(error, t)
        throw new Error(uploadError.message, { cause: error })
      }
    },
    [isVideoMode, t]
  )

  const handleSubmit = (message: PromptInputMessage) => {
    const hasText = !!message.text?.trim()
    const hasReferences = !!message.files?.length
    if (
      isSubmitDisabled ||
      (!hasText && ((!isVideoMode && !isImageMode) || !hasReferences))
    ) {
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
    setIsMaterialSelectorOpen(true)
  }

  const handleVideoDurationChange = (value: string) => {
    onVideoDurationChange(
      normalizeVideoDurationForModel(modelValue, Number.parseInt(value, 10))
    )
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
        multiple={isVideoMode || isImageMode}
        onError={(error) => toast.error(error.message)}
        onFilesPreparingChange={setIsPreparingReferences}
        prepareFiles={
          isImageMode || isVideoMode ? prepareReferenceFiles : undefined
        }
        onSubmit={handleSubmit}
      >
        {(isVideoMode || isImageMode) && <ReferenceAttachments />}

        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='px-5 md:text-base'
          disabled={controlsDisabled}
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
                    disabled={controlsDisabled}
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

            {isVideoMode || isImageMode ? (
              <DirectAttachmentButton disabled={controlsDisabled} />
            ) : (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <PromptInputButton
                      className='border font-medium'
                      disabled={controlsDisabled}
                      variant='outline'
                    />
                  }
                >
                  <PaperclipIcon size={16} />
                  <span className='hidden sm:inline'>{t('Attach')}</span>
                  <span className='sr-only sm:hidden'>{t('Attach')}</span>
                </DropdownMenuTrigger>
                <DropdownMenuContent align='start'>
                  <DropdownMenuItem
                    onClick={() => toast.info(t('Feature in development'))}
                  >
                    <FileIcon className='mr-2' size={16} />
                    {t('Upload file')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => toast.info(t('Feature in development'))}
                  >
                    <ImageIcon className='mr-2' size={16} />
                    {t('Upload photo')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => toast.info(t('Feature in development'))}
                  >
                    <ScreenShareIcon className='mr-2' size={16} />
                    {t('Take screenshot')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => toast.info(t('Feature in development'))}
                  >
                    <CameraIcon className='mr-2' size={16} />
                    {t('Take photo')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}

            {isImageMode && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <PromptInputButton
                      className='border font-medium'
                      disabled={controlsDisabled}
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
                {usesVideoSizeOptions ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <PromptInputButton
                          className='border font-medium'
                          disabled={controlsDisabled}
                          type='button'
                          variant='outline'
                        />
                      }
                    >
                      <MonitorIcon size={16} />
                      <span>{videoSize}</span>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align='start' className='min-w-36'>
                      <DropdownMenuGroup>
                        <DropdownMenuLabel>{t('Size')}</DropdownMenuLabel>
                        <DropdownMenuRadioGroup
                          value={videoSize}
                          onValueChange={onVideoSizeChange}
                        >
                          {videoSizeOptions.map((size) => (
                            <DropdownMenuRadioItem key={size} value={size}>
                              {size}
                            </DropdownMenuRadioItem>
                          ))}
                        </DropdownMenuRadioGroup>
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  <>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <PromptInputButton
                            className='border font-medium'
                            disabled={controlsDisabled}
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
                          <DropdownMenuLabel>
                            {t('Aspect ratio')}
                          </DropdownMenuLabel>
                          <DropdownMenuRadioGroup
                            value={videoRatio}
                            onValueChange={onVideoRatioChange}
                          >
                            {getVideoRatioOptionsForModel(modelValue).map(
                              (ratio) => (
                                <DropdownMenuRadioItem
                                  key={ratio}
                                  value={ratio}
                                >
                                  {ratio}
                                </DropdownMenuRadioItem>
                              )
                            )}
                          </DropdownMenuRadioGroup>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>

                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <PromptInputButton
                            className='border font-medium'
                            disabled={controlsDisabled}
                            type='button'
                            variant='outline'
                          />
                        }
                      >
                        <MonitorIcon size={16} />
                        <span>{videoResolution}</span>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='start' className='min-w-36'>
                        <DropdownMenuGroup>
                          <DropdownMenuLabel>
                            {t('Resolution')}
                          </DropdownMenuLabel>
                          <DropdownMenuRadioGroup
                            value={videoResolution}
                            onValueChange={onVideoResolutionChange}
                          >
                            {videoResolutionOptions.map((resolution) => (
                              <DropdownMenuRadioItem
                                key={resolution}
                                value={resolution}
                              >
                                {resolution}
                              </DropdownMenuRadioItem>
                            ))}
                          </DropdownMenuRadioGroup>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </>
                )}

                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <PromptInputButton
                        className='border font-medium'
                        disabled={controlsDisabled}
                        type='button'
                        variant='outline'
                      />
                    }
                  >
                    <ClockIcon size={16} />
                    <span>{normalizedVideoDuration}s</span>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align='start' className='w-64'>
                    <DropdownMenuGroup>
                      <div className='flex items-center justify-between px-2 py-1.5'>
                        <DropdownMenuLabel className='p-0'>
                          {t('Duration')}
                        </DropdownMenuLabel>
                        <span className='text-foreground text-sm font-semibold'>
                          {normalizedVideoDuration}s
                        </span>
                      </div>
                      <div className='space-y-2 px-2 pb-2'>
                        <input
                          aria-label={t('Duration')}
                          className='accent-primary h-2 w-full cursor-pointer rounded-full'
                          disabled={controlsDisabled}
                          max={videoDurationRange.max}
                          min={videoDurationRange.min}
                          onChange={(event) =>
                            handleVideoDurationChange(event.target.value)
                          }
                          step={videoDurationRange.step}
                          style={{
                            background: `linear-gradient(to right, hsl(var(--primary)) ${videoDurationPercent}%, hsl(var(--muted)) ${videoDurationPercent}%)`,
                          }}
                          type='range'
                          value={normalizedVideoDuration}
                        />
                        <div className='text-muted-foreground flex items-center justify-between text-xs'>
                          <span>{videoDurationRange.min}s</span>
                          <span>{videoDurationRange.max}s</span>
                        </div>
                      </div>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </>
            )}

            {(isImageMode || isVideoMode) && (
              <PromptInputButton
                className='border font-medium'
                disabled={controlsDisabled}
                onClick={handleInsertReferenceMarker}
                type='button'
                variant='outline'
              >
                <AtSignIcon size={16} />
                <span className='sr-only'>
                  {t('Select reference material')}
                </span>
              </PromptInputButton>
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

        {mode !== 'chat' && (
          <MaterialSelectorDialog
            open={isMaterialSelectorOpen}
            onOpenChange={setIsMaterialSelectorOpen}
            mode={mode as 'image' | 'video'}
          />
        )}
      </PromptInput>
    </div>
  )
}
