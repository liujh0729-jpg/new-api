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
export type VirtualCharacterScope = 'public' | 'private'
export type VirtualCharacterStatus =
  | 'creating'
  | 'active'
  | 'blocked'
  | 'offline'
  | 'deleting'
  | 'failed'
export type VirtualCharacterValidationSessionStatus =
  | 'pending'
  | 'succeeded'
  | 'failed'
  | 'expired'
  | 'cancelled'
export type VirtualCharacterSourceType =
  | 'volc_preset'
  | 'volc_aigc'
  | 'volc_real_person'

export type VirtualCharacterAuthorizationStatus =
  | 'pending'
  | 'synchronizing'
  | 'active'
  | 'ambiguous'
  | 'provider_unavailable'
  | 'expired'
  | 'revoked'
  | 'failed'

export interface VirtualCharacterAuthorization {
  status: VirtualCharacterAuthorizationStatus
  provider_group_status?: string
  provider_asset_status?: string
  provider_checked_at?: number
  authorized_at?: number
  revoked_at?: number
  expired_at?: number
  last_error?: string
}

export interface VirtualCharacter {
  id: number
  scope: VirtualCharacterScope
  source_type: VirtualCharacterSourceType
  name: string
  description: string
  tags: string[]
  nationality?: string
  gender?: string
  age_min?: number
  age_max?: number
  occupation?: string
  temperament?: string
  status: VirtualCharacterStatus
  validation_status: 'unverified' | 'accepted' | 'rejected'
  cover_url?: string
  provider_asset_id?: string
  mime_type?: string
  file_size?: number
  created_at: number
  updated_at: number
  last_error?: string
  authorization?: VirtualCharacterAuthorization
}

export interface VirtualCharacterListParams {
  scope: 'private' | 'public'
  page?: number
  pageSize?: number
  keyword?: string
  nationality?: string
  gender?: string
  ageBand?: string
  status?: string
  sourceType?: VirtualCharacterSourceType
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
  real_person_used?: number
  real_person_limit?: number
}

export interface VirtualCharacterConfig {
  image_max_mb: number
  task_retention_days: number
  official_enabled: boolean
  virtual_enabled: boolean
  real_person_enabled: boolean
  account_asset_cap?: number
  real_person_limit?: number
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
  provider_asset_id?: string
  link_status: string
  created_at: number
  error?: string
  references?: Array<{
    character_id: number
    character_name: string
    character_scope: VirtualCharacterScope
    source_type: VirtualCharacterSourceType
    provider_asset_id: string
    authorization_snapshot?: {
      authorization_status?: string
      agreement_reference?: string
      consent_receipt_hash?: string
      provider_group_status?: string
      provider_asset_status?: string
      provider_checked_at?: number
      authorized_at?: number
      captured_at: number
    }
  }>
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

export type VirtualCharacterQuotaPlan = 'free' | 'paid' | 'custom'

export interface VirtualCharacterSettings {
  enabled: boolean
  quota_plan: VirtualCharacterQuotaPlan
  create_asset_qpm: number
  access_key_masked: string
  secret_key_masked: string
  region: string
  project_name: string
  crypto_ready: boolean
  last_check_status?: string
  last_check_error?: string
  last_checked_at?: number
  global_limit: number
  real_person_limit: number
  real_person_enabled: boolean
  virtual_enabled: boolean
  account_asset_cap: number
  catalog?: {
    version: string
    content_hash: string
    total: number
    created: number
    updated: number
    offlined: number
    created_at: number
  }
  catalog_last_synced_at?: number
}

export interface VirtualCharacterAIPDDCatalogSyncResult {
  version: string
  revision: string
  total: number
  created: number
  updated: number
  offlined: number
  skipped: boolean
  skip_reason?: string
  last_synced_at: number
}

export interface CharacterVideoInput {
  character_id: number
  model: string
  prompt: string
  duration: number
  ratio: string
  resolution: string
}
