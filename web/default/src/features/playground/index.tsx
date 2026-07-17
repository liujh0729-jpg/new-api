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
import { useQuery } from '@tanstack/react-query'
import type { FileUIPart } from 'ai'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type {
  PromptInputMessage,
  PromptInputSubmittedFile,
} from '@/components/ai-elements/prompt-input'
import { getUserModels, getUserGroups, uploadReferenceMedia } from './api'
import { PlaygroundChat } from './components/playground-chat'
import {
  PlaygroundHistoryMobileHeader,
  PlaygroundHistorySidebar,
} from './components/playground-history'
import { PlaygroundInput } from './components/playground-input'
import {
  DEFAULT_GROUP,
  ERROR_MESSAGES,
  SEEDANCE_REFERENCE_LIMITS,
  isAIPDDFluxImageToImageModel,
  isLTX23StartEndModel,
  isLTX23PolicyModel,
  isLTXVideoModel,
  normalizeLTXVideoSizeForModel,
  normalizeImageSizeForModel,
  normalizeVideoDurationForModel,
  normalizeVideoRatioForModel,
  normalizeVideoResolutionForModel,
} from './constants'
import { usePlaygroundState, useChatHandler } from './hooks'
import {
  createUserMessage,
  createLoadingAssistantMessage,
  createClientTaskId,
  normalizePlaygroundError,
} from './lib'
import {
  resolveLTXStartEndTimeline,
  validateLTXStartEndImageCount,
} from './lib/ltx-start-end'
import type {
  Message as MessageType,
  SeedanceReference,
  SeedanceReferenceKind,
} from './types'

