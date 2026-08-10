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
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { uploadVirtualCharacterAsset } from '../api'
import type { VirtualCharacter } from '../types'
import { errorMessage } from './utils'

export function UploadAssetDialog({
  character,
  open,
  maxAssetsPerCharacter,
  onClose,
  onUploaded,
}: {
  character: VirtualCharacter
  open: boolean
  maxAssetsPerCharacter: number
  onClose: () => void
  onUploaded: () => void
}) {
  const { t } = useTranslation()
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [assetType, setAssetType] = useState<'Image' | 'Video' | 'Audio'>(
    'Image'
  )
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!file) return
    if (character.assets.length >= maxAssetsPerCharacter) {
      toast.error(
        t(
          'Each character can have up to {{limit}} related assets. Delete an existing asset before uploading another.',
          { limit: maxAssetsPerCharacter }
        )
      )
      return
    }
    setBusy(true)
    try {
      await uploadVirtualCharacterAsset({
        characterId: character.id,
        file,
        name,
        assetType,
      })
      toast.success(t('Asset uploaded and queued for provider processing'))
      setFile(null)
      setName('')
      onClose()
      onUploaded()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to upload asset')))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(value) => !value && onClose()}>
      <DialogContent>
        <form className='flex flex-col gap-5' onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t('Upload character-related asset')}</DialogTitle>
            <DialogDescription>
              {t(
                'The file is staged privately, imported into the Volc asset group, then removed from staging. Limit: {{count}} / {{limit}}.',
                {
                  count: character.assets.length,
                  limit: maxAssetsPerCharacter,
                }
              )}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='asset-type'>{t('Asset type')}</FieldLabel>
              <NativeSelect
                id='asset-type'
                className='w-full'
                value={assetType}
                onChange={(event) =>
                  setAssetType(event.target.value as typeof assetType)
                }
              >
                <NativeSelectOption value='Image'>
                  {t('Image')}
                </NativeSelectOption>
                <NativeSelectOption value='Video'>
                  {t('Video')}
                </NativeSelectOption>
                <NativeSelectOption value='Audio'>
                  {t('Audio')}
                </NativeSelectOption>
              </NativeSelect>
              <FieldDescription>
                {t(
                  'Images up to 30 MB, videos up to 50 MB, audio up to 15 MB.'
                )}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor='asset-name'>{t('Asset name')}</FieldLabel>
              <Input
                id='asset-name'
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='asset-file'>{t('File')}</FieldLabel>
              <Input
                id='asset-file'
                type='file'
                required
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type='button' variant='outline' onClick={onClose}>
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={busy || !file}>
              {busy && <Spinner />}
              {t('Upload')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
