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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  createGeneratedMaterial,
  uploadMaterial,
} from '@/features/materials/api'
import {
  MATERIAL_SOURCE_TYPE,
  MATERIAL_TYPE,
} from '@/features/materials/constants'
import {
  getImageGenerationTask,
  getVideoGenerationTask,
  sendChatCompletion,
  sendImageGeneration,
  sendVideoGeneration,
} from '../api'
import { MESSAGE_STATUS, ERROR_MESSAGES } from '../constants'
import {
  buildChatCompletionPayload,
  buildImageGenerationPayload,
  buildVideoGenerationPayload,
  extractImageResults,
  extractVideoResults,
  isImageTaskResponse,
  isVideoTaskResponse,
  parseImageTaskResponse,
  parseVideoTaskResponse,
  normalizePlaygroundError,
  updateAssistantMessageWithError,
  updateLastAssistantMessage,
  updateCurrentVersionContent,
  processStreamingContent,
  finalizeMessage,
} from '../lib'
import type {
  GeneratedImage,
  GeneratedVideo,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  SeedanceReference,
  TaskFetchResponse,
} from '../types'
import { useStreamRequest } from './use-stream-request'

interface UseChatHandlerOptions {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
}

interface SendChatOptions {
  imageReferences?: string[]
  videoReferences?: SeedanceReference[]
  clientTaskId?: string
}

const IMAGE_TASK_POLL_INTERVAL_MS = 2000
const IMAGE_TASK_INITIAL_POLL_DELAY_MS = 1000
const IMAGE_TASK_POLL_TIMEOUT_MS = 20 * 60 * 1000
const VIDEO_TASK_POLL_INTERVAL_MS = 3000
const VIDEO_TASK_INITIAL_POLL_DELAY_MS = 1500
const VIDEO_TASK_POLL_TIMEOUT_MS = 30 * 60 * 1000
type TaskType = 'image' | 'video'

const taskAbortTokens: Record<TaskType, number> = {
  image: 0,
  video: 0,
}
const activeTaskPolls = new Map<string, Promise<void>>()

function isPersistableGeneratedUrl(url?: string): url is string {
  if (!url) return false
  const value = url.trim().toLowerCase()
  return !!value && !value.startsWith('data:') && !value.startsWith('blob:')
}

function getLastAssistantTaskId(messages: Message[]): string | undefined {
  return [...messages].reverse().find((message) => message.from === 'assistant')
    ?.taskId
}

function shouldIgnoreTaskLoadingUpdate(
  message: Message,
  taskType: TaskType,
  taskId?: string
): boolean {
  if (
    message.status === MESSAGE_STATUS.COMPLETE ||
    message.status === MESSAGE_STATUS.ERROR
  ) {
    return true
  }
  if (message.taskType && message.taskType !== taskType) {
    return true
  }
  return !!taskId && !!message.taskId && message.taskId !== taskId
}

function shouldIgnoreTaskCompleteUpdate(
  message: Message,
  taskType: TaskType,
  taskId?: string
): boolean {
  if (message.taskType && message.taskType !== taskType) {
    return true
  }
  return !!taskId && !!message.taskId && message.taskId !== taskId
}

function updateTaskAssistantMessage(
  messages: Message[],
  taskType: TaskType,
  taskId: string | undefined,
  updater: (message: Message) => Message
): Message[] {
  if (!taskId) {
    return updateLastAssistantMessage(messages, updater)
  }

  const index = [...messages]
    .reverse()
    .findIndex(
      (message) =>
        message.from === 'assistant' &&
        message.taskId === taskId &&
        (!message.taskType || message.taskType === taskType)
    )
  if (index === -1) return messages

  const targetIndex = messages.length - 1 - index
  const updated = [...messages]
  updated[targetIndex] = updater(messages[targetIndex]!)
  return updated
}

function extensionForMime(mimeType: string, fallback: string): string {
  const normalized = mimeType.toLowerCase()
  if (normalized.includes('jpeg')) return '.jpg'
  if (normalized.includes('png')) return '.png'
  if (normalized.includes('webp')) return '.webp'
  if (normalized.includes('gif')) return '.gif'
  if (normalized.includes('mp4')) return '.mp4'
  if (normalized.includes('webm')) return '.webm'
  if (normalized.includes('quicktime')) return '.mov'
  return fallback
}