export function Playground() {
  const { t } = useTranslation()
  const {
    config,
    activeConversationId,
    conversations,
    parameterEnabled,
    messages,
    models,
    groups,
    updateMessages,
    setModels,
    setGroups,
    updateConfig,
    createConversation,
    selectConversation,
    deleteConversation,
  } = usePlaygroundState()

  const { sendChat, stopGeneration, isGenerating, resumeTaskPolling } =
    useChatHandler({
      config,
      parameterEnabled,
      onMessageUpdate: updateMessages,
    })
  const hasPendingMessage = useMemo(
    () =>
      messages.some(
        (message) =>
          message.from === 'assistant' &&
          (message.status === 'loading' || message.status === 'streaming')
      ),
    [messages]
  )
  const isGenerationActive = isGenerating || hasPendingMessage

  // Edit dialog state
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )
  const [isUploadingReferences, setIsUploadingReferences] = useState(false)

  // Load models
  const { data: modelsData, isLoading: isLoadingModels } = useQuery({
    queryKey: ['playground-models', config.mode],
    queryFn: () => getUserModels(config.mode),
  })

  // Load groups
  const { data: groupsData } = useQuery({
    queryKey: ['playground-groups'],
    queryFn: getUserGroups,
  })

  // Update models when data changes
  useEffect(() => {
    if (!modelsData) return

    setModels(modelsData)

    // Set default model if current model is not available
    const isCurrentModelValid = modelsData.some((m) => m.value === config.model)
    if (modelsData.length > 0 && !isCurrentModelValid) {
      updateConfig('model', modelsData[0].value)
    }
  }, [modelsData, config.model, setModels, updateConfig])

  useEffect(() => {
    if (config.mode !== 'image') return

    const normalizedSize = normalizeImageSizeForModel(
      config.model,
      config.image_size
    )
    if (normalizedSize !== config.image_size) {
      updateConfig('image_size', normalizedSize)
    }
  }, [config.mode, config.model, config.image_size, updateConfig])

  useEffect(() => {
    if (config.mode !== 'video') return

    const normalizedRatio = normalizeVideoRatioForModel(
      config.model,
      config.video_ratio
    )
    if (normalizedRatio !== config.video_ratio) {
      updateConfig('video_ratio', normalizedRatio)
    }
  }, [config.mode, config.model, config.video_ratio, updateConfig])

  useEffect(() => {
    if (config.mode !== 'video') return

    const normalizedDuration = normalizeVideoDurationForModel(
      config.model,
      config.video_duration
    )
    if (normalizedDuration !== config.video_duration) {
      updateConfig('video_duration', normalizedDuration)
    }
  }, [config.mode, config.model, config.video_duration, updateConfig])

  useEffect(() => {
    if (config.mode !== 'video') return

    const effectiveResolutions = models.find(
      (model) => model.value === config.model
    )?.video_resolutions
    const normalizedResolution = normalizeVideoResolutionForModel(
      config.model,
      config.video_resolution,
      effectiveResolutions
    )
    if (normalizedResolution !== config.video_resolution) {
      updateConfig('video_resolution', normalizedResolution)
    }
  }, [config.mode, config.model, config.video_resolution, models, updateConfig])

  useEffect(() => {
    if (config.mode !== 'video') return

    const normalizedSize = normalizeLTXVideoSizeForModel(
      config.model,
      config.video_size
    )
    if (normalizedSize !== config.video_size) {
      updateConfig('video_size', normalizedSize)
    }
  }, [config.mode, config.model, config.video_size, updateConfig])

  // Update groups when data changes
  useEffect(() => {
    if (!groupsData) return

    // Add auto group if not present
    const hasAutoGroup = groupsData.some((g) => g.value === DEFAULT_GROUP)
    const processedGroups = hasAutoGroup
      ? groupsData
      : [
          {
            value: DEFAULT_GROUP,
            label: 'Auto',
            ratio: 1,
            desc: 'Circuit Breaker',
          },
          ...groupsData,
        ]

    setGroups(processedGroups)
  }, [groupsData, setGroups])

  const resumedTaskKeyRef = useRef<string | null>(null)

  useEffect(() => {
    const last = messages[messages.length - 1]
    if (
      last?.from === 'assistant' &&
      last.taskId &&
      last.taskType &&
      (last.status === 'loading' || last.status === 'streaming')
    ) {
      const taskKey = `${activeConversationId}:${last.taskType}:${last.taskId}`
      if (resumedTaskKeyRef.current === taskKey) return
      resumedTaskKeyRef.current = taskKey
      resumeTaskPolling(last.taskId, last.taskType)
    }
  }, [activeConversationId, messages, resumeTaskPolling])

  const createGenerationLoadingMessage = useCallback(() => {
    if (config.mode !== 'image' && config.mode !== 'video') {
      return {
        assistantMessage: createLoadingAssistantMessage(),
        clientTaskId: undefined,
      }
    }

    const clientTaskId = createClientTaskId()
    return {
      assistantMessage: createLoadingAssistantMessage({
        taskId: clientTaskId,
        taskType: config.mode,
        activity:
          config.mode === 'image' ? 'image_generation' : 'video_generation',
      }),
      clientTaskId,
    }
  }, [config.mode])

  const handleSendMessage = async (message: PromptInputMessage) => {
    const text = message.text?.trim() || ''
    let messageReferences: SeedanceReference[] = []
    let imageReferences: string[] | undefined
    let videoReferences: SeedanceReference[] | undefined

    if (config.mode === 'video') {
      const referenceCandidates = buildSeedanceReferenceCandidates(
        message.files || []
      )
      const validationError = validateVideoInput(
        text,
        referenceCandidates,
        message.files?.length || 0,
        config.model,
        config.ltx_timeline_data,
        config.video_duration,
        t
      )
      if (validationError) {
        toast.error(validationError)
        throw new Error(validationError)
      }
      const durationValidationError = await validateSeedanceReferenceDurations(
        referenceCandidates,
        t
      )
      if (durationValidationError) {
        toast.error(durationValidationError)
        throw new Error(durationValidationError)
      }
      if (referenceCandidates.some((reference) => reference.kind === 'video')) {
        toast.warning(
          t('Seedance may reject reference videos containing real people.')
        )
      }

      const hasReferencesRequiringUpload = referenceCandidates.some(
        (reference) =>
          reference.sourceFile && !hasUsableReferenceUrl(reference.url)
      )
      if (hasReferencesRequiringUpload) {
        setIsUploadingReferences(true)
      }
      try {
        const resolvedReferences =
          await resolveSeedanceReferenceURLs(referenceCandidates)
        messageReferences = resolvedReferences.displayReferences
        videoReferences = resolvedReferences.requestReferences
      } catch (error) {
        const uploadError = normalizePlaygroundError(error, t)
        toast.error(uploadError.message)
        throw new Error(uploadError.message, { cause: error })
      } finally {
        if (hasReferencesRequiringUpload) {
          setIsUploadingReferences(false)
        }
      }
    }

    if (config.mode === 'image') {
      try {
        const resolvedReferences = await resolveImageReferences(
          message.files || []
        )
        messageReferences = resolvedReferences.displayReferences
        imageReferences = resolvedReferences.requestUrls
        if (imageReferences.length === 0) {
          imageReferences = undefined
        }
      } catch (error) {
        const uploadError = normalizePlaygroundError(error, t)
        toast.error(uploadError.message)
        throw new Error(uploadError.message, { cause: error })
      }

      if (isAIPDDFluxImageToImageModel(config.model) && !imageReferences) {
        const validationError = t(
          'An image URL or uploaded image is required. Sending only prompt will fail.'
        )
        toast.error(validationError)
        throw new Error(validationError)
      }
    }

    const userMessage = createUserMessage(text, messageReferences)
    const { assistantMessage, clientTaskId } = createGenerationLoadingMessage()

    const newMessages = [...messages, userMessage, assistantMessage]
    updateMessages(newMessages)

    // Send chat request
    sendChat(newMessages, {
      imageReferences,
      videoReferences,
      clientTaskId,
    })
  }

  const handleCopyMessage = (message: MessageType) => {
    // Copy is handled in MessageActions component
    // eslint-disable-next-line no-console
    console.log('Message copied:', message.key)
  }

  const handleRegenerateMessage = (message: MessageType) => {
    // Find the message index and regenerate from there
    const messageIndex = messages.findIndex((m) => m.key === message.key)
    if (messageIndex === -1) return

    // Remove messages after this one and regenerate
    const messagesUpToHere = messages.slice(0, messageIndex)
    const { assistantMessage, clientTaskId } = createGenerationLoadingMessage()
    const newMessages = [...messagesUpToHere, assistantMessage]

    updateMessages(newMessages)
    sendChat(newMessages, { clientTaskId })
  }

  const handleEditMessage = useCallback((message: MessageType) => {
    setEditingMessageKey(message.key)
  }, [])

  const handleEditOpenChange = useCallback((open: boolean) => {
    if (!open) setEditingMessageKey(null)
  }, [])

  // Apply edit and optionally re-submit from the edited user message
  const applyEdit = useCallback(
    (newContent: string, submit: boolean) => {
      if (!editingMessageKey) return
      const index = messages.findIndex((m) => m.key === editingMessageKey)
      if (index === -1) return

      const updated = messages.map((m) =>
        m.key === editingMessageKey
          ? { ...m, versions: [{ ...m.versions[0], content: newContent }] }
          : m
      )

      setEditingMessageKey(null)

      if (!submit || updated[index].from !== 'user') {
        updateMessages(updated)
        return
      }

      const { assistantMessage, clientTaskId } =
        createGenerationLoadingMessage()
      const toSubmit = [...updated.slice(0, index + 1), assistantMessage]
      updateMessages(toSubmit)
      sendChat(toSubmit, { clientTaskId })
    },
    [
      editingMessageKey,
      messages,
      updateMessages,
      sendChat,
      createGenerationLoadingMessage,
    ]
  )

  const handleDeleteMessage = (message: MessageType) => {
    const newMessages = messages.filter((m) => m.key !== message.key)
    updateMessages(newMessages)
  }

  return (
    <div className='relative flex size-full overflow-hidden'>
      <PlaygroundHistorySidebar
        activeConversationId={activeConversationId}
        conversations={conversations}
        disabled={isGenerationActive}
        onDeleteConversation={deleteConversation}
        onNewConversation={createConversation}
        onSelectConversation={selectConversation}
      />

      <div className='flex min-w-0 flex-1 flex-col overflow-hidden'>
        <PlaygroundHistoryMobileHeader
          activeConversationId={activeConversationId}
          conversations={conversations}
          disabled={isGenerationActive}
          onDeleteConversation={deleteConversation}
          onNewConversation={createConversation}
          onSelectConversation={selectConversation}
        />

        {/* Full-width scroll container: scrolling works even over side whitespace */}
        <div className='flex flex-1 flex-col overflow-hidden'>
          <PlaygroundChat
            messages={messages}
            onCopyMessage={handleCopyMessage}
            onRegenerateMessage={handleRegenerateMessage}
            onEditMessage={handleEditMessage}
            onDeleteMessage={handleDeleteMessage}
            isGenerating={isGenerationActive}
            editingKey={editingMessageKey}
            onCancelEdit={handleEditOpenChange}
            onSaveEdit={(newContent) => applyEdit(newContent, false)}
            onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
          />
        </div>

        {/* Input area: center content and constrain to the same container width */}
        <div className='mx-auto w-full max-w-4xl'>
          <PlaygroundInput
            disabled={isUploadingReferences}
            groups={groups}
            groupValue={config.group}
            imageCount={config.image_count}
            imageQuality={config.image_quality}
            imageSize={config.image_size}
            isGenerating={isGenerationActive}
            isModelLoading={isLoadingModels}
            mode={config.mode}
            modelValue={config.model}
            models={models}
            thinkingMode={config.thinking_mode}
            onGroupChange={(value) => updateConfig('group', value)}
            onImageCountChange={(value) => updateConfig('image_count', value)}
            onImageQualityChange={(value) =>
              updateConfig('image_quality', value)
            }
            onImageSizeChange={(value) => updateConfig('image_size', value)}
            onModeChange={(value) => updateConfig('mode', value)}
            onModelChange={(value) => updateConfig('model', value)}
            onThinkingModeChange={(value) =>
              updateConfig('thinking_mode', value)
            }
            onStop={stopGeneration}
            onSubmit={handleSendMessage}
            onVideoDurationChange={(value) =>
              updateConfig('video_duration', value)
            }
            onVideoRatioChange={(value) => updateConfig('video_ratio', value)}
            onVideoResolutionChange={(value) =>
              updateConfig('video_resolution', value)
            }
            onVideoSizeChange={(value) => updateConfig('video_size', value)}
            ltxTimelineData={config.ltx_timeline_data}
            onLtxTimelineDataChange={(value) =>
              updateConfig('ltx_timeline_data', value)
            }
            videoDuration={config.video_duration}
            videoRatio={config.video_ratio}
            videoResolution={config.video_resolution}
            videoSize={config.video_size}
          />
        </div>
      </div>
    </div>
  )
}

