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
import type { PromptInputFile } from '../src/components/ai-elements/prompt-input'
import {
  getVideoDurationRangeForModel,
  getVideoRatioOptionsForModel,
  getVideoResolutionOptionsForModel,
  normalizeVideoDurationForModel,
  normalizeVideoRatioForModel,
  normalizeVideoResolutionForModel,
} from '../src/features/playground/constants'
import {
  getMinimaxH3ModelSpec,
  isPublicHttpReferenceUrl,
  validateMinimaxH3VideoInput,
} from '../src/features/playground/lib/minimax-h3'
import { normalizeMinimaxH3InputFiles } from '../src/features/playground/lib/minimax-h3-input'
import { buildVideoGenerationPayload } from '../src/features/playground/lib/payload-builder'
import type {
  PlaygroundConfig,
  SeedanceReference,
  VideoGenerationRequest,
} from '../src/features/playground/types'

const models = {
  text: 'ap-minimax-h3',
  reference: 'ap-minimax-h3',
  multimodal: 'ap-minimax-h3',
  lipsync: 'ap-minimax-h3',
  firstLast: 'ap-minimax-h3',
} as const

const firstImage = 'https://cdn.example.com/first.png'
const secondImage = 'https://cdn.example.com/second.png'
const audio = 'https://cdn.example.com/voice.wav'

function playgroundConfig(
  model: string,
  overrides: Partial<PlaygroundConfig> = {}
): PlaygroundConfig {
  return {
    mode: 'video',
    model,
    group: 'auto',
    temperature: 1,
    top_p: 1,
    max_tokens: 1024,
    frequency_penalty: 0,
    presence_penalty: 0,
    seed: 42,
    thinking_mode: 'auto',
    stream: false,
    image_size: '1024x1024',
    image_quality: 'standard',
    image_count: 1,
    video_ratio: '16:9',
    video_duration: 5,
    video_resolution: '768p',
    video_size: '1280x720',
    ltx_variant: 'standard',
    ltx_timeline_data: '',
    ...overrides,
  }
}

function image(
  url: string,
  role?: SeedanceReference['role']
): SeedanceReference {
  return { kind: 'image', url, role }
}

function audioReference(url = audio): SeedanceReference {
  return { kind: 'audio', url }
}

function inputFile(
  id: string,
  mediaType: string,
  role?: PromptInputFile['role']
): PromptInputFile {
  return {
    id,
    type: 'file',
    url: `https://cdn.example.com/${id}`,
    filename: id,
    mediaType,
    role,
  }
}

function expectNoLegacyVideoFields(payload: VideoGenerationRequest) {
  const legacyKeys: Array<keyof VideoGenerationRequest> = [
    'duration',
    'seconds',
    'resolution',
    'images',
    'audio',
    'content',
    'metadata',
    'seed',
  ]
  for (const key of legacyKeys) expect(payload[key]).toBeUndefined()
}

describe('MiniMax H3 Playground payloads', () => {
  test('builds the text-to-video contract', () => {
    const payload = buildVideoGenerationPayload(
      'A cat drinks water',
      [],
      playgroundConfig(models.text),
      'task_client_12345678'
    )

    expect(payload).toEqual({
      model: models.text,
      group: 'auto',
      client_task_id: 'task_client_12345678',
      prompt: 'A cat drinks water',
      duration_seconds: 5,
      video_resolution: '768p',
      ratio: '16:9',
    })
    expectNoLegacyVideoFields(payload)
  })

  test('builds the reference-to-video contract', () => {
    const payload = buildVideoGenerationPayload(
      'Keep the character consistent',
      [image(firstImage), image(secondImage)],
      playgroundConfig(models.reference, {
        video_duration: 10,
        video_resolution: '1080p',
        video_ratio: '1:1',
      })
    )

    expect(payload).toEqual({
      model: models.reference,
      group: 'auto',
      prompt: 'Keep the character consistent',
      duration_seconds: 10,
      video_resolution: '1080p',
      ratio: '1:1',
      image_urls: [firstImage, secondImage],
    })
    expectNoLegacyVideoFields(payload)
  })

  test('builds the multimodal contract', () => {
    const payload = buildVideoGenerationPayload(
      'Speak to the rhythm',
      [image(firstImage), audioReference()],
      playgroundConfig(models.multimodal)
    )

    expect(payload).toEqual({
      model: models.multimodal,
      group: 'auto',
      prompt: 'Speak to the rhythm',
      duration_seconds: 5,
      video_resolution: '768p',
      ratio: '16:9',
      image_urls: [firstImage],
      audio_urls: [audio],
    })
    expectNoLegacyVideoFields(payload)
  })

  test('builds the image-audio lipsync contract without prompt or seed', () => {
    const payload = buildVideoGenerationPayload(
      '',
      [image(firstImage), audioReference()],
      playgroundConfig(models.lipsync, { video_resolution: '1080p' })
    )

    expect(payload).toEqual({
      model: models.lipsync,
      group: 'auto',
      duration_seconds: 5,
      video_resolution: '1080p',
      ratio: '16:9',
      image_urls: [firstImage],
      audio_urls: [audio],
    })
    expectNoLegacyVideoFields(payload)
  })

  test('builds the first-last-frame contract with explicit slots', () => {
    const payload = buildVideoGenerationPayload(
      'Transition naturally',
      [
        image(firstImage, 'first_frame'),
        image(secondImage, 'last_frame'),
      ],
      playgroundConfig(models.firstLast)
    )

    expect(payload).toEqual({
      model: models.firstLast,
      group: 'auto',
      prompt: 'Transition naturally',
      duration_seconds: 5,
      video_resolution: '768p',
      ratio: '16:9',
      first_frame: firstImage,
      last_frame: secondImage,
    })
    expectNoLegacyVideoFields(payload)
  })
})

