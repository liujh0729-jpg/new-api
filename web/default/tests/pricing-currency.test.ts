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
import { describe, expect, test } from 'bun:test'
import { normalizePricingModelsToUSD } from '../src/features/pricing/api'
import type { PricingModel } from '../src/features/pricing/types'

const model: PricingModel = {
  id: 1,
  model_name: 'AP Seedance-2.0 轻量版',
  quota_type: 1,
  model_ratio: 0,
  model_price: 0.32,
  completion_ratio: 0,
  enable_groups: ['default'],
  billing_mode: 'task_pricing',
  task_pricing_resolutions: ['480p', '720p'],
  task_pricing: {
    unit: 'second',
    by_resolution: {
      '480p': {
        no_reference_video_unit_price: 0.32,
        reference_video_policy: 'custom',
        reference_video_unit_price: 0.44,
      },
      '720p': {
        no_reference_video_unit_price: 0.496,
        reference_video_policy: 'same',
      },
    },
  },
}

describe('pricing API currency normalization', () => {
  test('normalizes explicit CNY monetary fields back to internal USD values', () => {
    const [normalized] = normalizePricingModelsToUSD([model], 'CNY', 7.3)

    expect(normalized.model_price).toBeCloseTo(0.32 / 7.3)
    if (normalized.task_pricing?.by_resolution) {
      expect(
        normalized.task_pricing.by_resolution['720p']
          .no_reference_video_unit_price
      ).toBeCloseTo(0.496 / 7.3)
      expect(
        normalized.task_pricing.by_resolution['480p']
          .reference_video_unit_price
      ).toBeCloseTo(0.44 / 7.3)
    }

    expect(model.model_price).toBe(0.32)
    if (model.task_pricing?.by_resolution) {
      expect(
        model.task_pricing.by_resolution['720p']
          .no_reference_video_unit_price
      ).toBe(0.496)
    }
  })

  test('keeps legacy USD responses unchanged', () => {
    const models = [model]
    expect(normalizePricingModelsToUSD(models, undefined, undefined)).toBe(
      models
    )
    expect(normalizePricingModelsToUSD(models, 'USD', 7.3)).toBe(models)
  })
})
