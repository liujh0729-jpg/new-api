export type VirtualCharacterScope = 'public' | 'private'
export type VirtualCharacterStatus =
  'creating' | 'active' | 'blocked' | 'offline' | 'deleting' | 'failed'
export type VirtualCharacterAssetStatus =
  'Processing' | 'Active' | 'Failed' | 'Deleting'
export type VirtualCharacterValidationSessionStatus =
  'pending' | 'succeeded' | 'failed' | 'expired'

export interface VirtualCharacterAsset {
  id: number
  name: string
  asset_type: 'Image' | 'Video' | 'Audio'
  status: VirtualCharacterAssetStatus
  is_primary: boolean
  cover_url?: string
  mime_type?: string
  file_size?: number
  last_error?: string
  provider_asset_id?: string
  created_at: number
  updated_at: number
}

export interface VirtualCharacter {
  id: number
  scope: VirtualCharacterScope
  source_type: 'volc_preset' | 'volc_real_person'
  name: string
  description: string
  tags: string[]
  status: VirtualCharacterStatus
  validation_status: 'unverified' | 'accepted' | 'rejected'
  cover_url?: string
  primary_asset_id?: number
  assets: VirtualCharacterAsset[]
  created_at: number
  updated_at: number
  last_error?: string
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
  image_max_mb: number
  video_max_mb: number
  audio_max_mb: number
  task_retention_days: number
  official_enabled: boolean
  real_person_enabled: boolean
}

export interface VirtualCharacterValidationSession {
  id: string
  status: VirtualCharacterValidationSessionStatus
  launch_url?: string
  expires_at: number
  character_id?: number
  last_error?: string
  created_at: number
  updated_at: number
}

export interface VirtualCharacterTask {
  task_id: string
  character_id: number
  character_name: string
  character_scope: VirtualCharacterScope
  character_asset_id?: number
  character_asset_name?: string
  provider_asset_id?: string
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

export interface VirtualCharacterSettings {
  enabled: boolean
  official_enabled: boolean
  real_person_enabled: boolean
  access_key_masked: string
  secret_key_masked: string
  region: string
  project_name: string
  channel_id: number
  crypto_ready: boolean
  last_check_status?: string
  last_check_error?: string
  last_checked_at?: number
  global_limit: number
  models: string[]
  default_model: string
  channels: Array<{
    id: number
    name: string
    type: number
    models: string
  }>
  catalog?: {
    version: string
    content_hash: string
    total: number
    created: number
    updated: number
    offlined: number
    created_at: number
  }
}

export interface CharacterVideoInput {
  character_id: number
  character_asset_id?: number
  model: string
  prompt: string
  duration: number
  ratio: string
  resolution: string
}
