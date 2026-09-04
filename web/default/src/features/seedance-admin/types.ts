export type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type SeedanceChannel = { id: number; name: string; status: number }

export type SeedanceConfig = {
  channel_id: number
  instance_id: string
  aipdd_billing_base_url?: string
  billing_auth_paused_at?: number
  billing_auth_last_http_status?: number
  volcengine_bill_sync_enabled: boolean
  volcengine_bill_product_codes: string
  volcengine_bill_configuration_codes: string
  default_enhancement_provider_id?: number
  status: 'ACTIVE' | 'DISABLED'
  last_verified_at?: number
  updated_at?: number
}

export type SeedanceBillCursor = {
  id: number
  channel_id: number
  billing_period: string
  cursor: string
  status: string
  last_error?: string
  last_sync_at?: number
  next_attempt_at?: number
  updated_at: number
}

export type SeedanceCredential = {
  id: number
  channel_id: number
  version: number
  fingerprint: string
  masked_suffix: string
  status: string
  validated_at?: number
  billing_validated_at?: number
  created_at: number
}

export type SeedanceProvider = {
  id: number
  provider_type: 'DIRECT_EXTERNAL'
  adapter_type: 'GENERIC_HTTP' | 'VOLCENGINE_MEDIAKIT'
  version: number
  display_name: string
  service_endpoint: string
  service_code: string
  capabilities: string
  credential_configured: boolean
  status: 'ACTIVE' | 'DISABLED'
  timeout_policy: string
  retry_policy: string
  fallback_policy: string
  created_at: number
  updated_at: number
}

export type SeedanceProviderSaveRequest = Partial<SeedanceProvider> & {
  credential?: string
  mediakit_api_key?: string
  clear_credential?: boolean
}

export type SeedanceOffering = {
  id: number
  channel_id: number
  display_name: string
  base_model_id: number
  enhancement_model_id?: number
  source_resolution: SeedanceResolution
  target_resolution: SeedanceResolution
  output_fps: number
  no_reference_unit_price_micro_rmb: number
  reference_unit_price_micro_rmb: number
  migration_needs_review: boolean
  archived_at?: number
  provider_model_id: string
  resolution_rules: string
  duration_rules: string
  enhancement_provider_id: number
  enhancement_service_code: string
  enhancement_specification: string
  enhancement_specification_version: string
  model_sale_micro_rmb: number
  service_charge_micro_rmb: number
  provider_cost_micro_rmb?: number
  volcengine_unit_cost_micro_rmb: number
  pricing_version: string
  enabled: boolean
  published_at?: number
}

export type SeedanceResolution = '480p' | '720p' | '1080p' | '2k' | '4k'

export type SeedanceBaseCost = {
  source_resolution: SeedanceResolution
  has_reference_video: boolean
  cost_micro_rmb_per_second: number
}

export type SeedanceBaseModel = {
  id: number
  code: string
  revision: number
  display_name: string
  provider_model_id: string
  cost_matrix: string
  enabled: boolean
  current: boolean
  archived_at?: number
  created_at: number
  updated_at: number
}

export type SeedanceEnhancementCost = {
  target_resolution: SeedanceResolution
  fps_bucket: 'LE_30' | 'GT_30'
  cost_micro_rmb_per_second: number
}

export type SeedanceEnhancementModel = {
  id: number
  code: string
  revision: number
  display_name: string
  provider_id: number
  service_code: string
  quality_tier: string
  specification: string
  specification_version: string
  cost_matrix: string
  enabled: boolean
  current: boolean
  archived_at?: number
  created_at: number
  updated_at: number
}

export type SeedanceOverview = {
  config?: SeedanceConfig
  configured: boolean
  crypto_ready: boolean
  billing_credential_configured: boolean
  site_instance_id: string
  site_instance_id_configured: boolean
  credentials: SeedanceCredential[]
  providers: SeedanceProvider[]
  offerings: SeedanceOffering[]
  bill_cursors: SeedanceBillCursor[]
}

export type SeedanceMetrics = {
  seedance_tasks_total: Array<{ status: string; model: string; value: number }>
  seedance_generation_latency_seconds: Array<{
    model: string
    count: number
    average_seconds: number
    maximum_seconds: number
  }>
  media_enhancement_latency_seconds: Array<{
    provider_type: string
    service_code: string
    count: number
    average_seconds: number
    maximum_seconds: number
  }>
  media_enhancement_failures_total: Array<{
    provider_type: string
    service_code: string
    reason: string
    value: number
  }>
  media_enhancement_unknown_submissions_total: Array<{
    provider_type: string
    value: number
  }>
  seedance_billing_outbox_pending: number
  seedance_billing_outbox_oldest_age_seconds: number
  seedance_billing_sync_failures_total: Array<{
    status_code: number
    value: number
  }>
  seedance_volcengine_cost_pending: number
  seedance_volcengine_cost_reconciliation_required: number
  seedance_aipdd_arrears_total: number
}

export type SeedanceOrder = {
  id: number
  platform_order_id: string
  newapi_task_id: string
  model: string
  offering_id: number
  base_model_id: number
  enhancement_model_id?: number
  source_resolution: string
  target_resolution: string
  output_fps: number
  has_reference_video: boolean
  requested_duration_millis: number
  actual_duration_millis: number
  sale_unit_price_micro_rmb: number
  super_resolution_unit_cost_micro_rmb: number
  super_resolution_cost_micro_rmb: number
  order_status: string
  enhancement_status?: string
  enhancement_failure_reason?: string
  sync_status: string
  volcengine_cost_status: string
  callback_status: string
  callback_attempt_count: number
  callback_next_attempt_at: number
  callback_last_http_status?: number
  callback_last_error?: string
  model_sale_micro_rmb: number
  service_charge_total_micro_rmb: number
  volcengine_estimated_micro_rmb: number
  volcengine_actual_micro_rmb?: number
  newapi_estimated_profit_micro_rmb: number
  newapi_actual_profit_micro_rmb?: number
  created_at: number
}

export type SeedanceOutbox = {
  id: number
  event_id: string
  platform_order_id: string
  service_line_item_id: string
  channel_id: number
  instance_id: string
  status: string
  attempt_count: number
  next_attempt_at: number
  last_http_status?: number
  last_error?: string
  updated_at: number
}

export type SeedanceCostIssue = {
  id: number
  bill_item_id: number
  channel_id: number
  reason_code: string
  status: string
  created_at: number
  resolved_at?: number
}

export type Paged<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
}
