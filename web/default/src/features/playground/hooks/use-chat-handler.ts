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
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
  TaskFetchResponse,
} from '../types'
import { useStreamRequest } from './use-stream-request'

interface UseChatHandlerOptions {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
}

const IMAGE_TASK_POLL_INTERVAL_MS = 2000
const IMAGE_TASK_INITIAL_POLL_DELAY_MS = 1000
const IMAGE_TASK_POLL_TIMEOUT_MS = 20 * 60 * 1000
const VIDEO_TASK_POLL_INTERVAL_MS = 3000
const VIDEO_TASK_INITIAL_POLL_DELAY_MS = 1500
const VIDEO_TASK_POLL_TIMEOUT_MS = 30 * 60 * 1000

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as { response?: { status?: unknown } }).response
  return typeof response?.status === 'number' ? response.status : undefined
}

function isTransientPollingError(error: unknown): boolean {
  const status = getHttpStatus(error)
  return (
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
  const imageAbortRef = useRef(false)
  const videoAbortRef = useRef(false)

  const completeWithImages = useCallback(
    (images: GeneratedImage[]) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => ({
          ...updateCurrentVersionContent(message, t('Generated image')),
          images,
          activity: undefined,
          status: MESSAGE_STATUS.COMPLETE,
          isReasoningStreaming: false,
        }))
      )
    },
    [onMessageUpdate, t]
  )

  const markImageGenerationLoading = useCallback(() => {
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) => ({
        ...updateCurrentVersionContent(message, ''),
        images: undefined,
        activity: 'image_generation',
        status: MESSAGE_STATUS.STREAMING,
        isReasoningStreaming: false,
      }))
    )
  }, [onMessageUpdate])

  const completeWithVideos = useCallback(
    (videos: GeneratedVideo[]) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => ({
          ...updateCurrentVersionContent(message, t('Generated video')),
          videos,
          activity: undefined,
          status: MESSAGE_STATUS.COMPLETE,
          isReasoningStreaming: false,
        }))
      )
    },
    [onMessageUpdate, t]
  )

  const markVideoGenerationLoading = useCallback(() => {
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) => ({
        ...updateCurrentVersionContent(message, ''),
        videos: undefined,
        activity: 'video_generation',
        status: MESSAGE_STATUS.STREAMING,
        isReasoningStreaming: false,
      }))
    )
  }, [onMessageUpdate])

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
    async (taskId: string) => {
      const deadline = Date.now() + IMAGE_TASK_POLL_TIMEOUT_MS
      let attempt = 0

      while (Date.now() < deadline) {
        if (imageAbortRef.current) return

        await new Promise((resolve) =>
          setTimeout(
            resolve,
            attempt === 0
              ? IMAGE_TASK_INITIAL_POLL_DELAY_MS
              : IMAGE_TASK_POLL_INTERVAL_MS
          )
        )
        attempt += 1
        if (imageAbortRef.current) return

        const response = await getImageGenerationTask(taskId)
        const task = parseImageTaskResponse(response)
        const status = task.status || 'processing'
        markImageGenerationLoading()

        if (status === 'failed') {
          throw new Error(task.error || ERROR_MESSAGES.IMAGE_TASK_FAILED)
        }

        if (status === 'succeeded') {
          if (task.images.length > 0) {
            completeWithImages(task.images)
            return
          }
          throw new Error(ERROR_MESSAGES.PARSE_ERROR)
        }
      }

      throw new Error(ERROR_MESSAGES.IMAGE_TASK_TIMEOUT)
    },
    [completeWithImages, markImageGenerationLoading]
  )

  const pollVideoTask = useCallback(
    async (taskId: string) => {
      const deadline = Date.now() + VIDEO_TASK_POLL_TIMEOUT_MS
      let attempt = 0

      while (Date.now() < deadline) {
        if (videoAbortRef.current) return

        await new Promise((resolve) =>
          setTimeout(
            resolve,
            attempt === 0
              ? VIDEO_TASK_INITIAL_POLL_DELAY_MS
              : VIDEO_TASK_POLL_INTERVAL_MS
          )
        )
        attempt += 1
        if (videoAbortRef.current) return

        let response: TaskFetchResponse
        try {
          response = await getVideoGenerationTask(taskId)
        } catch (error: unknown) {
          if (isTransientPollingError(error)) {
            markVideoGenerationLoading()
            continue
          }
          throw error
        }
        const task = parseVideoTaskResponse(response)
        const status = task.status || 'processing'
        markVideoGenerationLoading()

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
          completeWithVideos(videos)
          return
        }
      }

      throw new Error(ERROR_MESSAGES.VIDEO_TASK_TIMEOUT)
    },
    [completeWithVideos, markVideoGenerationLoading]
  )

  const sendImageChat = useCallback(
    async (messages: Message[]) => {
      const prompt = [...messages]
        .reverse()
        .find((message) => message.from === 'user')
        ?.versions?.[0]?.content?.trim()

      if (!prompt) {
        handleStreamError(ERROR_MESSAGES.API_REQUEST_ERROR)
        return
      }

      imageAbortRef.current = false
      setIsGenerating(true)
      markImageGenerationLoading()

      try {
        const response = await sendImageGeneration(
          buildImageGenerationPayload(prompt, config)
        )
        const images = extractImageResults(response)
        if (images.length > 0) {
          completeWithImages(images)
          return
        }

        if (isImageTaskResponse(response)) {
          const taskId = response.task_id || response.id
          if (!taskId) {
            throw new Error(ERROR_MESSAGES.PARSE_ERROR)
          }
          markImageGenerationLoading()
          await pollImageTask(taskId)
          return
        }

        throw new Error(ERROR_MESSAGES.PARSE_ERROR)
      } catch (error: unknown) {
        if (imageAbortRef.current) return
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
    async (messages: Message[]) => {
      const userMessage = [...messages]
        .reverse()
        .find((message) => message.from === 'user')
      const prompt = userMessage?.versions?.[0]?.content?.trim() || ''
      const references = userMessage?.seedanceReferences || []

      if (!prompt && references.length === 0) {
        handleStreamError(ERROR_MESSAGES.API_REQUEST_ERROR)
        return
      }

      videoAbortRef.current = false
      setIsGenerating(true)
      markVideoGenerationLoading()

      try {
        const response = await sendVideoGeneration(
          buildVideoGenerationPayload(prompt, references, config)
        )
        const videos = extractVideoResults(response)
        if (videos.length > 0) {
          completeWithVideos(videos)
          return
        }

        if (isVideoTaskResponse(response)) {
          const taskId = response.task_id || response.id
          if (!taskId) {
            throw new Error(ERROR_MESSAGES.PARSE_ERROR)
          }
          markVideoGenerationLoading()
          await pollVideoTask(taskId)
          return
        }

        throw new Error(ERROR_MESSAGES.PARSE_ERROR)
      } catch (error: unknown) {
        if (videoAbortRef.current) return
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
    (messages: Message[]) => {
      if (config.mode === 'image') {
        void sendImageChat(messages)
        return
      }
      if (config.mode === 'video') {
        void sendVideoChat(messages)
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
    imageAbortRef.current = true
    videoAbortRef.current = true
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

  return {
    sendChat,
    stopGeneration,
    isGenerating,
  }
}
