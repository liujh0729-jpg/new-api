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
import { useState, useEffect, useCallback, useMemo } from 'react'
import type { WechatPayOrderView } from '@/types/wechat-pay'
import { useTranslation } from 'react-i18next'
import { getSelf } from '@/lib/api'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { SectionPageLayout } from '@/components/layout'
import { WechatPayQrDialog } from '@/components/payment/wechat-pay-qr-dialog'
import { getWechatPayNativeOrder } from './api'
import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { CreemConfirmDialog } from './components/dialogs/creem-confirm-dialog'
import { PaymentConfirmDialog } from './components/dialogs/payment-confirm-dialog'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import { RechargeFormCard } from './components/recharge-form-card'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import { WalletStatsCard } from './components/wallet-stats-card'
import { DEFAULT_DISCOUNT_RATE } from './constants'
import {
  useTopupInfo,
  usePayment,
  useAffiliate,
  useRedemption,
  useCreemPayment,
  useWaffoPayment,
  useWaffoPancakePayment,
  useWechatPay,
} from './hooks'
import {
  getDefaultPaymentType,
  getMinTopupAmount,
  isWaffoPancakePayment,
  isWechatPayNative,
} from './lib'
import type {
  UserWalletData,
  PaymentMethod,
  PresetAmount,
  CreemProduct,
  WaffoPayMethod,
} from './types'

