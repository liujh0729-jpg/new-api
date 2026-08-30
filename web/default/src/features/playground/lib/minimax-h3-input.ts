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
import type { PromptInputFile } from '@/components/ai-elements/prompt-input'
import type { SeedanceReference, SeedanceReferenceKind } from '../types'
import {
  getMinimaxH3ModelSpec,
  type MinimaxH3ValidationIssue,
  validateMinimaxH3VideoInput,
} from './minimax-h3'

export interface MinimaxH3InputState {
  model: string
  prompt: string
  duration: number
  resolution: string
  ratio: string
}

export function inferMinimaxH3ReferenceKind(
  file: PromptInputFile
): SeedanceReferenceKind | null {
  const mediaType = file.mediaType?.trim().toLowerCase() || ''
  if (mediaType.startsWith('image/')) return 'image'
  if (mediaType.startsWith('video/')) return 'video'
  if (mediaType.startsWith('audio/')) return 'audio'

  const dataUrlType = file.url.match(/^data:(image|video|audio)\//i)?.[1]
  if (dataUrlType) {
    return dataUrlType.toLowerCase() as SeedanceReferenceKind
  }

  const path = (file.filename || file.url).split(/[?#]/, 1)[0]
  if (/\.(avif|bmp|gif|heic|heif|jpe?g|png|webp)$/i.test(path)) return 'image'
  if (/\.(3gp|avi|m4v|mkv|mov|mp4|mpe?g|webm)$/i.test(path)) return 'video'
  if (/\.(aac|flac|m4a|mp3|oga|ogg|opus|wav)$/i.test(path)) return 'audio'
  return null
}

export function buildMinimaxH3InputReferences(
  files: PromptInputFile[]
): SeedanceReference[] {
  return files.flatMap((file) => {
    const kind = inferMinimaxH3ReferenceKind(file)
    if (!kind) return []
    return [
      {
        kind,
        url: file.url,
        filename: file.filename,
        media_type: file.mediaType,
        role: file.role,
      },
    ]
  })
}

export function getMinimaxH3PromptInputIssue(
  state: MinimaxH3InputState,
  files: PromptInputFile[]
): MinimaxH3ValidationIssue | null {
  if (!getMinimaxH3ModelSpec(state.model)) return null

  const references = buildMinimaxH3InputReferences(files)
  if (references.length !== files.length) {
    return { key: 'Only image, video, and audio references are supported' }
  }

  return validateMinimaxH3VideoInput({
    model: state.model,
    prompt: state.prompt,
    references,
    duration: state.duration,
    resolution: state.resolution,
    ratio: state.ratio,
  })
}

export function normalizeMinimaxH3InputFiles(
  model: string,
  files: PromptInputFile[]
): PromptInputFile[] {
  const spec = getMinimaxH3ModelSpec(model)
  if (!spec) return files

  const images = files.filter(
    (file) => inferMinimaxH3ReferenceKind(file) === 'image'
  )
  const audios = files.filter(
    (file) => inferMinimaxH3ReferenceKind(file) === 'audio'
  )

  if (spec.kind === 'reference-to-video') {
    return images.slice(0, spec.imageRange[1])
  }
  if (spec.kind === 'multimodal-to-video') {
    return [
      ...images.slice(0, spec.imageRange[1]),
      ...audios.slice(0, spec.audioRange[1]),
    ]
  }
  if (spec.kind === 'image-audio-lipsync') {
    return [...images.slice(0, 1), ...audios.slice(0, 1)]
  }
  if (spec.kind !== 'first-last-frame-to-video') return []

  const firstFrame =
    images.find((file) => file.role === 'first_frame') || images[0]
  const lastFrame =
    images.find((file) => file.role === 'last_frame' && file !== firstFrame) ||
    images.find((file) => file !== firstFrame)
  const normalizedFrames: PromptInputFile[] = []
  if (firstFrame) {
    normalizedFrames.push({ ...firstFrame, role: 'first_frame' })
  }
  if (lastFrame) {
    normalizedFrames.push({ ...lastFrame, role: 'last_frame' })
  }
  return normalizedFrames
}
