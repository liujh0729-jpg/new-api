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
import type {
  GeneratedImage,
  GeneratedVideo,
  ImageGenerationResponse,
  VideoGenerationResponse,
} from '../types'

export interface ImageTaskState {
  taskId?: string
  status?: string
  progress?: string
  images: GeneratedImage[]
  error?: string
}

export interface VideoTaskState {
  taskId?: string
  status?: string
  progress?: string
  videos: GeneratedVideo[]
  error?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function normalizeImage(value: unknown): GeneratedImage | null {
  if (typeof value === 'string' && value.trim()) {
    return { url: value.trim() }
  }

  if (!isRecord(value)) return null

  const url = firstString(
    value.url,
    value.image_url,
    value.image,
    value.file,
    value.result
  )
  const b64Json = firstString(value.b64_json, value.b64)
  if (!url && !b64Json) return null

  return {
    url,
    b64_json: b64Json,
    mime_type: firstString(value.mime_type, value.mimeType),
    revised_prompt: firstString(value.revised_prompt, value.prompt),
  }
}

function normalizeVideo(value: unknown): GeneratedVideo | null {
  if (typeof value === 'string' && value.trim()) {
    return { url: value.trim() }
  }

  if (!isRecord(value)) return null

  const url = firstString(
    value.url,
    value.video_url,
    value.result_url,
    value.result,
    value.file
  )
  if (!url) return null

  return {
    url,
    task_id: firstString(value.task_id, value.id),
    mime_type: firstString(value.mime_type, value.mimeType, value.format),
  }
}

function firstString(...values: unknown[]): string | undefined {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return undefined
}

function extractImages(value: unknown): GeneratedImage[] {
  if (!value) return []

  if (typeof value === 'string') {
    const image = normalizeImage(value)
    return image ? [image] : []
  }

  if (Array.isArray(value)) {
    return value
      .flatMap((item) => extractImages(item))
      .filter((image) => image.url || image.b64_json)
  }

  if (!isRecord(value)) return []

  const direct = normalizeImage(value)
  if (direct) return [direct]

  for (const key of [
    'data',
    'output',
    'outputs',
    'url',
    'urls',
    'image',
    'images',
    'result',
    'results',
    'file',
    'files',
    'task_result',
    'metadata',
  ]) {
    if (key in value) {
      const nested = extractImages(value[key])
      if (nested.length > 0) return nested
    }
  }

  return []
}

function extractVideos(value: unknown): GeneratedVideo[] {
  if (!value) return []

  if (typeof value === 'string') {
    const video = normalizeVideo(value)
    return video ? [video] : []
  }

  if (Array.isArray(value)) {
    return value
      .flatMap((item) => extractVideos(item))
      .filter((video) => video.url)
  }

  if (!isRecord(value)) return []

  const direct = normalizeVideo(value)
  if (direct) return [direct]

  for (const key of [
    'data',
    'output',
    'outputs',
    'url',
    'urls',
    'video',
    'videos',
    'result',
    'results',
    'result_url',
    'file',
    'files',
    'task_result',
    'metadata',
  ]) {
    if (key in value) {
      const nested = extractVideos(value[key])
      if (nested.length > 0) return nested
    }
  }

  return []
}

function normalizeTaskStatus(status?: string): string | undefined {
  if (!status) return undefined
  const normalized = status.toLowerCase()
  if (['success', 'succeeded', 'completed', 'complete'].includes(normalized)) {
    return 'succeeded'
  }
  if (['failure', 'failed', 'error'].includes(normalized)) {
    return 'failed'
  }
  if (['submitted', 'queued', 'not_start'].includes(normalized)) {
    return 'queued'
  }
  if (['in_progress', 'processing', 'running'].includes(normalized)) {
    return 'processing'
  }
  return normalized
}

export function extractImageResults(
  response: ImageGenerationResponse | unknown
): GeneratedImage[] {
  return extractImages(response)
}

export function getImageSrc(image: GeneratedImage): string {
  if (image.url) return image.url
  if (image.b64_json) {
    return `data:${image.mime_type || 'image/png'};base64,${image.b64_json}`
  }
  return ''
}

export function extractVideoResults(
  response: VideoGenerationResponse | unknown
): GeneratedVideo[] {
  return extractVideos(response)
}

export function getVideoSrc(video: GeneratedVideo): string {
  return video.url
}

export function parseImageTaskResponse(response: unknown): ImageTaskState {
  const root = isRecord(response) ? response : {}
  const data = isRecord(root.data) ? root.data : root
  const status = normalizeTaskStatus(
    firstString(data.status, root.status, data.state, root.state)
  )
  const taskId = firstString(
    data.task_id,
    root.task_id,
    data.id,
    root.id,
    data.video_id,
    root.video_id
  )
  const images = extractImages(data)
  const error = firstString(
    data.fail_reason,
    root.fail_reason,
    data.error,
    root.error,
    data.message,
    root.message
  )

  return {
    taskId,
    status,
    progress: firstString(data.progress, root.progress),
    images,
    error,
  }
}

export function parseVideoTaskResponse(response: unknown): VideoTaskState {
  const root = isRecord(response) ? response : {}
  const data = isRecord(root.data) ? root.data : root
  const status = normalizeTaskStatus(
    firstString(data.status, root.status, data.state, root.state)
  )
  const taskId = firstString(
    data.task_id,
    root.task_id,
    data.id,
    root.id,
    data.video_id,
    root.video_id
  )
  const videos = extractVideos(data)
  const error = firstString(
    isRecord(data.error) ? data.error.message : undefined,
    isRecord(root.error) ? root.error.message : undefined,
    data.fail_reason,
    root.fail_reason,
    data.error,
    root.error,
    data.message,
    root.message
  )

  return {
    taskId,
    status,
    progress: firstString(data.progress, root.progress),
    videos,
    error,
  }
}

export function isImageTaskResponse(
  response: ImageGenerationResponse
): boolean {
  if (response.task_id) return true
  if (response.object?.includes('task')) return true
  const status = normalizeTaskStatus(response.status)
  return !!status && status !== 'succeeded'
}

export function isVideoTaskResponse(
  response: VideoGenerationResponse
): boolean {
  if (response.task_id || response.id) return true
  const status = normalizeTaskStatus(response.status)
  return !!status && status !== 'succeeded'
}
