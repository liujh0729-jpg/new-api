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
import { useQuery } from '@tanstack/react-query'
import { CheckmarkCircle02Icon, RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { getValidationSession, virtualCharacterQueryKeys } from '../api'
import type { VirtualCharacterValidationSession } from '../types'
import { validationStatusLabel } from './utils'

export function ValidationDialog({
  session,
  onClose,
  onRetry,
  onSucceeded,
}: {
  session: VirtualCharacterValidationSession | null
  onClose: () => void
  onRetry: () => void
  onSucceeded: (characterID?: number) => void
}) {
  const { t } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  const notified = useRef(false)
  const query = useQuery({
    queryKey: virtualCharacterQueryKeys.validation(session?.id ?? ''),
    queryFn: () => getValidationSession(session?.id ?? ''),
    enabled: Boolean(session?.id),
    refetchInterval: (current) =>
      current.state.data?.data.status === 'pending' ? 3000 : false,
    initialData: session ? { success: true, data: session } : undefined,
  })
  const current = query.data?.data ?? session
  useEffect(() => {
    if (!session) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [session])
  useEffect(() => {
    if (current?.status === 'succeeded' && !notified.current) {
      notified.current = true
      onSucceeded(current.character_id)
    }
  }, [current?.character_id, current?.status, onSucceeded])
  if (!session || !current) return null
  const remaining = Math.max(0, current.expires_at * 1000 - now)
  const remainingSeconds = Math.ceil(remaining / 1000)
  const progress = Math.max(
    0,
    Math.min(100, (remaining / (30 * 60 * 1000)) * 100)
  )
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Real-person validation')}</DialogTitle>
          <DialogDescription>
            {t(
              'Scan the QR code with the person being validated and finish the H5 flow.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col items-center gap-5'>
          {current.status === 'pending' && session.launch_url ? (
            <div className='rounded-xl border bg-white p-4'>
              <QRCodeSVG value={session.launch_url} size={220} level='M' />
            </div>
          ) : current.status === 'succeeded' ? (
            <HugeiconsIcon
              icon={CheckmarkCircle02Icon}
              className='text-primary size-16'
            />
          ) : (
            <Alert variant='destructive'>
              <AlertTitle>
                {current.status === 'expired'
                  ? t('Validation expired')
                  : t('Validation failed')}
              </AlertTitle>
              <AlertDescription>{current.last_error}</AlertDescription>
            </Alert>
          )}
          <div className='w-full'>
            <Progress value={progress}>
              <ProgressLabel>
                {validationStatusLabel(current.status, t)}
              </ProgressLabel>
              <span className='text-muted-foreground ml-auto text-sm tabular-nums'>
                {t('{{count}} seconds', { count: remainingSeconds })}
              </span>
            </Progress>
          </div>
          {current.status === 'pending' && (
            <div className='flex flex-wrap justify-center gap-2'>
              <Button
                type='button'
                onClick={() =>
                  window.open(
                    session.launch_url,
                    '_blank',
                    'noopener,noreferrer'
                  )
                }
              >
                {t('Open validation page')}
              </Button>
              <Button
                type='button'
                variant='outline'
                onClick={() => query.refetch()}
              >
                <HugeiconsIcon icon={RefreshIcon} data-icon='inline-start' />
                {t('Check status')}
              </Button>
            </div>
          )}
          {(current.status === 'failed' || current.status === 'expired') && (
            <Button type='button' onClick={onRetry}>
              {t('Retry validation')}
            </Button>
          )}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
