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
import type { PlaygroundConfig, ParameterEnabled } from './types'

// Message constants
export const MESSAGE_ROLES = {
  USER: 'user',
  ASSISTANT: 'assistant',
  SYSTEM: 'system',
} as const

export const MESSAGE_STATUS = {
  LOADING: 'loading',
  STREAMING: 'streaming',
  COMPLETE: 'complete',
  ERROR: 'error',
} as const

// API endpoints
export const API_ENDPOINTS = {
  CHAT_COMPLETIONS: '/pg/chat/completions',
  IMAGE_GENERATIONS: '/pg/images/generations',
  VIDEO_GENERATIONS: '/pg/video/generations',
  REFERENCE_MEDIA_UPLOAD: '/pg/reference-media/upload',
  USER_MODELS: '/api/user/models',
  USER_GROUPS: '/api/user/self/groups',
} as const

// Default group
export const DEFAULT_GROUP = 'auto' as const

export const DEFAULT_IMAGE_SIZE = '1024x1024'
export const SEEDREAM_45_MIN_PIXELS = 3686400
export const SEEDREAM_45_SAFE_IMAGE_SIZE = '1920x1920'
export const SEEDREAM_50_LITE_DEFAULT_IMAGE_SIZE = '2K'
export const SEEDREAM_50_LITE_MIN_PIXELS = 2560 * 1440
export const SEEDREAM_50_LITE_MAX_PIXELS = 4096 * 4096

export const IMAGE_SIZE_OPTIONS = [
  SEEDREAM_45_SAFE_IMAGE_SIZE,
  '2560x1440',
  '1440x2560',
  DEFAULT_IMAGE_SIZE,
  '1024x1536',
  '1536x1024',
  '512x512',
] as const

export const SEEDREAM_50_LITE_IMAGE_SIZE_OPTIONS = [
  SEEDREAM_50_LITE_DEFAULT_IMAGE_SIZE,
  '3K',
  '4K',
  SEEDREAM_45_SAFE_IMAGE_SIZE,
  '2048x2048',
  '2560x1440',
  '1440x2560',
  '4096x4096',
] as const

export const VIDEO_RATIO_OPTIONS = [
  '16:9',
  '9:16',
  '1:1',
  '4:3',
  '3:4',
] as const
export const VIDEO_DURATION_OPTIONS = [5, 10, 15] as const
export const DEFAULT_VIDEO_RATIO = '16:9'
export const DEFAULT_VIDEO_DURATION = 5
export const DEFAULT_VIDEO_RESOLUTION = '720p'
export const SEEDANCE_REFERENCE_LIMITS = {
  total: 12,
  image: 9,
  video: 3,
  audio: 3,
  maxFileSize: 30 * 1024 * 1024,
  minVideoDurationSeconds: 2,
  maxVideoDurationSeconds: 15.2,
  maxVideoTotalDurationSeconds: 15.2,
  maxAudioTotalDurationSeconds: 15.2,
} as const
export const SEEDANCE_REFERENCE_ACCEPT = [
  'image/*',
  'video/*',
  'audio/*',
  '.png',
  '.jpg',
  '.jpeg',
  '.webp',
  '.gif',
  '.bmp',
  '.heic',
  '.heif',
  '.mp4',
  '.mov',
  '.m4v',
  '.webm',
  '.mkv',
  '.avi',
  '.mpeg',
  '.mpg',
  '.3gp',
  '.mp3',
  '.wav',
  '.m4a',
  '.aac',
  '.ogg',
  '.oga',
  '.flac',
  '.opus',
].join(',')

export function isSeedance20VideoModel(model: string): boolean {
  const normalized = model.toLowerCase().trim()
  const compact = normalized.replace(/[\s_.]+/g, '-')
  return (
    normalized.includes('seedance-2-0') ||
    normalized.includes('seedance-2.0') ||
    compact.includes('seedance-2-0')
  )
}

export function isSeedream45Model(model: string): boolean {
  return model.includes('seedream-4-5')
}

export function isSeedream50LiteModel(model: string): boolean {
  const normalized = model.trim().toLowerCase().replace(/[_.]/g, '-')
  return (
    normalized.includes('seedream-5-0-lite') ||
    normalized.includes('seedream-5-0-260128')
  )
}

export function getImageSizePixels(size: string): number | null {
  const match = size.trim().match(/^(\d+)x(\d+)$/i)
  if (!match) return null

  const width = Number(match[1])
  const height = Number(match[2])
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null

  return width * height
}

