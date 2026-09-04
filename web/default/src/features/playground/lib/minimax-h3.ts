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
import type { SeedanceReference } from '../types'

export type MinimaxH3ModelKind =
  | 'auto'
  | 'text-to-video'
  | 'reference-to-video'
  | 'multimodal-to-video'
  | 'image-audio-lipsync'
  | 'first-last-frame-to-video'

export interface MinimaxH3ModelSpec {
  kind: MinimaxH3ModelKind
  prompt: 'required' | 'unused'
  ratios: readonly string[]
  resolutions: readonly string[]
  imageRange: readonly [number, number]
  audioRange: readonly [number, number]
}

export interface MinimaxH3ValidationIssue {
  key: string
  options?: Record<string, number | string>
}

interface ValidateMinimaxH3Input {
  model: string
  prompt: string
  references: SeedanceReference[]
  duration: number
  resolution: string
  ratio: string
  requirePublicUrls?: boolean
}

const TEXT_TO_VIDEO_SPEC: MinimaxH3ModelSpec = {
  kind: 'text-to-video',
  prompt: 'required',
  ratios: ['16:9', '9:16', '1:1'],
  resolutions: ['480p', '768p'],
  imageRange: [0, 0],
  audioRange: [0, 0],
}

const REFERENCE_TO_VIDEO_SPEC: MinimaxH3ModelSpec = {
  kind: 'reference-to-video',
  prompt: 'required',
  ratios: ['16:9', '9:16', '1:1'],
  resolutions: ['480p', '768p', '1080p'],
  imageRange: [1, 9],
  audioRange: [0, 0],
}

const MULTIMODAL_TO_VIDEO_SPEC: MinimaxH3ModelSpec = {
  kind: 'multimodal-to-video',
  prompt: 'required',
  ratios: ['16:9', '9:16'],
  resolutions: ['480p', '768p', '1080p'],
  imageRange: [1, 9],
  audioRange: [1, 3],
}

const IMAGE_AUDIO_LIPSYNC_SPEC: MinimaxH3ModelSpec = {
  kind: 'image-audio-lipsync',
  prompt: 'unused',
  ratios: ['16:9', '9:16'],
  resolutions: ['480p', '768p', '1080p'],
  imageRange: [1, 1],
  audioRange: [1, 1],
}

const FIRST_LAST_FRAME_SPEC: MinimaxH3ModelSpec = {
  kind: 'first-last-frame-to-video',
  prompt: 'required',
  ratios: ['16:9', '9:16'],
  resolutions: ['480p', '768p'],
  imageRange: [2, 2],
  audioRange: [0, 0],
}

const AUTO_SPEC: MinimaxH3ModelSpec = {
  kind: 'auto',
  prompt: 'required',
  ratios: ['16:9', '9:16', '1:1'],
  resolutions: ['480p', '768p', '1080p'],
  imageRange: [0, 9],
  audioRange: [0, 3],
}

const MINIMAX_H3_MODEL_SPECS: Readonly<Record<string, MinimaxH3ModelSpec>> = {
  'ap-minimax-h3': AUTO_SPEC,
}

function normalizeMinimaxH3ModelName(model: string): string {
  return model.trim().toLowerCase().replace(/[_.]/g, '-')
}

export function getMinimaxH3ModelSpec(
  model: string
): MinimaxH3ModelSpec | undefined {
  return MINIMAX_H3_MODEL_SPECS[normalizeMinimaxH3ModelName(model)]
}

export function isMinimaxH3Model(model: string): boolean {
  return getMinimaxH3ModelSpec(model) !== undefined
}

export function inferMinimaxH3ModelSpec(
  input: Pick<ValidateMinimaxH3Input, 'model' | 'prompt' | 'references'>
): MinimaxH3ModelSpec | undefined {
  if (!isMinimaxH3Model(input.model)) return undefined

  const images = input.references.filter(
    (reference) => reference.kind === 'image'
  )
  const audioCount = input.references.filter(
    (reference) => reference.kind === 'audio'
  ).length
  const hasFrameRoles = images.some(
    (reference) =>
      reference.role === 'first_frame' || reference.role === 'last_frame'
  )

  if (hasFrameRoles) return FIRST_LAST_FRAME_SPEC
  if (images.length === 0 && audioCount === 0) return TEXT_TO_VIDEO_SPEC
  if (images.length > 0 && audioCount === 0) return REFERENCE_TO_VIDEO_SPEC
  if (images.length === 1 && audioCount === 1 && !input.prompt.trim()) {
    return IMAGE_AUDIO_LIPSYNC_SPEC
  }
  return MULTIMODAL_TO_VIDEO_SPEC
}

