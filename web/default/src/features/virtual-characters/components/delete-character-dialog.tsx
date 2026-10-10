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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { deleteVirtualCharacter } from '../api'
import type { VirtualCharacter } from '../types'
import { errorMessage } from './utils'

export function DeleteCharacterDialog({
  target,
  onClose,
  onDeleted,
}: {
  target: VirtualCharacter | null
  onClose: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!target) return
    setBusy(true)
    try {
      await deleteVirtualCharacter(target.id)
      toast.success(
        target.source_type === 'volc_real_person'
          ? t('Authorization revocation queued')
          : t('Character deletion queued')
      )
      onClose()
      onDeleted()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to delete character')))
    } finally {
      setBusy(false)
    }
  }
  return (
    <AlertDialog
      open={target != null}
      onOpenChange={(open) => !open && onClose()}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {target?.source_type === 'volc_real_person'
              ? t('Revoke this real-person authorization?')
              : t('Delete this character?')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {target?.source_type === 'volc_real_person'
              ? t(
                  'New video requests are blocked immediately. In-flight tasks may finish, then the Asset and verified group are deleted with automatic retries.'
                )
              : t(
                  'The character is hidden immediately. Its image and provider group are deleted in the background with automatic retries.'
                )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
          <AlertDialogAction disabled={busy} onClick={remove}>
            {target?.source_type === 'volc_real_person'
              ? t('Revoke authorization')
              : t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
