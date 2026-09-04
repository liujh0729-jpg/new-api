export type MembershipLevel = {
  id: number
  code: string
  display_name: string
  multiplier_ppm: number
  rank: number
  sort_order: number
  enabled: boolean
  is_default: boolean
  archived_at: number
  created_at: number
  updated_at: number
}

export type MembershipSnapshot = {
  grant_id: number
  level_id: number
  code: string
  display_name: string
  multiplier_ppm: number
  rank: number
  starts_at: number
  ends_at: number
  resolved_at: number
  fallback_normal: boolean
}

export type UserMembershipGrant = {
  id: number
  user_id: number
  membership_level_id: number
  starts_at: number
  ends_at: number
  status: 'active' | 'revoked'
  source: string
  note: string
  created_by: number
  revoked_by: number
  revoked_at: number
  created_at: number
  updated_at: number
  level?: MembershipLevel
}

export type UserMembershipHistory = {
  current: MembershipSnapshot
  grants: UserMembershipGrant[]
}

export type LegacyVIPGroupPreflight = {
  group: string
  group_ratio: number
  proposed_multiplier_ppm: number
  users: number
  tokens: number
  abilities: number
  channels: number
  subscription_plans: number
  user_subscriptions: number
  membership_level_conflict: boolean
}

export type LegacyVIPMigrationPreflight = {
  groups: LegacyVIPGroupPreflight[]
  total_users: number
  total_tokens: number
  total_abilities: number
  total_channels: number
  total_subscription_plans: number
  total_user_subscriptions: number
  conflicting_level_codes: string[]
  ready: boolean
}

export type MembershipLevelInput = Omit<
  MembershipLevel,
  'id' | 'is_default' | 'archived_at' | 'created_at' | 'updated_at'
>

export type MembershipLevelDraft = {
  code: string
  display_name: string
  multiplier: string
  rank: string
  sort_order: string
  enabled: boolean
}

export type UserMembershipGrantInput = {
  user_id: number
  membership_level_id: number
  starts_at: number
  ends_at: number
  note: string
}

export type ApiResult<T> = {
  success: boolean
  message?: string
  data: T
}