interface WalletProps {
  initialShowHistory?: boolean
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [topupAmount, setTopupAmount] = useState(0)
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null)
  const [selectedPaymentMethod, setSelectedPaymentMethod] =
    useState<PaymentMethod>()
  const [selectedWaffoIndex, setSelectedWaffoIndex] = useState<number | null>(
    null
  )
  const [paymentLoading, setPaymentLoading] = useState<string | null>(null)
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [redemptionCode, setRedemptionCode] = useState('')
  const [creemDialogOpen, setCreemDialogOpen] = useState(false)
  const [selectedCreemProduct, setSelectedCreemProduct] =
    useState<CreemProduct | null>(null)
  const [showSubscriptionPanel, setShowSubscriptionPanel] = useState(true)
  const [wechatPayOrder, setWechatPayOrder] =
    useState<WechatPayOrderView | null>(null)
  const [wechatPayDialogOpen, setWechatPayDialogOpen] = useState(false)

  const { status } = useStatus()
  const { currency } = useSystemConfig()
  const { topupInfo, presetAmounts, loading: topupLoading } = useTopupInfo()

  // Calculate effective exchange rate - when display type is USD, use rate of 1
  const effectiveUsdExchangeRate = useMemo(() => {
    return currency?.quotaDisplayType === 'USD'
      ? 1
      : currency?.usdExchangeRate || 1
  }, [currency?.quotaDisplayType, currency?.usdExchangeRate])
  const providerAmountUnit =
    currency?.quotaDisplayType === 'TOKENS' ? 'TOKENS' : 'USD'
  const {
    amount: paymentAmount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    calculationError,
  } = usePayment()
  const {
    affiliateLink,
    loading: affiliateLoading,
    transferQuota,
    transferring,
  } = useAffiliate()
  const { redeeming, redeemCode } = useRedemption()
  const { processing: creemProcessing, processCreemPayment } = useCreemPayment()
  const { processing: waffoProcessing, processWaffoPayment } = useWaffoPayment()
  const { processing: wechatPayProcessing, createOrder: createWechatPayOrder } =
    useWechatPay()
  const { processing: pancakeProcessing, processWaffoPancakePayment } =
    useWaffoPancakePayment()

  // Fetch and refresh user data
  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch user data:', error)
    } finally {
      setUserLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (props.initialShowHistory) {
      setBillingDialogOpen(true)
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [props.initialShowHistory])

  // Initialize payment method and amount when topup info is loaded.
  useEffect(() => {
    if (topupInfo && !selectedPaymentMethod) {
      const method = topupInfo.pay_methods?.[0]
      const firstWaffoMethod = topupInfo.waffo_pay_methods?.[0]
      const initialMethod =
        method ||
        (firstWaffoMethod
          ? {
              name: firstWaffoMethod.name,
              type: 'waffo',
              icon: firstWaffoMethod.icon,
              min_topup: topupInfo.waffo_min_topup || 1,
              amount_unit: 'PROVIDER' as const,
            }
          : undefined)
      if (!initialMethod) return
      const minTopup = initialMethod.min_topup || getMinTopupAmount(topupInfo)
      setSelectedPaymentMethod(initialMethod)
      if (!method) setSelectedWaffoIndex(0)
      setTopupAmount(minTopup)
      calculatePaymentAmount(
        minTopup,
        initialMethod.type,
        initialMethod.amount_unit
      )
    }
  }, [topupInfo, selectedPaymentMethod, calculatePaymentAmount])

  // Get current payment type (selected or default)
  const getCurrentPaymentType = useCallback(() => {
    return selectedPaymentMethod?.type || getDefaultPaymentType(topupInfo)
  }, [selectedPaymentMethod, topupInfo])

  // Handle preset selection
  const handleSelectPreset = (preset: PresetAmount) => {
    setTopupAmount(preset.value)
    setSelectedPreset(preset.value)
    calculatePaymentAmount(
      preset.value,
      getCurrentPaymentType(),
      selectedPaymentMethod?.amount_unit
    )
  }

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    setTopupAmount(amount)
    setSelectedPreset(null)
    calculatePaymentAmount(
      amount,
      getCurrentPaymentType(),
      selectedPaymentMethod?.amount_unit
    )
  }

  // Handle payment method selection
  const handlePaymentMethodSelect = (method: PaymentMethod) => {
    setSelectedPaymentMethod(method)
    setSelectedWaffoIndex(null)
    setSelectedPreset(null)
    const minTopup = method.min_topup || getMinTopupAmount(topupInfo)
    setTopupAmount(minTopup)
    calculatePaymentAmount(minTopup, method.type, method.amount_unit)
  }

  const handleWaffoMethodSelect = (method: WaffoPayMethod, index: number) => {
    const minTopup = topupInfo?.waffo_min_topup || 1
    const paymentMethod: PaymentMethod = {
      name: method.name,
      type: 'waffo',
      icon: method.icon,
      min_topup: minTopup,
      amount_unit: 'PROVIDER',
    }
    setSelectedPaymentMethod(paymentMethod)
    setSelectedWaffoIndex(index)
    setSelectedPreset(null)
    setTopupAmount(minTopup)
    calculatePaymentAmount(minTopup, 'waffo', 'PROVIDER')
  }

  const handleContinuePayment = async () => {
    if (!selectedPaymentMethod) return
    const minTopup =
      selectedPaymentMethod.min_topup || getMinTopupAmount(topupInfo)
    if (!Number.isInteger(topupAmount) || topupAmount < minTopup) return

    setPaymentLoading(selectedPaymentMethod.type)
    try {
      const calculated = await calculatePaymentAmount(
        topupAmount,
        selectedPaymentMethod.type,
        selectedPaymentMethod.amount_unit
      )
      if (calculated <= 0) return
      setConfirmDialogOpen(true)
    } finally {
      setPaymentLoading(null)
    }
  }

  // Handle payment confirmation
  const handlePaymentConfirm = async () => {
    if (!selectedPaymentMethod) return

    if (isWechatPayNative(selectedPaymentMethod.type)) {
      const order = await createWechatPayOrder(
        topupAmount,
        selectedPaymentMethod.amount_unit
      )
      if (order) {
        setConfirmDialogOpen(false)
        setWechatPayOrder(order)
        setWechatPayDialogOpen(true)
      }
      return
    }

    let success: boolean
    if (selectedPaymentMethod.type === 'waffo' && selectedWaffoIndex !== null) {
      success = await processWaffoPayment(topupAmount, selectedWaffoIndex)
    } else if (isWaffoPancakePayment(selectedPaymentMethod.type)) {
      success = await processWaffoPancakePayment(topupAmount)
    } else {
      success = await processPayment(
        topupAmount,
        selectedPaymentMethod.type,
        selectedPaymentMethod.amount_unit
      )
    }

    if (success) {
      setConfirmDialogOpen(false)
      await fetchUser()
    }
  }

  // Handle redemption
  const handleRedeem = async () => {
    if (!redemptionCode) return

    const success = await redeemCode(redemptionCode)
    if (success) {
      setRedemptionCode('')
      await fetchUser()
    }
  }

  // Handle transfer
  const handleTransfer = async (amount: number) => {
    const success = await transferQuota(amount)
    if (success) {
      await fetchUser()
    }
    return success
  }

  // Handle Creem product selection
  const handleCreemProductSelect = (product: CreemProduct) => {
    setSelectedCreemProduct(product)
    setCreemDialogOpen(true)
  }

  // Handle Creem payment confirmation
  const handleCreemConfirm = async () => {
    if (!selectedCreemProduct) return

    const success = await processCreemPayment(selectedCreemProduct.productId)
    if (success) {
      setCreemDialogOpen(false)
      setSelectedCreemProduct(null)
      await fetchUser()
    }
  }

  // Get discount rate for current topup amount
  const getDiscountRate = useCallback(() => {
    return topupInfo?.discount?.[topupAmount] || DEFAULT_DISCOUNT_RATE
  }, [topupInfo, topupAmount])

  const handleSubscriptionAvailabilityChange = useCallback(
    (available: boolean) => {
      setShowSubscriptionPanel(available)
    },
    []
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Wallet')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage your balance and payment methods')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <WalletStatsCard user={user} loading={userLoading} />

            <div
              className={
                showSubscriptionPanel
                  ? 'grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)] xl:items-start'
                  : 'grid gap-4'
              }
            >
              <div id='wallet-add-funds' className='scroll-mt-4'>
                <RechargeFormCard
                  topupInfo={topupInfo}
                  presetAmounts={presetAmounts}
                  selectedPreset={selectedPreset}
                  onSelectPreset={handleSelectPreset}
                  topupAmount={topupAmount}
                  onTopupAmountChange={handleTopupAmountChange}
                  paymentAmount={paymentAmount}
                  calculating={calculating}
                  calculationError={calculationError}
                  selectedPaymentMethod={selectedPaymentMethod}
                  onPaymentMethodSelect={handlePaymentMethodSelect}
                  onContinuePayment={handleContinuePayment}
                  paymentLoading={paymentLoading}
                  redemptionCode={redemptionCode}
                  onRedemptionCodeChange={setRedemptionCode}
                  onRedeem={handleRedeem}
                  redeeming={redeeming}
                  topupLink={topupInfo?.topup_link}
                  loading={topupLoading}
                  priceRatio={(status?.price as number) || 1}
                  usdExchangeRate={effectiveUsdExchangeRate}
                  providerAmountUnit={providerAmountUnit}
                  onOpenBilling={() => setBillingDialogOpen(true)}
                  creemProducts={topupInfo?.creem_products}
                  enableCreemTopup={topupInfo?.enable_creem_topup}
                  onCreemProductSelect={handleCreemProductSelect}
                  enableWaffoTopup={topupInfo?.enable_waffo_topup}
                  waffoPayMethods={topupInfo?.waffo_pay_methods}
                  selectedWaffoIndex={selectedWaffoIndex}
                  onWaffoMethodSelect={handleWaffoMethodSelect}
                  enableWaffoPancakeTopup={
                    topupInfo?.enable_waffo_pancake_topup
                  }
                />
              </div>

              <SubscriptionPlansCard
                topupInfo={topupInfo}
                onAvailabilityChange={handleSubscriptionAvailabilityChange}
              />
            </div>

            <AffiliateRewardsCard
              user={user}
              affiliateLink={affiliateLink}
              onTransfer={() => setTransferDialogOpen(true)}
              loading={affiliateLoading}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PaymentConfirmDialog
        open={confirmDialogOpen}
        onOpenChange={setConfirmDialogOpen}
        onConfirm={handlePaymentConfirm}
        topupAmount={topupAmount}
        paymentAmount={paymentAmount}
        paymentMethod={selectedPaymentMethod}
        calculating={calculating}
        processing={
          processing ||
          pancakeProcessing ||
          waffoProcessing ||
          wechatPayProcessing
        }
        discountRate={getDiscountRate()}
        usdExchangeRate={effectiveUsdExchangeRate}
        amountUnit={selectedPaymentMethod?.amount_unit}
        providerAmountUnit={providerAmountUnit}
      />

      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={user?.aff_quota ?? 0}
        transferring={transferring}
      />

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
      />

      <CreemConfirmDialog
        open={creemDialogOpen}
        onOpenChange={setCreemDialogOpen}
        onConfirm={handleCreemConfirm}
        product={selectedCreemProduct}
        processing={creemProcessing}
      />

      <WechatPayQrDialog
        open={wechatPayDialogOpen}
        onOpenChange={setWechatPayDialogOpen}
        order={wechatPayOrder}
        refreshOrder={getWechatPayNativeOrder}
        onPaid={fetchUser}
      />
    </>
  )
}
