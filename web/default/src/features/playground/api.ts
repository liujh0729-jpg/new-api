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
import { api } from '@/lib/api'
import { API_ENDPOINTS, ERROR_MESSAGES } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ImageGenerationRequest,
  ImageGenerationResponse,
  VideoGenerationRequest,
  VideoGenerationResponse,
  TaskFetchResponse,
  ModelOption,
  GroupOption,
  PlaygroundMode,
} from './types'

export interface ReferenceMediaUploadResult {
  url: string
  filename?: string
  media_type?: string
}

function isWebUrl(url: string): boolean {
  return /^https?:\/\//i.test(url.trim())
}

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Send image generation request
 */
export async function sendImageGeneration(
  payload: ImageGenerationRequest
): Promise<ImageGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_GENERATIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Fetch image generation task status
 */
export async function getImageGenerationTask(
  taskId: string
): Promise<TaskFetchResponse> {
  const res = await api.get(
    `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}`,
    {
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Send video generation request
 */
export async function sendVideoGeneration(
  payload: VideoGenerationRequest
): Promise<VideoGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.VIDEO_GENERATIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Fetch video generation task status
 */
export async function getVideoGenerationTask(
  taskId: string
): Promise<TaskFetchResponse> {
  const res = await api.get(
    `${API_ENDPOINTS.VIDEO_GENERATIONS}/${encodeURIComponent(taskId)}`,
    {
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Upload local reference media and return a web URL.
 */
export async function uploadReferenceMedia(
  file: File
): Promise<ReferenceMediaUploadResult> {
  const formData = new FormData()
  formData.append('file', file)

  const res = await api.post(API_ENDPOINTS.REFERENCE_MEDIA_UPLOAD, formData, {
    skipErrorHandler: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  const responseData = res.data?.data ?? res.data
  const url =
    typeof responseData?.url === 'string' ? responseData.url.trim() : ''
  if (!url) {
    throw new Error('Reference media upload response URL is empty')
  }
  if (!isWebUrl(url)) {
    throw new Error(ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED)
  }

  return {
    url,
    filename:
      typeof responseData?.filename === 'string'
        ? responseData.filename
        : undefined,
    media_type:
      typeof responseData?.media_type === 'string'
        ? responseData.media_type
        : undefined,
  }
}

/**
 * Get user available models
 */
export async function getUserModels(
  mode: PlaygroundMode = 'chat'
): Promise<ModelOption[]> {
  const endpointType =
    mode === 'image'
      ? 'image-generation'
      : mode === 'video'
        ? 'openai-video'
        : 'openai'
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: {
      endpoint_type: endpointType,
      ...(mode === 'video' ? { details: true } : {}),
    },
    skipErrorHandler: true,
  } as Record<string, unknown>)
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.flatMap((item: unknown) => {
    if (typeof item === 'string') {
      return [{ label: item, value: item }]
    }
    if (!item || typeof item !== 'object') return []
    const detail = item as {
      model?: unknown
      video_resolutions?: unknown
    }
    if (typeof detail.model !== 'string') return []
    return [
      {
        label: detail.model,
        value: detail.model,
        ...(Array.isArray(detail.video_resolutions)
          ? {
              video_resolutions: detail.video_resolutions.filter(
                (resolution): resolution is string =>
                  typeof resolution === 'string'
              ),
            }
          : {}),
      },
    ]
  })
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}
