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
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Add01Icon,
  AiUserIcon,
  CheckmarkCircle02Icon,
  Clock01Icon,
  Delete02Icon,
  FileUploadIcon,
  MagicWand01Icon,
  RefreshIcon,
  Settings01Icon,
  Video01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Badge } from '@/components/ui/badge'
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
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import {
  createCharacterVideo,
  createValidationSession,
  deleteVirtualCharacter,
  deleteVirtualCharacterAsset,
  getAvailableVideoModels,
  getValidationSession,
  getVirtualCharacter,
  getVirtualCharacterConfig,
  getVirtualCharacterHistory,
  getVirtualCharacterSettings,
  importPublicVirtualCharacters,
  listVirtualCharacters,
  setPrimaryVirtualCharacterAsset,
  testVirtualCharacterProvider,
  updateVirtualCharacterSettings,
  uploadVirtualCharacterAsset,
  virtualCharacterQueryKeys,
} from './api'
import type {
  VirtualCharacter,
  VirtualCharacterAsset,
  VirtualCharacterAssetStatus,
  VirtualCharacterSettings,
  VirtualCharacterStatus,
  VirtualCharacterTask,
  VirtualCharacterValidationSession,
} from './types'

type LibraryTab = 'public' | 'private' | 'history'

export function VirtualCharacters() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userRole = useAuthStore((state) => state.auth.user?.role ?? 0)
  const isAdmin = userRole >= ROLE.ADMIN
  const [tab, setTab] = useState<LibraryTab>('public')
  const [publicPage, setPublicPage] = useState(1)
  const [privatePage, setPrivatePage] = useState(1)
  const [historyPage, setHistoryPage] = useState(1)
  const [filter, setFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [createOpen, setCreateOpen] = useState(false)
  const [validation, setValidation] =
    useState<VirtualCharacterValidationSession | null>(null)
  const [detailID, setDetailID] = useState<number | null>(null)
  const [generateTarget, setGenerateTarget] = useState<{
    character: VirtualCharacter
    asset?: VirtualCharacterAsset
  } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<VirtualCharacter | null>(
    null
  )
  const [settingsOpen, setSettingsOpen] = useState(false)

  const publicQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list('public', publicPage),
    queryFn: () => listVirtualCharacters('public', publicPage),
  })
  const privateQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list('private', privatePage),
    queryFn: () => listVirtualCharacters('private', privatePage),
    refetchInterval: (query) =>
      query.state.data?.data.page.items.some((item) =>
        item.assets?.some((asset) => asset.status === 'Processing')
      )
        ? 5000
        : false,
  })
  const historyQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.history(historyPage),
    queryFn: () => getVirtualCharacterHistory(historyPage),
  })
  const configQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.config(),
    queryFn: getVirtualCharacterConfig,
  })
  const userModelsQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.userModels(),
    queryFn: getAvailableVideoModels,
  })
  const settingsQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.settings(),
    queryFn: getVirtualCharacterSettings,
    enabled: isAdmin,
  })

  useEffect(() => {
    const sessionID = new URLSearchParams(window.location.search).get(
      'validation_session'
    )
    if (!sessionID) return
    getValidationSession(sessionID)
      .then((response) => setValidation(response.data))
      .catch(() => undefined)
  }, [])

  const currentPage =
    tab === 'public'
      ? publicQuery.data?.data.page
      : tab === 'private'
        ? privateQuery.data?.data.page
        : historyQuery.data?.data.page
  const characters = useMemo(() => {
    const source =
      tab === 'public'
        ? publicQuery.data?.data.page.items
        : privateQuery.data?.data.page.items
    const keyword = filter.trim().toLowerCase()
    if (!source) return []
    return source.filter(
      (item) =>
        (statusFilter === 'all' || item.status === statusFilter) &&
        (!keyword ||
          [item.name, item.description, ...item.tags]
            .join(' ')
            .toLowerCase()
            .includes(keyword))
    )
  }, [filter, privateQuery.data, publicQuery.data, statusFilter, tab])
  const loading =
    tab === 'public'
      ? publicQuery.isLoading
      : tab === 'private'
        ? privateQuery.isLoading
        : historyQuery.isLoading

  const refreshAll = async () => {
    await queryClient.invalidateQueries({
      queryKey: virtualCharacterQueryKeys.all,
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Character Library')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {isAdmin && (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setSettingsOpen(true)}
          >
            <HugeiconsIcon icon={Settings01Icon} data-icon='inline-start' />
            {t('Library settings')}
          </Button>
        )}
        {tab === 'private' && (
          <Button
            size='sm'
            disabled={!configQuery.data?.data.real_person_enabled}
            onClick={() => setCreateOpen(true)}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create real-person character')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <Alert>
            <HugeiconsIcon icon={AiUserIcon} />
            <AlertTitle>{t('Provider-backed character library')}</AlertTitle>
            <AlertDescription>
              {t(
                'Official characters are shared read-only assets. My real-person characters require H5 validation and Active provider assets.'
              )}
            </AlertDescription>
          </Alert>

          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as LibraryTab)}
          >
            <TabsList>
              <TabsTrigger value='public'>
                {t('Official characters')}
              </TabsTrigger>
              <TabsTrigger value='private'>
                {t('My real-person characters')}
              </TabsTrigger>
              <TabsTrigger value='history'>{t('Task history')}</TabsTrigger>
            </TabsList>
          </Tabs>

          <div className='flex flex-wrap items-center gap-2'>
            {tab !== 'history' && (
              <>
                <Input
                  className='max-w-sm'
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                  placeholder={t('Search by name or tag')}
                />
                <NativeSelect
                  value={statusFilter}
                  onChange={(event) => setStatusFilter(event.target.value)}
                >
                  <NativeSelectOption value='all'>
                    {t('All statuses')}
                  </NativeSelectOption>
                  <NativeSelectOption value='active'>
                    {t('Active')}
                  </NativeSelectOption>
                  <NativeSelectOption value='creating'>
                    {t('Creating')}
                  </NativeSelectOption>
                  <NativeSelectOption value='blocked'>
                    {t('Blocked')}
                  </NativeSelectOption>
                  <NativeSelectOption value='deleting'>
                    {t('Deleting')}
                  </NativeSelectOption>
                  <NativeSelectOption value='failed'>
                    {t('Failed')}
                  </NativeSelectOption>
                </NativeSelect>
              </>
            )}
            <Button variant='outline' size='sm' onClick={refreshAll}>
              <HugeiconsIcon icon={RefreshIcon} data-icon='inline-start' />
              {t('Refresh')}
            </Button>
            {tab === 'private' && privateQuery.data && (
              <Badge variant='outline'>
                {t('{{used}} of {{limit}} actor groups', {
                  used: privateQuery.data.data.used ?? 0,
                  limit: privateQuery.data.data.limit ?? 0,
                })}
              </Badge>
            )}
          </div>

          {tab === 'public' && !configQuery.data?.data.official_enabled && (
            <Alert variant='destructive'>
              <AlertTitle>
                {t('Official character library is disabled')}
              </AlertTitle>
              <AlertDescription>
                {t(
                  'An administrator must configure the provider and import an authoritative catalog.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {tab === 'private' && !configQuery.data?.data.real_person_enabled && (
            <Alert variant='destructive'>
              <AlertTitle>
                {t('Real-person character library is disabled')}
              </AlertTitle>
              <AlertDescription>
                {t(
                  'An administrator must complete provider and channel configuration first.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {loading ? (
            <CharacterGridSkeleton />
          ) : tab === 'history' ? (
            <TaskHistory
              items={historyQuery.data?.data.page.items ?? []}
              outputNotice={historyQuery.data?.data.output_notice}
              onRefresh={() => historyQuery.refetch()}
            />
          ) : characters.length > 0 ? (
            <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
              {characters.map((item) => (
                <CharacterCard
                  key={item.id}
                  item={item}
                  onOpen={() =>
                    item.scope === 'private'
                      ? setDetailID(item.id)
                      : setGenerateTarget({ character: item })
                  }
                  onGenerate={() => setGenerateTarget({ character: item })}
                  onDelete={() => setDeleteTarget(item)}
                />
              ))}
            </div>
          ) : (
            <Card>
              <CardHeader>
                <CardTitle>{t('No characters found')}</CardTitle>
                <CardDescription>
                  {tab === 'public'
                    ? t(
                        'The authoritative official catalog has no matching active assets.'
                      )
                    : t(
                        'Complete H5 validation to create your first real-person actor group.'
                      )}
                </CardDescription>
              </CardHeader>
            </Card>
          )}

          {currentPage && currentPage.total > currentPage.page_size && (
            <Pagination
              page={currentPage.page}
              total={currentPage.total}
              pageSize={currentPage.page_size}
              onChange={
                tab === 'public'
                  ? setPublicPage
                  : tab === 'private'
                    ? setPrivatePage
                    : setHistoryPage
              }
            />
          )}
        </div>
      </SectionPageLayout.Content>

      <CreateRealPersonDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(session) => {
          setCreateOpen(false)
          setValidation(session)
        }}
      />
      <ValidationDialog
        session={validation}
        onClose={() => setValidation(null)}
        onRetry={() => {
          setValidation(null)
          setCreateOpen(true)
        }}
        onSucceeded={async (characterID) => {
          await refreshAll()
          if (characterID) setDetailID(characterID)
        }}
      />
      <CharacterDetailDialog
        characterID={detailID}
        onClose={() => setDetailID(null)}
        onGenerate={(character, asset) =>
          setGenerateTarget({ character, asset })
        }
      />
      <GenerateDialog
        target={generateTarget}
        models={userModelsQuery.data ?? []}
        defaultModel={configQuery.data?.data.default_model ?? ''}
        onClose={() => setGenerateTarget(null)}
        onCreated={async () => {
          setGenerateTarget(null)
          setTab('history')
          await refreshAll()
        }}
      />
      <DeleteCharacterDialog
        target={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDeleted={refreshAll}
      />
      {isAdmin && (
        <SettingsDialog
          open={settingsOpen}
          settings={settingsQuery.data?.data}
          onClose={() => setSettingsOpen(false)}
          onSaved={refreshAll}
        />
      )}
    </SectionPageLayout>
  )
}

function CharacterCard({
  item,
  onOpen,
  onGenerate,
  onDelete,
}: {
  item: VirtualCharacter
  onOpen: () => void
  onGenerate: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const canGenerate =
    item.status === 'active' &&
    (item.scope === 'public' ||
      item.assets.some((asset) => asset.status === 'Active'))
  return (
    <Card className='overflow-hidden'>
      <div className='bg-muted aspect-video overflow-hidden'>
        {item.cover_url ? (
          <img
            src={item.cover_url}
            alt={item.name}
            className='size-full object-cover'
          />
        ) : (
          <div className='text-muted-foreground flex size-full items-center justify-center'>
            <HugeiconsIcon icon={AiUserIcon} className='size-10' />
          </div>
        )}
      </div>
      <CardHeader>
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='truncate'>{item.name}</CardTitle>
            <CardDescription className='line-clamp-2'>
              {item.description || t('No description')}
            </CardDescription>
          </div>
          <Badge variant={item.status === 'active' ? 'default' : 'secondary'}>
            {statusLabel(item.status, t)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <div className='flex min-h-6 flex-wrap gap-1.5'>
          {item.tags.length > 0 ? (
            item.tags.map((tag) => (
              <Badge key={tag} variant='outline'>
                {tag}
              </Badge>
            ))
          ) : (
            <span className='text-muted-foreground text-sm'>
              {t('No tags')}
            </span>
          )}
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button size='sm' variant='outline' onClick={onOpen}>
            {item.scope === 'private' ? t('Manage assets') : t('View')}
          </Button>
          <Button size='sm' disabled={!canGenerate} onClick={onGenerate}>
            <HugeiconsIcon icon={MagicWand01Icon} data-icon='inline-start' />
            {t('Create video')}
          </Button>
          {item.scope === 'private' && (
            <Button size='icon-sm' variant='ghost' onClick={onDelete}>
              <HugeiconsIcon icon={Delete02Icon} />
              <span className='sr-only'>{t('Delete')}</span>
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function CreateRealPersonDialog({
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
      })
      onCreated(response.data)
      setName('')
      setDescription('')
      setTags('')
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to create validation session')))
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
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
              {t(
                'Only the validated user may upload assets into this actor group.'
              )}
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

function ValidationDialog({
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

function CharacterDetailDialog({
  characterID,
  onClose,
  onGenerate,
}: {
  characterID: number | null
  onClose: () => void
  onGenerate: (
    character: VirtualCharacter,
    asset?: VirtualCharacterAsset
  ) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [deletingAsset, setDeletingAsset] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const query = useQuery({
    queryKey: virtualCharacterQueryKeys.detail(characterID ?? 0),
    queryFn: () => getVirtualCharacter(characterID ?? 0),
    enabled: Boolean(characterID),
    refetchInterval: (current) =>
      current.state.data?.data.assets.some(
        (asset) => asset.status === 'Processing'
      )
        ? 5000
        : false,
  })
  const character = query.data?.data
  const refresh = async () => {
    await query.refetch()
    await queryClient.invalidateQueries({
      queryKey: virtualCharacterQueryKeys.all,
    })
  }
  const setPrimary = async (assetID: number) => {
    if (!character) return
    setBusy(true)
    try {
      await setPrimaryVirtualCharacterAsset(character.id, assetID)
      toast.success(t('Primary asset updated'))
      await refresh()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to update primary asset')))
    } finally {
      setBusy(false)
    }
  }
  const removeAsset = async () => {
    if (!character || deletingAsset == null) return
    setBusy(true)
    try {
      await deleteVirtualCharacterAsset(character.id, deletingAsset)
      toast.success(t('Asset deletion queued'))
      setDeletingAsset(null)
      await refresh()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to delete asset')))
    } finally {
      setBusy(false)
    }
  }
  return (
    <>
      <Dialog
        open={characterID != null}
        onOpenChange={(open) => !open && onClose()}
      >
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>{character?.name ?? t('Actor group')}</DialogTitle>
            <DialogDescription>
              {t(
                'Manage the validated actor group and its provider-hosted styling assets.'
              )}
            </DialogDescription>
          </DialogHeader>
          {!character ? (
            <div className='flex justify-center py-12'>
              <Spinner className='size-6' />
            </div>
          ) : (
            <div className='flex flex-col gap-4'>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge>{t('Validated')}</Badge>
                <Badge variant='outline'>
                  {t('{{count}} assets', { count: character.assets.length })}
                </Badge>
                <Button size='sm' onClick={() => setUploadOpen(true)}>
                  <HugeiconsIcon
                    icon={FileUploadIcon}
                    data-icon='inline-start'
                  />
                  {t('Upload styling asset')}
                </Button>
              </div>
              {character.assets.length === 0 ? (
                <Alert>
                  <AlertTitle>{t('No styling assets')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Upload an image, video, or audio asset. Video generation unlocks after it becomes Active.'
                    )}
                  </AlertDescription>
                </Alert>
              ) : (
                <div className='grid gap-3 sm:grid-cols-2'>
                  {character.assets.map((asset) => (
                    <Card key={asset.id}>
                      <CardHeader>
                        <div className='flex items-start justify-between gap-2'>
                          <div>
                            <CardTitle className='text-base'>
                              {asset.name}
                            </CardTitle>
                            <CardDescription>
                              {t(asset.asset_type)}
                            </CardDescription>
                          </div>
                          <AssetStatusBadge status={asset.status} />
                        </div>
                      </CardHeader>
                      <CardContent className='flex flex-col gap-3'>
                        {asset.status === 'Processing' && (
                          <Progress value={null}>
                            <ProgressLabel>
                              {t('Provider processing')}
                            </ProgressLabel>
                          </Progress>
                        )}
                        {asset.last_error && (
                          <Alert variant='destructive'>
                            <AlertDescription>
                              {asset.last_error}
                            </AlertDescription>
                          </Alert>
                        )}
                        <div className='flex flex-wrap gap-2'>
                          <Button
                            size='sm'
                            disabled={asset.status !== 'Active'}
                            onClick={() => onGenerate(character, asset)}
                          >
                            <HugeiconsIcon
                              icon={Video01Icon}
                              data-icon='inline-start'
                            />
                            {t('Create video')}
                          </Button>
                          {!asset.is_primary && asset.status === 'Active' && (
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={busy}
                              onClick={() => setPrimary(asset.id)}
                            >
                              {t('Set as primary')}
                            </Button>
                          )}
                          {asset.is_primary && (
                            <Badge variant='secondary'>{t('Primary')}</Badge>
                          )}
                          <Button
                            size='icon-sm'
                            variant='ghost'
                            disabled={busy || asset.status === 'Deleting'}
                            onClick={() => setDeletingAsset(asset.id)}
                          >
                            <HugeiconsIcon icon={Delete02Icon} />
                            <span className='sr-only'>{t('Delete')}</span>
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant='outline' onClick={onClose}>
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {character && (
        <UploadAssetDialog
          character={character}
          open={uploadOpen}
          onClose={() => setUploadOpen(false)}
          onUploaded={refresh}
        />
      )}
      <AlertDialog
        open={deletingAsset != null}
        onOpenChange={(open) => !open && setDeletingAsset(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete this asset?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'New tasks are blocked immediately. Provider deletion continues in the background and retries on failure.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction disabled={busy} onClick={removeAsset}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function UploadAssetDialog({
  character,
  open,
  onClose,
  onUploaded,
}: {
  character: VirtualCharacter
  open: boolean
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
            <DialogTitle>{t('Upload styling asset')}</DialogTitle>
            <DialogDescription>
              {t(
                'The file is staged privately, imported into the Volc asset group, then removed from staging.'
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

function GenerateDialog({
  target,
  models,
  defaultModel,
  onClose,
  onCreated,
}: {
  target: { character: VirtualCharacter; asset?: VirtualCharacterAsset } | null
  models: string[]
  defaultModel: string
  onClose: () => void
  onCreated: () => void
}) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const [modelName, setModelName] = useState(defaultModel)
  const [duration, setDuration] = useState(5)
  const [ratio, setRatio] = useState('16:9')
  const [resolution, setResolution] = useState('720p')
  const [assetID, setAssetID] = useState<number | undefined>(target?.asset?.id)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    setAssetID(target?.asset?.id ?? target?.character.primary_asset_id)
    setModelName(defaultModel || models[0] || '')
  }, [defaultModel, models, target])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!target) return
    setBusy(true)
    try {
      const response = await createCharacterVideo({
        character_id: target.character.id,
        character_asset_id: assetID,
        model: modelName,
        prompt,
        duration,
        ratio,
        resolution,
      })
      if (response.error?.message) throw new Error(response.error.message)
      toast.success(t('Video task created'))
      onCreated()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to create video task')))
    } finally {
      setBusy(false)
    }
  }
  const assets =
    target?.character.assets?.filter((asset) => asset.status === 'Active') ?? []
  return (
    <Dialog open={target != null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form className='flex flex-col gap-5' onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>
              {t('Create video with {{name}}', {
                name: target?.character.name ?? '',
              })}
            </DialogTitle>
            <DialogDescription>
              {t(
                'The selected provider asset is sent as an asset:// reference through the fixed channel.'
              )}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            {target?.character.scope === 'private' && assets.length > 0 && (
              <Field>
                <FieldLabel htmlFor='generation-asset'>
                  {t('Styling asset')}
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
              >
                {models.map((model) => (
                  <NativeSelectOption key={model} value={model}>
                    {model}
                  </NativeSelectOption>
                ))}
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
            <Button type='button' variant='outline' onClick={onClose}>
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              disabled={
                busy ||
                !modelName ||
                !prompt ||
                (target?.character.scope === 'private' && !assetID)
              }
            >
              {busy && <Spinner />}
              {t('Create video')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SettingsDialog({
  open,
  settings,
  onClose,
  onSaved,
}: {
  open: boolean
  settings?: VirtualCharacterSettings
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    enabled: false,
    official_enabled: false,
    real_person_enabled: false,
    access_key: '',
    secret_key: '',
    region: 'cn-beijing',
    project_name: 'default',
    channel_id: 0,
    global_limit: 100,
    models: '',
    default_model: '',
  })
  const [file, setFile] = useState<File | null>(null)
  const [version, setVersion] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!settings) return
    setForm((current) => ({
      ...current,
      enabled: settings.enabled,
      official_enabled: settings.official_enabled,
      real_person_enabled: settings.real_person_enabled,
      region: settings.region || 'cn-beijing',
      project_name: settings.project_name || 'default',
      channel_id: settings.channel_id,
      global_limit: settings.global_limit,
      models: settings.models.join(','),
      default_model: settings.default_model,
      access_key: '',
      secret_key: '',
    }))
  }, [settings])
  const save = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    try {
      await updateVirtualCharacterSettings({
        ...form,
        access_key: form.access_key || undefined,
        secret_key: form.secret_key || undefined,
        models: splitTags(form.models),
      })
      toast.success(t('Library settings updated'))
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.settings(),
      })
      onSaved()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to update settings')))
    } finally {
      setBusy(false)
    }
  }
  const testConnection = async () => {
    setBusy(true)
    try {
      await testVirtualCharacterProvider()
      toast.success(t('Provider connection and permission check passed'))
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.settings(),
      })
    } catch (error) {
      toast.error(errorMessage(error, t('Provider connection test failed')))
    } finally {
      setBusy(false)
    }
  }
  const importCatalog = async (dryRun: boolean) => {
    if (!file) return
    setBusy(true)
    try {
      await importPublicVirtualCharacters(file, dryRun, version)
      toast.success(
        dryRun ? t('Catalog validation passed') : t('Official catalog imported')
      )
      await queryClient.invalidateQueries({
        queryKey: virtualCharacterQueryKeys.all,
      })
    } catch (error) {
      toast.error(errorMessage(error, t('Catalog import failed')))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(value) => !value && onClose()}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
        <form className='flex flex-col gap-5' onSubmit={save}>
          <DialogHeader>
            <DialogTitle>{t('Character library settings')}</DialogTitle>
            <DialogDescription>
              {t(
                'Configure the single Volc account, Project, stable video channel, feature flags, and official catalog.'
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
                  'AK, SK, H5 links, and validation tokens are encrypted at rest.'
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
                      {t('Access Key')}
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
                      {t('Secret Key')}
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
                  <FieldLabel htmlFor='provider-channel'>
                    {t('Stable video channel')}
                  </FieldLabel>
                  <NativeSelect
                    id='provider-channel'
                    className='w-full'
                    value={form.channel_id}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        channel_id: Number(event.target.value),
                      })
                    }
                  >
                    <NativeSelectOption value={0}>
                      {t('Select a channel')}
                    </NativeSelectOption>
                    {settings?.channels.map((channel) => (
                      <NativeSelectOption key={channel.id} value={channel.id}>
                        {channel.name}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <FieldDescription>
                    {t(
                      'Only enabled single-key Volc or DoubaoVideo channels are listed.'
                    )}
                  </FieldDescription>
                </Field>
                <div className='grid gap-4 sm:grid-cols-2'>
                  <Field>
                    <FieldLabel htmlFor='provider-models'>
                      {t('Seedance 2.0 model whitelist')}
                    </FieldLabel>
                    <Input
                      id='provider-models'
                      value={form.models}
                      onChange={(event) =>
                        setForm({ ...form, models: event.target.value })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='provider-default-model'>
                      {t('Default model')}
                    </FieldLabel>
                    <Input
                      id='provider-default-model'
                      value={form.default_model}
                      onChange={(event) =>
                        setForm({ ...form, default_model: event.target.value })
                      }
                    />
                  </Field>
                </div>
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
                      'Quota is counted by actor group, not by styling asset.'
                    )}
                  </FieldDescription>
                </Field>
                <div className='grid gap-3 sm:grid-cols-2'>
                  <ToggleField
                    label={t('Enable official catalog (A)')}
                    checked={form.official_enabled}
                    onChange={(checked) =>
                      setForm({ ...form, official_enabled: checked })
                    }
                  />
                  <ToggleField
                    label={t('Enable real-person groups (B)')}
                    checked={form.real_person_enabled}
                    onChange={(checked) =>
                      setForm({ ...form, real_person_enabled: checked })
                    }
                  />
                </div>
                <Button
                  type='button'
                  variant='outline'
                  disabled={busy || !settings?.enabled}
                  onClick={testConnection}
                >
                  {busy && <Spinner />}
                  {t('Test connection and permissions')}
                </Button>
              </FieldGroup>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('Authoritative official catalog')}</CardTitle>
              <CardDescription>
                {settings?.catalog
                  ? t('Last import: version {{version}}, {{count}} entries', {
                      version: settings.catalog.version,
                      count: settings.catalog.total,
                    })
                  : t('No authoritative catalog has been imported.')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor='catalog-file'>
                    {t('JSON or CSV catalog')}
                  </FieldLabel>
                  <Input
                    id='catalog-file'
                    type='file'
                    accept='.json,.csv'
                    onChange={(event) =>
                      setFile(event.target.files?.[0] ?? null)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='catalog-version'>
                    {t('Catalog version')}
                  </FieldLabel>
                  <Input
                    id='catalog-version'
                    value={version}
                    onChange={(event) => setVersion(event.target.value)}
                    placeholder={t(
                      'Required for CSV; JSON may include version'
                    )}
                  />
                </Field>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    disabled={busy || !file}
                    onClick={() => importCatalog(true)}
                  >
                    {t('Dry run')}
                  </Button>
                  <Button
                    type='button'
                    disabled={busy || !file}
                    onClick={() => importCatalog(false)}
                  >
                    {t('Import catalog')}
                  </Button>
                </div>
              </FieldGroup>
            </CardContent>
          </Card>
          <DialogFooter>
            <Button type='button' variant='outline' onClick={onClose}>
              {t('Close')}
            </Button>
            <Button type='submit' disabled={busy}>
              {busy && <Spinner />}
              {t('Save settings')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DeleteCharacterDialog({
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
      toast.success(t('Character deletion queued'))
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
          <AlertDialogTitle>{t('Delete this actor group?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(
              'All assets are hidden immediately. Provider asset and group deletion continues with retries in the background.'
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
          <AlertDialogAction disabled={busy} onClick={remove}>
            {t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function TaskHistory({
  items,
  outputNotice,
  onRefresh,
}: {
  items: VirtualCharacterTask[]
  outputNotice?: string
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('No character tasks yet')}</CardTitle>
          <CardDescription>
            {t(
              'Tasks created from official or real-person assets will appear here.'
            )}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }
  return (
    <div className='flex flex-col gap-3'>
      {outputNotice && (
        <Alert>
          <HugeiconsIcon icon={Clock01Icon} />
          <AlertTitle>{t('Temporary output')}</AlertTitle>
          <AlertDescription>{t(outputNotice)}</AlertDescription>
        </Alert>
      )}
      {items.map((item) => {
        const failure = item.task?.fail_reason || item.error
        return (
          <Card key={item.task_id}>
            <CardHeader>
              <div className='flex flex-wrap items-start justify-between gap-3'>
                <div>
                  <CardTitle>{item.character_name}</CardTitle>
                  <CardDescription>
                    {item.character_asset_name
                      ? t('Asset: {{name}}', {
                          name: item.character_asset_name,
                        })
                      : item.task_id}
                  </CardDescription>
                </div>
                <Badge variant={failure ? 'destructive' : 'secondary'}>
                  {taskStatusLabel(item.task?.status || item.link_status, t)}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className='flex flex-col gap-3'>
              {failure && (
                <Alert variant='destructive'>
                  <AlertTitle>{t('Task failed')}</AlertTitle>
                  <AlertDescription>{failure}</AlertDescription>
                </Alert>
              )}
              <div className='flex flex-wrap gap-2'>
                {item.task?.result_url && (
                  <Button
                    size='sm'
                    onClick={() =>
                      window.open(
                        item.task?.result_url,
                        '_blank',
                        'noopener,noreferrer'
                      )
                    }
                  >
                    {t('Open video')}
                  </Button>
                )}
                <Button size='sm' variant='outline' onClick={onRefresh}>
                  {t('Refresh')}
                </Button>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}

function ToggleField({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <Field orientation='horizontal'>
      <FieldLabel>{label}</FieldLabel>
      <Switch checked={checked} onCheckedChange={onChange} />
    </Field>
  )
}

function AssetStatusBadge({ status }: { status: VirtualCharacterAssetStatus }) {
  const { t } = useTranslation()
  return (
    <Badge
      variant={
        status === 'Active'
          ? 'default'
          : status === 'Failed'
            ? 'destructive'
            : 'secondary'
      }
    >
      {t(status)}
    </Badge>
  )
}

function CharacterGridSkeleton() {
  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
      {[0, 1, 2].map((item) => (
        <Card key={item}>
          <Skeleton className='aspect-video w-full' />
          <CardContent className='flex flex-col gap-3 pt-5'>
            <Skeleton className='h-5 w-1/2' />
            <Skeleton className='h-4 w-full' />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function Pagination({
  page,
  total,
  pageSize,
  onChange,
}: {
  page: number
  total: number
  pageSize: number
  onChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div className='flex items-center justify-end gap-2'>
      <Button
        variant='outline'
        size='sm'
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        {t('Previous')}
      </Button>
      <span className='text-muted-foreground text-sm'>
        {t('Page {{page}} of {{pages}}', { page, pages })}
      </span>
      <Button
        variant='outline'
        size='sm'
        disabled={page >= pages}
        onClick={() => onChange(page + 1)}
      >
        {t('Next')}
      </Button>
    </div>
  )
}

function splitTags(value: string): string[] {
  return value
    .split(',')
    .map((tag) => tag.trim())
    .filter(Boolean)
}

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object') {
    const response = error as {
      response?: { data?: { error?: { message?: string }; message?: string } }
    }
    const message =
      response.response?.data?.error?.message ||
      response.response?.data?.message
    if (message) return message
  }
  return error instanceof Error && error.message ? error.message : fallback
}

function statusLabel(
  status: VirtualCharacterStatus,
  t: (key: string) => string
): string {
  return t(
    {
      creating: 'Creating',
      active: 'Active',
      blocked: 'Blocked',
      offline: 'Offline',
      deleting: 'Deleting',
      failed: 'Failed',
    }[status]
  )
}

function validationStatusLabel(
  status: VirtualCharacterValidationSession['status'],
  t: (key: string) => string
): string {
  return t(
    {
      pending: 'Waiting for validation',
      succeeded: 'Validation succeeded',
      failed: 'Validation failed',
      expired: 'Validation expired',
    }[status]
  )
}

function taskStatusLabel(status: string, t: (key: string) => string): string {
  const normalized = status.toUpperCase()
  if (normalized === 'SUCCESS') return t('Succeeded')
  if (normalized === 'FAILURE' || normalized === 'FAILED') return t('Failed')
  if (normalized === 'IN_PROGRESS' || normalized === 'ACTIVE')
    return t('Running')
  if (['SUBMITTED', 'QUEUED', 'READY', 'SUBMITTING'].includes(normalized))
    return t('Queued')
  return status
}
