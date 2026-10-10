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
import { useState, useEffect } from 'react'
import { Gift, ExternalLink, Loader2, Receipt, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  formatCurrency,
  getDiscountLabel,
  getPaymentIcon,
  getMinTopupAmount,
  calculatePresetPricing,
} from '../lib'
import type {
  PaymentMethod,
  PresetAmount,
  TopupInfo,
  CreemProduct,
  WaffoPayMethod,
  TopupAmountUnit,
} from '../types'
import { CreemProductsSection } from './creem-products-section'

interface RechargeFormCardProps {
  topupInfo: TopupInfo | null
  presetAmounts: PresetAmount[]
  selectedPreset: number | null
  onSelectPreset: (preset: PresetAmount) => void
  topupAmount: number
  onTopupAmountChange: (amount: number) => void
  paymentAmount: number
  calculating: boolean
  calculationError: string
  selectedPaymentMethod?: PaymentMethod
  onPaymentMethodSelect: (method: PaymentMethod) => void
  onContinuePayment: () => void
  paymentLoading: string | null
  redemptionCode: string
  onRedemptionCodeChange: (code: string) => void
  onRedeem: () => void
  redeeming: boolean
  topupLink?: string
  loading?: boolean
  priceRatio?: number
  usdExchangeRate?: number
  providerAmountUnit?: 'USD' | 'TOKENS'
  onOpenBilling?: () => void
  creemProducts?: CreemProduct[]
  enableCreemTopup?: boolean
  onCreemProductSelect?: (product: CreemProduct) => void
  enableWaffoTopup?: boolean
  waffoPayMethods?: WaffoPayMethod[]
  selectedWaffoIndex?: number | null
  onWaffoMethodSelect?: (method: WaffoPayMethod, index: number) => void
  enableWaffoPancakeTopup?: boolean
}