function generatedOutputFileName(
  prefix: string,
  timestamp: number,
  index: number,
  mimeType: string | undefined,
  fallback: string
): string | undefined {
  if (!mimeType) return undefined
  return `${prefix}-${timestamp}-${index + 1}${extensionForMime(mimeType, fallback)}`
}

function base64ToFile(
  base64: string,
  mimeType: string,
  fileName: string
): File {
  const binary = window.atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }

  const buffer = new ArrayBuffer(bytes.byteLength)
  new Uint8Array(buffer).set(bytes)
  return new File([buffer], fileName, { type: mimeType })
}

function logGeneratedMaterialFailures(
  results: PromiseSettledResult<unknown>[]
) {
  for (const result of results) {
    if (result.status === 'rejected') {
      // eslint-disable-next-line no-console
      console.warn(
        'Failed to save generated output to material library:',
        result.reason
      )
    }
  }
}

function getTaskAbortToken(taskType: TaskType): number {
  return taskAbortTokens[taskType]
}

function abortTaskGeneration(taskType: TaskType): void {
  taskAbortTokens[taskType] += 1
}

function isTaskAbortRequested(taskType: TaskType, token: number): boolean {
  return taskAbortTokens[taskType] !== token
}

function runExclusiveTaskPoll(
  key: string,
  poller: () => Promise<void>
): Promise<void> {
  const activePoll = activeTaskPolls.get(key)
  if (activePoll) return activePoll

  const poll = poller().finally(() => {
    if (activeTaskPolls.get(key) === poll) {
      activeTaskPolls.delete(key)
    }
  })

  activeTaskPolls.set(key, poll)
  return poll
}

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as { response?: { status?: unknown } }).response
  return typeof response?.status === 'number' ? response.status : undefined
}

function getErrorResponseData(error: unknown): unknown {
  if (typeof error !== 'object' || error === null) return undefined
  return (error as { response?: { data?: unknown } }).response?.data
}

function collectErrorText(value: unknown): string {
  if (typeof value === 'string') return value
  if (typeof value !== 'object' || value === null) return ''
  const fields = ['code', 'message', 'reason', 'detail', 'error']
  return fields
    .map((field) => collectErrorText((value as Record<string, unknown>)[field]))
    .filter(Boolean)
    .join(' ')
}

function isTaskLookupPendingError(error: unknown): boolean {
  const status = getHttpStatus(error)
  if (status !== 400 && status !== 404) return false

  const text = collectErrorText(getErrorResponseData(error)).toLowerCase()
  return (
    text.includes('task_not_exist') ||
    text.includes('task not exist') ||
    text.includes('task does not exist') ||
    text.includes('task not found') ||
    text.includes('not found') ||
    text.includes('不存在')
  )
}

function isTransientPollingError(error: unknown): boolean {
  const status = getHttpStatus(error)
  return (
    isTaskLookupPendingError(error) ||
    status === 429 ||
    (status !== undefined && status >= 500 && status < 600)
  )
}

/**
 * Hook for handling chat message sending and receiving
 */
