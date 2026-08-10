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
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { createCharacterVideo } from '../api'
import type { VirtualCharacter, VirtualCharacterAsset } from '../types'
import { errorMessage } from './utils'

type GenerateTarget = {
  character: VirtualCharacter
  asset?: VirtualCharacterAsset
}

export function GenerateDialog(props: {
  target: GenerateTarget | null
  models: string[]
  onClose: () => void
  onCreated: () => void
}) {
  return (
    <Dialog
      open={props.target != null}
      onOpenChange={(open) => !open && props.onClose()}
    >
      {props.target ? (
        <GenerateDialogForm
          key={`${props.target.character.id}-${props.target.asset?.id ?? 0}-${props.models.join(',')}`}
          target={props.target}
          models={props.models}
          onClose={props.onClose}
          onCreated={props.onCreated}
        />
      ) : null}
    </Dialog>
  )
}

function GenerateDialogForm(props: {
  target: GenerateTarget
  models: string[]
  onClose: () => void
  onCreated: () => void
}) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const [modelName, setModelName] = useState(props.models[0] || '')
  const [duration, setDuration] = useState(5)
  const [ratio, setRatio] = useState('16:9')
  const [resolution, setResolution] = useState('720p')
  const [assetID, setAssetID] = useState<number | undefined>(
    props.target.asset?.id ?? props.target.character.primary_asset_id
  )
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    try {
      const response = await createCharacterVideo({
        character_id: props.target.character.id,
        character_asset_id: assetID,
        model: modelName,
        prompt,
        duration,
        ratio,
        resolution,
      })
      if (response.error?.message) throw new Error(response.error.message)
      toast.success(t('Video task created'))
      props.onCreated()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to create video task')))
    } finally {
      setBusy(false)
    }
  }

  const assets =
    props.target.character.assets?.filter(
      (asset) => asset.status === 'Active'
    ) ?? []

  return (
    <DialogContent>
      <form className='flex flex-col gap-5' onSubmit={submit}>
        <DialogHeader>
          <DialogTitle>
            {t('Create video with {{name}}', {
              name: props.target.character.name,
            })}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Choose a Seedance model available to your account. The selected character asset is sent as an asset:// reference.'
            )}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          {props.target.character.scope === 'private' && assets.length > 0 && (
            <Field>
              <FieldLabel htmlFor='generation-asset'>
                {t('Character-related asset')}
              </FieldLabel>
              <NativeSelect
                id='generation-asset'
                className='w-full'
                value={assetID ?? ''}
                onChange={(event) => setAssetID(Number(event.target.value))}
              >
                {assets.map((asset) => (
                  <NativeSelectOption key={asset.id} value={asset.id}>
                    {asset.name}
                    {asset.is_primary ? ` · ${t('Primary')}` : ''}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
          )}
          <Field>
            <FieldLabel htmlFor='generation-model'>{t('Model')}</FieldLabel>
            <NativeSelect
              id='generation-model'
              className='w-full'
              required
              value={modelName}
              onChange={(event) => setModelName(event.target.value)}
              disabled={props.models.length === 0}
            >
              {props.models.length === 0 ? (
                <NativeSelectOption value=''>
                  {t('No Seedance models available for your account')}
                </NativeSelectOption>
              ) : (
                props.models.map((model) => (
                  <NativeSelectOption key={model} value={model}>
                    {model}
                  </NativeSelectOption>
                ))
              )}
            </NativeSelect>
          </Field>
          <Field>
            <FieldLabel htmlFor='generation-prompt'>{t('Prompt')}</FieldLabel>
            <Textarea
              id='generation-prompt'
              required
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              placeholder={t(
                'Describe how 图片1中的角色 should move and what scene to create'
              )}
            />
          </Field>
          <div className='grid gap-4 sm:grid-cols-3'>
            <Field>
              <FieldLabel htmlFor='generation-duration'>
                {t('Duration')}
              </FieldLabel>
              <Input
                id='generation-duration'
                type='number'
                min={2}
                max={15}
                value={duration}
                onChange={(event) => setDuration(Number(event.target.value))}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='generation-ratio'>{t('Ratio')}</FieldLabel>
              <NativeSelect
                id='generation-ratio'
                className='w-full'
                value={ratio}
                onChange={(event) => setRatio(event.target.value)}
              >
                <NativeSelectOption value='16:9'>16:9</NativeSelectOption>
                <NativeSelectOption value='9:16'>9:16</NativeSelectOption>
                <NativeSelectOption value='1:1'>1:1</NativeSelectOption>
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor='generation-resolution'>
                {t('Resolution')}
              </FieldLabel>
              <NativeSelect
                id='generation-resolution'
                className='w-full'
                value={resolution}
                onChange={(event) => setResolution(event.target.value)}
              >
                <NativeSelectOption value='720p'>720p</NativeSelectOption>
                <NativeSelectOption value='1080p'>1080p</NativeSelectOption>
              </NativeSelect>
            </Field>
          </div>
        </FieldGroup>
        <DialogFooter>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            disabled={
              busy ||
              !modelName ||
              !prompt ||
              (props.target.character.scope === 'private' && !assetID)
            }
          >
            {busy && <Spinner />}
            {t('Create video')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
