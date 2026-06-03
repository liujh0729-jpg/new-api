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
import { useCallback, useEffect, useState } from 'react'
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
import { PlaygroundInput } from './components/playground-input'
import {
  DEFAULT_GROUP,
  ERROR_MESSAGES,
  SEEDANCE_REFERENCE_LIMITS,
  isSeedance20VideoModel,
  normalizeImageSizeForModel,
} from './constants'
import { usePlaygroundState, useChatHandler } from './hooks'
import {
  createUserMessage,
  createLoadingAssistantMessage,
  normalizePlaygroundError,
} from './lib'
import type {
  Message as MessageType,
  SeedanceReference,
  SeedanceReferenceKind,
} from './types'

export function Playground() {
  const { t } = useTranslation()
  const {
    config,
    parameterEnabled,
    messages,
    models,
    groups,
    updateMessages,
    setModels,
    setGroups,
    updateConfig,
  } = usePlaygroundState()

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

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

  const handleSendMessage = async (message: PromptInputMessage) => {
    const text = message.text?.trim() || ''
    let seedanceReferences: SeedanceReference[] = []

    if (config.mode === 'video') {
      const referenceCandidates = buildSeedanceReferenceCandidates(
        message.files || []
      )
      const validationError = validateSeedanceVideoInput(
        text,
        referenceCandidates,
        message.files?.length || 0,
        config.model,
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

      const hasLocalReferences = referenceCandidates.some(
        (reference) => reference.sourceFile
      )
      if (hasLocalReferences) {
        setIsUploadingReferences(true)
      }
      try {
        seedanceReferences =
          await resolveSeedanceReferenceURLs(referenceCandidates)
      } catch (error) {
        const uploadError = normalizePlaygroundError(error, t)
        toast.error(uploadError.message)
        throw new Error(uploadError.message)
      } finally {
        if (hasLocalReferences) {
          setIsUploadingReferences(false)
        }
      }
    }

    const userMessage = createUserMessage(text, seedanceReferences)
    const assistantMessage = createLoadingAssistantMessage()

    const newMessages = [...messages, userMessage, assistantMessage]
    updateMessages(newMessages)

    // Send chat request
    sendChat(newMessages)
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
    const loadingMessage = createLoadingAssistantMessage()
    const newMessages = [...messagesUpToHere, loadingMessage]

    updateMessages(newMessages)
    sendChat(newMessages)
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

      const toSubmit = [
        ...updated.slice(0, index + 1),
        createLoadingAssistantMessage(),
      ]
      updateMessages(toSubmit)
      sendChat(toSubmit)
    },
    [editingMessageKey, messages, updateMessages, sendChat]
  )

  const handleDeleteMessage = (message: MessageType) => {
    const newMessages = messages.filter((m) => m.key !== message.key)
    updateMessages(newMessages)
  }

  return (
    <div className='relative flex size-full flex-col overflow-hidden'>
      {/* Full-width scroll container: scrolling works even over side whitespace */}
      <div className='flex flex-1 flex-col overflow-hidden'>
        <PlaygroundChat
          messages={messages}
          onCopyMessage={handleCopyMessage}
          onRegenerateMessage={handleRegenerateMessage}
          onEditMessage={handleEditMessage}
          onDeleteMessage={handleDeleteMessage}
          isGenerating={isGenerating}
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
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          mode={config.mode}
          modelValue={config.model}
          models={models}
          onGroupChange={(value) => updateConfig('group', value)}
          onImageCountChange={(value) => updateConfig('image_count', value)}
          onImageQualityChange={(value) => updateConfig('image_quality', value)}
          onImageSizeChange={(value) => updateConfig('image_size', value)}
          onModeChange={(value) => updateConfig('mode', value)}
          onModelChange={(value) => updateConfig('model', value)}
          onStop={stopGeneration}
          onSubmit={handleSendMessage}
          onVideoDurationChange={(value) =>
            updateConfig('video_duration', value)
          }
          onVideoRatioChange={(value) => updateConfig('video_ratio', value)}
          videoDuration={config.video_duration}
          videoRatio={config.video_ratio}
        />
      </div>
    </div>
  )
}

type SeedanceReferenceCandidate = SeedanceReference & { sourceFile?: File }

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
        sourceFile: file.sourceFile,
      },
    ]
  })
}

async function resolveSeedanceReferenceURLs(
  references: SeedanceReferenceCandidate[]
): Promise<SeedanceReference[]> {
  const resolvedReferences = await Promise.all(
    references.map(async ({ sourceFile, ...reference }) => {
      if (!sourceFile) return reference

      const uploaded = await uploadReferenceMedia(sourceFile)
      return {
        ...reference,
        url: uploaded.url,
        filename: uploaded.filename || reference.filename,
        media_type: uploaded.media_type || reference.media_type,
      }
    })
  )

  if (resolvedReferences.some((reference) => !isWebUrl(reference.url))) {
    throw new Error(ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED)
  }

  return resolvedReferences
}

function isWebUrl(url: string): boolean {
  return /^https?:\/\//i.test(url.trim())
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

function validateSeedanceVideoInput(
  text: string,
  references: SeedanceReference[],
  rawFileCount: number,
  model: string,
  t: (key: string) => string
): string | null {
  if (!isSeedance20VideoModel(model)) {
    return t('Seedance video mode requires a Seedance 2.0 model')
  }
  if (!text && references.length === 0) {
    return t('Add text or reference media before generating')
  }
  if (rawFileCount !== references.length) {
    return t('Only image, video, and audio references are supported')
  }

  const imageCount = references.filter((item) => item.kind === 'image').length
  const videoCount = references.filter((item) => item.kind === 'video').length
  const audioCount = references.filter((item) => item.kind === 'audio').length

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
