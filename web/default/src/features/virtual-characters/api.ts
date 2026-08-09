import { api } from '@/lib/api'
import type {
  ApiResponse,
  CharacterVideoInput,
  PageData,
  PublicVirtualCharacterInput,
  VirtualCharacter,
  VirtualCharacterConfig,
  VirtualCharacterListData,
  VirtualCharacterSettings,
  VirtualCharacterTaskHistory,
} from './types'

export const virtualCharacterQueryKeys = {
  all: ['virtual-characters'] as const,
  list: (scope: string, page: number, admin: boolean) =>
    [...virtualCharacterQueryKeys.all, 'list', scope, page, admin] as const,
  history: (page: number) =>
    [...virtualCharacterQueryKeys.all, 'history', page] as const,
  config: () => [...virtualCharacterQueryKeys.all, 'config'] as const,
  settings: () => [...virtualCharacterQueryKeys.all, 'settings'] as const,
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

export async function listAdminPublicCharacters(
  page: number,
  pageSize = 20
): Promise<ApiResponse<PageData<VirtualCharacter>>> {
  const res = await api.get('/api/virtual-characters/admin', {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function getVirtualCharacterConfig(): Promise<
  ApiResponse<VirtualCharacterConfig>
> {
  const res = await api.get('/api/virtual-characters/config')
  return res.data
}

export async function uploadVirtualCharacter(input: {
  file: File
  name: string
  description: string
  tags: string[]
}): Promise<ApiResponse<VirtualCharacter>> {
  const form = new FormData()
  form.append('file', input.file)
  form.append('name', input.name)
  form.append('description', input.description)
  form.append('tags', JSON.stringify(input.tags))
  form.append('non_real_person', 'true')
  const res = await api.post('/api/virtual-characters/upload', form)
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

export async function createPublicVirtualCharacter(
  input: PublicVirtualCharacterInput
): Promise<ApiResponse<VirtualCharacter>> {
  const res = await api.post('/api/virtual-characters/admin', input)
  return res.data
}

export async function updatePublicVirtualCharacter(
  id: number,
  input: PublicVirtualCharacterInput
): Promise<ApiResponse<VirtualCharacter>> {
  const res = await api.put(`/api/virtual-characters/admin/${id}`, input)
  return res.data
}

export async function offlinePublicVirtualCharacter(
  id: number
): Promise<ApiResponse<{ id: number; status: string }>> {
  const res = await api.delete(`/api/virtual-characters/admin/${id}`)
  return res.data
}

export async function importPublicVirtualCharacters(
  file: File,
  publicChannelId?: number
): Promise<ApiResponse<{ processed: number }>> {
  const form = new FormData()
  form.append('file', file)
  if (publicChannelId) {
    form.append('public_channel_id', String(publicChannelId))
  }
  const res = await api.post('/api/virtual-characters/admin/import', form)
  return res.data
}

export async function getVirtualCharacterSettings(): Promise<
  ApiResponse<VirtualCharacterSettings>
> {
  const res = await api.get('/api/virtual-characters/admin/settings')
  return res.data
}

export async function updateVirtualCharacterSettings(input: {
  global_limit: number
  models: string[]
  default_model: string
}): Promise<ApiResponse<VirtualCharacterSettings>> {
  const res = await api.put('/api/virtual-characters/admin/settings', input)
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
