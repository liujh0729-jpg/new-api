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
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { createVirtualCharacter } from '../api'
import { errorMessage, splitTags } from './utils'

export function CreateVirtualCharacterDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => Promise<void> | void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const reset = () => {
    setName('')
    setDescription('')
    setTags('')
    setFile(null)
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!file) {
      toast.error(t('Character image is required'))
      return
    }
    setSubmitting(true)
    try {
      await createVirtualCharacter({
        name,
        description,
        tags: splitTags(tags),
        file,
      })
      toast.success(t('Character uploaded'))
      reset()
      props.onOpenChange(false)
      await props.onCreated()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to upload character')))
    } finally {
      setSubmitting(false)
    }
  }

  const changeOpen = (open: boolean) => {
    if (!open) reset()
    props.onOpenChange(open)
  }

  return (
    <Dialog open={props.open} onOpenChange={changeOpen}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Upload character')}</DialogTitle>
          <DialogDescription>
            {t(
              'Upload one image for this character. A new image must be uploaded as a new character.'
            )}
          </DialogDescription>
        </DialogHeader>
        <form className='flex flex-col gap-4' onSubmit={onSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='virtual-character-name'>
                {t('Name')}
              </FieldLabel>
              <Input
                id='virtual-character-name'
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                maxLength={191}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='virtual-character-description'>
                {t('Description')}
              </FieldLabel>
              <Textarea
                id='virtual-character-description'
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                rows={3}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='virtual-character-tags'>
                {t('Tags (comma separated)')}
              </FieldLabel>
              <Input
                id='virtual-character-tags'
                value={tags}
                onChange={(event) => setTags(event.target.value)}
                placeholder={t('female, young, casual')}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='virtual-character-primary-image'>
                {t('Character image')}
              </FieldLabel>
              <Input
                id='virtual-character-primary-image'
                type='file'
                accept='image/jpeg,image/png,image/webp,image/gif,image/heic,.jpg,.jpeg,.png,.webp,.gif,.heic'
                required
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <FieldDescription>
                {t(
                  'This image is the character subject reference (asset://). Images up to 30 MB.'
                )}
              </FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => changeOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={submitting || !file}>
              {submitting ? <Spinner /> : null}
              {t('Upload')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