export function RechargeFormCard({
  topupInfo,
  presetAmounts,
  selectedPreset,
  onSelectPreset,
  topupAmount,
  onTopupAmountChange,
  paymentAmount,
  calculating,
  calculationError,
  selectedPaymentMethod,
  onPaymentMethodSelect,
  onContinuePayment,
  paymentLoading,
  redemptionCode,
  onRedemptionCodeChange,
  onRedeem,
  redeeming,
  topupLink,
  loading,
  priceRatio = 1,
  usdExchangeRate = 1,
  providerAmountUnit = 'USD',
  onOpenBilling,
  creemProducts,
  enableCreemTopup,
  onCreemProductSelect,
  enableWaffoTopup,
  waffoPayMethods,
  selectedWaffoIndex,
  onWaffoMethodSelect,
  enableWaffoPancakeTopup,
}: RechargeFormCardProps) {
  const { t } = useTranslation()
  const [localAmount, setLocalAmount] = useState(topupAmount.toString())
  const minTopup =
    selectedPaymentMethod?.min_topup || getMinTopupAmount(topupInfo)
  const amountUnit: TopupAmountUnit =
    selectedPaymentMethod?.amount_unit || 'USD'
  const effectiveAmountUnit =
    amountUnit === 'PROVIDER' ? providerAmountUnit : amountUnit
  const amountUnitLabel =
    effectiveAmountUnit === 'TOKENS' ? t('Tokens') : effectiveAmountUnit
  const formatPaymentAmount = (value: number) =>
    amountUnit === 'CNY' ? `¥${formatCurrency(value)}` : formatCurrency(value)
  const getInputError = (value: string) => {
    const numericValue = Number(value)
    if (!Number.isInteger(numericValue) || numericValue < 1) {
      return t('Top-up amount must be a positive integer')
    }
    if (numericValue < minTopup) {
      return t('Top-up amount must be an integer of at least {{amount}}', {
        amount: minTopup,
      })
    }
    return ''
  }
  const inputError = getInputError(localAmount)

  useEffect(() => {
    // Keep the editable string in sync when a preset or gateway changes it.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLocalAmount(topupAmount.toString())
  }, [selectedPaymentMethod?.name, selectedPaymentMethod?.type, topupAmount])

  const handleAmountChange = (value: string) => {
    setLocalAmount(value)
    if (getInputError(value)) return
    onTopupAmountChange(Number(value))
  }

  const hasConfigurableTopup =
    topupInfo?.enable_online_topup ||
    topupInfo?.enable_stripe_topup ||
    enableWaffoTopup ||
    enableWaffoPancakeTopup
  const hasAnyTopup = hasConfigurableTopup || enableCreemTopup
  const hasStandardPaymentMethods =
    Array.isArray(topupInfo?.pay_methods) && topupInfo.pay_methods.length > 0
  const hasWaffoPaymentMethods =
    Array.isArray(waffoPayMethods) && waffoPayMethods.length > 0

  if (loading) {
    return (
      <Card className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
          <Skeleton className='mt-2 h-4 w-48' />
        </CardHeader>
        <CardContent className='space-y-4 p-3 sm:space-y-6 sm:p-5'>
          <div className='space-y-4 sm:space-y-6'>
            {/* Preset Amounts Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-16' />
              <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className='h-[72px] rounded-lg' />
                ))}
              </div>
            </div>

            {/* Custom Amount Input Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-28' />
              <Skeleton className='h-[42px] w-full' />
            </div>

            {/* Payment Methods Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-32' />
              <div className='flex flex-wrap gap-3'>
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className='h-10 w-24 rounded-lg' />
                ))}
              </div>
            </div>
          </div>

          {/* Redemption Code Section Skeleton */}
          <div className='space-y-3 border-t pt-8'>
            <Skeleton className='h-3 w-24' />
            <div className='flex gap-2'>
              <Skeleton className='h-10 flex-1' />
              <Skeleton className='h-10 w-20' />
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <TitledCard
      title={t('Add Funds')}
      description={t('Choose a payment method, then enter an amount')}
      icon={<WalletCards className='h-4 w-4' />}
      action={
        onOpenBilling ? (
          <Button
            variant='outline'
            size='sm'
            onClick={onOpenBilling}
            className='w-full gap-2 sm:w-auto'
          >
            <Receipt className='h-4 w-4' />
            {t('Order History')}
          </Button>
        ) : null
      }
      contentClassName='space-y-4 sm:space-y-6'
    >
      {/* Online Topup Section */}
      {hasAnyTopup ? (
        <div className='space-y-4 sm:space-y-6'>
          {hasConfigurableTopup && (
            <>
              <div className='space-y-2.5 sm:space-y-3'>
                <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                  {t('Payment Method')}
                </Label>
                {hasStandardPaymentMethods ? (
                  <ToggleGroup
                    value={
                      selectedPaymentMethod &&
                      topupInfo?.pay_methods?.some(
                        (method) => method.type === selectedPaymentMethod.type
                      )
                        ? [selectedPaymentMethod.type]
                        : []
                    }
                    onValueChange={(values) => {
                      const method = topupInfo?.pay_methods?.find(
                        (item) => item.type === values[0]
                      )
                      if (method) onPaymentMethodSelect(method)
                    }}
                    variant='outline'
                    spacing={2}
                    className='grid w-full grid-cols-2 sm:grid-cols-3'
                  >
                    {topupInfo?.pay_methods?.map((method) => (
                      <ToggleGroupItem
                        key={method.type}
                        value={method.type}
                        disabled={!!paymentLoading}
                        className='min-w-0 justify-start'
                        aria-label={method.name}
                      >
                        {getPaymentIcon(
                          method.type,
                          'h-4 w-4',
                          method.icon,
                          method.name
                        )}
                        <span className='truncate'>{method.name}</span>
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                ) : hasWaffoPaymentMethods ? null : (
                  <Alert>
                    <AlertDescription>
                      {t(
                        'No payment methods available. Please contact administrator.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                {enableWaffoTopup &&
                  hasWaffoPaymentMethods &&
                  onWaffoMethodSelect && (
                    <ToggleGroup
                      value={
                        selectedWaffoIndex == null
                          ? []
                          : [`waffo-${selectedWaffoIndex}`]
                      }
                      onValueChange={(values) => {
                        const value = values[0]
                        if (!value) return
                        const index = Number(value.replace('waffo-', ''))
                        const method = waffoPayMethods?.[index]
                        if (method) onWaffoMethodSelect(method, index)
                      }}
                      variant='outline'
                      spacing={2}
                      className='grid w-full grid-cols-2 sm:grid-cols-3'
                    >
                      {waffoPayMethods?.map((method, index) => (
                        <ToggleGroupItem
                          key={`${method.name}-${index}`}
                          value={`waffo-${index}`}
                          disabled={!!paymentLoading}
                          className='min-w-0 justify-start'
                          aria-label={method.name}
                        >
                          {method.icon ? (
                            <img
                              src={method.icon}
                              alt=''
                              className='h-4 w-4 object-contain'
                            />
                          ) : (
                            getPaymentIcon('waffo')
                          )}
                          <span className='truncate'>{method.name}</span>
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  )}
              </div>

              {presetAmounts.length > 0 && (
                <div className='space-y-2.5 sm:space-y-3'>
                  <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                    {t('Amount')}
                  </Label>
                  <div className='grid grid-cols-2 gap-1.5 sm:gap-3 md:grid-cols-4'>
                    {presetAmounts.map((preset, index) => {
                      const discount =
                        preset.discount ||
                        topupInfo?.discount?.[preset.value] ||
                        1.0
                      const {
                        displayValue,
                        actualPrice,
                        savedAmount,
                        hasDiscount,
                      } = calculatePresetPricing(
                        preset.value,
                        priceRatio,
                        discount,
                        usdExchangeRate,
                        amountUnit
                      )
                      return (
                        <Button
                          key={index}
                          variant='outline'
                          disabled={preset.value < minTopup}
                          className={cn(
                            'hover:border-foreground flex min-h-16 flex-col items-start rounded-lg px-3 py-2.5 text-left whitespace-normal sm:min-h-[72px] sm:p-4',
                            selectedPreset === preset.value
                              ? 'border-foreground bg-foreground/5'
                              : 'border-muted'
                          )}
                          onClick={() => onSelectPreset(preset)}
                        >
                          <div className='flex w-full items-center justify-between'>
                            <div className='text-base font-semibold sm:text-lg'>
                              {amountUnit === 'CNY'
                                ? `¥${formatCurrency(displayValue)}`
                                : formatNumber(displayValue)}
                            </div>
                            {hasDiscount && (
                              <div className='text-xs font-medium text-green-600'>
                                {getDiscountLabel(discount)}
                              </div>
                            )}
                          </div>
                          {amountUnit !== 'PROVIDER' && (
                            <div className='text-muted-foreground mt-1.5 w-full text-xs sm:mt-2'>
                              {t('Pay')} {formatPaymentAmount(actualPrice)}
                              {hasDiscount && savedAmount > 0 && (
                                <span className='text-green-600'>
                                  {' '}
                                  • {t('Save')}{' '}
                                  {formatPaymentAmount(savedAmount)}
                                </span>
                              )}
                            </div>
                          )}
                        </Button>
                      )
                    })}
                  </div>
                </div>
              )}

              <div className='space-y-2.5 sm:space-y-3'>
                <Label
                  htmlFor='topup-amount'
                  className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
                >
                  {t('Amount ({{unit}})', { unit: amountUnitLabel })}
                </Label>
                <div className='grid grid-cols-[minmax(0,1fr)_minmax(110px,0.55fr)] gap-2 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
                  <Input
                    id='topup-amount'
                    type='number'
                    value={localAmount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    min={minTopup}
                    step={1}
                    aria-invalid={!!inputError}
                    placeholder={t('Minimum {{amount}}', { amount: minTopup })}
                    className='h-9 text-base sm:h-10 sm:text-lg'
                  />
                  <div className='bg-muted/30 flex min-h-9 items-center justify-between gap-2 rounded-md border px-3 lg:min-w-52'>
                    <span className='text-muted-foreground truncate text-xs'>
                      {t('Amount to pay:')}
                    </span>
                    {calculating ? (
                      <Skeleton className='h-5 w-16' />
                    ) : calculationError || inputError ? (
                      <span className='text-destructive text-xs'>
                        {t('Unavailable')}
                      </span>
                    ) : (
                      <span className='text-sm font-semibold'>
                        {formatPaymentAmount(paymentAmount)}
                      </span>
                    )}
                  </div>
                </div>
                {(inputError || calculationError) && (
                  <p className='text-destructive text-xs'>
                    {inputError || calculationError}
                  </p>
                )}
                <Button
                  type='button'
                  onClick={onContinuePayment}
                  disabled={
                    !selectedPaymentMethod ||
                    !!inputError ||
                    !!calculationError ||
                    calculating ||
                    !!paymentLoading
                  }
                  className='w-full'
                >
                  {paymentLoading ? (
                    <Loader2
                      className='animate-spin'
                      data-icon='inline-start'
                    />
                  ) : null}
                  {t('Continue to payment')}
                </Button>
              </div>
            </>
          )}
        </div>
      ) : (
        <Alert>
          <AlertDescription>
            {t(
              'Online topup is not enabled. Please use redemption code or contact administrator.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Creem Products Section */}
      {enableCreemTopup &&
        Array.isArray(creemProducts) &&
        creemProducts.length > 0 &&
        onCreemProductSelect && (
          <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
            <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Creem Payment')}
            </Label>
            <CreemProductsSection
              products={creemProducts}
              onProductSelect={onCreemProductSelect}
            />
          </div>
        )}

      {/* Redemption Code Section */}
      <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
        <div className='flex items-center gap-2'>
          <Gift className='text-muted-foreground h-4 w-4' />
          <Label
            htmlFor='redemption-code'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Have a Code?')}
          </Label>
        </div>
        <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
          <Input
            id='redemption-code'
            value={redemptionCode}
            onChange={(e) => onRedemptionCodeChange(e.target.value)}
            placeholder={t('Enter your redemption code')}
            className='h-9 min-w-0'
          />
          <Button
            onClick={onRedeem}
            disabled={redeeming}
            variant='outline'
            className='h-9 px-4'
          >
            {redeeming && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Redeem')}
          </Button>
        </div>
        {topupLink && (
          <p className='text-muted-foreground text-xs'>
            {t('Need a code?')}{' '}
            <a
              href={topupLink}
              target='_blank'
              rel='noopener noreferrer'
              className='inline-flex items-center gap-1 underline-offset-4 hover:underline'
            >
              {t('Purchase here')}
              <ExternalLink className='h-3 w-3' />
            </a>
          </p>
        )}
      </div>
    </TitledCard>
  )
}
