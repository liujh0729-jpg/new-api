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
import type {
  VirtualCharacter,
  VirtualCharacterAuthorizationStatus,
  VirtualCharacterStatus,
  VirtualCharacterValidationSession,
} from '../types'

export function splitTags(value: string): string[] {
  return value
    .split(',')
    .map((tag) => tag.trim())
    .filter(Boolean)
}

export function authorizationStatusLabel(
  status: VirtualCharacterAuthorizationStatus,
  t: (key: string) => string
): string {
  return t(
    {
      pending: 'Pending',
      synchronizing: 'Creating',
      active: 'Active',
      ambiguous: 'Blocked',
      provider_unavailable: 'Blocked',
      expired: 'Expired',
      revoked: 'Offline',
      failed: 'Failed',
    }[status]
  )
}

export function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object') {
    const response = error as {
      response?: { data?: { error?: { message?: string }; message?: string } }
    }
    const message =
      response.response?.data?.error?.message ||
      response.response?.data?.message
    if (message) return message
  }
  return error instanceof Error && error.message ? error.message : fallback
}

export function statusLabel(
  status: VirtualCharacterStatus,
  t: (key: string) => string
): string {
  return t(
    {
      creating: 'Creating',
      active: 'Active',
      blocked: 'Blocked',
      offline: 'Offline',
      deleting: 'Deleting',
      failed: 'Failed',
    }[status]
  )
}

export function validationStatusLabel(
  status: VirtualCharacterValidationSession['status'],
  t: (key: string) => string
): string {
  return t(
    {
      pending: 'Waiting for validation',
      succeeded: 'Validation succeeded',
      failed: 'Validation failed',
      expired: 'Validation expired',
    }[status]
  )
}

export function taskStatusLabel(
  status: string,
  t: (key: string) => string
): string {
  const normalized = status.toUpperCase()
  if (normalized === 'SUCCESS') return t('Succeeded')
  if (normalized === 'FAILURE' || normalized === 'FAILED') return t('Failed')
  if (normalized === 'IN_PROGRESS' || normalized === 'ACTIVE')
    return t('Running')
  if (['SUBMITTED', 'QUEUED', 'READY', 'SUBMITTING'].includes(normalized))
    return t('Queued')
  return status
}

export function formatVirtualCharacterAgeLabel(
  item: Pick<VirtualCharacter, 'age_min' | 'age_max'>
): string {
  if (
    typeof item.age_min !== 'number' ||
    typeof item.age_max !== 'number' ||
    Number.isNaN(item.age_min) ||
    Number.isNaN(item.age_max)
  ) {
    return ''
  }
  return `${item.age_min}-${item.age_max}`
}

export function virtualCharacterFacetMeta(
  item: Pick<VirtualCharacter, 'nationality' | 'gender' | 'age_min' | 'age_max'>
): string {
  const parts = [
    item.nationality?.trim(),
    item.gender?.trim(),
    formatVirtualCharacterAgeLabel(item),
  ].filter(Boolean)
  return parts.join(' · ')
}