type SeedanceReferenceCandidate = SeedanceReference & { sourceFile?: File }
type ResolvedSeedanceReferences = {
  displayReferences: SeedanceReference[]
  requestReferences: SeedanceReference[]
}
type ResolvedImageReferences = {
  displayReferences: SeedanceReference[]
  requestUrls: string[]
}

function buildSeedanceReferenceCandidates(
  files: PromptInputSubmittedFile[]
): SeedanceReferenceCandidate[] {
  return files.flatMap((file) => {
    const url = file.url?.trim()
    const kind = getSeedanceReferenceKind(file)
    if (!url || !kind) return []
    return [
      {
        kind,
        url,
        filename: file.filename,
        media_type: file.mediaType,
        role: file.role,
        sourceFile: file.sourceFile,
      },
    ]
  })
}

async function resolveSeedanceReferenceURLs(
  references: SeedanceReferenceCandidate[]
): Promise<ResolvedSeedanceReferences> {
  const resolvedReferences = await Promise.all(
    references.map(async ({ sourceFile, ...reference }) => {
      if (!sourceFile) {
        const requestUrl = hasUsableReferenceUrl(reference.url)
          ? reference.url
          : await fetchReferenceURLAsDataURL(reference.url)
        return {
          displayReference: reference,
          requestReference: { ...reference, url: requestUrl },
        }
      }

      if (hasUsableReferenceUrl(reference.url)) {
        return {
          displayReference: reference,
          requestReference: reference,
        }
      }

      try {
        const uploaded = await uploadReferenceMedia(sourceFile)
        const displayReference = {
          ...reference,
          url: uploaded.url,
          filename: uploaded.filename || reference.filename,
          media_type: uploaded.media_type || reference.media_type,
        }
        const requestUrl = isProbablyPublicReferenceUrl(uploaded.url)
          ? uploaded.url
          : await resolveInlineReferenceURL(reference.url, sourceFile)
        return {
          displayReference,
          requestReference: {
            ...displayReference,
            url: requestUrl,
          },
        }
      } catch (error) {
        if (
          shouldInlineLocalReference(error) &&
          isDataReferenceUrl(reference.url)
        ) {
          return {
            displayReference: reference,
            requestReference: reference,
          }
        }
        throw error
      }
    })
  )

  if (
    resolvedReferences.some(
      (reference) => !isValidReferenceUrl(reference.requestReference.url)
    )
  ) {
    throw new Error(ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED)
  }

  return {
    displayReferences: resolvedReferences.map(
      (reference) => reference.displayReference
    ),
    requestReferences: resolvedReferences.map(
      (reference) => reference.requestReference
    ),
  }
}

