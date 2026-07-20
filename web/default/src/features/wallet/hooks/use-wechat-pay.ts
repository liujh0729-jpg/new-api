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
import { useCallback, useState } from 'react'
import type { WechatPayOrderView } from '@/types/wechat-pay'
import i18next from 'i18next'
import { toast } from 'sonner'
import { requestWechatPayNative } from '../api'

export function useWechatPay() {
  const [processing, setProcessing] = useState(false)

  const createOrder = useCallback(async (amount: number) => {
    setProcessing(true)
    try {
      return await requestWechatPayNative({
        amount: Math.floor(amount),
        payment_method: 'wechat_native',
      })
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : i18next.t('Payment request failed')
      )
      return null as WechatPayOrderView | null
    } finally {
      setProcessing(false)
    }
  }, [])

  return { processing, createOrder }
}
