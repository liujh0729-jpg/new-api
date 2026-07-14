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
export const SEEDREAM_MIN_PIXELS = 3686400
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

export const SEEDREAM_40_45_IMAGE_SIZE_OPTIONS = [
  SEEDREAM_45_SAFE_IMAGE_SIZE,
  '2048x2048',
  '2560x1440',
  '1440x2560',
  '3072x3072',
  '4096x4096',
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
export const SEEDANCE_VIDEO_RATIO_OPTIONS: readonly string[] =
  VIDEO_RATIO_OPTIONS

export type VideoDurationRange = {
  min: number
  max: number
  step: number
}

export const DEFAULT_VIDEO_DURATION_RANGE: VideoDurationRange = {
  min: 5,
  max: 15,
  step: 5,
}
export const SEEDANCE_10_VIDEO_DURATION_RANGE: VideoDurationRange = {
  min: 2,
  max: 12,
  step: 1,
}
export const SEEDANCE_15_20_VIDEO_DURATION_RANGE: VideoDurationRange = {
  min: 4,
  max: 15,
  step: 1,
}
export const LTX_23_VIDEO_DURATION_RANGE: VideoDurationRange = {
  min: 1,
  max: 20,
  step: 1,
}
export const DEFAULT_VIDEO_RATIO = '16:9'
export const DEFAULT_VIDEO_DURATION = 5
export const DEFAULT_VIDEO_RESOLUTION = '720p'
export const VIDEO_RESOLUTION_OPTIONS = [
  '480p',
  DEFAULT_VIDEO_RESOLUTION,
  '1080p',
  '4k',
] as const
export const SEEDANCE_20_FAST_VIDEO_RESOLUTION_OPTIONS = [
  '480p',
  DEFAULT_VIDEO_RESOLUTION,
] as const
export const SEEDANCE_15_PRO_VIDEO_RESOLUTION_OPTIONS = [
  '480p',
  DEFAULT_VIDEO_RESOLUTION,
  '1080p',
] as const
export const AP_SEEDANCE_20_LITE_VIDEO_RESOLUTION_OPTIONS = [
  '480p',
  DEFAULT_VIDEO_RESOLUTION,
  '1080p',
] as const
export const AP_SEEDANCE_20_COST_EFFECTIVE_VIDEO_RESOLUTION_OPTIONS = [
  '1080p',
  '4k',
] as const
export const DEFAULT_LTX_VIDEO_SIZE = '1920x1088'
export const LTX_VIDEO_SIZE_OPTIONS = [
  '1280x720',
  '720x1280',
  DEFAULT_LTX_VIDEO_SIZE,
  '1088x1920',
  '2560x1440',
  '1440x2560',
  '3840x2176',
] as const
export const DEFAULT_LTX_23_VIDEO_SIZE = '1280x704'
export const LTX_23_VIDEO_SIZE_OPTIONS = [
  DEFAULT_LTX_23_VIDEO_SIZE,
  '704x1280',
  '704x704',
  '640x480',
  '480x640',
  '480x480',
] as const

const SEEDANCE_VIDEO_RESOLUTION_OPTIONS_BY_MODEL: Record<
  string,
  readonly string[]
> = {
  'ap seedance-2-0 vip': VIDEO_RESOLUTION_OPTIONS,
  'ap seedance-2-0 标准版': VIDEO_RESOLUTION_OPTIONS,
  'ap seedance-2-0 轻量版': AP_SEEDANCE_20_LITE_VIDEO_RESOLUTION_OPTIONS,
  'ap seedance-2-0 高性价比版':
    AP_SEEDANCE_20_COST_EFFECTIVE_VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-2-0-260128': VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-2-0-fast-260128': SEEDANCE_20_FAST_VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-2-0-mini-260615': SEEDANCE_20_FAST_VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-1-5-pro-251215': SEEDANCE_15_PRO_VIDEO_RESOLUTION_OPTIONS,
}

const SEEDANCE_VIDEO_RESOLUTION_OPTIONS_BY_MODEL_PREFIX: Record<
  string,
  readonly string[]
> = {
  'doubao-seedance-2-0-fast': SEEDANCE_20_FAST_VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-2-0-mini': SEEDANCE_20_FAST_VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-2-0': VIDEO_RESOLUTION_OPTIONS,
  'doubao-seedance-1-5-pro': SEEDANCE_15_PRO_VIDEO_RESOLUTION_OPTIONS,
}

const LTX_VIDEO_SIZE_OPTIONS_BY_MODEL_PREFIX: Record<
  string,
  readonly string[]
> = {
  'aipdd-ltx': LTX_VIDEO_SIZE_OPTIONS,
  ltx: LTX_VIDEO_SIZE_OPTIONS,
}
export const SEEDANCE_REFERENCE_LIMITS = {
  total: 12,
  image: 9,
  video: 3,
  audio: 3,
  maxFileSize: 300 * 1024 * 1024,
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

export const IMAGE_REFERENCE_ACCEPT = [
  'image/*',
  '.png',
  '.jpg',
  '.jpeg',
  '.webp',
  '.gif',
  '.bmp',
].join(',')

export const LTX_START_END_REFERENCE_ACCEPT = [
  'image/*',
  'audio/*',
  '.png',
  '.jpg',
  '.jpeg',
  '.webp',
  '.gif',
  '.bmp',
  '.heic',
  '.heif',
  '.mp3',
  '.wav',
  '.m4a',
  '.aac',
  '.ogg',
  '.oga',
  '.flac',
  '.opus',
].join(',')
export const IMAGE_REFERENCE_LIMITS = {
  maxFiles: 10,
  maxFileSize: 300 * 1024 * 1024,
} as const

function normalizeModelName(model: string): string {
  return model.trim().toLowerCase().replace(/[_.]/g, '-')
}

export function isSeedreamModel(model: string): boolean {
  return normalizeModelName(model).includes('seedream')
}

export function isAIPDDFluxImageToImageModel(model: string): boolean {
  const normalized = normalizeModelName(model)
  return normalized === 'aipdd-flux-gguf' || normalized === 'flux-gguf-v2'
}

export function isSeedream40xModel(model: string): boolean {
  const normalized = normalizeModelName(model)
  return (
    normalized.includes('seedream-4-0') || normalized.includes('seedream-4-5')
  )
}

export function isSeedream50LiteModel(model: string): boolean {
  const normalized = normalizeModelName(model)
  return (
    normalized.includes('seedream-5-0-lite') ||
    normalized.includes('seedream-5-0-260128')
  )
}

export function isSeedanceModel(model: string): boolean {
  return normalizeModelName(model).includes('seedance')
}

export function isSeedance20Model(model: string): boolean {
  const normalized = normalizeModelName(model)
  return normalized.includes('seedance-2-0')
}

export function isSeedance15Model(model: string): boolean {
  const normalized = normalizeModelName(model)
  return normalized.includes('seedance-1-5')
}

export function isSeedance10Model(model: string): boolean {
  const normalized = normalizeModelName(model)
  return normalized.includes('seedance-1-0')
}

export function isLTXVideoModel(model: string): boolean {
  const normalized = normalizeModelName(model)
  return normalized.includes('ltx')
}

export function isLTX23StartEndModel(model: string): boolean {
  const normalized = normalizeModelName(model)
  if (!normalized.includes('aipdd-ltx-2-3')) return false
  return (
    normalized.includes('首尾帧') ||
    (normalized.includes('first') && normalized.includes('last'))
  )
}

export function isLTX23PolicyModel(model: string): boolean {
  return normalizeModelName(model) === 'aipdd-ltx-2-3'
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
  if (isSeedreamModel(model)) return SEEDREAM_40_45_IMAGE_SIZE_OPTIONS
  return IMAGE_SIZE_OPTIONS
}

export function getVideoRatioOptionsForModel(model: string): readonly string[] {
  if (isSeedanceModel(model)) return SEEDANCE_VIDEO_RATIO_OPTIONS
  return VIDEO_RATIO_OPTIONS
}

export function getVideoResolutionOptionsForModel(
  model: string
): readonly string[] {
  const normalized = normalizeModelName(model)
  const exactOptions = SEEDANCE_VIDEO_RESOLUTION_OPTIONS_BY_MODEL[normalized]
  if (exactOptions) return exactOptions

  const prefixOptions = Object.entries(
    SEEDANCE_VIDEO_RESOLUTION_OPTIONS_BY_MODEL_PREFIX
  ).find(([prefix]) => normalized.startsWith(prefix))?.[1]
  if (prefixOptions) return prefixOptions

  return VIDEO_RESOLUTION_OPTIONS
}

export function getLTXVideoSizeOptionsForModel(
  model: string
): readonly string[] {
  if (isLTX23StartEndModel(model) || isLTX23PolicyModel(model)) {
    return LTX_23_VIDEO_SIZE_OPTIONS
  }

  const normalized = normalizeModelName(model)
  return (
    Object.entries(LTX_VIDEO_SIZE_OPTIONS_BY_MODEL_PREFIX).find(([prefix]) =>
      normalized.startsWith(prefix)
    )?.[1] || []
  )
}

export function getVideoDurationRangeForModel(model: string) {
  if (isLTX23PolicyModel(model) || isLTX23StartEndModel(model)) {
    return LTX_23_VIDEO_DURATION_RANGE
  }
  if (isSeedance20Model(model) || isSeedance15Model(model)) {
    return SEEDANCE_15_20_VIDEO_DURATION_RANGE
  }
  if (isSeedance10Model(model)) return SEEDANCE_10_VIDEO_DURATION_RANGE
  return DEFAULT_VIDEO_DURATION_RANGE
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

  if (!isSeedreamModel(model)) return size

  const pixels = getImageSizePixels(size)
  if (pixels !== null && pixels >= SEEDREAM_MIN_PIXELS) return size

  return SEEDREAM_45_SAFE_IMAGE_SIZE
}

export function normalizeVideoRatioForModel(
  model: string,
  ratio: string
): string {
  if (!isSeedanceModel(model)) return ratio

  const options = getVideoRatioOptionsForModel(model)
  if (options.includes(ratio)) return ratio

  return DEFAULT_VIDEO_RATIO
}

export function normalizeVideoResolutionForModel(
  model: string,
  resolution: string
): string {
  const options = getVideoResolutionOptionsForModel(model)
  if (options.includes(resolution)) return resolution
  if (options.includes(DEFAULT_VIDEO_RESOLUTION))
    return DEFAULT_VIDEO_RESOLUTION
  return options[0] || DEFAULT_VIDEO_RESOLUTION
}

export function normalizeLTXVideoSizeForModel(
  model: string,
  size: string
): string {
  const options = getLTXVideoSizeOptionsForModel(model)
  if (options.length === 0) return size
  if (options.includes(size)) return size
  if (options.includes(DEFAULT_LTX_23_VIDEO_SIZE)) {
    return DEFAULT_LTX_23_VIDEO_SIZE
  }
  if (options.includes(DEFAULT_LTX_VIDEO_SIZE)) return DEFAULT_LTX_VIDEO_SIZE
  return options[0] || DEFAULT_LTX_VIDEO_SIZE
}

export function getLTXVideoDimensions(size: string):
  | {
      width: number
      height: number
    }
  | undefined {
  const match = size.trim().match(/^(\d+)x(\d+)$/i)
  if (!match) return undefined

  const width = Number(match[1])
  const height = Number(match[2])
  if (!Number.isFinite(width) || !Number.isFinite(height)) return undefined
  if (width <= 0 || height <= 0) return undefined

  return { width, height }
}

export function normalizeVideoDurationForModel(
  model: string,
  duration: number
): number {
  const range = getVideoDurationRangeForModel(model)
  const numericDuration = Number(duration)
  if (!Number.isFinite(numericDuration)) return DEFAULT_VIDEO_DURATION

  const roundedDuration = Math.round(numericDuration)
  return Math.min(range.max, Math.max(range.min, roundedDuration))
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
  thinking_mode: 'auto',
  stream: true,
  image_size: DEFAULT_IMAGE_SIZE,
  image_quality: 'standard',
  image_count: 1,
  video_ratio: DEFAULT_VIDEO_RATIO,
  video_duration: DEFAULT_VIDEO_DURATION,
  video_resolution: DEFAULT_VIDEO_RESOLUTION,
  video_size: DEFAULT_LTX_VIDEO_SIZE,
  ltx_timeline_data: '',
}

export const DEFAULT_PARAMETER_ENABLED: ParameterEnabled = {
  temperature: true,
  top_p: true,
  max_tokens: false,
  frequency_penalty: false,
  presence_penalty: false,
  seed: false,
}

// Storage keys
export const STORAGE_KEYS = {
  CONFIG: 'playground_config',
  MESSAGES: 'playground_messages',
  PARAMETER_ENABLED: 'playground_parameter_enabled',
  CONVERSATIONS_PREFIX: 'playground_conversations_v1',
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
  TIMELINE_JSON_INVALID: 'Timeline JSON is invalid',
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
