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
import { api } from '@/lib/api'
import type { PricingData, PricingModel, TaskPricing } from './types'

// ----------------------------------------------------------------------------
// Pricing APIs
// ----------------------------------------------------------------------------

// Get model pricing data
export async function getPricing(): Promise<PricingData> {
  const res = await api.get('/api/pricing')
  return res.data
}

function cnyPriceToUSD(amount: number | undefined, exchangeRate: number) {
  if (amount === undefined || amount <= 0 || !Number.isFinite(amount)) {
    return amount
  }
  return amount / exchangeRate
}

function normalizeTaskPricingToUSD(
  pricing: TaskPricing,
  exchangeRate: number
): TaskPricing {
  if (pricing.by_resolution) {
    return {
      ...pricing,
      by_resolution: Object.fromEntries(
        Object.entries(pricing.by_resolution).map(([resolution, tier]) => [
          resolution,
          {
            ...tier,
            no_reference_video_unit_price:
              cnyPriceToUSD(tier.no_reference_video_unit_price, exchangeRate) ??
              tier.no_reference_video_unit_price,
            ...(tier.reference_video_unit_price === undefined
              ? {}
              : {
                  reference_video_unit_price: cnyPriceToUSD(
                    tier.reference_video_unit_price,
                    exchangeRate
                  ),
                }),
          },
        ])
      ),
    }
  }

  return {
    ...pricing,
    no_reference_video_unit_price:
      cnyPriceToUSD(pricing.no_reference_video_unit_price, exchangeRate) ??
      pricing.no_reference_video_unit_price,
    ...(pricing.reference_video_unit_price === undefined
      ? {}
      : {
          reference_video_unit_price: cnyPriceToUSD(
            pricing.reference_video_unit_price,
            exchangeRate
          ),
        }),
  }
}

// The public pricing API exposes explicit monetary fields in CNY. Pricing UI
// calculations still use the existing internal USD helpers, so normalize only
// those fields at the API boundary and let the configured formatter render CNY.
export function normalizePricingModelsToUSD(
  models: PricingModel[],
  currency?: string,
  exchangeRate?: number
): PricingModel[] {
  if (
    currency?.trim().toUpperCase() !== 'CNY' ||
    exchangeRate === undefined ||
    !Number.isFinite(exchangeRate) ||
    exchangeRate <= 0
  ) {
    return models
  }

  return models.map((model) => ({
    ...model,
    ...(model.model_price === undefined
      ? {}
      : { model_price: cnyPriceToUSD(model.model_price, exchangeRate) }),
    ...(model.task_pricing
      ? {
          task_pricing: normalizeTaskPricingToUSD(
            model.task_pricing,
            exchangeRate
          ),
        }
      : {}),
  }))
}
