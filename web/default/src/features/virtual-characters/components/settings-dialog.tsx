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
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import {
  syncVirtualCharacterCatalogFromAIPDD,
  testVirtualCharacterProvider,
  updateVirtualCharacterSettings,
  virtualCharacterQueryKeys,
} from '../api'
import type {
  VirtualCharacterQuotaPlan,
  VirtualCharacterSettings,
} from '../types'
import { ToggleField } from './ui-bits'
import { errorMessage } from './utils'

const QUOTA_PLAN_PRESETS: Record<
  Exclude<VirtualCharacterQuotaPlan, 'custom'>,
  { account_asset_cap: number; create_asset_qpm: number }
> = {
  free: { account_asset_cap: 50, create_asset_qpm: 3 },
  paid: { account_asset_cap: 1000000, create_asset_qpm: 120 },
}

function buildSettingsForm(settings?: VirtualCharacterSettings) {
  const quotaPlan = settings?.quota_plan || 'free'
  const preset =
    quotaPlan === 'custom'
      ? null
      : QUOTA_PLAN_PRESETS[quotaPlan] || QUOTA_PLAN_PRESETS.free
  return {
    enabled: settings?.enabled ?? false,
    quota_plan: quotaPlan as VirtualCharacterQuotaPlan,
    create_asset_qpm:
      preset?.create_asset_qpm ?? settings?.create_asset_qpm ?? 3,
    access_key: '',
    secret_key: '',
    region: settings?.region || 'cn-beijing',
    project_name: settings?.project_name || 'default',
    global_limit: settings?.global_limit ?? 100,
    account_asset_cap:
      preset?.account_asset_cap ?? settings?.account_asset_cap ?? 50,
    max_assets_per_character: settings?.max_assets_per_character ?? 10,
  }
}

function settingsFormKey(settings?: VirtualCharacterSettings): string {
  if (!settings) return 'loading'
  return [
    settings.enabled,
    settings.quota_plan,
    settings.create_asset_qpm,
    settings.region,
    settings.project_name,
    settings.global_limit,
    settings.account_asset_cap,
    settings.max_assets_per_character,
    settings.access_key_masked,
    settings.secret_key_masked,
  ].join('|')
}

export function SettingsDialog(props: {
  open: boolean
  settings?: VirtualCharacterSettings
  onClose: () => void
  onSaved: () => void
}) {
  return (
    <Dialog
      open={props.open}
      onOpenChange={(value) => !value && props.onClose()}
    >
      {props.open ? (
        <SettingsDialogForm
          key={settingsFormKey(props.settings)}
          settings={props.settings}
          onClose={props.onClose}
          onSaved={props.onSaved}
        />
      ) : null}
    </Dialog>
  )
}

