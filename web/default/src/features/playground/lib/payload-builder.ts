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
import {
  ERROR_MESSAGES,
  getLTXVideoDimensions,
  isLTX23StartEndModel,
  isLTXVideoModel,
  normalizeImageSizeForModel,
} from '../constants'
import type {
  ChatCompletionRequest,
  ImageGenerationRequest,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  SeedanceReference,
  VideoGenerationRequest,
  VideoGenerationContentItem,
} from '../types'
import { formatMessageForAPI, isValidMessage } from './message-utils'

function isWebUrl(url: string): boolean {
  return /^https?:\/\//i.test(url.trim())
}

function isDataReferenceUrl(url: string): boolean {
  return /^data:(image|video|audio)\//i.test(url.trim())
}

function isValidReferenceUrl(url: string): boolean {
  return isWebUrl(url) || isDataReferenceUrl(url)
}

function getOpenAIVideoSize(ratio: string): string | undefined {
  if (ratio === '16:9') return '1280x720'
  if (ratio === '9:16') return '720x1280'
  return undefined
}

/**
 * Build API request payload from messages and config
 */
export function buildChatCompletionPayload(
  messages: Message[],
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled
): ChatCompletionRequest {
  // Filter and format valid messages
  const processedMessages = messages
    .filter(isValidMessage)
    .map(formatMessageForAPI)

  const payload: ChatCompletionRequest = {
    model: config.model,
    group: config.group,
    messages: processedMessages,
    stream: config.stream,
  }

  // Add enabled parameters
  const parameterKeys: Array<keyof ParameterEnabled> = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ]

  parameterKeys.forEach((key) => {
    if (parameterEnabled[key]) {
      const value = config[key as keyof PlaygroundConfig]
      if (value !== undefined && value !== null) {
        ;(payload as unknown as Record<string, unknown>)[key] = value
      }
    }
  })

  return payload
}

/**
 * Build image generation request payload from prompt and config
 */
export function buildImageGenerationPayload(
  prompt: string,
  config: PlaygroundConfig,
  images?: string[],
  clientTaskId?: string
): ImageGenerationRequest {
  const payload: ImageGenerationRequest = {
    model: config.model,
    group: config.group,
    prompt,
    size: normalizeImageSizeForModel(config.model, config.image_size),
    quality: config.image_quality,
    n: config.image_count,
  }
  if (clientTaskId) {
    payload.client_task_id = clientTaskId
  }

  const imageReferences = images?.filter((image) => image.trim()) || []
  if (imageReferences.length > 0) {
    payload.image = imageReferences[0]
  }
  if (imageReferences.length > 1) {
    payload.images = imageReferences
  }

  return payload
}

/**
 * Build Seedance video generation request payload from prompt, references, and config
 */
export function buildVideoGenerationPayload(
  prompt: string,
  references: SeedanceReference[],
  config: PlaygroundConfig,
  clientTaskId?: string
): VideoGenerationRequest {
  if (references.some((reference) => !isValidReferenceUrl(reference.url))) {
    throw new Error(ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED)
  }

  const sortedReferences = [
    ...references.filter((reference) => reference.kind === 'image'),
    ...references.filter((reference) => reference.kind === 'video'),
    ...references.filter((reference) => reference.kind === 'audio'),
  ]

  const content = sortedReferences
    .map<VideoGenerationContentItem | null>((reference) => {
      if (reference.kind === 'image') {
        return {
          type: 'image_url',
          role: 'reference_image',
          image_url: { url: reference.url },
        }
      }
      if (reference.kind === 'video') {
        return {
          type: 'video_url',
          role: 'reference_video',
          video_url: { url: reference.url },
        }
      }
      if (reference.kind === 'audio') {
        return {
          type: 'audio_url',
          role: 'reference_audio',
          audio_url: { url: reference.url },
        }
      }
      return null
    })
    .filter((item): item is VideoGenerationContentItem => item !== null)

  const isLTXVideo = isLTXVideoModel(config.model)
  const isLTXStartEnd = isLTX23StartEndModel(config.model)
  const size = isLTXVideo ? undefined : getOpenAIVideoSize(config.video_ratio)
  const ltxDimensions =
    isLTXVideo && !isLTXStartEnd
      ? getLTXVideoDimensions(config.video_size)
      : undefined
  const ltxImageReferences = isLTXVideo
    ? references
        .filter((reference) => reference.kind === 'image')
        .map((reference) => reference.url)
    : []
  const ltxAudioReference = isLTXStartEnd
    ? references.find((reference) => reference.kind === 'audio')?.url
    : undefined
  let timelineData: unknown
  if (isLTXStartEnd && config.ltx_timeline_data.trim()) {
    try {
      timelineData = JSON.parse(config.ltx_timeline_data)
    } catch {
      throw new Error(ERROR_MESSAGES.TIMELINE_JSON_INVALID)
    }
  }
  const metadata: VideoGenerationRequest['metadata'] = {
    content,
    ...(ltxDimensions
      ? {
          width: ltxDimensions.width,
          height: ltxDimensions.height,
        }
      : isLTXStartEnd
        ? {}
        : {
            ratio: config.video_ratio,
            resolution: config.video_resolution,
          }),
    ...(ltxAudioReference ? { audio: ltxAudioReference } : {}),
    ...(timelineData !== undefined ? { timeline_data: timelineData } : {}),
  }

  return {
    model: config.model,
    group: config.group,
    prompt,
    ...(clientTaskId ? { client_task_id: clientTaskId } : {}),
    ...(ltxImageReferences.length > 0
      ? {
          image: ltxImageReferences[0],
          images: ltxImageReferences,
        }
      : {}),
    ...(isLTXStartEnd && ltxImageReferences.length > 0
      ? {
          first_frame: ltxImageReferences[0],
          ...(ltxImageReferences[1]
            ? { last_frame: ltxImageReferences[1] }
            : {}),
        }
      : {}),
    ...(ltxAudioReference ? { audio: ltxAudioReference } : {}),
    ...(timelineData !== undefined ? { timeline_data: timelineData } : {}),
    duration: config.video_duration,
    seconds: String(config.video_duration),
    ...(size ? { size } : {}),
    metadata,
  }
}
