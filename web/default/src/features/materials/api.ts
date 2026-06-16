import { api } from '@/lib/api'
import type {
  GetMaterialsParams,
  GetMaterialsResponse,
  SearchMaterialsParams,
  ApiResponse,
  Material,
  CreateGeneratedMaterialPayload,
} from './types'

function appendMaterialQueryParam(
  params: URLSearchParams,
  key: string,
  value?: string | string[] | number
) {
  if (value === undefined || value === null || value === '') return
  if (Array.isArray(value)) {
    const joined = value.filter(Boolean).join(',')
    if (joined) params.set(key, joined)
    return
  }
  params.set(key, String(value))
}

function buildMaterialQueryParams(
  params: GetMaterialsParams | SearchMaterialsParams
): URLSearchParams {
  const {
    p = 1,
    page_size = 10,
    type,
    source_type,
    created_after,
    created_before,
  } = params
  const searchParams = new URLSearchParams()
  searchParams.set('p', String(p))
  searchParams.set('page_size', String(page_size))
  appendMaterialQueryParam(searchParams, 'type', type)
  appendMaterialQueryParam(searchParams, 'source_type', source_type)
  appendMaterialQueryParam(searchParams, 'created_after', created_after)
  appendMaterialQueryParam(searchParams, 'created_before', created_before)
  return searchParams
}

export async function getMaterials(
  params: GetMaterialsParams = {}
): Promise<GetMaterialsResponse> {
  const searchParams = buildMaterialQueryParams(params)
  const res = await api.get(`/pg/material?${searchParams.toString()}`)
  return res.data
}

export async function searchMaterials(
  params: SearchMaterialsParams
): Promise<GetMaterialsResponse> {
  const { keyword = '' } = params
  const searchParams = buildMaterialQueryParams(params)
  searchParams.set('keyword', keyword)
  const res = await api.get(`/pg/material/search?${searchParams.toString()}`)
  return res.data
}

export async function uploadMaterial(
  file: File,
  sourceType?: string
): Promise<ApiResponse<Material>> {
  const formData = new FormData()
  formData.append('file', file)
  if (sourceType) {
    formData.append('source_type', sourceType)
  }
  const res = await api.post('/pg/material/upload', formData)
  return res.data
}

export async function createGeneratedMaterial(
  payload: CreateGeneratedMaterialPayload
): Promise<ApiResponse<Material>> {
  const res = await api.post('/pg/material/ai-output', payload)
  return res.data
}

export async function updateMaterial(
  id: number,
  name: string
): Promise<ApiResponse<Material>> {
  const res = await api.put('/pg/material', { id, name })
  return res.data
}

export async function deleteMaterial(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/pg/material/${id}`)
  return res.data
}
