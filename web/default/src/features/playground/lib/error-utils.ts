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
import { ERROR_MESSAGES } from '../constants'

export interface PlaygroundErrorDetails {
  message: string
  code?: string
}

const SEEDANCE_PRIVACY_ERROR_MESSAGE =
  'Seedance rejected the reference media because it may contain a real person. Use text-only generation or replace the reference with non-real-person media.'
const UPSTREAM_BALANCE_INSUFFICIENT_MESSAGE =
  'The selected model service is temporarily unavailable because its upstream balance is insufficient. Please try again later or contact an administrator.'
const UPSTREAM_MODEL_NOT_OPEN_MESSAGE =
  'The upstream account has not activated the selected model. Ask an administrator to enable it in the provider console, or switch to another available model.'
const USER_QUOTA_INSUFFICIENT_MESSAGE =
  'Your account balance is insufficient. Please add credits and try again.'
const TASK_PRICING_ERROR_MESSAGES: Record<string, string> = {
  unsupported_resolution:
    'The selected resolution is not supported by the current upstream model.',
  resolution_price_not_configured:
    'The selected resolution is supported upstream but has no local selling price.',
  task_pricing_facts_unavailable:
    'The request could not provide the resolution and duration required for task pricing.',
  missing_resolution:
    'Select a video resolution before submitting the request.',
}

const LOCAL_QUOTA_CODE_PARTS = [
  'insufficientuserquota',
  'preconsumetokenquotafailed',
]

const UPSTREAM_BALANCE_CODE_PARTS = [
  'accountoverdue',
  'arrearage',
  'balanceinsufficient',
  'balancenotenough',
  'billinghardlimitreached',
  'creditnotenough',
  'creditsnotenough',
  'freequotaexceeded',
  'insufficientbalance',
  'insufficientcredit',
  'insufficientcredits',
  'insufficientquota',
  'noavailablecredit',
  'paymentrequired',
  'prepaidquotaexhausted',
  'quotaexceeded',
  'quotaexhausted',
]

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function firstString(...values: unknown[]): string | undefined {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return undefined
}

function parseJsonObjectFromString(value: string): unknown {
  const trimmed = value.trim()
  const candidates = [trimmed]

  const firstBrace = trimmed.indexOf('{')
  const lastBrace = trimmed.lastIndexOf('}')
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    candidates.push(trimmed.slice(firstBrace, lastBrace + 1))
  }

  for (const candidate of candidates) {
    if (!candidate.startsWith('{') || !candidate.endsWith('}')) continue
    try {
      return JSON.parse(candidate) as unknown
    } catch {
      // Keep trying less strict candidates.
    }
  }
  return undefined
}

function normalizeForCodeMatch(value?: string): string {
  return (value || '').toLowerCase().replace(/[^a-z0-9\u4e00-\u9fa5]/g, '')
}

function normalizeForMessageMatch(value?: string): string {
  return (value || '').toLowerCase()
}

function extractPlaygroundErrorDetails(
  value: unknown
): Partial<PlaygroundErrorDetails> {
  if (!value) return {}

  if (typeof value === 'string') {
    const parsed = parseJsonObjectFromString(value)
    if (parsed !== undefined) {
      const details = extractPlaygroundErrorDetails(parsed)
      if (details.message || details.code) return details
    }
    return { message: value.trim() || undefined }
  }

  if (!isRecord(value)) return {}

  const response = value.response
  if (isRecord(response)) {
    const responseDetails = extractPlaygroundErrorDetails(response.data)
    if (responseDetails.message || responseDetails.code) {
      return responseDetails
    }
  }

  const nestedError = value.error
  if (nestedError) {
    const nestedDetails = extractPlaygroundErrorDetails(nestedError)
    if (nestedDetails.message || nestedDetails.code) {
      return {
        message: nestedDetails.message,
        code: nestedDetails.code || firstString(value.code, value.error_code),
      }
    }
  }

  const message = firstString(
    value.message,
    value.error_message,
    value.fail_reason,
    value.reason
  )
  if (message) {
    const messageDetails = extractPlaygroundErrorDetails(message)
    if (
      messageDetails.code ||
      (messageDetails.message && messageDetails.message !== message)
    ) {
      return messageDetails
    }
  }

  return {
    message,
    code: firstString(value.code, value.error_code),
  }
}

