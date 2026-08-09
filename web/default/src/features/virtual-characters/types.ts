export type VirtualCharacterScope = 'public' | 'private'
export type VirtualCharacterStatus =
  'creating' | 'active' | 'blocked' | 'offline' | 'deleting' | 'failed'

export interface VirtualCharacter {
  id: number
  scope: VirtualCharacterScope
  name: string
  description: string
  tags: string[]
  status: VirtualCharacterStatus
  validation_status: 'unverified' | 'accepted' | 'rejected'
  cover_url: string
  mime_type?: string
  file_size?: number
  created_at: number
  updated_at: number
  last_error?: string
  asset_id?: string
  public_channel_id?: number
}

export interface PageData<T> {
  page: number
  page_size: number
  total: number
  items: T[]
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data: T
}

export interface VirtualCharacterListData {
  page: PageData<VirtualCharacter>
  used?: number
  limit?: number
}

export interface VirtualCharacterConfig {
  models: string[]
  default_model: string
  max_file_mb: number
  task_retention_days: number
}

export interface VirtualCharacterTask {
  task_id: string
  character_id: number
  character_name: string
  character_scope: VirtualCharacterScope
  link_status: string
  created_at: number
  error?: string
  task?: {
    task_id: string
    status: string
    progress: string
    fail_reason?: string
    result_url?: string
    submit_time: number
    finish_time?: number
    model?: string
    properties?: {
      origin_model_name?: string
      upstream_model_name?: string
    }
  }
}

export interface VirtualCharacterTaskHistory {
  page: PageData<VirtualCharacterTask>
  retention_days: number
  output_notice: string
}

export interface PublicVirtualCharacterInput {
  name: string
  description: string
  tags: string[]
  cover_url: string
  asset_id: string
  public_channel_id: number
  status: 'active' | 'offline'
}

export interface VirtualCharacterSettings {
  global_limit: number
  models: string[]
  default_model: string
  public_channels: Array<{
    id: number
    name: string
    type: number
    models: string
  }>
}

export interface CharacterVideoInput {
  character_id: number
  model: string
  prompt: string
  duration: number
  ratio: string
  resolution: string
}
