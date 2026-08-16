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
import { api } from '@/lib/api'
import type {
  ApiResponse,
  CharacterVideoInput,
  VirtualCharacter,
  VirtualCharacterAIPDDCatalogSyncResult,
  VirtualCharacterConfig,
  VirtualCharacterListData,
  VirtualCharacterListParams,
  VirtualCharacterSettings,
  VirtualCharacterTaskHistory,
  VirtualCharacterValidationSession,
} from './types'

export const virtualCharacterQueryKeys = {
  all: ['virtual-characters'] as const,
  list: (params: VirtualCharacterListParams) =>
    [
      ...virtualCharacterQueryKeys.all,
      'list',
      params.scope,
      params.page ?? 1,
      params.pageSize ?? 20,
      params.keyword ?? '',
      params.nationality ?? '',
      params.gender ?? '',
      params.ageBand ?? '',
      params.status ?? '',
      params.sourceType ?? '',
    ] as const,
  detail: (id: number) =>
    [...virtualCharacterQueryKeys.all, 'detail', id] as const,
  history: (page: number) =>
    [...virtualCharacterQueryKeys.all, 'history', page] as const,
  config: () => [...virtualCharacterQueryKeys.all, 'config'] as const,
  settings: () => [...virtualCharacterQueryKeys.all, 'settings'] as const,
  validation: (id: string) =>
    [...virtualCharacterQueryKeys.all, 'validation', id] as const,
  userModels: () => [...virtualCharacterQueryKeys.all, 'user-models'] as const,
}

export async function listVirtualCharacters(
  params: VirtualCharacterListParams
): Promise<ApiResponse<VirtualCharacterListData>> {
  const res = await api.get('/api/virtual-characters', {
    params: {
      scope: params.scope,
      p: params.page ?? 1,
      page_size: params.pageSize ?? 20,
      keyword: params.keyword || undefined,
      nationality: params.nationality || undefined,
      gender: params.gender || undefined,
      age_band: params.ageBand || undefined,
      status: params.status || undefined,
      source_type: params.sourceType || undefined,
    },
  })
  return res.data
}

export async function getVirtualCharacter(
  id: number
): Promise<ApiResponse<VirtualCharacter>> {
  const res = await api.get(`/api/virtual-characters/${id}`)
  return res.data
}

export async function getVirtualCharacterConfig(): Promise<
  ApiResponse<VirtualCharacterConfig>
> {
  const res = await api.get('/api/virtual-characters/config')
  return res.data
}

export async function createVirtualCharacter(input: {
  name: string
  description: string
  tags: string[]
  file: File
}): Promise<ApiResponse<VirtualCharacter>> {
  const form = new FormData()
  form.append('name', input.name)
  form.append('description', input.description)
  form.append('tags', JSON.stringify(input.tags))
  form.append('file', input.file)
  const res = await api.post('/api/virtual-characters', form)
  return res.data
}

export function virtualCharacterPreviewURL(characterId: number): string {
  return `/api/virtual-characters/${characterId}/preview`
}

export async function createValidationSession(input: {
  name: string
  description: string
  tags: string[]
  language: 'zh' | 'en'
}): Promise<ApiResponse<VirtualCharacterValidationSession>> {
  const res = await api.post(
    '/api/virtual-characters/validation-sessions',
    input
  )
  return res.data
}

export async function uploadRealPersonAsset(
  id: number,
  file: File
): Promise<ApiResponse<VirtualCharacter>> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post(`/api/virtual-characters/${id}/asset`, form)
  return res.data
}

export async function cancelValidationSession(
  id: string
): Promise<ApiResponse<{ id: string; cancelled: boolean }>> {
  const res = await api.delete(
    `/api/virtual-characters/validation-sessions/${encodeURIComponent(id)}`
  )
  return res.data
}

export async function getValidationSession(
  id: string
): Promise<ApiResponse<VirtualCharacterValidationSession>> {
  const res = await api.get(
    `/api/virtual-characters/validation-sessions/${encodeURIComponent(id)}`
  )
  return res.data
}

export async function updateVirtualCharacter(
  id: number,
  input: { name: string; description: string; tags: string[] }
): Promise<ApiResponse<VirtualCharacter>> {
  const res = await api.put(`/api/virtual-characters/${id}`, input)
  return res.data
}

export async function deleteVirtualCharacter(
  id: number
): Promise<ApiResponse<{ id: number; status: string }>> {
  const res = await api.delete(`/api/virtual-characters/${id}`)
  return res.data
}

export async function syncRealPersonVirtualCharacter(
  id: number
): Promise<ApiResponse<VirtualCharacter>> {
  const res = await api.post(`/api/virtual-characters/${id}/sync`)
  return res.data
}

export async function getVirtualCharacterHistory(
  page: number,
  pageSize = 20
): Promise<ApiResponse<VirtualCharacterTaskHistory>> {
  const res = await api.get('/api/virtual-characters/tasks', {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function createCharacterVideo(
  input: CharacterVideoInput
): Promise<{ id?: string; task_id?: string; error?: { message?: string } }> {
  const prompt = input.prompt.includes('图片1')
    ? input.prompt
    : `以图片1中的角色为主体，${input.prompt}`
  const res = await api.post(
    '/pg/video/generations',
    {
      character_id: input.character_id,
      model: input.model,
      prompt,
      duration: input.duration,
      metadata: {
        ratio: input.ratio,
        resolution: input.resolution,
      },
    },
    { skipErrorHandler: true } as Record<string, unknown>
  )
  return res.data
}

export async function getVirtualCharacterSettings(): Promise<
  ApiResponse<VirtualCharacterSettings>
> {
  const res = await api.get('/api/virtual-characters/admin/settings')
  return res.data
}

export async function updateVirtualCharacterSettings(input: {
  enabled: boolean
  quota_plan: 'free' | 'paid' | 'custom'
  create_asset_qpm: number
  access_key?: string
  secret_key?: string
  region: string
  project_name: string
  global_limit: number
  real_person_limit: number
  real_person_enabled: boolean
  account_asset_cap: number
}): Promise<ApiResponse<VirtualCharacterSettings>> {
  const res = await api.put('/api/virtual-characters/admin/settings', input)
  return res.data
}

export async function testVirtualCharacterProvider(): Promise<
  ApiResponse<{ status: string; checked_at: number }>
> {
  const res = await api.post('/api/virtual-characters/admin/provider/test')
  return res.data
}

export async function syncVirtualCharacterCatalogFromAIPDD(
  force = false
): Promise<ApiResponse<VirtualCharacterAIPDDCatalogSyncResult>> {
  const res = await api.post('/api/virtual-characters/admin/sync-aipdd', {
    force,
  })
  return res.data
}

export async function getAvailableVideoModels(): Promise<string[]> {
  const res = await api.get('/api/user/models', {
    params: { endpoint_type: 'openai-video', details: true },
  })
  if (!res.data?.success || !Array.isArray(res.data.data)) return []
  const models = res.data.data.flatMap((item: unknown) => {
    if (typeof item === 'string') return [item]
    if (!item || typeof item !== 'object') return []
    const value = (item as { model?: unknown }).model
    return typeof value === 'string' ? [value] : []
  })
  return models.filter((name: string) =>
    name.toLowerCase().includes('seedance')
  )
}