describe('MiniMax H3 Playground model policy', () => {
  test('exposes the union of ratios and resolutions on the unified model', () => {
    expect(getVideoRatioOptionsForModel(models.text)).toEqual([
      '16:9',
      '9:16',
      '1:1',
    ])
    expect(getVideoRatioOptionsForModel(models.multimodal)).toEqual([
      '16:9',
      '9:16',
      '1:1',
    ])
    expect(getVideoResolutionOptionsForModel(models.firstLast)).toEqual([
      '480p',
      '768p',
      '1080p',
    ])
    expect(getVideoResolutionOptionsForModel(models.lipsync)).toEqual([
      '480p',
      '768p',
      '1080p',
    ])
  })

  test('intersects catalog resolutions and normalizes stale selections', () => {
    expect(
      getVideoResolutionOptionsForModel(models.reference, ['720p', '1080p'])
    ).toEqual(['1080p'])
    expect(
      normalizeVideoResolutionForModel(models.text, '720p')
    ).toBe('480p')
    expect(normalizeVideoRatioForModel(models.multimodal, '1:1')).toBe('1:1')
  })

  test('uses one-second duration steps and leaves route-specific limits to validation', () => {
    expect(getVideoDurationRangeForModel(models.reference, '1080p')).toEqual({
      min: 1,
      max: 15,
      step: 1,
    })
    expect(getVideoDurationRangeForModel(models.lipsync, '1080p')).toEqual({
      min: 1,
      max: 15,
      step: 1,
    })
    expect(normalizeVideoDurationForModel(models.reference, 15, '1080p')).toBe(15)
    expect(normalizeVideoDurationForModel(models.text, 6.7, '768p')).toBe(7)
  })

  test('infers a route from the unified model parameters', () => {
    const textIssue = validateMinimaxH3VideoInput({
      model: models.text,
      prompt: 'Generate a clip',
      references: [image(firstImage)],
      duration: 5,
      resolution: '768p',
      ratio: '16:9',
    })
    const referenceIssue = validateMinimaxH3VideoInput({
      model: models.reference,
      prompt: 'Generate a clip',
      references: [image(firstImage), audioReference()],
      duration: 5,
      resolution: '768p',
      ratio: '16:9',
    })

    expect(textIssue).toBeNull()
    expect(referenceIssue).toBeNull()
  })

  test('keeps all supported image and audio inputs for route inference', () => {
    const files = [
      inputFile('first.png', 'image/png'),
      inputFile('second.png', 'image/png'),
      inputFile('voice.wav', 'audio/wav'),
      inputFile('clip.mp4', 'video/mp4'),
    ]

    const expected = ['first.png', 'second.png', 'voice.wav']
    expect(
      normalizeMinimaxH3InputFiles(models.text, files).map((file) => file.id)
    ).toEqual(expected)
    expect(
      normalizeMinimaxH3InputFiles(models.reference, files).map(
        (file) => file.id
      )
    ).toEqual(expected)
    expect(
      normalizeMinimaxH3InputFiles(models.multimodal, files).map(
        (file) => file.id
      )
    ).toEqual(expected)
    expect(
      normalizeMinimaxH3InputFiles(models.lipsync, files).map(
        (file) => file.id
      )
    ).toEqual(expected)
    expect(
      normalizeMinimaxH3InputFiles(models.firstLast, files).map((file) => ({
        id: file.id,
        role: file.role,
      }))
    ).toEqual([
      { id: 'first.png', role: undefined },
      { id: 'second.png', role: undefined },
      { id: 'voice.wav', role: undefined },
    ])
  })

  test('validates prompt, media roles, integer duration, and public URLs', () => {
    expect(
      validateMinimaxH3VideoInput({
        model: models.firstLast,
        prompt: '',
        references: [],
        duration: 5,
        resolution: '768p',
        ratio: '16:9',
      })?.key
    ).toBe('This MiniMax H3 model requires a prompt')

    expect(
      validateMinimaxH3VideoInput({
        model: models.firstLast,
        prompt: 'Transition',
        references: [
          image(firstImage, 'first_frame'),
          image(secondImage),
        ],
        duration: 5,
        resolution: '768p',
        ratio: '16:9',
      })?.key
    ).toBe('MiniMax H3 requires both first and last frames')

    expect(
      validateMinimaxH3VideoInput({
        model: models.reference,
        prompt: 'Animate',
        references: [image(firstImage)],
        duration: 5.5,
        resolution: '768p',
        ratio: '16:9',
      })?.key
    ).toContain('duration must be an integer')

    expect(
      validateMinimaxH3VideoInput({
        model: models.reference,
        prompt: 'Animate',
        references: [image('http://192.168.1.8/private.png')],
        duration: 5,
        resolution: '768p',
        ratio: '16:9',
        requirePublicUrls: true,
      })?.key
    ).toContain('public HTTP(S) URL')
  })

  test('recognizes normalized model names and public reference URLs', () => {
    expect(
      getMinimaxH3ModelSpec('AP_MINIMAX_H3')?.kind
    ).toBe('auto')
    expect(getMinimaxH3ModelSpec('ap-minimax-h3-text-to-video')).toBeUndefined()
    expect(isPublicHttpReferenceUrl(firstImage)).toBe(true)
    expect(isPublicHttpReferenceUrl('data:image/png;base64,AAAA')).toBe(false)
    expect(isPublicHttpReferenceUrl('http://localhost/image.png')).toBe(false)
  })
})
