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
import { describe, expect, test } from 'bun:test'
import { buildVideoGenerationPayload } from '../src/features/playground/lib/payload-builder'
import { validateLTXStartEndImageCount } from '../src/features/playground/lib/ltx-start-end'
import { getLTXStartEndAttachmentState } from '../src/features/playground/lib/ltx-start-end-attachments'
import type {
  PlaygroundConfig,
  SeedanceReference,
} from '../src/features/playground/types'

const model = 'aipdd_ltx_2.3 (首尾帧)'
const firstFrame = 'https://cdn.example.com/first.png'
const lastFrame = 'https://cdn.example.com/last.png'

function playgroundConfig(): PlaygroundConfig {
  return {
    mode: 'video',
    model,
    group: 'auto',
    temperature: 1,
    top_p: 1,
    max_tokens: 1024,
    frequency_penalty: 0,
    presence_penalty: 0,
    seed: null,
    thinking_mode: 'auto',
    stream: false,
    image_size: '1024x1024',
    image_quality: 'standard',
    image_count: 1,
    video_ratio: '16:9',
    video_duration: 5,
    video_resolution: '720p',
    video_size: '1280x704',
    ltx_variant: 'start_end',
    ltx_timeline_data: '',
  }
}

function image(url: string, role: SeedanceReference['role']): SeedanceReference {
  return { kind: 'image', role, url }
}

describe('LTX 2.3 start-end Playground validation', () => {
  test('allows one first-frame image', () => {
    expect(validateLTXStartEndImageCount(1)).toBeNull()
  })

  test('allows an optional last frame and rejects more than two images', () => {
    expect(validateLTXStartEndImageCount(0)).toBe(
      'LTX start-end requires a first frame'
    )
    expect(validateLTXStartEndImageCount(2)).toBeNull()
    expect(validateLTXStartEndImageCount(3)).toBe(
      'LTX start-end supports at most two images'
    )
  })

  test('assigns first frame, optional last frame, and optional audio by role', () => {
    const files = [
      {
        url: firstFrame,
        mediaType: 'image/png',
        filename: 'first.png',
        role: 'first_frame' as const,
      },
      {
        url: lastFrame,
        mediaType: 'image/png',
        filename: 'last.png',
        role: 'last_frame' as const,
      },
      {
        url: 'https://cdn.example.com/audio.wav',
        mediaType: 'audio/wav',
        filename: 'audio.wav',
        role: 'audio' as const,
      },
    ]
    const state = getLTXStartEndAttachmentState(files)

    expect(state.firstFrame?.filename).toBe('first.png')
    expect(state.lastFrame?.filename).toBe('last.png')
    expect(state.audio?.filename).toBe('audio.wav')
    expect(state.extras).toEqual([])
    expect(state.isValid).toBeTrue()
  })
})

describe('LTX 2.3 start-end Playground payload', () => {
  test('omits the optional last frame when only the first frame is provided', () => {
    const payload = buildVideoGenerationPayload(
      'camera push in',
      [image(firstFrame, 'first_frame')],
      playgroundConfig()
    )

    expect(payload.model).toBe(model)
    expect(payload.first_frame).toBe(firstFrame)
    expect(payload.last_frame).toBeUndefined()
    expect(payload.images).toEqual([firstFrame])
  })

  test('preserves the last-frame mapping when two frames are provided', () => {
    const payload = buildVideoGenerationPayload(
      'camera push in',
      [
        image(firstFrame, 'first_frame'),
        image(lastFrame, 'last_frame'),
      ],
      playgroundConfig()
    )

    expect(payload.model).toBe(model)
    expect(payload.first_frame).toBe(firstFrame)
    expect(payload.last_frame).toBe(lastFrame)
    expect(payload.images).toEqual([firstFrame, lastFrame])
  })
})

describe('canonical LTX 2.3 multi-route payload', () => {
  const canonical = 'ap-ltx-2.3'

  test('sends the required standard variant', () => {
    const config = {
      ...playgroundConfig(),
      model: canonical,
      ltx_variant: 'standard' as const,
    }
    const payload = buildVideoGenerationPayload(
      'camera push in',
      [image(firstFrame, 'reference_image')],
      config
    )

    expect(payload.variant).toBe('standard')
    expect(payload.metadata?.variant).toBe('standard')
    expect(payload.image).toBe(firstFrame)
  })

  test('preserves ordered Licon references under one model id', () => {
    const config = {
      ...playgroundConfig(),
      model: canonical,
      video_size: '1280x720',
      ltx_variant: 'licon_2role' as const,
    }
    const images = [
      image(firstFrame, 'reference_image'),
      image(lastFrame, 'reference_image'),
      image('https://cdn.example.com/third.png', 'reference_image'),
    ]
    const payload = buildVideoGenerationPayload('three images', images, config)

    expect(payload.variant).toBe('licon_2role')
    expect(payload.images).toEqual(images.map((item) => item.url))
    expect(payload.metadata?.height).toBe(720)
  })
})