async function resolveInlineReferenceURL(
  url: string,
  file?: File
): Promise<string> {
  if (isDataReferenceUrl(url)) return url
  if (file) return readFileAsDataURL(file)
  return fetchReferenceURLAsDataURL(url)
}

function shouldInlineLocalReference(error: unknown): boolean {
  const code = extractUploadErrorCode(error)
  if (
    code === 'aipdd_channel_unavailable' ||
    code === 'aipdd_channel_key_unavailable' ||
    code === 'aipdd_channel_key_empty'
  ) {
    return true
  }

  const message = extractUploadErrorMessage(error).toLowerCase()
  return (
    message.includes('enabled aipdd channel is not configured') ||
    message.includes('aipdd channel key unavailable') ||
    message.includes('aipdd channel key is empty')
  )
}

function extractUploadErrorCode(error: unknown): string {
  if (typeof error !== 'object' || error === null) return ''
  const response = (error as { response?: { data?: unknown } }).response
  const data = response?.data
  if (typeof data !== 'object' || data === null) return ''
  const nestedError = (data as { error?: unknown }).error
  if (typeof nestedError === 'object' && nestedError !== null) {
    const code = (nestedError as { code?: unknown }).code
    return typeof code === 'string' ? code : ''
  }
  const code = (data as { code?: unknown }).code
  return typeof code === 'string' ? code : ''
}

