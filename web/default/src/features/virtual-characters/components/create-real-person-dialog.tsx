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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { createValidationSession } from '../api'
import type { VirtualCharacterValidationSession } from '../types'
import { errorMessage, splitTags } from './utils'

function defaultAuthorizationExpiry() {
  const value = new Date()
  value.setFullYear(value.getFullYear() + 1)
  value.setMinutes(value.getMinutes() - value.getTimezoneOffset())
  return value.toISOString().slice(0, 16)
}

export function CreateRealPersonDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (session: VirtualCharacterValidationSession) => void
}) {
  const { t, i18n } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [validUntil, setValidUntil] = useState(defaultAuthorizationExpiry)
  const [purposes, setPurposes] = useState('')
  const [regions, setRegions] = useState('')
  const [platforms, setPlatforms] = useState('')
  const [industries, setIndustries] = useState('')
  const [commercialUseAllowed, setCommercialUseAllowed] = useState(false)
  const [agreementAccepted, setAgreementAccepted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    try {
      const response = await createValidationSession({
        name,
        description,
        tags: splitTags(tags),
        language: i18n.language.startsWith('zh') ? 'zh' : 'en',
        authorization: {
          valid_until: Math.floor(new Date(validUntil).getTime() / 1000),
          commercial_use_allowed: commercialUseAllowed,
          purposes: splitTags(purposes),
          regions: splitTags(regions),
          platforms: splitTags(platforms),
          industries: splitTags(industries),
          agreement_accepted: agreementAccepted,
        },
      })
      onCreated(response.data)
      setName('')
      setDescription('')
      setTags('')
      setValidUntil(defaultAuthorizationExpiry())
      setPurposes('')
      setRegions('')
      setPlatforms('')
      setIndustries('')
      setCommercialUseAllowed(false)
      setAgreementAccepted(false)
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to create validation session')))
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <form className='flex flex-col gap-5' onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t('Create real-person character')}</DialogTitle>
            <DialogDescription>
              {t(
                'A 30-minute H5 identity validation starts before the actor group is created.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Alert>
            <AlertTitle>{t('Validation protects portrait rights')}</AlertTitle>
            <AlertDescription>
              {t('Only the validated user may use this actor group.')}
            </AlertDescription>
          </Alert>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='real-character-name'>{t('Name')}</FieldLabel>
              <Input
                id='real-character-name'
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field>
                <FieldLabel htmlFor='real-character-valid-until'>
                  {t('Authorization expires at')}
                </FieldLabel>
                <Input
                  id='real-character-valid-until'
                  type='datetime-local'
                  required
                  value={validUntil}
                  onChange={(event) => setValidUntil(event.target.value)}
                />
                <FieldDescription>
                  {t('The character is disabled immediately after this time.')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='real-character-purposes'>
                  {t('Authorized purposes')}
                </FieldLabel>
                <Input
                  id='real-character-purposes'
                  required
                  value={purposes}
                  onChange={(event) => setPurposes(event.target.value)}
                  placeholder={t('Advertising, product demo')}
                />
                <FieldDescription>
                  {t('Separate multiple scope values with commas.')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='real-character-regions'>
                  {t('Authorized regions')}
                </FieldLabel>
                <Input
                  id='real-character-regions'
                  required
                  value={regions}
                  onChange={(event) => setRegions(event.target.value)}
                  placeholder={t('Mainland China')}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='real-character-platforms'>
                  {t('Authorized platforms')}
                </FieldLabel>
                <Input
                  id='real-character-platforms'
                  required
                  value={platforms}
                  onChange={(event) => setPlatforms(event.target.value)}
                  placeholder={t('Brand website, social media')}
                />
              </Field>
            </div>
            <Field>
              <FieldLabel htmlFor='real-character-industries'>
                {t('Authorized industries (optional)')}
              </FieldLabel>
              <Input
                id='real-character-industries'
                value={industries}
                onChange={(event) => setIndustries(event.target.value)}
                placeholder={t('Retail, education')}
              />
            </Field>
            <Field orientation='horizontal'>
              <Checkbox
                id='real-character-commercial'
                checked={commercialUseAllowed}
                onCheckedChange={(checked) =>
                  setCommercialUseAllowed(Boolean(checked))
                }
              />
              <FieldLabel htmlFor='real-character-commercial'>
                {t('Commercial use is included in this authorization')}
              </FieldLabel>
            </Field>
            <Field orientation='horizontal'>
              <Checkbox
                id='real-character-agreement'
                required
                checked={agreementAccepted}
                onCheckedChange={(checked) =>
                  setAgreementAccepted(Boolean(checked))
                }
              />
              <FieldLabel htmlFor='real-character-agreement'>
                {t(
                  'I confirm the portrait-right holder will review the scope above and complete Volc identity authorization.'
                )}
              </FieldLabel>
            </Field>
            <Field>
              <FieldLabel htmlFor='real-character-description'>
                {t('Description')}
              </FieldLabel>
              <Textarea
                id='real-character-description'
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='real-character-tags'>{t('Tags')}</FieldLabel>
              <Input
                id='real-character-tags'
                value={tags}
                onChange={(event) => setTags(event.target.value)}
                placeholder={t('Separate tags with commas')}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={submitting}>
              {submitting && <Spinner />}
              {t('Start validation')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
