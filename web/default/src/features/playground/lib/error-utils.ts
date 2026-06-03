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

export function normalizePlaygroundError(
  error: unknown,
  t: (key: string) => string,
  fallback = ERROR_MESSAGES.API_REQUEST_ERROR
): PlaygroundErrorDetails {
  const extracted = extractPlaygroundErrorDetails(error)
  const code = extracted.code
  const message = extracted.message || fallback

  if (isSeedancePrivacyError(code, message)) {
    return {
      message: t(SEEDANCE_PRIVACY_ERROR_MESSAGE),
      code,
    }
  }

  if (message === ERROR_MESSAGES.VIDEO_REFERENCE_UPLOAD_REQUIRED) {
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
