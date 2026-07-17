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
import { EXCLUDED_GROUPS, QUOTA_TYPE_VALUES } from '../constants'
import type {
  PricingModel,
  ResolutionTaskPricing,
  TaskPricing,
  TaskPricingTier,
} from '../types'

// ----------------------------------------------------------------------------
// Model Helper Utilities
// ----------------------------------------------------------------------------

/**
 * Get available groups for a model
 */
export function getAvailableGroups(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>
): string[] {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  return Object.keys(usableGroup)
    .filter((g) => !EXCLUDED_GROUPS.includes(g))
    .filter((g) => modelEnableGroups.includes(g))
}

/**
 * Replace model placeholder in endpoint path
 */
export function replaceModelInPath(path: string, modelName: string): string {
  return path.replace(/\{model\}/g, modelName)
}

/**
 * Check if model is token-based pricing
 */
export function isTokenBasedModel(model: PricingModel): boolean {
  return model.quota_type === QUOTA_TYPE_VALUES.TOKEN
}

const MAX_TASK_PRICING_RESOLUTION_LENGTH = 128

export function normalizeTaskPricingResolution(value: string): string {
  return value.trim().toLowerCase()
}

export function compareTaskPricingResolutions(
  left: string,
  right: string
): number {
  const scale = (value: string) => {
    const match = normalizeTaskPricingResolution(value).match(
      /^(\d+(?:\.\d+)?)([pk])$/
    )
    if (!match) return null
    const number = Number(match[1])
    if (!Number.isFinite(number) || number <= 0) return null
    return match[2] === 'k' ? number * 1000 : number
  }
  const leftScale = scale(left)
  const rightScale = scale(right)
  if (leftScale !== null && rightScale !== null && leftScale !== rightScale) {
    return leftScale - rightScale
  }
  if (leftScale !== null && rightScale === null) return -1
  if (leftScale === null && rightScale !== null) return 1
  return left.localeCompare(right)
}

export function isValidTaskPricingTier(
  value: unknown
): value is TaskPricingTier {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const tier = value as Partial<TaskPricingTier>
  return (
    Number.isFinite(tier.no_reference_video_unit_price) &&
    Number(tier.no_reference_video_unit_price) > 0 &&
    (tier.reference_video_policy === 'same' ||
      tier.reference_video_policy === 'disabled' ||
      (tier.reference_video_policy === 'custom' &&
        Number.isFinite(tier.reference_video_unit_price) &&
        Number(tier.reference_video_unit_price) > 0)) &&
    (tier.group_ratio_policy === undefined ||
      tier.group_ratio_policy === 'global' ||
      tier.group_ratio_policy === 'none')
  )
}

export function isResolutionTaskPricing(
  value: TaskPricing | undefined
): value is ResolutionTaskPricing {
  return (
    !!value &&
    typeof value === 'object' &&
    !Array.isArray(value) &&
    'by_resolution' in value &&
    !!value.by_resolution &&
    typeof value.by_resolution === 'object' &&
    !Array.isArray(value.by_resolution)
  )
}

/** Check whether a model exposes valid local per-second task pricing. */
export function isValidTaskPricing(
  value: TaskPricing | undefined
): value is TaskPricing {
  if (!value || value.unit !== 'second') return false
  if (isResolutionTaskPricing(value)) {
    if (
      'no_reference_video_unit_price' in value ||
      'reference_video_policy' in value ||
      'reference_video_unit_price' in value
    ) {
      return false
    }
    const entries = Object.entries(value.by_resolution)
    return (
      entries.length > 0 &&
      entries.every(([resolution, tier]) => {
        const normalized = normalizeTaskPricingResolution(resolution)
        return (
          normalized.length > 0 &&
          normalized.length <= MAX_TASK_PRICING_RESOLUTION_LENGTH &&
          resolution === normalized &&
          isValidTaskPricingTier(tier)
        )
      })
    )
  }
  return isValidTaskPricingTier(value)
}

export type TaskPricingTierEntry = TaskPricingTier & {
  resolution: string
}

/** Enumerate effective tiers, optionally restricting them to active capabilities. */
export function getTaskPricingTiers(
  pricing: TaskPricing,
  activeResolutions?: string[]
): TaskPricingTierEntry[] {
  const active =
    activeResolutions === undefined
      ? undefined
      : new Set(activeResolutions.map(normalizeTaskPricingResolution))

  if (!isResolutionTaskPricing(pricing)) {
    const resolutions =
      activeResolutions === undefined ? [''] : activeResolutions
    return resolutions.map((resolution) => ({
      resolution: normalizeTaskPricingResolution(resolution),
      no_reference_video_unit_price: pricing.no_reference_video_unit_price,
      reference_video_policy: pricing.reference_video_policy,
      ...(pricing.reference_video_unit_price !== undefined
        ? { reference_video_unit_price: pricing.reference_video_unit_price }
        : {}),
      ...(pricing.group_ratio_policy !== undefined
        ? { group_ratio_policy: pricing.group_ratio_policy }
        : {}),
    }))
  }

  return Object.entries(pricing.by_resolution)
    .filter(([resolution]) => !active || active.has(resolution))
    .sort(([left], [right]) => compareTaskPricingResolutions(left, right))
    .map(([resolution, tier]) => ({ resolution, ...tier }))
}

export function getTaskPricingUnitPrices(
  pricing: TaskPricing,
  activeResolutions?: string[]
): number[] {
  return getTaskPricingTiers(pricing, activeResolutions).flatMap((tier) => {
    const prices = [tier.no_reference_video_unit_price]
    if (
      tier.reference_video_policy === 'custom' &&
      Number(tier.reference_video_unit_price) > 0
    ) {
      prices.push(Number(tier.reference_video_unit_price))
    }
    return prices
  })
}

export function cloneTaskPricing(pricing: TaskPricing): TaskPricing {
  return structuredClone(pricing)
}

export function isTaskPricingModel(model: PricingModel): boolean {
  return (
    model.billing_mode === 'task_pricing' &&
    isValidTaskPricing(model.task_pricing)
  )
}
