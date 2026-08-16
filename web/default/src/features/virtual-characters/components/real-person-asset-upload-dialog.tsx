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
import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { uploadRealPersonAsset } from '../api'
import { errorMessage } from './utils'

const MAX_PORTRAIT_BYTES = 30 * 1024 * 1024

export function RealPersonAssetUploadDialog({
  characterID,
  onClose,
  onUploaded,
}: {
  characterID: number | null
  onClose: () => void
  onUploaded: () => Promise<void> | void
}) {
  const { t } = useTranslation()
  const [file, setFile] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => setFile(null), [characterID])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!characterID || !file) {
      toast.error(t('Portrait image is required'))
      return
    }
    if (file.size > MAX_PORTRAIT_BYTES) {
      toast.error(t('Portrait image exceeds the 30 MB limit'))
      return
    }
    setSubmitting(true)
    try {
      await uploadRealPersonAsset(characterID, file)
      toast.success(t('Portrait asset uploaded and submitted for review'))
      await onUploaded()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to upload portrait asset')))
    } finally {
      setSubmitting(false)
    }
  }

  const changeOpen = (open: boolean) => {
    if (!open && !submitting) onClose()
  }

  return (
    <Dialog open={characterID !== null} onOpenChange={changeOpen}>
      <DialogContent className='sm:max-w-lg'>
        <form className='flex flex-col gap-5' onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t('Step 2 of 2: upload portrait asset')}</DialogTitle>
            <DialogDescription>
              {t(
                'Identity verification is complete. Upload one clear image of the same person to create the official Volc asset.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Alert>
            <AlertTitle>
              {t('The portrait must match the verified person')}
            </AlertTitle>
            <AlertDescription>
              {t(
                'After upload, Volc reviews the image asynchronously. The character becomes available when the asset is Active.'
              )}
            </AlertDescription>
          </Alert>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='real-person-portrait-image'>
                {t('Real-person portrait image')}
              </FieldLabel>
              <Input
                id='real-person-portrait-image'
                type='file'
                accept='image/jpeg,image/png,image/webp,image/gif,image/heic,.jpg,.jpeg,.png,.webp,.gif,.heic'
                required
                disabled={submitting}
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <FieldDescription>
                {t('JPG, PNG, WebP, GIF, or HEIC, up to 30 MB.')}
              </FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={submitting}
              onClick={onClose}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={submitting || !file}>
              {submitting && <Spinner data-icon='inline-start' />}
              {t('Upload and start review')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
