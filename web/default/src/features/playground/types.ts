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
// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export type MessageActivity = 'image_generation' | 'video_generation'

export interface MessageVersion {
  id: string
  content: string
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  images?: GeneratedImage[]
  videos?: GeneratedVideo[]
  seedanceReferences?: SeedanceReference[]
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  activity?: MessageActivity
  status?: MessageStatus
  errorCode?: string | null
  taskId?: string
  taskType?: 'image' | 'video'
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
  mime_type?: string
  revised_prompt?: string
}

export interface GeneratedVideo {
  url: string
  task_id?: string
  mime_type?: string
}

export type SeedanceReferenceKind = 'image' | 'video' | 'audio'

export interface SeedanceReference {
  kind: SeedanceReferenceKind
  url: string
  filename?: string
  media_type?: string
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface ImageGenerationRequest {
  model: string
  group?: string
  prompt: string
  image?: string
  images?: string[]
  size?: string
  quality?: string
  n?: number
}

export interface ImageGenerationResponse {
  created?: number
  data?: GeneratedImage[]
  id?: string
  task_id?: string
  object?: string
  status?: string
  model?: string
  metadata?: Record<string, unknown>
}

export interface VideoGenerationContentItem {
  type: 'image_url' | 'video_url' | 'audio_url'
  role?: 'reference_image' | 'reference_video' | 'reference_audio'
  image_url?: {
    url: string
  }
  video_url?: {
    url: string
  }
  audio_url?: {
    url: string
  }
}

export interface VideoGenerationRequest {
  model: string
  group?: string
  prompt: string
  duration?: number
  seconds?: string
  size?: string
  metadata?: {
    content?: VideoGenerationContentItem[]
    ratio?: string
    resolution?: string
  }
}

export interface VideoGenerationResponse {
  id?: string
  task_id?: string
  object?: string
  model?: string
  status?: string
  progress?: number
  metadata?: Record<string, unknown>
  error?: {
    message?: string
    code?: string
  }
}

export interface TaskFetchResponse {
  code?: string
  data?: unknown
  error?: unknown
}

export type PlaygroundMode = 'chat' | 'image' | 'video'

// Configuration types
export interface PlaygroundConfig {
  mode: PlaygroundMode
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
  image_size: string
  image_quality: string
  image_count: number
  video_ratio: string
  video_duration: number
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
}

export interface PlaygroundConversation {
  id: string
  title: string
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  messages: Message[]
  createdAt: number
  updatedAt: number
}

export interface PlaygroundConversationState {
  version: 1
  activeConversationId: string
  conversations: PlaygroundConversation[]
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
}