function extractUploadErrorMessage(error: unknown): string {
  if (typeof error !== 'object' || error === null) {
    return error instanceof Error ? error.message : ''
  }
  const response = (error as { response?: { data?: unknown } }).response
  const data = response?.data
  if (typeof data === 'object' && data !== null) {
    const nestedError = (data as { error?: unknown }).error
    if (typeof nestedError === 'object' && nestedError !== null) {
      const message = (nestedError as { message?: unknown }).message
      if (typeof message === 'string') return message
    }
    const message = (data as { message?: unknown }).message
    if (typeof message === 'string') return message
  }
  return error instanceof Error ? error.message : ''
}

function isValidReferenceUrl(url: string): boolean {
  return isWebUrl(url) || isDataReferenceUrl(url)
}

function isDataReferenceUrl(url: string): boolean {
  return /^data:(image|video|audio)\//i.test(url.trim())
}

function isWebUrl(url: string): boolean {
  return /^https?:\/\//i.test(url.trim())
}

function hasUsableReferenceUrl(url: string): boolean {
  return (
    isValidReferenceUrl(url) &&
    (isDataReferenceUrl(url) || isProbablyPublicReferenceUrl(url))
  )
}

function isProbablyPublicReferenceUrl(url: string): boolean {
  const value = url.trim()
  if (!isWebUrl(value)) return false

  try {
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    if (
      hostname === 'localhost' ||
      hostname === '0.0.0.0' ||
      hostname === '::1' ||
      hostname === '::' ||
      hostname.endsWith('.local') ||
      hostname.startsWith('127.') ||
      hostname.startsWith('10.') ||
      hostname.startsWith('192.168.') ||
      hostname.startsWith('169.254.') ||
      hostname.startsWith('fc') ||
      hostname.startsWith('fd') ||
      hostname.startsWith('fe80:')
    ) {
      return false
    }

    const parts = hostname.split('.').map((part) => Number(part))
    if (
      parts.length === 4 &&
      parts.every((part) => Number.isInteger(part) && part >= 0 && part <= 255)
    ) {
      if (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) {
        return false
      }
    }

    return true
  } catch {
    return false
  }
}

