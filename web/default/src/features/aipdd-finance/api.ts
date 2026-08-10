import { api } from '@/lib/api'
import type {
  FinanceExportJob,
  FinanceFilter,
  FinanceOrder,
  FinanceOrderDetail,
  FinanceSummary,
  FinanceSyncStatus,
} from './types'

export async function getFinanceOrders(
  filter: FinanceFilter,
  page: number,
  pageSize: number
) {
  const response = await api.get('/api/aipdd-finance/orders', {
    params: { ...filter, page, page_size: pageSize },
  })
  return response.data as {
    success: boolean
    data: FinanceOrder[]
    total: number
    page: number
    page_size: number
  }
}

export async function getFinanceSummary(filter: FinanceFilter) {
  const response = await api.get('/api/aipdd-finance/summary', {
    params: filter,
  })
  return response.data as { success: boolean; data: FinanceSummary }
}

export async function getFinanceOrderDetail(id: string) {
  const response = await api.get(`/api/aipdd-finance/orders/${id}`)
  return response.data as { success: boolean; data: FinanceOrderDetail }
}

export async function getFinanceSyncStatus(instanceId?: string) {
  const response = await api.get('/api/aipdd-finance/sync-status', {
    params: instanceId ? { instance_id: instanceId } : undefined,
  })
  return response.data as { success: boolean; data: FinanceSyncStatus[] }
}

export async function retryFinanceSync(filter: FinanceFilter) {
  const response = await api.post('/api/aipdd-finance/sync/retry', filter)
  return response.data as { success: boolean; data: { queued: number } }
}

export async function closeOrphanFinanceOutbox() {
  const response = await api.post('/api/aipdd-finance/sync/orphans/close')
  return response.data as { success: boolean; data: { closed: number } }
}

export async function skipFinancePoisonEvent(input: {
  channel_id: number
  instance_id: string
}) {
  const response = await api.post('/api/aipdd-finance/sync/poison/skip', input)
  return response.data as { success: boolean }
}

export async function closeFinanceOutbox(
  id: string,
  input?: { state?: 'IGNORED' | 'DEAD'; reason?: string }
) {
  const response = await api.post(
    `/api/aipdd-finance/sync/outbox/${id}/close`,
    input ?? { state: 'IGNORED' }
  )
  return response.data as { success: boolean }
}

export async function createFinanceExport(filter: FinanceFilter) {
  const response = await api.post('/api/aipdd-finance/exports', filter)
  return response.data as { success: boolean; data: FinanceExportJob }
}

export async function getFinanceExport(id: string) {
  const response = await api.get(`/api/aipdd-finance/exports/${id}`)
  return response.data as { success: boolean; data: FinanceExportJob }
}

export async function downloadFinanceExport(job: FinanceExportJob) {
  const response = await api.get(
    `/api/aipdd-finance/exports/${job.id}/download`,
    { responseType: 'blob' }
  )
  const objectUrl = URL.createObjectURL(response.data as Blob)
  const anchor = document.createElement('a')
  anchor.href = objectUrl
  anchor.download = job.file_name || 'aipdd-profit-report.xlsx'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(objectUrl)
}