export function useChatHandler({
  config,
  parameterEnabled,
  onMessageUpdate,
}: UseChatHandlerOptions) {
  const { t } = useTranslation()
  const { sendStreamRequest, stopStream } = useStreamRequest()
  const [isGenerating, setIsGenerating] = useState(false)

  const saveGeneratedImagesToMaterials = useCallback(
    (images: GeneratedImage[]) => {
      const now = Date.now()
      const jobs: Promise<unknown>[] = []

      images.forEach((image, index) => {
        try {
          if (isPersistableGeneratedUrl(image.url)) {
            jobs.push(
              createGeneratedMaterial({
                name: image.revised_prompt || t('Generated image'),
                type: MATERIAL_TYPE.IMAGE,
                mime_type: image.mime_type,
                file_name: generatedOutputFileName(
                  'generated-image',
                  now,
                  index,
                  image.mime_type,
                  '.png'
                ),
                url: image.url,
              })
            )
            return
          }
          if (image.b64_json) {
            const mimeType = image.mime_type || 'image/png'
            const fileName = `generated-image-${now}-${index + 1}${extensionForMime(mimeType, '.png')}`
            const file = base64ToFile(image.b64_json, mimeType, fileName)
            jobs.push(uploadMaterial(file, MATERIAL_SOURCE_TYPE.AI_OUTPUT))
          }
        } catch (error) {
          jobs.push(Promise.reject(error))
        }
      })

      if (jobs.length > 0) {
        void Promise.allSettled(jobs).then(logGeneratedMaterialFailures)
      }
    },
    [t]
  )

  const saveGeneratedVideosToMaterials = useCallback(
    (videos: GeneratedVideo[]) => {
      const now = Date.now()
      const jobs = videos
        .filter((video) => isPersistableGeneratedUrl(video.url))
        .map((video, index) => {
          return createGeneratedMaterial({
            name: video.task_id || t('Generated video'),
            type: MATERIAL_TYPE.VIDEO,
            mime_type: video.mime_type,
            file_name: generatedOutputFileName(
              'generated-video',
              now,
              index,
              video.mime_type,
              '.mp4'
            ),
            url: video.url,
          })
        })

      if (jobs.length > 0) {
        void Promise.allSettled(jobs).then(logGeneratedMaterialFailures)
      }
    },
    [t]
  )

  const completeWithImages = useCallback(
    (images: GeneratedImage[], taskId?: string) => {
      saveGeneratedImagesToMaterials(images)
      onMessageUpdate((prev) =>
        updateTaskAssistantMessage(prev, 'image', taskId, (message) => {
          if (shouldIgnoreTaskCompleteUpdate(message, 'image', taskId)) {
            return message
          }
          return {
            ...updateCurrentVersionContent(message, t('Generated image')),
            images,
            activity: undefined,
            status: MESSAGE_STATUS.COMPLETE,
            isReasoningStreaming: false,
          }
        })
      )
    },
    [onMessageUpdate, saveGeneratedImagesToMaterials, t]
  )

  const markImageGenerationLoading = useCallback(
    (taskId?: string) => {
      onMessageUpdate((prev) =>
        updateTaskAssistantMessage(prev, 'image', taskId, (message) => {
          if (shouldIgnoreTaskLoadingUpdate(message, 'image', taskId)) {
            return message
          }
          return {
            ...updateCurrentVersionContent(message, ''),
            images: undefined,
            activity: 'image_generation',
            status: MESSAGE_STATUS.STREAMING,
            isReasoningStreaming: false,
            taskId: taskId || message.taskId,
            taskType: 'image',
          }
        })
      )
    },
    [onMessageUpdate]
  )

  const completeWithVideos = useCallback(
    (videos: GeneratedVideo[], taskId?: string) => {
      saveGeneratedVideosToMaterials(videos)
      onMessageUpdate((prev) =>
        updateTaskAssistantMessage(prev, 'video', taskId, (message) => {
          if (shouldIgnoreTaskCompleteUpdate(message, 'video', taskId)) {
            return message
          }
          return {
            ...updateCurrentVersionContent(message, t('Generated video')),
            videos,
            activity: undefined,
            status: MESSAGE_STATUS.COMPLETE,
            isReasoningStreaming: false,
          }
        })
      )
    },
    [onMessageUpdate, saveGeneratedVideosToMaterials, t]
  )

  const markVideoGenerationLoading = useCallback(
    (taskId?: string) => {
      onMessageUpdate((prev) =>
        updateTaskAssistantMessage(prev, 'video', taskId, (message) => {
          if (shouldIgnoreTaskLoadingUpdate(message, 'video', taskId)) {
            return message
          }
          return {
            ...updateCurrentVersionContent(message, ''),
            videos: undefined,
            activity: 'video_generation',
            status: MESSAGE_STATUS.STREAMING,
            isReasoningStreaming: false,
            taskId: taskId || message.taskId,
            taskType: 'video',
          }
        })
      )
    },
    [onMessageUpdate]
  )

  // Handle stream update
  const handleStreamUpdate = useCallback(
    (type: 'reasoning' | 'content', chunk: string) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => {
          if (message.status === MESSAGE_STATUS.ERROR) return message

          if (type === 'reasoning') {
            // Direct API reasoning_content
            return {
              ...message,
              reasoning: {
                content: (message.reasoning?.content || '') + chunk,
                duration: 0,
              },
              isReasoningStreaming: true,
              status: MESSAGE_STATUS.STREAMING,
            }
          }

          // Content streaming: handle <think> tags
          return {
            ...processStreamingContent(message, chunk),
            status: MESSAGE_STATUS.STREAMING,
          }
        })
      )
    },
    [onMessageUpdate]
  )

  // Handle stream complete
  const handleStreamComplete = useCallback(() => {
    setIsGenerating(false)
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.COMPLETE ||
        message.status === MESSAGE_STATUS.ERROR
          ? message
          : { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
      )
    )
  }, [onMessageUpdate])

  // Handle stream error
  const handleStreamError = useCallback(
    (error: unknown, errorCode?: string) => {
      const normalizedError = normalizePlaygroundError(
        errorCode ? { message: error, code: errorCode } : error,
        t
      )
      setIsGenerating(false)
      toast.error(normalizedError.message)
      onMessageUpdate((prev) =>
        updateAssistantMessageWithError(
          prev,
          normalizedError.message,
          normalizedError.code
        )
      )
    },
    [onMessageUpdate, t]
  )

  // Send streaming chat request
  const sendStreamingChat = useCallback(
    (messages: Message[]) => {
      setIsGenerating(true)
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )
      sendStreamRequest(
        payload,
        handleStreamUpdate,
        handleStreamComplete,
        handleStreamError
      )
    },
    [
      config,
      parameterEnabled,
      sendStreamRequest,
      handleStreamUpdate,
      handleStreamComplete,
      handleStreamError,
    ]
  )

  // Send non-streaming chat request
  const sendNonStreamingChat = useCallback(
    async (messages: Message[]) => {
      setIsGenerating(true)
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )

      try {
        const response = await sendChatCompletion(payload)
        const choice = response.choices?.[0]
        if (!choice) return

        onMessageUpdate((prev) =>
          updateLastAssistantMessage(prev, (message) => ({
            ...finalizeMessage(
              {
                ...message,
                versions: [
                  {
                    ...message.versions[0],
                    content: choice.message?.content || '',
                  },
                ],
              },
              choice.message?.reasoning_content
            ),
            status: MESSAGE_STATUS.COMPLETE,
          }))
        )
      } catch (error: unknown) {
        handleStreamError(error)
      } finally {
        setIsGenerating(false)
      }
    },
    [config, parameterEnabled, onMessageUpdate, handleStreamError]
  )

  const pollImageTask = useCallback(
    async (taskId: string, abortToken = getTaskAbortToken('image')) => {
      return runExclusiveTaskPoll(`image:${taskId}`, async () => {
        const deadline = Date.now() + IMAGE_TASK_POLL_TIMEOUT_MS
        let attempt = 0

        while (Date.now() < deadline) {
          if (isTaskAbortRequested('image', abortToken)) return

          await new Promise((resolve) =>
            setTimeout(
              resolve,
              attempt === 0
                ? IMAGE_TASK_INITIAL_POLL_DELAY_MS
                : IMAGE_TASK_POLL_INTERVAL_MS
            )
          )
          attempt += 1
          if (isTaskAbortRequested('image', abortToken)) return

          let response: TaskFetchResponse
          try {
            response = await getImageGenerationTask(taskId)
          } catch (error: unknown) {
            if (isTaskAbortRequested('image', abortToken)) return
            if (isTransientPollingError(error)) {
              markImageGenerationLoading(taskId)
              continue
            }
            throw error
          }
          if (isTaskAbortRequested('image', abortToken)) return

          const task = parseImageTaskResponse(response)
          const status = task.status || 'processing'
          markImageGenerationLoading(taskId)

          if (status === 'failed') {
            throw new Error(task.error || ERROR_MESSAGES.IMAGE_TASK_FAILED)
          }

          if (status === 'succeeded') {
            if (task.images.length > 0) {
              completeWithImages(task.images, taskId)
              return
            }
            throw new Error(ERROR_MESSAGES.PARSE_ERROR)
          }
        }

        throw new Error(ERROR_MESSAGES.IMAGE_TASK_TIMEOUT)
      })
    },
    [completeWithImages, markImageGenerationLoading]
  )

  const pollVideoTask = useCallback(
    async (taskId: string, abortToken = getTaskAbortToken('video')) => {
      return runExclusiveTaskPoll(`video:${taskId}`, async () => {
        const deadline = Date.now() + VIDEO_TASK_POLL_TIMEOUT_MS
        let attempt = 0

        while (Date.now() < deadline) {
          if (isTaskAbortRequested('video', abortToken)) return

          await new Promise((resolve) =>
            setTimeout(
              resolve,
              attempt === 0
                ? VIDEO_TASK_INITIAL_POLL_DELAY_MS
                : VIDEO_TASK_POLL_INTERVAL_MS
            )
          )
          attempt += 1
          if (isTaskAbortRequested('video', abortToken)) return

          let response: TaskFetchResponse
          try {
            response = await getVideoGenerationTask(taskId)
          } catch (error: unknown) {
            if (isTaskAbortRequested('video', abortToken)) return
            if (isTransientPollingError(error)) {
              markVideoGenerationLoading(taskId)
              continue
            }
            throw error
          }
          if (isTaskAbortRequested('video', abortToken)) return

          const task = parseVideoTaskResponse(response)
          const status = task.status || 'processing'
          markVideoGenerationLoading(taskId)

          if (status === 'failed') {
            throw new Error(task.error || ERROR_MESSAGES.VIDEO_TASK_FAILED)
          }

          if (status === 'succeeded') {
            const resolvedTaskId = task.taskId || taskId
            const proxyUrl = `/v1/videos/${encodeURIComponent(resolvedTaskId)}/content`
            const videos =
              task.videos.length > 0
                ? task.videos.map((video) => ({
                    ...video,
                    url: video.url?.trim() || proxyUrl,
                    task_id: video.task_id || resolvedTaskId,
                  }))
                : [{ url: proxyUrl, task_id: resolvedTaskId }]
            completeWithVideos(videos, taskId)
            return
          }
        }

        throw new Error(ERROR_MESSAGES.VIDEO_TASK_TIMEOUT)
      })
    },
    [completeWithVideos, markVideoGenerationLoading]
  )

  const sendImageChat = useCallback(
    async (
      messages: Message[],
      imageReferences?: string[],
      clientTaskId?: string
    ) => {
      const userMessage = [...messages]
        .reverse()
        .find((message) => message.from === 'user')
      const prompt = userMessage?.versions?.[0]?.content?.trim() || ''
      const references =
        imageReferences ??
        userMessage?.seedanceReferences
          ?.filter((reference) => reference.kind === 'image')
          .map((reference) => reference.url) ??
        []

      if (!prompt && references.length === 0) {
        handleStreamError(ERROR_MESSAGES.API_REQUEST_ERROR)
        return
      }

      const abortToken = getTaskAbortToken('image')
      setIsGenerating(true)
      const requestTaskId = clientTaskId || getLastAssistantTaskId(messages)
      markImageGenerationLoading(requestTaskId)

      try {
        const response = await sendImageGeneration(
          buildImageGenerationPayload(prompt, config, references, requestTaskId)
        )
        if (isTaskAbortRequested('image', abortToken)) return

        const images = extractImageResults(response)
        if (images.length > 0) {
          completeWithImages(images, requestTaskId)
          return
        }

        if (isImageTaskResponse(response)) {
          const taskId = response.task_id || response.id || requestTaskId
          if (!taskId) {
            throw new Error(ERROR_MESSAGES.PARSE_ERROR)
          }
          markImageGenerationLoading(taskId)
          await pollImageTask(taskId, abortToken)
          return
        }

        throw new Error(ERROR_MESSAGES.PARSE_ERROR)
      } catch (error: unknown) {
        if (isTaskAbortRequested('image', abortToken)) return
        handleStreamError(error)
      } finally {
        setIsGenerating(false)
      }
    },
    [
      config,
      completeWithImages,
      handleStreamError,
      markImageGenerationLoading,
      pollImageTask,
    ]
  )

  const sendVideoChat = useCallback(
    async (
      messages: Message[],
      videoReferences?: SeedanceReference[],
      clientTaskId?: string
    ) => {
      const userMessage = [...messages]
        .reverse()
        .find((message) => message.from === 'user')
      const prompt = userMessage?.versions?.[0]?.content?.trim() || ''
      const references =
        videoReferences ?? userMessage?.seedanceReferences ?? []

      if (!prompt && references.length === 0) {
        handleStreamError(ERROR_MESSAGES.API_REQUEST_ERROR)
        return
      }

      const abortToken = getTaskAbortToken('video')
      setIsGenerating(true)
      const requestTaskId = clientTaskId || getLastAssistantTaskId(messages)
      markVideoGenerationLoading(requestTaskId)

      try {
        const response = await sendVideoGeneration(
          buildVideoGenerationPayload(prompt, references, config, requestTaskId)
        )
        if (isTaskAbortRequested('video', abortToken)) return

        const videos = extractVideoResults(response)
        if (videos.length > 0) {
          completeWithVideos(videos, requestTaskId)
          return
        }

        if (isVideoTaskResponse(response)) {
          const taskId = response.task_id || response.id || requestTaskId
          if (!taskId) {
            throw new Error(ERROR_MESSAGES.PARSE_ERROR)
          }
          markVideoGenerationLoading(taskId)
          await pollVideoTask(taskId, abortToken)
          return
        }

        throw new Error(ERROR_MESSAGES.PARSE_ERROR)
      } catch (error: unknown) {
        if (isTaskAbortRequested('video', abortToken)) return
        handleStreamError(error)
      } finally {
        setIsGenerating(false)
      }
    },
    [
      config,
      completeWithVideos,
      handleStreamError,
      markVideoGenerationLoading,
      pollVideoTask,
    ]
  )

  // Send chat request (stream or non-stream based on config)
  const sendChat = useCallback(
    (messages: Message[], options: SendChatOptions = {}) => {
      if (config.mode === 'image') {
        void sendImageChat(
          messages,
          options.imageReferences,
          options.clientTaskId
        )
        return
      }
      if (config.mode === 'video') {
        void sendVideoChat(
          messages,
          options.videoReferences,
          options.clientTaskId
        )
        return
      }
      if (config.stream) {
        sendStreamingChat(messages)
      } else {
        sendNonStreamingChat(messages)
      }
    },
    [
      config.mode,
      config.stream,
      sendImageChat,
      sendVideoChat,
      sendStreamingChat,
      sendNonStreamingChat,
    ]
  )

  // Stop generation
  const stopGeneration = useCallback(() => {
    abortTaskGeneration('image')
    abortTaskGeneration('video')
    stopStream()
    setIsGenerating(false)
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.LOADING ||
        message.status === MESSAGE_STATUS.STREAMING
          ? { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
          : message
      )
    )
  }, [stopStream, onMessageUpdate])

  const resumeTaskPolling = useCallback(
    (taskId: string, taskType: 'image' | 'video') => {
      const abortToken = getTaskAbortToken(taskType)
      if (taskType === 'image') {
        setIsGenerating(true)
        pollImageTask(taskId, abortToken)
          .catch((error: unknown) => {
            if (!isTaskAbortRequested('image', abortToken)) {
              handleStreamError(error)
            }
          })
          .finally(() => {
            if (!isTaskAbortRequested('image', abortToken)) {
              setIsGenerating(false)
            }
          })
      } else {
        setIsGenerating(true)
        pollVideoTask(taskId, abortToken)
          .catch((error: unknown) => {
            if (!isTaskAbortRequested('video', abortToken)) {
              handleStreamError(error)
            }
          })
          .finally(() => {
            if (!isTaskAbortRequested('video', abortToken)) {
              setIsGenerating(false)
            }
          })
      }
    },
    [pollImageTask, pollVideoTask, handleStreamError]
  )

  return {
    sendChat,
    stopGeneration,
    isGenerating,
    resumeTaskPolling,
  }
}
