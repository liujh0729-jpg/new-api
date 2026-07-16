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
import { isTaskPricingModel } from '../src/features/pricing/lib/model-helpers'
import { getTaskPriceInfo } from '../src/features/pricing/lib/price'
import {
  isValidTaskPricing,
  parseTaskPricingRequiredModels,
} from '../src/features/system-settings/models/task-pricing-utils'
import type {
  PricingModel,
  ReferenceVideoPolicy,
} from '../src/features/pricing/types'

function createTaskModel(
  policy: ReferenceVideoPolicy,
  referencePrice?: number
): PricingModel {
  return {
    id: 1,
    model_name: 'AP Seedance-2.0 标准版',
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    enable_groups: ['default', 'free'],
    group_ratio: { default: 1, free: 0 },
    billing_mode: 'task_pricing',
    task_pricing: {
      unit: 'second',
      no_reference_video_unit_price: 0.12,
      reference_video_policy: policy,
      ...(referencePrice === undefined
        ? {}
        : { reference_video_unit_price: referencePrice }),
    },
  }
}

describe('local task pricing display', () => {
  test('normalizes the backend task-pricing-required model list', () => {
    expect(
      parseTaskPricingRequiredModels(
        '["AP Seedance-2.0 标准版", " AP Seedance-2.0 VIP ", "AP Seedance-2.0 标准版", 7]'
      )
    ).toEqual(['AP Seedance-2.0 标准版', 'AP Seedance-2.0 VIP'])
    expect(parseTaskPricingRequiredModels('{"invalid":true}')).toEqual([])
  })

  test('validates every task-pricing policy and positive price boundary', () => {
    expect(isValidTaskPricing(createTaskModel('same').task_pricing)).toBe(true)
    expect(isValidTaskPricing(createTaskModel('disabled').task_pricing)).toBe(
      true
    )
    expect(isValidTaskPricing(createTaskModel('custom', 0.18).task_pricing)).toBe(
      true
    )
    expect(isValidTaskPricing(createTaskModel('custom').task_pricing)).toBe(
      false
    )
    expect(isValidTaskPricing(createTaskModel('custom', 0).task_pricing)).toBe(
      false
    )

    const negative = createTaskModel('same').task_pricing!
    negative.no_reference_video_unit_price = -0.01
    expect(isValidTaskPricing(negative)).toBe(false)

    const unknown = createTaskModel('same').task_pricing!
    unknown.reference_video_policy = 'unknown' as ReferenceVideoPolicy
    expect(isValidTaskPricing(unknown)).toBe(false)
  })

  test('uses two local prices for custom video input pricing', () => {
    const model = createTaskModel('custom', 0.18)
    const info = getTaskPriceInfo(model, {
      group: 'default',
      groupRatio: model.group_ratio,
    })

    expect(isTaskPricingModel(model)).toBe(true)
    expect(info?.hasRange).toBe(true)
    expect(info?.noReferencePrice).toContain('0.12')
    expect(info?.referencePrice).toContain('0.18')
  })

  test('reuses the base price when video input has the same price', () => {
    const model = createTaskModel('same')
    const info = getTaskPriceInfo(model, {
      group: 'default',
      groupRatio: model.group_ratio,
    })

    expect(info?.hasRange).toBe(false)
    expect(info?.referencePrice).toBe(info?.noReferencePrice)
  })

  test('omits the video-input price when video input is disabled', () => {
    const model = createTaskModel('disabled')
    const info = getTaskPriceInfo(model, {
      group: 'default',
      groupRatio: model.group_ratio,
    })

    expect(info?.referencePrice).toBeUndefined()
  })

  test('preserves an explicit zero group ratio as free', () => {
    const model = createTaskModel('custom', 0.18)
    const info = getTaskPriceInfo(model, {
      group: 'free',
      groupRatio: model.group_ratio,
    })

    expect(info?.isFree).toBe(true)
    expect(info?.noReferencePrice).toContain('0')
    expect(info?.referencePrice).toContain('0')
  })

  test('rejects missing or non-positive task prices', () => {
    const model = createTaskModel('custom', 0)
    model.task_pricing!.no_reference_video_unit_price = 0

    expect(isTaskPricingModel(model)).toBe(false)
    expect(getTaskPriceInfo(model)).toBeNull()
  })
})
