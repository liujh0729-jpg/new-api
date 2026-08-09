import { api } from '@/lib/api'
import type {
  ApiResponse,
  CharacterVideoInput,
  VirtualCharacter,
  VirtualCharacterAsset,
  VirtualCharacterConfig,
  VirtualCharacterListData,
  VirtualCharacterSettings,
  VirtualCharacterTaskHistory,
  VirtualCharacterValidationSession,
} from './types'

export const virtualCharacterQueryKeys = {
  all: ['virtual-characters'] as const,
  list: (scope: string, page: number) =>
    [...virtualCharacterQueryKeys.all, 'list', scope, page] as const,
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
  scope: 'private' | 'public',
  page: number,
  pageSize = 20
): Promise<ApiResponse<VirtualCharacterListData>> {
  const res = await api.get('/api/virtual-characters', {
    params: { scope, p: page, page_size: pageSize },
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

export async function getValidationSession(
  id: string
): Promise<ApiResponse<VirtualCharacterValidationSession>> {
  const res = await api.get(
    `/api/virtual-characters/validation-sessions/${encodeURIComponent(id)}`
  )
  return res.data
}

export async function uploadVirtualCharacterAsset(input: {
  characterId: number
  file: File
  name: string
  assetType: 'Image' | 'Video' | 'Audio'
}): Promise<ApiResponse<VirtualCharacterAsset>> {
  const form = new FormData()
  form.append('file', input.file)
  form.append('name', input.name)
  form.append('asset_type', input.assetType)
  const res = await api.post(
    `/api/virtual-characters/${input.characterId}/assets`,
    form
  )
  return res.data
}

export async function setPrimaryVirtualCharacterAsset(
  characterId: number,
  assetId: number
): Promise<ApiResponse<{ character_id: number; asset_id: number }>> {
  const res = await api.put(
    `/api/virtual-characters/${characterId}/assets/${assetId}/primary`
  )
  return res.data
}

export async function deleteVirtualCharacterAsset(
  characterId: number,
  assetId: number
): Promise<ApiResponse<{ id: number; status: string }>> {
  const res = await api.delete(
    `/api/virtual-characters/${characterId}/assets/${assetId}`
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
      character_asset_id: input.character_asset_id,
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
  official_enabled: boolean
  real_person_enabled: boolean
  access_key?: string
  secret_key?: string
  region: string
  project_name: string
  channel_id: number
  global_limit: number
  models: string[]
  default_model: string
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

export async function importPublicVirtualCharacters(
  file: File,
  dryRun: boolean,
  version?: string
): Promise<ApiResponse<Record<string, unknown>>> {
  const form = new FormData()
  form.append('file', file)
  form.append('dry_run', String(dryRun))
  if (version) form.append('version', version)
  const res = await api.post('/api/virtual-characters/admin/import', form)
  return res.data
}

export async function setVirtualCharacterUserLimit(
  userId: number,
  limit: number
): Promise<
  ApiResponse<{ user_id: number; limit: number; overridden: boolean }>
> {
  const res = await api.put(
    `/api/virtual-characters/admin/users/${userId}/limit`,
    { limit }
  )
  return res.data
}

export async function getAvailableVideoModels(): Promise<string[]> {
  const res = await api.get('/api/user/models', {
    params: { endpoint_type: 'openai-video', details: true },
  })
  if (!res.data?.success || !Array.isArray(res.data.data)) return []
  return res.data.data.flatMap((item: unknown) => {
    if (typeof item === 'string') return [item]
    if (!item || typeof item !== 'object') return []
    const value = (item as { model?: unknown }).model
    return typeof value === 'string' ? [value] : []
  })
}