export function isValidSeedream50LiteImageSize(size: string): boolean {
  const normalizedSize = size.trim()
  if (!normalizedSize) return true
  if (['2K', '3K', '4K'].includes(normalizedSize.toUpperCase())) return true

  const match = normalizedSize.match(/^(\d+)x(\d+)$/i)
  if (!match) return false

  const width = Number(match[1])
  const height = Number(match[2])
  if (!Number.isFinite(width) || !Number.isFinite(height)) return false
  if (width <= 0 || height <= 0) return false

  const pixels = width * height
  const ratio = width / height

  return (
    pixels >= SEEDREAM_50_LITE_MIN_PIXELS &&
    pixels <= SEEDREAM_50_LITE_MAX_PIXELS &&
    ratio >= 1 / 16 &&
    ratio <= 16
  )
}

export function getImageSizeOptionsForModel(model: string): readonly string[] {
  if (isSeedream50LiteModel(model)) return SEEDREAM_50_LITE_IMAGE_SIZE_OPTIONS
  return IMAGE_SIZE_OPTIONS
}

export function normalizeImageSizeForModel(
  model: string,
  size: string
): string {
  if (isSeedream50LiteModel(model)) {
    return isValidSeedream50LiteImageSize(size)
      ? size
      : SEEDREAM_50_LITE_DEFAULT_IMAGE_SIZE
  }

  if (!isSeedream45Model(model)) return size

  const pixels = getImageSizePixels(size)
  if (pixels !== null && pixels >= SEEDREAM_45_MIN_PIXELS) return size

  return SEEDREAM_45_SAFE_IMAGE_SIZE
}

// Default configuration
export const DEFAULT_CONFIG: PlaygroundConfig = {
  mode: 'chat',
  model: 'gpt-4o',
  group: DEFAULT_GROUP,
  temperature: 0.7,
  top_p: 1,
  max_tokens: 4096,
  frequency_penalty: 0,
  presence_penalty: 0,
  seed: null,
  stream: true,
  image_size: DEFAULT_IMAGE_SIZE,
  image_quality: 'standard',
  image_count: 1,
  video_ratio: DEFAULT_VIDEO_RATIO,
  video_duration: DEFAULT_VIDEO_DURATION,
}

export const DEFAULT_PARAMETER_ENABLED: ParameterEnabled = {
  temperature: true,
  top_p: true,
  max_tokens: false,
  frequency_penalty: true,
  presence_penalty: true,
  seed: false,
}

// Storage keys
export const STORAGE_KEYS = {
  CONFIG: 'playground_config',
  MESSAGES: 'playground_messages',
  PARAMETER_ENABLED: 'playground_parameter_enabled',
  VIDEO_DURATION_DEFAULT_MIGRATED:
    'playground_video_duration_default_v5_migrated',
} as const

// Error messages
export const ERROR_MESSAGES = {
  API_REQUEST_ERROR: 'Request error occurred',
  NETWORK_ERROR: 'Network connection failed or server not responding',
  PARSE_ERROR: 'Error parsing response data',
  STREAM_START_ERROR: 'Error establishing connection',
  CONNECTION_CLOSED: 'Connection closed',
  INTERRUPTED: 'Generation was interrupted',
  IMAGE_TASK_TIMEOUT: 'Image task timed out',
  IMAGE_TASK_FAILED: 'Image task failed',
  VIDEO_TASK_TIMEOUT: 'Video task timed out',
  VIDEO_TASK_FAILED: 'Video task failed',
  VIDEO_REFERENCE_UPLOAD_REQUIRED:
    'Reference media must be uploaded before video generation. Please reselect the files and try again.',
  VIDEO_REFERENCE_DURATION_READ_FAILED:
    'Unable to read reference media duration. Please reselect the file.',
} as const

// Message action button styles
export const MESSAGE_ACTION_BUTTON_STYLES = {
  BASE: 'size-7 text-muted-foreground hover:text-foreground',
  DELETE: 'size-7 text-muted-foreground hover:text-destructive',
  ICON: 'size-4',
} as const

// Message action labels
export const MESSAGE_ACTION_LABELS = {
  COPY: 'Copy',
  COPIED: 'Copied!',
  REGENERATE: 'Regenerate',
  EDIT: 'Edit',
  DELETE: 'Delete',
  NO_CONTENT: 'No content to copy',
  WAIT_GENERATION: 'Please wait for the current generation to complete',
} as const
