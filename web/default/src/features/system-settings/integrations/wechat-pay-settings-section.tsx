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
import * as React from 'react'
import type {
  WechatPayConfigStatus,
  WechatPayOrderView,
  WechatPayVerificationMode,
} from '@/types/wechat-pay'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { WechatPayQrDialog } from '@/components/payment/wechat-pay-qr-dialog'

const steps = [
  'Merchant identity',
  'Merchant certificate',
  'APIv3 and verification',
  'Validate and enable',
] as const

function errorMessage(error: unknown, fallback: string) {
  const responseMessage = (
    error as { response?: { data?: { message?: string } } }
  )?.response?.data?.message
  if (responseMessage) return responseMessage
  return error instanceof Error ? error.message : fallback
}

async function fetchWechatPayConfig() {
  const response = await api.get('/api/wechat-pay/config')
  if (!response.data?.success || !response.data?.data) {
    throw new Error(
      response.data?.message || 'Failed to load WeChat Pay configuration'
    )
  }
  return response.data.data as WechatPayConfigStatus
}

async function fetchWechatPayTestOrder(tradeNo: string) {
  const response = await api.get(
    `/api/wechat-pay/config/test/${encodeURIComponent(tradeNo)}`,
    { skipBusinessError: true } as Record<string, unknown>
  )
  if (!response.data?.success || !response.data?.data) {
    throw new Error(response.data?.message || 'Failed to query test payment')
  }
  return response.data.data as WechatPayOrderView
}

