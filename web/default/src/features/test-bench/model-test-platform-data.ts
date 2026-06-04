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
export type TestFieldType =
  | 'text'
  | 'textarea'
  | 'number'
  | 'select'
  | 'checkbox'
  | 'url'

export type TestModelKind = 'text' | 'image' | 'video' | 'audio'
export type TestModelMode = 'sync' | 'async'
export type TestModelBuilder = 'chat' | 'default'

export type TestField = {
  key: string
  labelKey: string
  type: TestFieldType
  required?: boolean
  optional?: boolean
  example?: string | boolean
  options?: string[]
  attrs?: Record<string, string>
  file?: boolean
  fileLabelKey?: string
}

export type TestModel = {
  id: string
  labelKey: string
  label?: string
  vendor: string
  kind: TestModelKind
  mode: TestModelMode
  endpoint: string
  queryEndpoint?: string
  method: 'POST'
  billingKey: string
  noteKey: string
  fields: TestField[]
  builder?: TestModelBuilder
}

function videoSeedanceFields(examplePrompt: string): TestField[] {
  return [
    {
      key: 'prompt',
      labelKey: 'Video prompt',
      type: 'textarea',
      required: true,
      example: examplePrompt,
    },
    {
      key: 'image',
      labelKey: 'First frame image URL',
      type: 'url',
      optional: true,
      example: '',
    },
    {
      key: 'duration',
      labelKey: 'Duration',
      type: 'select',
      optional: true,
      example: '5',
      options: ['5', '10'],
    },
    {
      key: 'ratio',
      labelKey: 'Aspect ratio',
      type: 'select',
      optional: true,
      example: '16:9',
      options: ['16:9', '9:16', '1:1'],
    },
    {
      key: 'resolution',
      labelKey: 'Resolution',
      type: 'text',
      optional: true,
      example: '',
    },
    {
      key: 'watermark',
      labelKey: 'Watermark',
      type: 'checkbox',
      optional: true,
      example: false,
    },
  ]
}

export const MODEL_TEST_STORAGE_KEY = 'newapi-model-test-platform-v1'
export const MODEL_TEST_HISTORY_KEY = 'newapi-model-test-history-v1'
export const DEFAULT_TEST_MODEL = 'deepseek-v4-pro'

