export type FinanceFilter = {
  user?: string
  token?: string
  channel_id?: number
  instance_id?: string
  model?: string
  platform_order_id?: string
  order_status?: string
  local_billing_status?: string
  cost_status?: string
  issue_view?: string
  start_time?: number
  end_time?: number
}

export type FinanceOrder = {
  id: string
  platform_order_id: string
  latest_attempt_id: string
  instance_id: string
  request_id: string
  user_id: number
  username: string
  user_display_name: string
  token_id: number
  token_name: string
  token_masked_key: string
  channel_id: number
  channel_name: string
  model: string
  order_status: string
  local_billing_status: string
  cost_status: string
  settlement_revision: number
  customer_charge_quota: number
  customer_charge_rmb_mic: number
  pending_charge_quota: number
  pending_charge_rmb_mic: number
  aipdd_charge_awcoin: number
  aipdd_charge_rmb_mic: number | null
  actual_spend_awcoin: number | null
  base_model_cost_rmb_mic: number | null
  aipdd_model_cost_rmb_mic: number | null
  actual_spend_rmb_mic: number | null
  confirmed_profit_rmb_mic: number | null
  estimated_profit_rmb_mic: number | null
  upstream_reference: string
  source_type: string
  source_id: string
  occurred_at: number
  settled_at: number | null
  created_at: number
  updated_at: number
  requires_manual_review: boolean
  source_cost_confirmed: boolean
  financial_trace_completeness: string
}

export type FinanceMovement = {
  id: string
  component: string
  quota_delta: number
  rmb_delta_mic: number
  evidence: string
  occurred_at: number
}

export type FinanceSettlementEvent = {
  event_id: string
  source_sequence: number
  settlement_revision: number
  payload: string
  processed_at: number
  error_message: string
}

export type FinanceOrderDetail = {
  order: FinanceOrder
  customer_rate_snapshot: string
  upstream_snapshot: string
  movements: FinanceMovement[]
  settlement_events: FinanceSettlementEvent[]
  pending_or_failed_sync_jobs: Array<{
    id: string
    state: string
    attempts: number
    last_error: string
    updated_at: number
  }>
}

export type FinanceSummary = {
  order_count: number
  customer_net_consumption_rmb_mic: number
  confirmed_source_cost_rmb_mic: number
  confirmed_profit_rmb_mic: number
  estimated_source_cost_rmb_mic: number
  estimated_profit_rmb_mic: number
  loss_order_count: number
  pending_confirmation_count: number
  manual_review_count: number
}

export type FinanceSyncStatus = {
  channel_id: number
  channel_name: string
  instance_id: string
  last_sequence: number
  last_success_at: number
  backlog_count: number
  last_error: string
  last_error_at: number
  single_key_valid: boolean
  multi_key_enabled: boolean
}

export type FinanceExportJob = {
  id: string
  status: 'PENDING' | 'RUNNING' | 'READY' | 'FAILED'
  file_name: string
  sha256: string
  row_count: number
  failure_cause: string
  created_at: number
  completed_at: number
  expires_at: number
}
