import { api } from '@/lib/api'
import type {
  GetMaterialsParams,
  GetMaterialsResponse,
  SearchMaterialsParams,
  ApiResponse,
  Material,
} from './types'

export async function getMaterials(
  params: GetMaterialsParams = {}
): Promise<GetMaterialsResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/pg/material?p=${p}&page_size=${page_size}`)
  return res.data
}

export async function searchMaterials(
  params: SearchMaterialsParams
): Promise<GetMaterialsResponse> {
  const { keyword = '', type = '', p = 1, page_size = 10 } = params
  let url = `/pg/material/search?keyword=${encodeURIComponent(keyword)}&p=${p}&page_size=${page_size}`
  const typeFilter = Array.isArray(type) ? type.filter(Boolean).join(',') : type
  if (typeFilter) {
    url += `&type=${encodeURIComponent(typeFilter)}`
  }
  const res = await api.get(url)
  return res.data
}

export async function uploadMaterial(
  file: File
): Promise<ApiResponse<Material>> {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post('/pg/material/upload', formData)
  return res.data
}

export async function updateMaterial(
  id: number,
  name: string
): Promise<ApiResponse<Material>> {
  const res = await api.put('/pg/material', { id, name })
  return res.data
}

export async function deleteMaterial(
  id: number
): Promise<ApiResponse> {
  const res = await api.delete(`/pg/material/${id}`)
  return res.data
}
