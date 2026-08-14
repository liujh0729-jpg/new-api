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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import {
  getVirtualCharacter,
  updateVirtualCharacter,
  virtualCharacterPreviewURL,
  virtualCharacterQueryKeys,
} from '../api'
import type { VirtualCharacter } from '../types'
import { authorizationStatusLabel, errorMessage, statusLabel } from './utils'

export function CharacterDetailDialog({
  characterID,
  onClose,
}: {
  characterID: number | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: virtualCharacterQueryKeys.detail(characterID ?? 0),
    queryFn: () => getVirtualCharacter(characterID ?? 0),
    enabled: Boolean(characterID),
    refetchInterval: (current) =>
      current.state.data?.data.status === 'creating' ? 5000 : false,
  })
  const character = query.data?.data

  return (
    <Dialog
      open={characterID != null}
      onOpenChange={(open) => !open && onClose()}
    >
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{character?.name ?? t('Character details')}</DialogTitle>
          <DialogDescription>
            {t(
              'View the character image, provider reference, and profile information.'
            )}
          </DialogDescription>
        </DialogHeader>
        {!character ? (
          <div className='flex justify-center py-12'>
            <Spinner className='size-6' />
          </div>
        ) : (
          <CharacterDetailContent
            key={character.id}
            character={character}
            onRefresh={() => query.refetch()}
          />
        )}
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CharacterDetailContent({
  character,
  onRefresh,
}: {
  character: VirtualCharacter
  onRefresh: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const [name, setName] = useState(character.name)
  const [description, setDescription] = useState(character.description)
  const [tags, setTags] = useState(character.tags.join(', '))
  const [busy, setBusy] = useState(false)
  const editable = character.scope === 'private'
  const providerAssetID = character.provider_asset_id?.trim()
  const assetReference = providerAssetID
    ? `asset://${providerAssetID.replace(/^asset:\/\//, '')}`
    : ''
  const coverURL =
    character.cover_url ||
    (editable && providerAssetID
      ? virtualCharacterPreviewURL(character.id)
      : '')

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!editable) return
    setBusy(true)
    try {
      await updateVirtualCharacter(character.id, {
        name: name.trim(),
        description: description.trim(),
        tags: tags
          .split(/[,，\n]/)
          .map((item) => item.trim())
          .filter(Boolean),
      })
      toast.success(t('Character updated'))
      await onRefresh()
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.all,
      })
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to update character')))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className='flex flex-col gap-5' onSubmit={submit}>
      <div className='grid gap-5 sm:grid-cols-[minmax(0,240px)_minmax(0,1fr)]'>
        <div className='flex flex-col gap-3'>
          <div className='bg-muted aspect-[3/4] overflow-hidden rounded-md'>
            {coverURL ? (
              <img
                src={coverURL}
                alt={character.name}
                className='size-full object-contain'
              />
            ) : (
              <div className='text-muted-foreground flex size-full items-center justify-center text-sm'>
                {t('Preview is unavailable')}
              </div>
            )}
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge
              variant={
                character.status === 'failed' ? 'destructive' : 'secondary'
              }
            >
              {statusLabel(character.status, t)}
            </Badge>
            {character.mime_type ? (
              <Badge variant='outline'>{character.mime_type}</Badge>
            ) : null}
          </div>
          {character.status === 'creating' ? (
            <Progress value={null}>
              <ProgressLabel>{t('Processing character image')}</ProgressLabel>
            </Progress>
          ) : null}
          {assetReference ? (
            <div className='flex min-w-0 items-center gap-1'>
              <p
                className='text-muted-foreground truncate font-mono text-xs'
                title={assetReference}
              >
                {assetReference}
              </p>
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                onClick={() => void copyToClipboard(assetReference)}
              >
                <HugeiconsIcon
                  icon={copiedText === assetReference ? Tick02Icon : Copy01Icon}
                />
                <span className='sr-only'>{t('Copy')}</span>
              </Button>
            </div>
          ) : null}
          {character.last_error ? (
            <Alert variant='destructive'>
              <AlertDescription>{character.last_error}</AlertDescription>
            </Alert>
          ) : null}
        </div>
        <FieldGroup>
          <Field data-disabled={!editable}>
            <FieldLabel htmlFor='character-detail-name'>{t('Name')}</FieldLabel>
            <Input
              id='character-detail-name'
              required
              disabled={!editable}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field data-disabled={!editable}>
            <FieldLabel htmlFor='character-detail-description'>
              {t('Description')}
            </FieldLabel>
            <Textarea
              id='character-detail-description'
              disabled={!editable}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </Field>
          <Field data-disabled={!editable}>
            <FieldLabel htmlFor='character-detail-tags'>{t('Tags')}</FieldLabel>
            <Input
              id='character-detail-tags'
              disabled={!editable}
              value={tags}
              onChange={(event) => setTags(event.target.value)}
              placeholder={t('Comma-separated tags')}
            />
          </Field>
        </FieldGroup>
      </div>
      {character.source_type === 'volc_real_person' &&
      character.authorization ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('Portrait authorization')}</CardTitle>
          </CardHeader>
          <CardContent className='grid gap-3 text-sm sm:grid-cols-2'>
            <div>
              <p className='text-muted-foreground'>{t('Authorization status')}</p>
              <p>{authorizationStatusLabel(character.authorization.status, t)}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Valid period')}</p>
              <p>
                {new Date(
                  character.authorization.valid_from * 1000
                ).toLocaleString()}{' '}
                –{' '}
                {new Date(
                  character.authorization.valid_until * 1000
                ).toLocaleString()}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Authorized purposes')}</p>
              <p>{character.authorization.purposes.join(', ') || '-'}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Authorized regions')}</p>
              <p>{character.authorization.regions.join(', ') || '-'}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Authorized platforms')}</p>
              <p>{character.authorization.platforms.join(', ') || '-'}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>
                {t('Authorized industries (optional)')}
              </p>
              <p>{character.authorization.industries.join(', ') || '-'}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Commercial use')}</p>
              <p>
                {character.authorization.commercial_use_allowed
                  ? t('Allowed')
                  : t('Not allowed')}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Provider state')}</p>
              <p>
                {character.authorization.provider_group_status || '-'} /{' '}
                {character.authorization.provider_asset_status || '-'}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Last provider check')}</p>
              <p>
                {character.authorization.provider_checked_at
                  ? new Date(
                      character.authorization.provider_checked_at * 1000
                    ).toLocaleString()
                  : '-'}
              </p>
            </div>
          </CardContent>
        </Card>
      ) : null}
      {editable ? (
        <div className='flex justify-end'>
          <Button type='submit' disabled={busy || !name.trim()}>
            {busy ? <Spinner /> : null}
            {t('Save changes')}
          </Button>
        </div>
      ) : null}
    </form>
  )
}