function getSeedanceReferenceKind(
  file: Pick<FileUIPart, 'filename' | 'mediaType' | 'url'>
): SeedanceReferenceKind | null {
  const mediaType = (file.mediaType || inferDataUrlMediaType(file.url) || '')
    .trim()
    .toLowerCase()
  if (mediaType.startsWith('image/')) return 'image'
  if (mediaType.startsWith('video/')) return 'video'
  if (mediaType.startsWith('audio/')) return 'audio'

  const filename = (file.filename || '').toLowerCase()
  if (/\.(png|jpe?g|webp|gif|bmp|heic|heif)$/.test(filename)) return 'image'
  if (/\.(mp4|mov|m4v|webm|mkv|avi|mpeg|mpg|3gp)$/.test(filename)) {
    return 'video'
  }
  if (/\.(mp3|wav|m4a|aac|ogg|oga|flac|opus)$/.test(filename)) return 'audio'
  return null
}

function inferDataUrlMediaType(url?: string): string {
  if (!url?.startsWith('data:')) return ''
  const end = url.indexOf(';')
  if (end === -1) return ''
  return url.slice('data:'.length, end)
}

function validateVideoInput(
  text: string,
  references: SeedanceReference[],
  rawFileCount: number,
  model: string,
  ltxTimelineData: string,
  videoDuration: number,
  t: (key: string, options?: Record<string, unknown>) => string
): string | null {
  if (rawFileCount !== references.length) {
    return t('Only image, video, and audio references are supported')
  }

  const imageCount = references.filter((item) => item.kind === 'image').length
  const videoCount = references.filter((item) => item.kind === 'video').length
  const audioCount = references.filter((item) => item.kind === 'audio').length

  if (isLTX23StartEndModel(model)) {
    if (videoCount > 0) {
      return t('LTX start-end supports image and audio references only')
    }
    const imageCountError = validateLTXStartEndImageCount(imageCount)
    if (imageCountError) {
      return t(imageCountError)
    }
    if (!text) {
      return t('LTX start-end requires a prompt')
    }
    if (audioCount > 1) {
      return t('LTX start-end supports at most one audio reference')
    }
    const timelineResolution = resolveLTXStartEndTimeline(
      text,
      videoDuration,
      ltxTimelineData
    )
    if (timelineResolution.error) {
      return t(timelineResolution.error, {
        frames: timelineResolution.frameCount,
      })
    }
    return null
  }

  if (!text && references.length === 0) {
    return t('Add text or reference media before generating')
  }

  if (isLTXVideoModel(model)) {
    if (videoCount > 0 || audioCount > 0) {
      return t('LTX supports image references only')
    }
    if (imageCount > 1) {
      return t('LTX supports one reference image')
    }
    if (isLTX23PolicyModel(model) && imageCount === 0) {
      return t('LTX 2.3 requires a reference image')
    }
    return null
  }

  if (references.length > SEEDANCE_REFERENCE_LIMITS.total) {
    return t('Seedance supports up to 12 reference media items')
  }
  if (imageCount > SEEDANCE_REFERENCE_LIMITS.image) {
    return t('Seedance supports up to 9 reference images')
  }
  if (videoCount > SEEDANCE_REFERENCE_LIMITS.video) {
    return t('Seedance supports up to 3 reference videos')
  }
  if (audioCount > SEEDANCE_REFERENCE_LIMITS.audio) {
    return t('Seedance supports up to 3 reference audio files')
  }
  if (audioCount > 0 && imageCount + videoCount === 0) {
    return t('Audio references require at least one image or video reference')
  }
  return null
}