function SettingsDialogForm({
  settings,
  onClose,
  onSaved,
}: {
  settings?: VirtualCharacterSettings
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState(() => buildSettingsForm(settings))
  // Track which action is running so only its own button shows a spinner, while all
  // three stay disabled for the duration.
  const [pending, setPending] = useState<'save' | 'test' | 'sync' | null>(null)
  const busy = pending != null
  const save = async (event: FormEvent) => {
    event.preventDefault()
    setPending('save')
    try {
      await updateVirtualCharacterSettings({
        ...form,
        access_key: form.access_key || undefined,
        secret_key: form.secret_key || undefined,
      })
      toast.success(t('Library settings updated'))
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.settings(),
      })
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.config(),
      })
      onSaved()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to update settings')))
    } finally {
      setPending(null)
    }
  }
  const testConnection = async () => {
    setPending('test')
    try {
      await testVirtualCharacterProvider()
      toast.success(t('Provider connection and permission check passed'))
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.settings(),
      })
    } catch (error) {
      toast.error(errorMessage(error, t('Provider connection test failed')))
    } finally {
      setPending(null)
    }
  }
  const syncCatalog = async () => {
    setPending('sync')
    try {
      // Keep force=false so the backend can send If-None-Match and skip a full
      // re-import when the upstream revision is unchanged.
      const res = await syncVirtualCharacterCatalogFromAIPDD()
      const result = res.data
      if (result?.skipped) {
        toast.success(
          t('Official catalog is already up to date (revision unchanged)')
        )
      } else {
        toast.success(
          t(
            'Synced official catalog: {{total}} characters (created {{created}}, updated {{updated}}, offlined {{offlined}})',
            {
              total: result?.total ?? 0,
              created: result?.created ?? 0,
              updated: result?.updated ?? 0,
              offlined: result?.offlined ?? 0,
            }
          )
        )
      }
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.settings(),
      })
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.all,
      })
    } catch (error) {
      toast.error(
        errorMessage(error, t('Failed to sync official catalog from AIPDD'))
      )
    } finally {
      setPending(null)
    }
  }
  const lastSyncedAt = settings?.catalog_last_synced_at
  const catalogVersion = settings?.catalog?.version
  const catalogRevision = settings?.catalog?.content_hash
  return (
    <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
      <form className='flex flex-col gap-5' onSubmit={save}>
        <DialogHeader>
          <DialogTitle>{t('Character library settings')}</DialogTitle>
          <DialogDescription>
            {t(
              'Configure the single Volc account, Project, and character library switch.'
            )}
          </DialogDescription>
        </DialogHeader>
        {!settings?.crypto_ready && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Stable CRYPTO_SECRET required')}</AlertTitle>
            <AlertDescription>
              {t(
                'The character library cannot be enabled until a stable secret of at least 32 characters is configured.'
              )}
            </AlertDescription>
          </Alert>
        )}
        <Card>
          <CardHeader>
            <CardTitle>{t('Provider account')}</CardTitle>
            <CardDescription>
              {t(
                'Volc Assets API requires Access Key and Secret Key (AK/SK). Ark API Key is not supported. Credentials are encrypted at rest.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <ToggleField
                label={t('Enable character library')}
                checked={form.enabled}
                onChange={(checked) => setForm({ ...form, enabled: checked })}
              />
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor='provider-ak'>
                    {t('Access Key (AK)')}
                  </FieldLabel>
                  <Input
                    id='provider-ak'
                    type='password'
                    value={form.access_key}
                    onChange={(event) =>
                      setForm({ ...form, access_key: event.target.value })
                    }
                    placeholder={
                      settings?.access_key_masked || t('Not configured')
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-sk'>
                    {t('Secret Key (SK)')}
                  </FieldLabel>
                  <Input
                    id='provider-sk'
                    type='password'
                    value={form.secret_key}
                    onChange={(event) =>
                      setForm({ ...form, secret_key: event.target.value })
                    }
                    placeholder={
                      settings?.secret_key_masked || t('Not configured')
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-region'>
                    {t('Region')}
                  </FieldLabel>
                  <Input
                    id='provider-region'
                    value={form.region}
                    onChange={(event) =>
                      setForm({ ...form, region: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-project'>
                    {t('Project Name')}
                  </FieldLabel>
                  <Input
                    id='provider-project'
                    value={form.project_name}
                    onChange={(event) =>
                      setForm({ ...form, project_name: event.target.value })
                    }
                  />
                </Field>
              </div>
              <Field>
                <FieldLabel htmlFor='provider-quota-plan'>
                  {t('Volc quota plan')}
                </FieldLabel>
                <NativeSelect
                  id='provider-quota-plan'
                  className='w-full'
                  value={form.quota_plan}
                  onChange={(event) => {
                    const quota_plan = event.target
                      .value as VirtualCharacterQuotaPlan
                    if (quota_plan === 'custom') {
                      setForm({ ...form, quota_plan })
                      return
                    }
                    const preset = QUOTA_PLAN_PRESETS[quota_plan]
                    setForm({
                      ...form,
                      quota_plan,
                      account_asset_cap: preset.account_asset_cap,
                      create_asset_qpm: preset.create_asset_qpm,
                    })
                  }}
                >
                  <NativeSelectOption value='free'>
                    {t('Free tier (50 assets / 3 QPM)')}
                  </NativeSelectOption>
                  <NativeSelectOption value='paid'>
                    {t('Paid tier (1M assets / 120 QPM)')}
                  </NativeSelectOption>
                  <NativeSelectOption value='custom'>
                    {t('Custom limits')}
                  </NativeSelectOption>
                </NativeSelect>
                <FieldDescription>
                  {t(
                    'Select the Volc Assets package for this site. Free/Paid apply official guardrails; Custom lets you edit numbers manually. This is a local limit and is not queried from Volc.'
                  )}
                </FieldDescription>
              </Field>
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor='provider-quota'>
                    {t('Default private quota')}
                  </FieldLabel>
                  <Input
                    id='provider-quota'
                    type='number'
                    min={1}
                    max={10000}
                    value={form.global_limit}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        global_limit: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Per-user character group limit on this site (not the Volc plan).'
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-max-assets-per-character'>
                    {t('Max assets per character')}
                  </FieldLabel>
                  <Input
                    id='provider-max-assets-per-character'
                    type='number'
                    min={1}
                    max={100}
                    value={form.max_assets_per_character}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        max_assets_per_character: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Maximum related assets each character can hold on this site.'
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-asset-cap'>
                    {t('Account asset cap')}
                  </FieldLabel>
                  <Input
                    id='provider-asset-cap'
                    type='number'
                    min={1}
                    max={5000000}
                    value={form.account_asset_cap}
                    disabled={form.quota_plan !== 'custom'}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        account_asset_cap: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Site-wide Asset guardrail matching the selected Volc plan.'
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor='provider-qpm'>
                    {t('CreateAsset QPM')}
                  </FieldLabel>
                  <Input
                    id='provider-qpm'
                    type='number'
                    min={1}
                    max={300}
                    value={form.create_asset_qpm}
                    disabled={form.quota_plan !== 'custom'}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        create_asset_qpm: Number(event.target.value),
                      })
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Upload rate limit for CreateAsset under the selected plan.'
                    )}
                  </FieldDescription>
                </Field>
              </div>
              <Button
                type='button'
                variant='outline'
                disabled={busy || !settings?.enabled}
                onClick={testConnection}
              >
                {pending === 'test' && <Spinner />}
                {t('Test connection and permissions')}
              </Button>
            </FieldGroup>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('Official catalog sync')}</CardTitle>
            <CardDescription>
              {t(
                'Pull the published Volc preset catalog from AIPDD into the local public library. Sync runs only when you trigger it manually.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <FieldDescription>
                {lastSyncedAt
                  ? t('Last synced: {{time}}', {
                      time: new Date(lastSyncedAt * 1000).toLocaleString(),
                    })
                  : t('Not synced yet')}
              </FieldDescription>
              {catalogVersion ? (
                <FieldDescription>
                  {t(
                    'Local catalog version: {{version}} (revision {{revision}})',
                    {
                      version: catalogVersion,
                      revision: catalogRevision || '-',
                    }
                  )}
                </FieldDescription>
              ) : null}
              <Button
                type='button'
                variant='outline'
                disabled={busy || !settings?.enabled}
                onClick={syncCatalog}
              >
                {pending === 'sync' && <Spinner />}
                {t('Sync from AIPDD now')}
              </Button>
            </FieldGroup>
          </CardContent>
        </Card>
        <DialogFooter>
          <Button type='button' variant='outline' onClick={onClose}>
            {t('Close')}
          </Button>
          <Button type='submit' disabled={busy}>
            {pending === 'save' && <Spinner />}
            {t('Save settings')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
