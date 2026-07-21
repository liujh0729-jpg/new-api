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
export type WechatPayOrderStatus = 'pending' | 'success' | 'expired' | 'failed'

export type WechatPayVerificationMode = 'platform_certificate' | 'public_key'

export interface WechatPayOrderView {
  trade_no: string
  code_url: string
  status: WechatPayOrderStatus
  payment_amount_cents: number
  currency: string
  expires_at: number
}

export interface WechatPayConfigStatus {
  configured: boolean
  ready: boolean
  enabled: boolean
  show_epay_wechat: boolean
  verification_mode: WechatPayVerificationMode
  crypto_secret_configured: boolean
  appid?: string
  mchid?: string
  merchant_certificate_serial?: string
  merchant_certificate_fingerprint?: string
  wechatpay_public_key_id?: string
  wechatpay_public_key_fingerprint?: string
  has_merchant_certificate: boolean
  has_merchant_private_key: boolean
  has_api_v3_key: boolean
  has_wechatpay_public_key: boolean
  validated_at?: number
  verified_at?: number
  callback_url: string
  error?: string
}
