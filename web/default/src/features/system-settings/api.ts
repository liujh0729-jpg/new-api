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
  DeleteLogsResponse,
  FetchUpstreamRatiosRequest,
  SystemOptionsResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function deleteLogsBefore(targetTimestamp: number) {
  const res = await api.delete<DeleteLogsResponse>('/api/log/', {
    params: { target_timestamp: targetTimestamp },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}

export type TaskPricingCSVSummary = {
  models: string[]
  resolution_tiers: number
  source_rows: number
  exempt_resolutions: string[]
  groups?: Record<string, number>
  rmb_per_usd?: string
}

export type TaskPricingCSVOptionUpdate = {
  key: string
  value: string
}

export type TaskPricingCSVPlan = {
  generated_at: string
  summary: TaskPricingCSVSummary
  updates: TaskPricingCSVOptionUpdate[]
  rollback: TaskPricingCSVOptionUpdate[]
}

export type TaskPricingCSVPlanResponse = {
  success: boolean
  message: string
  data: TaskPricingCSVPlan
}

export type TaskPricingCSVImportResponse = {
  success: boolean
  message: string
  data?: {
    summary: TaskPricingCSVSummary
    updated_keys: string[]
    models_updated: string[]
  }
}

async function downloadTaskPricingCSVBlob(
  url: string,
  fallbackFilename: string
) {
  const res = await api.get(url, {
    responseType: 'blob',
    skipBusinessError: true,
    disableDuplicate: true,
  } as Record<string, unknown>)

  const blob = res.data as Blob
  const contentType = String(res.headers['content-type'] || '')
  if (contentType.includes('application/json') || blob.type.includes('json')) {
    const text = await blob.text()
    const parsed = JSON.parse(text) as { success?: boolean; message?: string }
    throw new Error(parsed.message || 'Download failed')
  }

  const disposition = String(res.headers['content-disposition'] || '')
  const filenameMatch = /filename\*?=(?:UTF-8''|")?([^\";]+)/i.exec(disposition)
  const filename = filenameMatch
    ? decodeURIComponent(filenameMatch[1].replace(/"/g, ''))
    : fallbackFilename

  const objectUrl = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectUrl
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(objectUrl)
}

export async function downloadTaskPricingCSVTemplate() {
  await downloadTaskPricingCSVBlob(
    '/api/option/task_pricing_csv/template',
    'task-pricing-template.csv'
  )
}

export async function exportTaskPricingCSV() {
  await downloadTaskPricingCSVBlob(
    '/api/option/task_pricing_csv/export',
    'task-pricing-export.csv'
  )
}

export async function previewTaskPricingCSV(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post<TaskPricingCSVPlanResponse>(
    '/api/option/task_pricing_csv/preview',
    formData,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function importTaskPricingCSV(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post<TaskPricingCSVImportResponse>(
    '/api/option/task_pricing_csv/import',
    formData,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}