async function validateSeedanceReferenceDurations(
  references: SeedanceReferenceCandidate[],
  t: (key: string) => string
): Promise<string | null> {
  const localVideos = references.filter(
    (reference) => reference.kind === 'video' && reference.sourceFile
  )
  const localAudios = references.filter(
    (reference) => reference.kind === 'audio' && reference.sourceFile
  )

  try {
    const videoDurations = await Promise.all(
      localVideos.map((reference) =>
        readLocalMediaDuration(reference.sourceFile!, 'video')
      )
    )
    const audioDurations = await Promise.all(
      localAudios.map((reference) =>
        readLocalMediaDuration(reference.sourceFile!, 'audio')
      )
    )

    if (
      videoDurations.some(
        (duration) =>
          duration < SEEDANCE_REFERENCE_LIMITS.minVideoDurationSeconds
      )
    ) {
      return t('Reference videos must be at least 2 seconds long')
    }
    if (
      videoDurations.some(
        (duration) =>
          duration > SEEDANCE_REFERENCE_LIMITS.maxVideoDurationSeconds
      )
    ) {
      return t('Reference videos must be 15 seconds or shorter')
    }
    if (
      sumDurations(videoDurations) >
      SEEDANCE_REFERENCE_LIMITS.maxVideoTotalDurationSeconds
    ) {
      return t('Reference video total duration must be 15 seconds or shorter')
    }
    if (
      sumDurations(audioDurations) >
      SEEDANCE_REFERENCE_LIMITS.maxAudioTotalDurationSeconds
    ) {
      return t('Reference audio total duration must be 15 seconds or shorter')
    }
  } catch {
    return t(ERROR_MESSAGES.VIDEO_REFERENCE_DURATION_READ_FAILED)
  }

  return null
}

function sumDurations(durations: number[]): number {
  return durations.reduce((total, duration) => total + duration, 0)
}

function readLocalMediaDuration(
  file: File,
  kind: 'audio' | 'video'
): Promise<number> {
  return new Promise((resolve, reject) => {
    const media = document.createElement(kind)
    const url = URL.createObjectURL(file)

    const cleanup = () => {
      media.removeAttribute('src')
      media.load()
      URL.revokeObjectURL(url)
    }

    media.preload = 'metadata'
    media.onloadedmetadata = () => {
      const duration = media.duration
      cleanup()
      if (Number.isFinite(duration) && duration > 0) {
        resolve(duration)
        return
      }
      reject(new Error('invalid media duration'))
    }
    media.onerror = () => {
      cleanup()
      reject(new Error('failed to read media duration'))
    }
    media.src = url
    media.load()
  })
}

async function resolveImageReferences(
  files: PromptInputSubmittedFile[]
): Promise<ResolvedImageReferences> {
  const images = await Promise.all(
    files.map(async (file) => {
      const url = file.url?.trim() || ''
      if (!url) return null

      const requestUrl =
        isDataReferenceUrl(url) || isProbablyPublicReferenceUrl(url)
          ? url
          : await resolveInlineReferenceURL(url, file.sourceFile)

      const reference: SeedanceReference = {
        kind: 'image',
        url,
        filename: file.filename,
        media_type: file.mediaType,
      }
      return {
        displayReference: reference,
        requestUrl,
      }
    })
  )

  const references = images.filter(
    (
      image
    ): image is {
      displayReference: SeedanceReference
      requestUrl: string
    } => image !== null
  )

  return {
    displayReferences: references.map(
      (reference) => reference.displayReference
    ),
    requestUrls: references.map((reference) => reference.requestUrl),
  }
}

async function fetchReferenceURLAsDataURL(url: string): Promise<string> {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED)
  }
  return blobToDataURL(await response.blob())
}

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        resolve(reader.result)
      } else {
        reject(new Error('failed to read file as base64'))
      }
    }
    reader.onerror = () => reject(new Error('failed to read file'))
    reader.readAsDataURL(blob)
  })
}

function readFileAsDataURL(file: File): Promise<string> {
  return blobToDataURL(file)
}