function isSeedancePrivacyError(code?: string, message?: string): boolean {
  const normalizedCode = (code || '').toLowerCase()
  const normalizedMessage = (message || '').toLowerCase()
  return (
    (normalizedCode.includes('sensitivecontentdetected') &&
      normalizedCode.includes('privacyinformation')) ||
    normalizedMessage.includes('input video may contain real person') ||
    normalizedMessage.includes('input image may contain real person')
  )
}

function isLocalQuotaError(code?: string, message?: string): boolean {
  const normalizedCode = normalizeForCodeMatch(code)
  const normalizedMessage = normalizeForMessageMatch(message)

  return (
    LOCAL_QUOTA_CODE_PARTS.some((part) => normalizedCode.includes(part)) ||
    normalizedMessage.includes('用户额度不足') ||
    normalizedMessage.includes('订阅额度不足') ||
    normalizedMessage.includes('quota_not_enough')
  )
}

function isUpstreamModelNotOpenError(code?: string, message?: string): boolean {
  const normalizedCode = normalizeForCodeMatch(code)
  const normalizedMessage = normalizeForMessageMatch(message)

  return (
    normalizedCode.includes('modelnotopen') ||
    normalizedMessage.includes('model not open') ||
    normalizedMessage.includes('model is not activated') ||
    normalizedMessage.includes('model service is not activated') ||
    normalizedMessage.includes('has not activated the model') ||
    (normalizedMessage.includes('模型') &&
      (normalizedMessage.includes('未开通') ||
        normalizedMessage.includes('尚未开通') ||
        normalizedMessage.includes('未启用')))
  )
}

function isUpstreamBalanceError(code?: string, message?: string): boolean {
  const normalizedCode = normalizeForCodeMatch(code)
  const normalizedMessage = normalizeForMessageMatch(message)

  if (
    UPSTREAM_BALANCE_CODE_PARTS.some((part) => normalizedCode.includes(part))
  ) {
    return true
  }

  return (
    normalizedMessage.includes('http 402') ||
    normalizedMessage.includes('payment required') ||
    normalizedMessage.includes('insufficient account balance') ||
    normalizedMessage.includes('insufficient balance') ||
    normalizedMessage.includes('account balance is insufficient') ||
    normalizedMessage.includes('credit balance is too low') ||
    normalizedMessage.includes('not enough credits') ||
    normalizedMessage.includes('requires more credits') ||
    normalizedMessage.includes('please add credits') ||
    normalizedMessage.includes('purchase credits') ||
    normalizedMessage.includes('out of credit') ||
    normalizedMessage.includes('hard limit') ||
    normalizedMessage.includes('exceeded your current quota') ||
    normalizedMessage.includes('quota has been exhausted') ||
    normalizedMessage.includes('allocated quota exceeded') ||
    normalizedMessage.includes('prepaid quota') ||
    normalizedMessage.includes('余额不足') ||
    normalizedMessage.includes('余额已耗尽') ||
    normalizedMessage.includes('额度不足') ||
    normalizedMessage.includes('额度已用完') ||
    normalizedMessage.includes('欠费')
  )
}

export function normalizePlaygroundError(
  error: unknown,
  t: (key: string) => string,
  fallback = ERROR_MESSAGES.API_REQUEST_ERROR
): PlaygroundErrorDetails {
  const extracted = extractPlaygroundErrorDetails(error)
  const code = extracted.code
  const message = extracted.message || fallback

  if (code && TASK_PRICING_ERROR_MESSAGES[code]) {
    return {
      message: t(TASK_PRICING_ERROR_MESSAGES[code]),
      code,
    }
  }

  if (isSeedancePrivacyError(code, message)) {
    return {
      message: t(SEEDANCE_PRIVACY_ERROR_MESSAGE),
      code,
    }
  }

  if (isUpstreamModelNotOpenError(code, message)) {
    return {
      message: t(UPSTREAM_MODEL_NOT_OPEN_MESSAGE),
      code,
    }
  }

  if (isLocalQuotaError(code, message)) {
    return {
      message: t(USER_QUOTA_INSUFFICIENT_MESSAGE),
      code,
    }
  }

  if (isUpstreamBalanceError(code, message)) {
    return {
      message: t(UPSTREAM_BALANCE_INSUFFICIENT_MESSAGE),
      code,
    }
  }

  if (
    message === ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED ||
    message === ERROR_MESSAGES.TIMELINE_JSON_INVALID
  ) {
    return {
      message: t(message),
      code,
    }
  }

  return {
    message,
    code,
  }
}
