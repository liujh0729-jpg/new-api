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
import type { PricingModel, TaskPricing } from '../types'

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

/** Check whether a model exposes valid local per-second task pricing. */
export function isValidTaskPricing(
  value: TaskPricing | undefined
): value is TaskPricing {
  return (
    value?.unit === 'second' &&
    Number.isFinite(value.no_reference_video_unit_price) &&
    value.no_reference_video_unit_price > 0 &&
    (value.reference_video_policy === 'same' ||
      value.reference_video_policy === 'disabled' ||
      (value.reference_video_policy === 'custom' &&
        Number.isFinite(value.reference_video_unit_price) &&
        Number(value.reference_video_unit_price) > 0))
  )
}

export function isTaskPricingModel(model: PricingModel): boolean {
  return (
    model.billing_mode === 'task_pricing' &&
    isValidTaskPricing(model.task_pricing)
  )
}
