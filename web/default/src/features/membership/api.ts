import { api } from '@/lib/api'
import type {
  ApiResult,
  LegacyVIPMigrationPreflight,
  MembershipLevel,
  MembershipLevelInput,
  UserMembershipGrantInput,
  UserMembershipHistory,
} from './types'

function expectSuccess<T>(result: ApiResult<T>): ApiResult<T> {
  if (!result.success) {
    throw new Error(result.message || 'Request failed')
  }
  return result
}

export async function getMembershipLevels(includeArchived = false) {
  const response = await api.get<ApiResult<MembershipLevel[]>>(
    '/api/membership/admin/levels',
    { params: { include_archived: includeArchived } }
  )
  return expectSuccess(response.data)
}

export async function createMembershipLevel(input: MembershipLevelInput) {
  const response = await api.post<ApiResult<MembershipLevel>>(
    '/api/membership/admin/levels',
    input
  )
  return expectSuccess(response.data)
}

export async function updateMembershipLevel(
  id: number,
  input: Omit<MembershipLevelInput, 'code'>
) {
  const response = await api.put<ApiResult<MembershipLevel>>(
    `/api/membership/admin/levels/${id}`,
    input
  )
  return expectSuccess(response.data)
}

export async function archiveMembershipLevel(id: number) {
  const response = await api.delete<ApiResult<null>>(
    `/api/membership/admin/levels/${id}`
  )
  return expectSuccess(response.data)
}

export async function getUserMemberships(userId: number) {
  const response = await api.get<ApiResult<UserMembershipHistory>>(
    `/api/membership/admin/users/${userId}`
  )
  return expectSuccess(response.data)
}

export async function createUserMembership(input: UserMembershipGrantInput) {
  const response = await api.post<ApiResult<unknown>>(
    '/api/membership/admin/grants',
    input
  )
  return expectSuccess(response.data)
}

export async function revokeUserMembership(id: number) {
  const response = await api.delete<ApiResult<null>>(
    `/api/membership/admin/grants/${id}`
  )
  return expectSuccess(response.data)
}

export async function getLegacyVIPMigrationPreflight() {
  const response = await api.get<ApiResult<LegacyVIPMigrationPreflight>>(
    '/api/membership/admin/migration/preflight'
  )
  return expectSuccess(response.data)
}

export async function applyLegacyVIPMigration() {
  const response = await api.post<ApiResult<unknown>>(
    '/api/membership/admin/migration/apply',
    { confirmation: 'MIGRATE_LEGACY_VIP_GROUPS' }
  )
  return expectSuccess(response.data)
}