export function WechatPaySettingsSection() {
  const { t } = useTranslation()
  const [step, setStep] = React.useState(0)
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)
  const [testing, setTesting] = React.useState(false)
  const [status, setStatus] = React.useState<WechatPayConfigStatus | null>(null)
  const [appid, setAppid] = React.useState('')
  const [mchid, setMchid] = React.useState('')
  const [apiV3Key, setApiV3Key] = React.useState('')
  const [verificationMode, setVerificationMode] =
    React.useState<WechatPayVerificationMode>('platform_certificate')
  const [publicKeyId, setPublicKeyId] = React.useState('')
  const [merchantCertificate, setMerchantCertificate] =
    React.useState<File | null>(null)
  const [merchantPrivateKey, setMerchantPrivateKey] =
    React.useState<File | null>(null)
  const [wechatPayPublicKey, setWechatPayPublicKey] =
    React.useState<File | null>(null)
  const [enabled, setEnabled] = React.useState(false)
  const [showEpayWechat, setShowEpayWechat] = React.useState(false)
  const [testOrder, setTestOrder] = React.useState<WechatPayOrderView | null>(
    null
  )
  const [testDialogOpen, setTestDialogOpen] = React.useState(false)
  const initializedRef = React.useRef(false)

  const reload = React.useCallback(async () => {
    const next = await fetchWechatPayConfig()
    setStatus(next)
    if (!initializedRef.current) {
      setAppid(next.appid || '')
      setMchid(next.mchid || '')
      setVerificationMode(next.verification_mode || 'platform_certificate')
      setPublicKeyId(next.wechatpay_public_key_id || '')
      setEnabled(next.enabled)
      setShowEpayWechat(next.show_epay_wechat)
      initializedRef.current = true
    }
    return next
  }, [])

  React.useEffect(() => {
    reload()
      .catch((error) => toast.error(errorMessage(error, t('Load failed'))))
      .finally(() => setLoading(false))
  }, [reload, t])

  const canContinue = () => {
    if (step === 0 && (!appid.trim() || !mchid.trim())) {
      toast.error(t('Enter AppID and merchant ID first.'))
      return false
    }
    if (
      step === 1 &&
      ((!status?.has_merchant_certificate && !merchantCertificate) ||
        (!status?.has_merchant_private_key && !merchantPrivateKey))
    ) {
      toast.error(t('Upload both apiclient_cert.pem and apiclient_key.pem.'))
      return false
    }
    if (step === 2) {
      if (!status?.has_api_v3_key && !apiV3Key.trim()) {
        toast.error(t('Enter the APIv3 key.'))
        return false
      }
      if (
        verificationMode === 'public_key' &&
        (!publicKeyId.trim() ||
          (!status?.has_wechatpay_public_key && !wechatPayPublicKey))
      ) {
        toast.error(
          t('Complete the WeChat Pay public key ID and public key file.')
        )
        return false
      }
    }
    return true
  }

  const save = async () => {
    setSaving(true)
    try {
      const form = new FormData()
      form.set('appid', appid.trim())
      form.set('mchid', mchid.trim())
      form.set('enabled', String(enabled))
      form.set('show_epay_wechat', String(showEpayWechat))
      form.set('verification_mode', verificationMode)
      if (verificationMode === 'public_key') {
        form.set('wechatpay_public_key_id', publicKeyId.trim())
      }
      if (apiV3Key.trim()) form.set('api_v3_key', apiV3Key.trim())
      if (merchantCertificate) {
        form.set('merchant_certificate', merchantCertificate)
      }
      if (merchantPrivateKey) {
        form.set('merchant_private_key', merchantPrivateKey)
      }
      if (verificationMode === 'public_key' && wechatPayPublicKey) {
        form.set('wechatpay_public_key', wechatPayPublicKey)
      }
      const response = await api.put('/api/wechat-pay/config', form, {
        skipBusinessError: true,
      } as Record<string, unknown>)
      if (!response.data?.success) {
        throw new Error(
          response.data?.message || 'Failed to save WeChat Pay configuration'
        )
      }
      setApiV3Key('')
      setMerchantCertificate(null)
      setMerchantPrivateKey(null)
      setWechatPayPublicKey(null)
      const next = response.data.data as WechatPayConfigStatus
      setStatus(next)
      setVerificationMode(next.verification_mode || 'platform_certificate')
      setPublicKeyId(next.wechatpay_public_key_id || '')
      setEnabled(next.enabled)
      setShowEpayWechat(next.show_epay_wechat)
      toast.success(t('WeChat Pay configuration saved and validated.'))
      return next
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    if (!canContinue()) return
    try {
      await save()
    } catch (error) {
      toast.error(errorMessage(error, t('Save failed')))
    }
  }

  const handleCreateTest = async () => {
    setTesting(true)
    try {
      const response = await api.post('/api/wechat-pay/config/test', {}, {
        skipBusinessError: true,
      } as Record<string, unknown>)
      if (!response.data?.success || !response.data?.data) {
        throw new Error(
          response.data?.message || 'Failed to create test payment'
        )
      }
      const order = response.data.data as WechatPayOrderView
      setTestOrder(order)
      setTestDialogOpen(true)
    } catch (error) {
      toast.error(errorMessage(error, t('Test payment failed')))
    } finally {
      setTesting(false)
    }
  }

  if (loading) {
    return (
      <div className='flex min-h-48 items-center justify-center'>
        <Spinner />
      </div>
    )
  }

  return (
    <>
      <Card>
        <CardHeader>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <CardTitle>{t('Official WeChat Pay')}</CardTitle>
              <CardDescription>
                {t('Configure API v3 Native QR payments for balance top-ups.')}
              </CardDescription>
            </div>
            <Badge
              variant={
                status?.enabled && status.ready ? 'default' : 'secondary'
              }
            >
              {status?.enabled && status.ready
                ? t('Enabled')
                : t('Not enabled')}
            </Badge>
          </div>
          <Progress value={(step + 1) * 25} />
          <div className='grid grid-cols-2 gap-2 text-xs sm:grid-cols-4'>
            {steps.map((label, index) => (
              <button
                key={label}
                type='button'
                className='text-left'
                onClick={() => index <= step && setStep(index)}
              >
                <span
                  className={
                    index === step ? 'text-foreground' : 'text-muted-foreground'
                  }
                >
                  {index + 1}. {t(label)}
                </span>
              </button>
            ))}
          </div>
        </CardHeader>

        <CardContent>
          {!status?.crypto_secret_configured && (
            <Alert variant='destructive' className='mb-5'>
              <AlertTitle>{t('CRYPTO_SECRET is not ready')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Set a permanent CRYPTO_SECRET with at least 32 characters in Docker Compose before saving credentials.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {status?.error && (
            <Alert variant='destructive' className='mb-5'>
              <AlertTitle>{t('Configuration needs attention')}</AlertTitle>
              <AlertDescription>{status.error}</AlertDescription>
            </Alert>
          )}

          {step === 0 && (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='wechatpay-appid'>{t('AppID')}</FieldLabel>
                <Input
                  id='wechatpay-appid'
                  value={appid}
                  onChange={(event) => setAppid(event.target.value)}
                  placeholder='wx1234567890abcdef'
                  autoComplete='off'
                />
                <FieldDescription>
                  {t('Use an AppID that is already bound to this merchant ID.')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='wechatpay-mchid'>
                  {t('Merchant ID')}
                </FieldLabel>
                <Input
                  id='wechatpay-mchid'
                  value={mchid}
                  onChange={(event) => setMchid(event.target.value)}
                  placeholder='1900000001'
                  inputMode='numeric'
                  autoComplete='off'
                />
              </Field>
            </FieldGroup>
          )}

          {step === 1 && (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='wechatpay-cert'>
                  apiclient_cert.pem
                </FieldLabel>
                <Input
                  id='wechatpay-cert'
                  type='file'
                  accept='.pem,application/x-pem-file'
                  onChange={(event) =>
                    setMerchantCertificate(event.target.files?.[0] || null)
                  }
                />
                <FieldDescription>
                  {merchantCertificate
                    ? t('Selected file: {{fileName}}', {
                        fileName: merchantCertificate.name,
                      })
                    : status?.has_merchant_certificate
                      ? t(
                          'A merchant certificate is stored. Upload a file only to rotate it.'
                        )
                      : t(
                          'Upload the merchant API certificate downloaded with your certificate package.'
                        )}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='wechatpay-private-key'>
                  apiclient_key.pem
                </FieldLabel>
                <Input
                  id='wechatpay-private-key'
                  type='file'
                  accept='.pem,application/x-pem-file'
                  onChange={(event) =>
                    setMerchantPrivateKey(event.target.files?.[0] || null)
                  }
                />
                <FieldDescription>
                  {merchantPrivateKey
                    ? t('Selected file: {{fileName}}', {
                        fileName: merchantPrivateKey.name,
                      })
                    : status?.has_merchant_private_key
                      ? t(
                          'A merchant private key is encrypted and stored. Upload only to rotate it.'
                        )
                      : t(
                          'The private key is encrypted before it is written to the database.'
                        )}
                </FieldDescription>
              </Field>
            </FieldGroup>
          )}

          {step === 2 && (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='wechatpay-api-v3-key'>
                  {t('APIv3 Key')}
                </FieldLabel>
                <Input
                  id='wechatpay-api-v3-key'
                  type='password'
                  value={apiV3Key}
                  onChange={(event) => setApiV3Key(event.target.value)}
                  placeholder={
                    status?.has_api_v3_key
                      ? t('Stored — leave blank to keep it')
                      : t('Exactly 32 characters')
                  }
                  autoComplete='new-password'
                />
              </Field>
              <Field>
                <FieldLabel>{t('Verification method')}</FieldLabel>
                <ToggleGroup
                  value={[verificationMode]}
                  onValueChange={(values) => {
                    const next = values.at(-1) as
                      | WechatPayVerificationMode
                      | undefined
                    if (next) setVerificationMode(next)
                  }}
                  variant='outline'
                  className='grid w-full grid-cols-1 sm:grid-cols-2'
                  aria-label={t('Verification method')}
                >
                  <ToggleGroupItem value='platform_certificate'>
                    {t('Platform certificate')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='public_key'>
                    {t('WeChat Pay public key')}
                  </ToggleGroupItem>
                </ToggleGroup>
                <FieldDescription>
                  {verificationMode === 'platform_certificate'
                    ? t(
                        'Use the existing platform certificate verification mode.'
                      )
                    : t(
                        'Use the newer WeChat Pay public key verification mode.'
                      )}
                </FieldDescription>
              </Field>

              {verificationMode === 'platform_certificate' ? (
                <Alert>
                  <AlertTitle>
                    {t('Platform certificates are managed automatically')}
                  </AlertTitle>
                  <AlertDescription>
                    {t(
                      'The server downloads and refreshes WeChat Pay platform certificates automatically. You do not need to switch verification modes in the merchant platform.'
                    )}
                  </AlertDescription>
                </Alert>
              ) : (
                <>
                  <Field>
                    <FieldLabel htmlFor='wechatpay-public-key-id'>
                      {t('WeChat Pay public key ID')}
                    </FieldLabel>
                    <Input
                      id='wechatpay-public-key-id'
                      value={publicKeyId}
                      onChange={(event) => setPublicKeyId(event.target.value)}
                      placeholder='PUB_KEY_ID_...'
                      autoComplete='off'
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='wechatpay-public-key'>
                      pub_key.pem
                    </FieldLabel>
                    <Input
                      id='wechatpay-public-key'
                      type='file'
                      accept='.pem,application/x-pem-file'
                      onChange={(event) =>
                        setWechatPayPublicKey(event.target.files?.[0] || null)
                      }
                    />
                    <FieldDescription>
                      {wechatPayPublicKey
                        ? t('Selected file: {{fileName}}', {
                            fileName: wechatPayPublicKey.name,
                          })
                        : status?.has_wechatpay_public_key
                          ? t(
                              'A WeChat Pay public key is stored. Upload only to rotate it.'
                            )
                          : t(
                              'Download this public key from Merchant Platform → API Security.'
                            )}
                    </FieldDescription>
                  </Field>
                </>
              )}
            </FieldGroup>
          )}

          {step === 3 && (
            <FieldGroup>
              <Alert>
                <AlertTitle>{t('Callback URL')}</AlertTitle>
                <AlertDescription>
                  <code className='break-all'>
                    {status?.callback_url ||
                      t('Configure an HTTPS server address first.')}
                  </code>
                </AlertDescription>
              </Alert>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>{t('Enable official WeChat Pay')}</FieldTitle>
                  <FieldDescription>
                    {status?.verified_at
                      ? t(
                          'Disabling blocks new orders but existing callbacks remain active.'
                        )
                      : t('Awaiting test')}
                  </FieldDescription>
                </FieldContent>
                <Switch
                  checked={enabled}
                  disabled={!status?.verified_at && !enabled}
                  onCheckedChange={setEnabled}
                />
              </Field>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>{t('Also show Epay WeChat')}</FieldTitle>
                  <FieldDescription>
                    {t(
                      'By default, official WeChat Pay replaces the aggregated Epay WeChat button.'
                    )}
                  </FieldDescription>
                </FieldContent>
                <Switch
                  checked={showEpayWechat}
                  onCheckedChange={setShowEpayWechat}
                />
              </Field>
              {status?.configured && (
                <div className='grid gap-3 text-sm sm:grid-cols-2'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Certificate serial')}
                    </span>
                    <p className='truncate font-mono text-xs'>
                      {status.merchant_certificate_serial}
                    </p>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Real payment verification')}
                    </span>
                    <p>
                      {status.verified_at ? t('Verified') : t('Awaiting test')}
                    </p>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Verification method')}
                    </span>
                    <p>
                      {verificationMode === 'platform_certificate'
                        ? t('Platform certificate')
                        : t('WeChat Pay public key')}
                    </p>
                  </div>
                </div>
              )}
            </FieldGroup>
          )}
        </CardContent>

        <CardFooter className='flex flex-wrap justify-between gap-2'>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={step === 0 || saving}
              onClick={() => setStep((value) => Math.max(0, value - 1))}
            >
              {t('Back')}
            </Button>
            {step < steps.length - 1 && (
              <Button
                type='button'
                disabled={saving}
                onClick={() => {
                  if (canContinue()) setStep((value) => value + 1)
                }}
              >
                {t('Next')}
              </Button>
            )}
          </div>
          <div className='flex flex-wrap gap-2'>
            {status?.configured && (
              <Button
                type='button'
                variant='outline'
                disabled={!status.ready || testing || saving}
                onClick={handleCreateTest}
              >
                {testing && <Spinner data-icon='inline-start' />}
                {t('Create ¥0.01 test')}
              </Button>
            )}
            {step === steps.length - 1 && (
              <Button
                type='button'
                disabled={!status?.crypto_secret_configured || saving}
                onClick={handleSave}
              >
                {saving && <Spinner data-icon='inline-start' />}
                {t('Validate and save')}
              </Button>
            )}
          </div>
        </CardFooter>
      </Card>

      <WechatPayQrDialog
        open={testDialogOpen}
        onOpenChange={setTestDialogOpen}
        order={testOrder}
        refreshOrder={fetchWechatPayTestOrder}
        testMode
        onPaid={reload}
      />
    </>
  )
}
