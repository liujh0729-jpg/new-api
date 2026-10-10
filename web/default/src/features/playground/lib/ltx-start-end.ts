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

export const LTX_23_FRAME_RATE = 24

export type LTXStartEndImageCountError =
  | 'LTX start-end requires a first frame'
  | 'LTX supports one reference image'

const LTX_TIMELINE_COLORS = [
  '#4f8edc',
  '#e07b3a',
  '#5cb85c',
  '#9b6cd6',
] as const

export type LTXTimelineValidationError =
  | 'Timeline JSON is invalid'
  | 'Timeline must contain at least one segment'
  | 'Each timeline segment requires a prompt'
  | 'Timeline segment lengths must be positive integers'
  | 'Timeline must total {{frames}} frames'

export interface LTXTimelineSegment {
  prompt: string
  length: number
  color: string
}

export interface LTXTimelineData {
  segments: LTXTimelineSegment[]
}

export interface LTXTimelineResolution {
  timeline?: LTXTimelineData
  error?: LTXTimelineValidationError
  frameCount: number
  isAutomatic: boolean
}

export function validateLTXStartEndImageCount(
  imageCount: number
): LTXStartEndImageCountError | null {
  if (imageCount === 0) return 'LTX start-end requires a first frame'
  if (imageCount > 1) return 'LTX supports one reference image'
  return null
}

export function getLTX23FrameCount(duration: number): number {
  const seconds = Number.isFinite(duration)
    ? Math.max(1, Math.round(duration))
    : 1
  return seconds * LTX_23_FRAME_RATE + 1
}

export function resolveLTXStartEndTimeline(
  prompt: string,
  duration: number,
  override: string
): LTXTimelineResolution {
  const frameCount = getLTX23FrameCount(duration)
  const rawOverride = override.trim()

  if (!rawOverride) {
    return {
      timeline: {
        segments: [
          {
            prompt: prompt.trim(),
            length: frameCount,
            color: LTX_TIMELINE_COLORS[0],
          },
        ],
      },
      frameCount,
      isAutomatic: true,
    }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(rawOverride)
  } catch {
    return {
      error: 'Timeline JSON is invalid',
      frameCount,
      isAutomatic: false,
    }
  }

  const rawSegments = Array.isArray(parsed)
    ? parsed
    : isRecord(parsed) && Array.isArray(parsed.segments)
      ? parsed.segments
      : undefined

  if (!rawSegments?.length) {
    return {
      error: 'Timeline must contain at least one segment',
      frameCount,
      isAutomatic: false,
    }
  }

  const segments: LTXTimelineSegment[] = []
  for (const [index, rawSegment] of rawSegments.entries()) {
    if (!isRecord(rawSegment)) {
      return {
        error: 'Timeline JSON is invalid',
        frameCount,
        isAutomatic: false,
      }
    }

    const segmentPrompt =
      typeof rawSegment.prompt === 'string' ? rawSegment.prompt.trim() : ''
    if (!segmentPrompt) {
      return {
        error: 'Each timeline segment requires a prompt',
        frameCount,
        isAutomatic: false,
      }
    }

    const segmentLength = rawSegment.length
    if (
      typeof segmentLength !== 'number' ||
      !Number.isInteger(segmentLength) ||
      segmentLength <= 0
    ) {
      return {
        error: 'Timeline segment lengths must be positive integers',
        frameCount,
        isAutomatic: false,
      }
    }

    const color =
      typeof rawSegment.color === 'string' && rawSegment.color.trim()
        ? rawSegment.color.trim()
        : LTX_TIMELINE_COLORS[index % LTX_TIMELINE_COLORS.length]
    segments.push({ prompt: segmentPrompt, length: segmentLength, color })
  }

  const totalFrames = segments.reduce(
    (total, segment) => total + segment.length,
    0
  )
  if (totalFrames !== frameCount) {
    return {
      error: 'Timeline must total {{frames}} frames',
      frameCount,
      isAutomatic: false,
    }
  }

  return {
    timeline: { segments },
    frameCount,
    isAutomatic: false,
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
