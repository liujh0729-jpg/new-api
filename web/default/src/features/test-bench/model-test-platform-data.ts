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
  | 'range'
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
  range?: {
    min: number
    max: number
    step?: number
    unit?: string
  }
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
      type: 'range',
      optional: true,
      example: '5',
      range: {
        min: 4,
        max: 15,
        step: 1,
        unit: 's',
      },
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
    id: 'ap-chat-3.8-flash',
    labelKey: 'AIPDD text model',
    vendor: 'AIPDD',
    kind: 'text',
    mode: 'sync',
    endpoint: '/v1/chat/completions',
    method: 'POST',
    billingKey: 'Token-based',
    noteKey: 'OpenAI-compatible chat completions on the AIPDD text endpoint.',
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
        example: 'Summarize what an AI gateway does in three sentences.',
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
    id: 'ap-qwen3-embedding:0.6b',
    labelKey: 'AIPDD embedding model',
    vendor: 'AIPDD',
    kind: 'text',
    mode: 'sync',
    endpoint: '/v1/embeddings',
    method: 'POST',
    billingKey: 'Token-based',
    noteKey: 'Text in, embedding vector out. Billed on input tokens.',
    fields: [
      {
        key: 'input',
        labelKey: 'Input text',
        type: 'textarea',
        required: true,
        example: 'The food was delicious and the waiter.',
      },
    ],
  },
  {
    id: 'ap-image-2.0',
    labelKey: 'AIPDD image generation',
    vendor: 'AIPDD',
    kind: 'image',
    mode: 'sync',
    endpoint: '/v1/images/generations',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Synchronous image generation. Reference images turn the same model into an edit request on /v1/images/edits.',
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
        example: '1024x1024',
        options: ['1024x1024', '1024x1536', '1536x1024'],
      },
      {
        key: 'quality',
        labelKey: 'Quality',
        type: 'select',
        optional: true,
        example: 'standard',
        options: ['standard', 'hd'],
      },
      {
        key: 'n',
        labelKey: 'Number of images',
        type: 'number',
        optional: true,
        example: '1',
        attrs: { min: '1', max: '4', step: '1' },
      },
    ],
  },
  {
    id: 'ap-qwen-image-edit',
    labelKey: 'AIPDD image edit',
    vendor: 'AIPDD',
    kind: 'image',
    mode: 'sync',
    endpoint: '/v1/images/edits',
    method: 'POST',
    billingKey: 'Per Request',
    noteKey:
      'Upload an image or pass an image URL, then describe the change. Sent as multipart.',
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
        labelKey: 'Edit instruction',
        type: 'textarea',
        required: true,
        example: 'replace the background with a plain studio backdrop',
      },
    ],
  },
  {
    id: 'ap-ltx-2.5',
    labelKey: 'AIPDD video generation',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Second',
    noteKey:
      'Text-to-video or image-to-video. Supplying image switches the request to image-to-video.',
    fields: [
      {
        key: 'prompt',
        labelKey: 'Video prompt',
        type: 'textarea',
        required: true,
        example: 'a cinematic slow push-in shot of a futuristic city at sunset',
      },
      {
        key: 'image',
        labelKey: 'First frame image URL',
        type: 'url',
        optional: true,
        example: '',
        file: true,
        fileLabelKey: 'Upload image',
      },
      {
        key: 'duration',
        labelKey: 'Duration',
        type: 'select',
        optional: true,
        example: '5',
        options: ['5', '10'],
      },
    ],
  },
  {
    id: 'ap-minimax-h3',
    labelKey: 'AIPDD MiniMax H3 video generation',
    vendor: 'AIPDD',
    kind: 'video',
    mode: 'async',
    endpoint: '/v1/videos',
    queryEndpoint: '/v1/videos/{task_id}',
    method: 'POST',
    billingKey: 'Per Second',
    noteKey:
      'This model is billed per second of video and requires both duration_seconds and video_resolution.',
    fields: [
      {
        key: 'prompt',
        labelKey: 'Video prompt',
        type: 'textarea',
        required: true,
        example: 'a cinematic slow push-in shot of a futuristic city at sunset',
      },
      {
        key: 'duration_seconds',
        labelKey: 'Duration (seconds)',
        type: 'range',
        optional: true,
        example: '1',
        range: { min: 1, max: 15, step: 1, unit: 's' },
      },
      {
        key: 'video_resolution',
        labelKey: 'Video resolution',
        type: 'select',
        optional: true,
        example: '480p',
        options: ['480p', '768p', '1080p'],
      },
      {
        key: 'ratio',
        labelKey: 'Aspect ratio',
        type: 'select',
        optional: true,
        example: '16:9',
        options: ['16:9', '9:16', '1:1'],
      },
    ],
  },
  {
    id: 'ap-indextts-2',
    labelKey: 'AIPDD voice cloning',
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
  {
    id: 'ap-funasr',
    labelKey: 'AIPDD speech transcription',
    vendor: 'AIPDD',
    kind: 'text',
    mode: 'sync',
    endpoint: '/v1/audio/transcriptions',
    method: 'POST',
    billingKey: 'Per Second',
    noteKey:
      'Upload an audio file or pass its URL. Billed on the processed duration the upstream reports.',
    fields: [
      {
        key: 'file',
        labelKey: 'Audio file URL',
        type: 'url',
        required: true,
        example: 'https://example.com/audio.mp3',
        file: true,
        fileLabelKey: 'Upload audio',
      },
      {
        key: 'language',
        labelKey: 'Language',
        type: 'select',
        optional: true,
        example: 'zh',
        options: ['zh', 'en', 'ja'],
      },
      {
        key: 'response_format',
        labelKey: 'Response format',
        type: 'select',
        optional: true,
        example: 'json',
        options: ['json', 'text', 'verbose_json', 'srt', 'vtt'],
      },
    ],
  },
]
