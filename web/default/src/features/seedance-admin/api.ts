import { api } from '@/lib/api'
import type {
  ApiResponse,
  Paged,
  SeedanceChannel,
  SeedanceBaseModel,
  SeedanceConfig,
  SeedanceCostIssue,
  SeedanceMetrics,
  SeedanceEnhancementModel,
  SeedanceOffering,
  SeedanceOrder,
  SeedanceOutbox,
  SeedanceOverview,
  SeedanceProvider,
  SeedanceProviderSaveRequest,
} from './types'

export async function listSeedanceChannels(): Promise<SeedanceChannel[]> {
  const response = await api.get('/api/channel', {
    params: { p: 1, page_size: 100, type: '59' },
  })
  const items = response.data?.data?.items ?? []
  return items.filter((item: { type: number }) => item.type === 59)
}

export async function getSeedanceOverview(channelId: number) {
  const response = await api.get<ApiResponse<SeedanceOverview>>(
    '/api/seedance-admin/overview',
    { params: { channel_id: channelId } }
  )
  return response.data.data
}

export async function getSeedanceMetrics(channelId: number) {
  const response = await api.get<ApiResponse<SeedanceMetrics>>(
    '/api/seedance-admin/metrics',
    { params: { channel_id: channelId } }
  )
  return response.data.data
}

export async function saveSeedanceConfig(
  channelId: number,
  data: Omit<
    Partial<SeedanceConfig>,
    'volcengine_bill_product_codes' | 'volcengine_bill_configuration_codes'
  > & {
    aipdd_billing_api_key?: string
    volcengine_bill_product_codes?: string[]
    volcengine_bill_configuration_codes?: string[]
  }
) {
  const response = await api.put<ApiResponse<SeedanceConfig>>(
    `/api/seedance-admin/channels/${channelId}/config`,
    data
  )
  return response.data.data
}

export async function createSeedanceCredential(
  channelId: number,
  data: {
    ark_api_key: string
    access_key_id?: string
    secret_access_key?: string
  }
) {
  const response = await api.post(
    `/api/seedance-admin/channels/${channelId}/credentials`,
    data
  )
  return response.data.data
}

export async function validateSeedanceCredential(
  _channelId: number,
  id: number
) {
  const response = await api.post(
    `/api/seedance-admin/credentials/${id}/validate`
  )
  return response.data.data
}

export async function saveSeedanceProvider(data: SeedanceProviderSaveRequest) {
  const response = await api.post<ApiResponse<SeedanceProvider>>(
    '/api/seedance-admin/providers',
    data
  )
  return response.data.data
}

export async function saveSeedanceOffering(data: Partial<SeedanceOffering>) {
  const response = await api.post<ApiResponse<SeedanceOffering>>(
    '/api/seedance-admin/offerings',
    data
  )
  return response.data.data
}

export async function archiveSeedanceOffering(id: number) {
  const response = await api.delete(`/api/seedance-admin/offerings/${id}`)
  return response.data.data
}

export async function listSeedanceBaseModels() {
  const response = await api.get<ApiResponse<SeedanceBaseModel[]>>(
    '/api/seedance-admin/base-models'
  )
  return response.data.data ?? []
}

export async function saveSeedanceBaseModel(data: Partial<SeedanceBaseModel>) {
  const response = await api.post<ApiResponse<SeedanceBaseModel>>(
    '/api/seedance-admin/base-models',
    data
  )
  return response.data.data
}

export async function archiveSeedanceBaseModel(id: number) {
  const response = await api.delete(`/api/seedance-admin/base-models/${id}`)
  return response.data.data
}

export async function listSeedanceEnhancementModels() {
  const response = await api.get<ApiResponse<SeedanceEnhancementModel[]>>(
    '/api/seedance-admin/enhancement-models'
  )
  return response.data.data ?? []
}

export async function saveSeedanceEnhancementModel(
  data: Partial<SeedanceEnhancementModel>
) {
  const response = await api.post<ApiResponse<SeedanceEnhancementModel>>(
    '/api/seedance-admin/enhancement-models',
    data
  )
  return response.data.data
}

export async function archiveSeedanceEnhancementModel(id: number) {
  const response = await api.delete(
    `/api/seedance-admin/enhancement-models/${id}`
  )
  return response.data.data
}

export async function listSeedanceOrders(channelId: number, status = '') {
  const response = await api.get<ApiResponse<Paged<SeedanceOrder>>>(
    '/api/seedance-admin/orders',
    { params: { channel_id: channelId, status, p: 1, page_size: 50 } }
  )
  return response.data.data
}

export async function listSeedanceOutbox(
  channelId: number,
  status = '',
  platformOrderId = ''
) {
  const response = await api.get<ApiResponse<Paged<SeedanceOutbox>>>(
    '/api/seedance-admin/outbox',
    {
      params: {
        channel_id: channelId,
        status,
        platform_order_id: platformOrderId,
        p: 1,
        page_size: 50,
      },
    }
  )
  return response.data.data
}

export async function replaySeedanceOutbox(eventId: string) {
  const response = await api.post(
    `/api/seedance-admin/outbox/${encodeURIComponent(eventId)}/replay`
  )
  return response.data.data
}

export async function reviseSeedanceOutbox(eventId: string) {
  const response = await api.post(
    `/api/seedance-admin/outbox/${encodeURIComponent(eventId)}/revise`
  )
  return response.data.data
}

export async function listSeedanceCostIssues() {
  const response = await api.get<ApiResponse<Paged<SeedanceCostIssue>>>(
    '/api/seedance-admin/cost-reconciliation',
    { params: { status: 'OPEN', p: 1, page_size: 50 } }
  )
  return response.data.data
}

export async function reconcileSeedanceBill(
  billItemId: number,
  candidates: Array<{ platform_order_id: string; weight: number }>
) {
  const response = await api.post(
    `/api/seedance-admin/volcengine-bills/${billItemId}/reconcile`,
    { candidates }
  )
  return response.data.data
}
