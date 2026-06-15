import { z } from 'zod'

export const materialSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  name: z.string(),
  type: z.string(),
  mime_type: z.string(),
  file_name: z.string(),
  url: z.string(),
  storage_type: z.string(),
  file_path: z.string(),
  file_size: z.number(),
  width: z.number().nullable().optional(),
  height: z.number().nullable().optional(),
  duration: z.number().nullable().optional(),
  status: z.number(),
  created_time: z.number(),
  updated_time: z.number(),
})

export type Material = z.infer<typeof materialSchema>

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetMaterialsParams {
  p?: number
  page_size?: number
}

export interface GetMaterialsResponse {
  success: boolean
  message?: string
  data?: {
    items: Material[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchMaterialsParams {
  keyword?: string
  type?: string | string[]
  p?: number
  page_size?: number
}

export type MaterialsDialogType = 'update' | 'delete'
