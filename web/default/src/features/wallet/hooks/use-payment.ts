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
import { useCallback, useRef, useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'
import type { TopupAmountUnit } from '../types'

// ============================================================================
// Payment Hook
// ============================================================================

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [calculationError, setCalculationError] = useState('')
  const calculationSequence = useRef(0)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      amountUnit?: TopupAmountUnit
    ) => {
      const sequence = ++calculationSequence.current
      if (!Number.isInteger(topupAmount) || topupAmount < 1) {
        setAmount(0)
        setCalculating(false)
        setCalculationError(
          i18next.t('Top-up amount must be a positive integer')
        )
        return 0
      }
      try {
        setCalculating(true)
        setCalculationError('')

        const isStripe = isStripePayment(paymentType)
        const isWaffo = paymentType === 'waffo'
        const isPancake = isWaffoPancakePayment(paymentType)
        const request = {
          amount: topupAmount,
          ...(amountUnit && amountUnit !== 'PROVIDER'
            ? { amount_unit: amountUnit }
            : {}),
        }
        const response = isStripe
          ? await calculateStripeAmount(request)
          : isWaffo
            ? await calculateWaffoAmount(request)
            : isPancake
              ? await calculateWaffoPancakeAmount(request)
              : await calculateAmount(request)

        if (sequence !== calculationSequence.current) return 0

        if (isApiSuccess(response)) {
          const calculatedAmount = Number(response.data)
          if (!Number.isFinite(calculatedAmount) || calculatedAmount <= 0) {
            setAmount(0)
            setCalculationError(i18next.t('Unable to calculate payment amount'))
            return 0
          }
          setAmount(calculatedAmount)
          setCalculationError('')
          return calculatedAmount
        }

        setAmount(0)
        setCalculationError(
          String(
            response.data ||
              response.message ||
              i18next.t('Unable to calculate payment amount')
          )
        )
        return 0
      } catch (error) {
        if (sequence !== calculationSequence.current) return 0
        setAmount(0)
        setCalculationError(
          error instanceof Error
            ? error.message
            : i18next.t('Unable to calculate payment amount')
        )
        return 0
      } finally {
        if (sequence === calculationSequence.current) {
          setCalculating(false)
        }
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      amountUnit?: TopupAmountUnit
    ) => {
      if (!Number.isInteger(topupAmount) || topupAmount < 1) {
        toast.error(i18next.t('Top-up amount must be a positive integer'))
        return false
      }
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)

        const response = isStripe
          ? await requestStripePayment({
              amount: topupAmount,
              payment_method: 'stripe',
            })
          : await requestPayment({
              amount: topupAmount,
              payment_method: paymentType,
              ...(amountUnit && amountUnit !== 'PROVIDER'
                ? { amount_unit: amountUnit }
                : {}),
            })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        // Handle Stripe payment
        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        return false
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
    calculationError,
  }
}