export const TEST_MODELS: TestModel[] = [
  {
    id: 'deepseek-v4-pro',
    labelKey: 'DeepSeek standard LLM',
    vendor: 'DeepSeek',
    kind: 'text',
    mode: 'sync',
    endpoint: '/v1/chat/completions',
    method: 'POST',
    billingKey: 'Token-based',
    noteKey:
      'Do not use /v1/responses here. This test uses Chat Completions for the minimal relay path.',
    builder: 'chat',
    fields: [
      {
        key: 'system',
        labelKey: 'System prompt',
        type: 'textarea',
        optional: true,
        example: 'You are a helpful assistant.',
      },
      {
        key: 'prompt',
        labelKey: 'User message',
        type: 'textarea',
        required: true,
        example: 'Explain the role of the NewAPI gateway in three sentences.',
      },
      {
        key: 'temperature',
        labelKey: 'temperature',
        type: 'number',
        optional: true,
        example: '0.6',
        attrs: { min: '0', max: '2', step: '0.1' },
      },
      {
        key: 'max_tokens',
        labelKey: 'max_tokens',
        type: 'number',
        optional: true,
        example: '512',
        attrs: { min: '1', step: '1' },
      },
      {
        key: 'stream',
        labelKey: 'stream',
        type: 'checkbox',
        optional: true,
        example: false,
      },
    ],
  },
  {
    id: 'doubao-seedream-5.0-lite',
    labelKey: 'Doubao Seedream image generation',
    vendor: 'Doubao',
    kind: 'image',
    mode: 'sync',
    endpoint: '/v1/images/generations',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Text-to-image returns synchronously. Seedream 5.0 lite supports 2K/3K/4K presets or matching pixel sizes.',
    fields: [
      {
        key: 'prompt',
        labelKey: 'Prompt',
        type: 'textarea',
        required: true,
        example: 'a small red cube on a white background, studio lighting',
      },
      {
        key: 'size',
        labelKey: 'Size',
        type: 'select',
        optional: true,
        example: '2K',
        options: [
          '2K',
          '3K',
          '4K',
          '1920x1920',
          '2048x2048',
          '2560x1440',
          '1440x2560',
          '4096x4096',
        ],
      },
      {
        key: 'n',
        labelKey: 'Count',
        type: 'number',
        optional: true,
        example: '1',
        attrs: { min: '1', max: '4', step: '1' },
      },
      {
        key: 'watermark',
        labelKey: 'Watermark',
        type: 'checkbox',
        optional: true,
        example: false,
      },
      {
        key: 'image',
        labelKey: 'Reference image URL',
        type: 'url',
        optional: true,
        example: '',
      },
    ],
  },
  {
    id: 'doubao-seedream-4-5',
    labelKey: 'Doubao Seedream image generation',
    vendor: 'Doubao',
    kind: 'image',
    mode: 'sync',
    endpoint: '/v1/images/generations',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Text-to-image returns synchronously. Seedream 4.5 requires at least 3,686,400 pixels; response_format is usually omitted.',
    fields: [
      {
        key: 'prompt',
        labelKey: 'Prompt',
        type: 'textarea',
        required: true,
        example: 'a small red cube on a white background, studio lighting',
      },
      {
        key: 'size',
        labelKey: 'Size',
        type: 'select',
        optional: true,
        example: '1920x1920',
        options: ['1920x1920', '2560x1440', '1440x2560', '2048x2048'],
      },
      {
        key: 'n',
        labelKey: 'Count',
        type: 'number',
        optional: true,
        example: '1',
        attrs: { min: '1', max: '4', step: '1' },
      },
      {
        key: 'watermark',
        labelKey: 'Watermark',
        type: 'checkbox',
        optional: true,
        example: false,
      },
      {
        key: 'image',
        labelKey: 'Reference image URL',
        type: 'url',
        optional: true,
        example: '',
      },
    ],
  },
  {
    id: 'doubao-seedance-2.0',
    labelKey: 'Doubao Seedance 2.0 video generation',
    vendor: 'Doubao',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Text-to-video or image-to-video. Supplying image switches the request to image-to-video.',
    fields: videoSeedanceFields(
      'a cinematic slow push-in shot of a futuristic city at sunset'
    ),
  },
  {
    id: 'doubao-seedance-1.5',
    labelKey: 'Doubao Seedance 1.5 video generation',
    vendor: 'Doubao',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'A lower-cost option for validating the video relay path. Supplying image switches the request to image-to-video.',
    fields: videoSeedanceFields(
      'the camera slowly moves forward, natural motion, cinematic lighting'
    ),
  },
  {
    id: 'aipdd-flux-gguf',
    labelKey: 'AIPDD Flux image-to-image',
    vendor: 'AIPDD',
    kind: 'image',
    mode: 'async',
    endpoint: '/v1/images/generations',
    queryEndpoint: '/v1/images/generations/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'An image URL or uploaded image is required. Sending only prompt will fail.',
    fields: [
      {
        key: 'image',
        labelKey: 'Input image URL',
        type: 'url',
        required: true,
        example: 'https://example.com/input.png',
        file: true,
        fileLabelKey: 'Upload image',
      },
      {
        key: 'prompt',
        labelKey: 'Positive prompt',
        type: 'textarea',
        optional: true,
        example: 'a cinematic product photo, soft studio lighting',
      },
      {
        key: 'negative_prompt',
        labelKey: 'Negative prompt',
        type: 'textarea',
        optional: true,
        example: 'low quality, blurry',
      },
    ],
  },
  {
    id: 'aipdd-flux-gguf-t2i',
    labelKey: 'AIPDD Flux text-to-image',
    vendor: 'AIPDD',
    kind: 'image',
    mode: 'async',
    endpoint: '/v1/images/generations',
    queryEndpoint: '/v1/images/generations/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Pure text-to-image. prompt, text, or input can be used; this test sends prompt.',
    fields: [
      {
        key: 'prompt',
        labelKey: 'Prompt',
        type: 'textarea',
        required: true,
        example: 'a cinematic product photo, soft studio lighting',
      },
    ],
  },
  {
    id: 'aipdd-wan2.2-wanx',
    labelKey: 'AIPDD Wan2.2 image-to-video',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per second',
    noteKey: 'image and prompt are required. duration only supports 5 or 10.',
    fields: [
      {
        key: 'image',
        labelKey: 'Input image URL',
        type: 'url',
        required: true,
        example: 'https://example.com/input.png',
        file: true,
        fileLabelKey: 'Upload image',
      },
      {
        key: 'prompt',
        labelKey: 'Video prompt',
        type: 'textarea',
        required: true,
        example: 'slow camera push in, stable motion, cinematic',
      },
      {
        key: 'duration',
        labelKey: 'Duration',
        type: 'select',
        required: true,
        example: '5',
        options: ['5', '10'],
      },
    ],
  },
  {
    id: 'aipdd-wan2.2-animater',
    labelKey: 'AIPDD Wan2.2 subject replacement',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey: 'load_video, prompt, and negative_prompt are required.',
    fields: [
      {
        key: 'load_video',
        labelKey: 'Source video URL',
        type: 'url',
        required: true,
        example: 'https://example.com/source.mp4',
        file: true,
        fileLabelKey: 'Upload load_video',
      },
      {
        key: 'prompt',
        labelKey: 'Positive prompt',
        type: 'textarea',
        required: true,
        example: 'natural motion, stable subject',
      },
      {
        key: 'negative_prompt',
        labelKey: 'Negative prompt',
        type: 'textarea',
        required: true,
        example: 'low quality, distorted, flicker',
      },
    ],
  },
  {
    id: 'aipdd-mimic-motion',
    labelKey: 'AIPDD MimicMotion motion transfer',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey: 'Motion reference video and target subject image are required.',
    fields: [
      {
        key: 'motion_video',
        labelKey: 'Motion video URL',
        type: 'url',
        required: true,
        example: 'https://example.com/motion.mp4',
        file: true,
        fileLabelKey: 'Upload motion_video',
      },
      {
        key: 'appearance_image',
        labelKey: 'Subject image URL',
        type: 'url',
        required: true,
        example: 'https://example.com/person.png',
        file: true,
        fileLabelKey: 'Upload appearance_image',
      },
    ],
  },
  {
    id: 'aipdd-latentsync-1.5',
    labelKey: 'AIPDD Latentsync lip sync',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey: 'The LoadAudio field must keep its exact capitalization.',
    fields: [
      {
        key: 'video',
        labelKey: 'Input video URL',
        type: 'url',
        required: true,
        example: 'https://example.com/source.mp4',
        file: true,
        fileLabelKey: 'Upload video',
      },
      {
        key: 'LoadAudio',
        labelKey: 'Driving audio URL',
        type: 'url',
        required: true,
        example: 'https://example.com/speech.wav',
        file: true,
        fileLabelKey: 'Upload LoadAudio',
      },
    ],
  },
  {
    id: 'aipdd-indextts',
    labelKey: 'AIPDD IndexTTS voice cloning',
    vendor: 'AIPDD',
    kind: 'audio',
    mode: 'async',
    endpoint: '/v1/audio/speech',
    queryEndpoint: '/v1/audio/speech/{task_id}',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey: 'input/text and reference audio are required.',
    fields: [
      {
        key: 'input',
        labelKey: 'Synthesis text',
        type: 'textarea',
        required: true,
        example: 'Enter the text to synthesize here.',
      },
      {
        key: 'audio',
        labelKey: 'Reference audio URL',
        type: 'url',
        required: true,
        example: 'https://example.com/reference.wav',
        file: true,
        fileLabelKey: 'Upload audio',
      },
      {
        key: 'emotion_audio',
        labelKey: 'Emotion audio URL',
        type: 'url',
        optional: true,
        example: '',
        file: true,
        fileLabelKey: 'Upload emotion_audio',
      },
    ],
  },
]
