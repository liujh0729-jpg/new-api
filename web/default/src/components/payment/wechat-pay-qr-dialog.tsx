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
import { useEffect, useRef, useState } from 'react'
import type { WechatPayOrderView } from '@/types/wechat-pay'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'

interface WechatPayQrDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: WechatPayOrderView | null
  refreshOrder: (tradeNo: string) => Promise<WechatPayOrderView>
  onPaid?: () => unknown | Promise<unknown>
  testMode?: boolean
}

function statusVariant(status: WechatPayOrderView['status']) {
  if (status === 'failed') return 'destructive' as const
  if (status === 'success') return 'default' as const
  if (status === 'expired') return 'outline' as const
  return 'secondary' as const
}

export function WechatPayQrDialog({
  open,
  onOpenChange,
  order,
  refreshOrder,
  onPaid,
  testMode = false,
}: WechatPayQrDialogProps) {
  const { t } = useTranslation()
  const [current, setCurrent] = useState(order)
  const [remaining, setRemaining] = useState(0)
  const paidTradeNoRef = useRef<string | null>(null)

  useEffect(() => {
    setCurrent(order)
    if (order?.trade_no !== paidTradeNoRef.current) {
      paidTradeNoRef.current = null
    }
  }, [order])

  useEffect(() => {
    if (!open || !current?.expires_at) return
    const updateRemaining = () => {
      setRemaining(
        Math.max(0, current.expires_at - Math.floor(Date.now() / 1000))
      )
    }
    updateRemaining()
    const timer = window.setInterval(updateRemaining, 1000)
    return () => window.clearInterval(timer)
  }, [open, current?.expires_at])

  const currentTradeNo = current?.trade_no
  const currentStatus = current?.status

  useEffect(() => {
    if (!open || !currentTradeNo || currentStatus !== 'pending') return
    let cancelled = false
    let refreshing = false

    const refresh = async () => {
      if (refreshing) return
      refreshing = true
      try {
        const next = await refreshOrder(currentTradeNo)
        if (!cancelled) setCurrent(next)
      } catch {
        // A temporary polling failure must not turn a valid payment into a UI failure.
      } finally {
        refreshing = false
      }
    }

    void refresh()
    const timer = window.setInterval(refresh, 2500)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [open, currentTradeNo, currentStatus, refreshOrder])

  useEffect(() => {
    if (
      current?.status === 'success' &&
      current.trade_no !== paidTradeNoRef.current
    ) {
      paidTradeNoRef.current = current.trade_no
      void onPaid?.()
    }
  }, [current?.status, current?.trade_no, onPaid])

  const statusLabel = current ? t(current.status) : t('pending')
  const minutes = Math.floor(remaining / 60)
  const seconds = remaining % 60

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-md'>
        <DialogHeader>
          <DialogTitle>
            {testMode ? t('Verify WeChat Pay') : t('Scan with WeChat to pay')}
          </DialogTitle>
          <DialogDescription>
            {testMode
              ? t('Pay ¥0.01 to verify the real callback and settlement path.')
              : t('Keep this window open while WeChat confirms the payment.')}
          </DialogDescription>
        </DialogHeader>

        {current && (
          <div className='flex flex-col items-center gap-4'>
            <div className='flex w-full items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Payment status')}
              </span>
              <Badge variant={statusVariant(current.status)}>
                {statusLabel}
              </Badge>
            </div>

            {current.status === 'pending' && current.code_url ? (
              <div className='ring-foreground/10 rounded-xl bg-white p-4 ring-1'>
                <QRCodeSVG value={current.code_url} size={220} level='M' />
              </div>
            ) : current.status === 'pending' ? (
              <div className='flex min-h-56 items-center justify-center'>
                <Spinner />
              </div>
            ) : null}

            <div className='flex w-full flex-col gap-2 text-sm'>
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground'>{t('You Pay')}</span>
                <span className='font-medium'>
                  ¥{(current.payment_amount_cents / 100).toFixed(2)}
                </span>
              </div>
              <div className='flex items-center justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Order number')}
                </span>
                <code className='truncate text-xs'>{current.trade_no}</code>
              </div>
              {current.status === 'pending' && (
                <div className='flex items-center justify-between'>
                  <span className='text-muted-foreground'>
                    {t('Expires in')}
                  </span>
                  <span className='tabular-nums'>
                    {minutes}:{seconds.toString().padStart(2, '0')}
                  </span>
                </div>
              )}
            </div>

            {testMode && (
              <Alert>
                <AlertTitle>{t('Configuration test only')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'This payment verifies the callback but never credits a user balance.'
                  )}
                </AlertDescription>
              </Alert>
            )}

            {current.status === 'success' && (
              <Alert>
                <AlertTitle>{t('Payment confirmed')}</AlertTitle>
                <AlertDescription>
                  {testMode
                    ? t('The WeChat Pay chain has been verified successfully.')
                    : t('Your balance has been credited.')}
                </AlertDescription>
              </Alert>
            )}

            {(current.status === 'expired' || current.status === 'failed') && (
              <Alert variant='destructive'>
                <AlertTitle>{t('Payment not completed')}</AlertTitle>
                <AlertDescription>
                  {t('Close this window and create a new payment order.')}
                </AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {current?.status === 'success' ? t('Done') : t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