export function getMinimaxH3DurationRange(
  model: string,
  resolution: string
): { min: number; max: number; step: number } | undefined {
  const spec = getMinimaxH3ModelSpec(model)
  if (!spec) return undefined

  return durationRangeForSpec(spec, resolution)
}

function durationRangeForSpec(
  spec: MinimaxH3ModelSpec,
  resolution: string
): { min: number; max: number; step: number } {
  const hasShort1080pLimit =
    resolution === '1080p' &&
    (spec.kind === 'reference-to-video' || spec.kind === 'multimodal-to-video')

  return {
    min: 1,
    max: hasShort1080pLimit ? 10 : 15,
    step: 1,
  }
}

export function isPublicHttpReferenceUrl(url: string): boolean {
  const value = url.trim()
  if (!/^https?:\/\//i.test(value)) return false

  try {
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    if (
      hostname === 'localhost' ||
      hostname === '0.0.0.0' ||
      hostname === '::' ||
      hostname === '::1' ||
      hostname.endsWith('.local') ||
      hostname.startsWith('fc') ||
      hostname.startsWith('fd') ||
      hostname.startsWith('fe80:')
    ) {
      return false
    }

    const ipv4Parts = hostname.split('.').map((part) => Number(part))
    if (
      ipv4Parts.length === 4 &&
      ipv4Parts.every(
        (part) => Number.isInteger(part) && part >= 0 && part <= 255
      )
    ) {
      const [first, second] = ipv4Parts
      if (
        first === 0 ||
        first === 10 ||
        first === 127 ||
        first >= 224 ||
        (first === 100 && second >= 64 && second <= 127) ||
        (first === 169 && second === 254) ||
        (first === 172 && second >= 16 && second <= 31) ||
        (first === 192 && second === 168)
      ) {
        return false
      }
    }

    return true
  } catch {
    return false
  }
}

function validateRange(
  count: number,
  range: readonly [number, number],
  key: string
): MinimaxH3ValidationIssue | null {
  if (count >= range[0] && count <= range[1]) return null
  return { key, options: { min: range[0], max: range[1] } }
}

export function validateMinimaxH3VideoInput(
  input: ValidateMinimaxH3Input
): MinimaxH3ValidationIssue | null {
  const spec = inferMinimaxH3ModelSpec(input)
  if (!spec) return null

  if (spec.prompt === 'required' && !input.prompt.trim()) {
    return { key: 'This MiniMax H3 model requires a prompt' }
  }

  const imageReferences = input.references.filter(
    (reference) => reference.kind === 'image'
  )
  const videoCount = input.references.filter(
    (reference) => reference.kind === 'video'
  ).length
  const audioReferences = input.references.filter(
    (reference) => reference.kind === 'audio'
  )

  if (videoCount > 0) {
    return { key: 'MiniMax H3 does not support video references' }
  }

  const imageIssue = validateRange(
    imageReferences.length,
    spec.imageRange,
    'MiniMax H3 requires {{min}}–{{max}} image references for this model'
  )
  if (imageIssue) return imageIssue

  const audioIssue = validateRange(
    audioReferences.length,
    spec.audioRange,
    'MiniMax H3 requires {{min}}–{{max}} audio references for this model'
  )
  if (audioIssue) return audioIssue

  if (spec.kind === 'first-last-frame-to-video') {
    const firstFrameCount = imageReferences.filter(
      (reference) => reference.role === 'first_frame'
    ).length
    const lastFrameCount = imageReferences.filter(
      (reference) => reference.role === 'last_frame'
    ).length
    if (firstFrameCount !== 1 || lastFrameCount !== 1) {
      return { key: 'MiniMax H3 requires both first and last frames' }
    }
  }

  if (!spec.ratios.includes(input.ratio)) {
    return { key: 'This aspect ratio is not supported by the MiniMax H3 model' }
  }
  if (!spec.resolutions.includes(input.resolution)) {
    return { key: 'This resolution is not supported by the MiniMax H3 model' }
  }

  const durationRange = durationRangeForSpec(spec, input.resolution)
  if (
    !durationRange ||
    !Number.isInteger(input.duration) ||
    input.duration < durationRange.min ||
    input.duration > durationRange.max
  ) {
    return {
      key: 'MiniMax H3 duration must be an integer from {{min}} to {{max}} seconds',
      options: {
        min: durationRange?.min ?? 1,
        max: durationRange?.max ?? 15,
      },
    }
  }

  if (
    input.requirePublicUrls &&
    input.references.some(
      (reference) => !isPublicHttpReferenceUrl(reference.url)
    )
  ) {
    return { key: 'MiniMax H3 reference media must use a public HTTP(S) URL' }
  }

  return null
}
