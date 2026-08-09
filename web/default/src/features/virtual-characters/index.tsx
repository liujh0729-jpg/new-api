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
import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Add01Icon,
  AiUserIcon,
  Clock01Icon,
  Delete02Icon,
  Edit02Icon,
  FileUploadIcon,
  Image01Icon,
  MagicWand01Icon,
  RefreshIcon,
  Settings01Icon,
  Video01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import {
  createCharacterVideo,
  createPublicVirtualCharacter,
  deleteVirtualCharacter,
  getAvailableVideoModels,
  getVirtualCharacterConfig,
  getVirtualCharacterHistory,
  getVirtualCharacterSettings,
  importPublicVirtualCharacters,
  listAdminPublicCharacters,
  listVirtualCharacters,
  offlinePublicVirtualCharacter,
  setVirtualCharacterUserLimit,
  updatePublicVirtualCharacter,
  updateVirtualCharacter,
  updateVirtualCharacterSettings,
  uploadVirtualCharacter,
  virtualCharacterQueryKeys,
} from './api'
import type {
  PublicVirtualCharacterInput,
  VirtualCharacter,
  VirtualCharacterSettings,
  VirtualCharacterStatus,
  VirtualCharacterTask,
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
  const [selected, setSelected] = useState<VirtualCharacter | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [generateTarget, setGenerateTarget] = useState<VirtualCharacter | null>(
    null
  )
  const [editTarget, setEditTarget] = useState<VirtualCharacter | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<VirtualCharacter | null>(
    null
  )
  const [publicEditor, setPublicEditor] = useState<
    VirtualCharacter | 'create' | null
  >(null)
  const [settingsOpen, setSettingsOpen] = useState(false)

  const publicUserQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list('public', publicPage, false),
    queryFn: () => listVirtualCharacters('public', publicPage),
    enabled: !isAdmin,
  })
  const publicAdminQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list('public', publicPage, true),
    queryFn: () => listAdminPublicCharacters(publicPage),
    enabled: isAdmin,
  })
  const privateQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list('private', privatePage, false),
    queryFn: () => listVirtualCharacters('private', privatePage),
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

  const publicData = isAdmin
    ? publicAdminQuery.data?.data
    : publicUserQuery.data?.data.page
  const privateData = privateQuery.data?.data.page
  const historyData = historyQuery.data?.data.page
  const activeLoading =
    tab === 'public'
      ? isAdmin
        ? publicAdminQuery.isLoading
        : publicUserQuery.isLoading
      : tab === 'private'
        ? privateQuery.isLoading
        : historyQuery.isLoading

  const filteredCharacters = useMemo(() => {
    const items = tab === 'public' ? publicData?.items : privateData?.items
    if (!items) return []
    const keyword = filter.trim().toLowerCase()
    if (!keyword) return items
    return items.filter((item) =>
      [item.name, item.description, ...item.tags]
        .join(' ')
        .toLowerCase()
        .includes(keyword)
    )
  }, [filter, privateData?.items, publicData?.items, tab])

  const allowedModels = useMemo(() => {
    const configured = configQuery.data?.data.models ?? []
    const available = new Set(userModelsQuery.data ?? [])
    return configured.filter((model) => available.has(model))
  }, [configQuery.data?.data.models, userModelsQuery.data])

  const refreshAll = async () => {
    await queryClient.invalidateQueries({
      queryKey: virtualCharacterQueryKeys.all,
    })
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      if (deleteTarget.scope === 'public') {
        await offlinePublicVirtualCharacter(deleteTarget.id)
        toast.success(t('Public character taken offline'))
      } else {
        await deleteVirtualCharacter(deleteTarget.id)
        toast.success(t('Character deletion queued'))
      }
      setDeleteTarget(null)
      setSelected(null)
      await refreshAll()
    } catch {
      // The shared API interceptor displays the server message.
    }
  }

  const activePage =
    tab === 'public'
      ? publicData
      : tab === 'private'
        ? privateData
        : historyData
  const pageValue =
    tab === 'public'
      ? publicPage
      : tab === 'private'
        ? privatePage
        : historyPage
  const setPage = (value: number) => {
    if (tab === 'public') setPublicPage(value)
    else if (tab === 'private') setPrivatePage(value)
    else setHistoryPage(value)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Character Library')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' size='sm' onClick={() => void refreshAll()}>
          <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
          {t('Refresh')}
        </Button>
        {isAdmin && (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setSettingsOpen(true)}
          >
            <HugeiconsIcon icon={Settings01Icon} strokeWidth={2} />
            {t('Library settings')}
          </Button>
        )}
        {tab === 'public' && isAdmin && (
          <Button size='sm' onClick={() => setPublicEditor('create')}>
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('Add public character')}
          </Button>
        )}
        {tab === 'private' && (
          <Button size='sm' onClick={() => setUploadOpen(true)}>
            <HugeiconsIcon icon={FileUploadIcon} strokeWidth={2} />
            {t('Create character')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='min-w-[960px] space-y-4'>
          <div className='flex items-end justify-between gap-4'>
            <div>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Use shared public characters or your private fictional characters to create Seedance videos.'
                )}
              </p>
              {tab === 'private' && (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Private quota')}: {privateQuery.data?.data.used ?? 0} /{' '}
                  {privateQuery.data?.data.limit ?? 0}
                </p>
              )}
            </div>
            {tab !== 'history' && (
              <Input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder={t('Search characters')}
                className='w-72'
              />
            )}
          </div>

          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as LibraryTab)}
          >
            <TabsList>
              <TabsTrigger value='public'>{t('Public characters')}</TabsTrigger>
              <TabsTrigger value='private'>{t('My characters')}</TabsTrigger>
              <TabsTrigger value='history'>{t('Task history')}</TabsTrigger>
            </TabsList>
          </Tabs>

          {tab === 'history' && historyQuery.data?.data.output_notice && (
            <Alert>
              <HugeiconsIcon icon={Clock01Icon} strokeWidth={2} />
              <AlertTitle>{t('Temporary output')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Task metadata is retained for 90 days. Generated video URLs may expire after about 24 hours.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {activeLoading ? (
            <CharacterGridSkeleton />
          ) : tab === 'history' ? (
            <TaskHistoryList items={historyData?.items ?? []} />
          ) : filteredCharacters.length > 0 ? (
            <div className='grid grid-cols-4 gap-4 2xl:grid-cols-5'>
              {filteredCharacters.map((item) => (
                <CharacterCard
                  key={item.id}
                  item={item}
                  isAdmin={isAdmin}
                  onSelect={() => setSelected(item)}
                  onGenerate={() => setGenerateTarget(item)}
                  onEdit={() =>
                    item.scope === 'public'
                      ? setPublicEditor(item)
                      : setEditTarget(item)
                  }
                  onDelete={() => setDeleteTarget(item)}
                />
              ))}
            </div>
          ) : (
            <EmptyLibrary tab={tab} />
          )}

          <Pagination
            page={pageValue}
            pageSize={activePage?.page_size ?? 20}
            total={activePage?.total ?? 0}
            onChange={setPage}
          />
        </div>

        <CharacterDetailsDialog
          item={selected}
          isAdmin={isAdmin}
          onOpenChange={(open) => !open && setSelected(null)}
          onGenerate={(item) => {
            setSelected(null)
            setGenerateTarget(item)
          }}
          onEdit={(item) => {
            setSelected(null)
            if (item.scope === 'public') setPublicEditor(item)
            else setEditTarget(item)
          }}
        />
        <UploadCharacterDialog
          open={uploadOpen}
          onOpenChange={setUploadOpen}
          onCompleted={refreshAll}
        />
        <EditPrivateCharacterDialog
          item={editTarget}
          onOpenChange={(open) => !open && setEditTarget(null)}
          onCompleted={refreshAll}
        />
        <GenerateVideoDialog
          item={generateTarget}
          models={allowedModels}
          defaultModel={configQuery.data?.data.default_model ?? ''}
          onOpenChange={(open) => !open && setGenerateTarget(null)}
          onCreated={async () => {
            setGenerateTarget(null)
            setTab('history')
            await refreshAll()
          }}
        />
        {isAdmin && (
          <PublicCharacterDialog
            target={publicEditor}
            settings={settingsQuery.data?.data}
            onOpenChange={(open) => !open && setPublicEditor(null)}
            onCompleted={refreshAll}
          />
        )}
        {isAdmin && (
          <LibrarySettingsDialog
            open={settingsOpen}
            settings={settingsQuery.data?.data}
            onOpenChange={setSettingsOpen}
            onCompleted={refreshAll}
          />
        )}
        <AlertDialog
          open={Boolean(deleteTarget)}
          onOpenChange={(open) => !open && setDeleteTarget(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {deleteTarget?.scope === 'public'
                  ? t('Take public character offline?')
                  : t('Delete this character?')}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {deleteTarget?.scope === 'public'
                  ? t('Users will no longer be able to start tasks with it.')
                  : t(
                      'New tasks are blocked immediately. The source is removed after running tasks finish.'
                    )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
              <AlertDialogAction onClick={() => void handleDelete()}>
                {t('Confirm')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function CharacterCard({
  item,
  isAdmin,
  onSelect,
  onGenerate,
  onEdit,
  onDelete,
}: {
  item: VirtualCharacter
  isAdmin: boolean
  onSelect: () => void
  onGenerate: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const manageable = item.scope === 'private' || isAdmin
  return (
    <Card
      className='group cursor-pointer overflow-hidden p-0 transition-shadow hover:shadow-md'
      onClick={onSelect}
    >
      <div className='bg-muted relative aspect-[4/5] overflow-hidden'>
        {item.cover_url ? (
          <img
            src={item.cover_url}
            alt={item.name}
            className='size-full object-cover transition-transform duration-300 group-hover:scale-[1.03]'
          />
        ) : (
          <div className='text-muted-foreground flex size-full items-center justify-center'>
            <HugeiconsIcon
              icon={AiUserIcon}
              strokeWidth={1.5}
              className='size-12'
            />
          </div>
        )}
        <Badge className='bg-background/90 text-foreground absolute top-2 left-2'>
          {item.scope === 'public' ? t('Public') : t('Private')}
        </Badge>
        {item.status !== 'active' && (
          <Badge className='absolute top-2 right-2' variant='outline'>
            {statusLabel(item.status, t)}
          </Badge>
        )}
      </div>
      <CardHeader className='gap-1 pb-2'>
        <CardTitle className='truncate'>{item.name}</CardTitle>
        <p className='text-muted-foreground line-clamp-2 min-h-10 text-xs'>
          {item.description || t('No description')}
        </p>
      </CardHeader>
      <CardContent className='space-y-3 pb-4'>
        <div className='flex min-h-5 flex-wrap gap-1'>
          {item.tags.slice(0, 3).map((tag) => (
            <Badge key={tag} variant='secondary' className='max-w-24 truncate'>
              {tag}
            </Badge>
          ))}
        </div>
        <div
          className='flex items-center gap-1.5'
          onClick={(e) => e.stopPropagation()}
        >
          <Button
            size='sm'
            className='flex-1'
            disabled={item.status !== 'active'}
            onClick={onGenerate}
          >
            <HugeiconsIcon icon={Video01Icon} strokeWidth={2} />
            {t('Create video')}
          </Button>
          {manageable && (
            <Button variant='outline' size='icon-sm' onClick={onEdit}>
              <HugeiconsIcon icon={Edit02Icon} strokeWidth={2} />
              <span className='sr-only'>{t('Edit')}</span>
            </Button>
          )}
          {manageable && (
            <Button variant='destructive' size='icon-sm' onClick={onDelete}>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              <span className='sr-only'>{t('Delete')}</span>
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function CharacterDetailsDialog({
  item,
  isAdmin,
  onOpenChange,
  onGenerate,
  onEdit,
}: {
  item: VirtualCharacter | null
  isAdmin: boolean
  onOpenChange: (open: boolean) => void
  onGenerate: (item: VirtualCharacter) => void
  onEdit: (item: VirtualCharacter) => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        {item && (
          <div className='grid grid-cols-[280px_1fr] gap-5'>
            <div className='bg-muted aspect-[4/5] overflow-hidden rounded-lg'>
              {item.cover_url ? (
                <img
                  src={item.cover_url}
                  alt={item.name}
                  className='size-full object-cover'
                />
              ) : (
                <div className='text-muted-foreground flex size-full items-center justify-center'>
                  <HugeiconsIcon
                    icon={AiUserIcon}
                    strokeWidth={1.5}
                    className='size-16'
                  />
                </div>
              )}
            </div>
            <div className='flex min-w-0 flex-col'>
              <DialogHeader>
                <DialogTitle className='text-xl'>{item.name}</DialogTitle>
                <DialogDescription>
                  {item.description || t('No description')}
                </DialogDescription>
              </DialogHeader>
              <div className='mt-5 space-y-4 text-sm'>
                <DetailRow
                  label={t('Library')}
                  value={
                    item.scope === 'public'
                      ? t('Public library')
                      : t('Private library')
                  }
                />
                <DetailRow
                  label={t('Status')}
                  value={statusLabel(item.status, t)}
                />
                <DetailRow
                  label={t('Verification')}
                  value={validationLabel(item.validation_status, t)}
                />
                <div>
                  <p className='text-muted-foreground mb-2 text-xs'>
                    {t('Tags')}
                  </p>
                  <div className='flex flex-wrap gap-1.5'>
                    {item.tags.length > 0 ? (
                      item.tags.map((tag) => (
                        <Badge key={tag} variant='secondary'>
                          {tag}
                        </Badge>
                      ))
                    ) : (
                      <span>{t('No tags')}</span>
                    )}
                  </div>
                </div>
                {item.last_error && (
                  <p className='text-destructive bg-destructive/10 rounded-lg p-3 text-xs'>
                    {item.last_error}
                  </p>
                )}
              </div>
              <div className='mt-auto flex justify-end gap-2 pt-6'>
                {(item.scope === 'private' || isAdmin) && (
                  <Button variant='outline' onClick={() => onEdit(item)}>
                    <HugeiconsIcon icon={Edit02Icon} strokeWidth={2} />
                    {t('Edit')}
                  </Button>
                )}
                <Button
                  disabled={item.status !== 'active'}
                  onClick={() => onGenerate(item)}
                >
                  <HugeiconsIcon icon={MagicWand01Icon} strokeWidth={2} />
                  {t('Create video')}
                </Button>
              </div>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function UploadCharacterDialog({
  open,
  onOpenChange,
  onCompleted,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCompleted: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!file || !name.trim()) return
    setSubmitting(true)
    try {
      await uploadVirtualCharacter({
        file,
        name: name.trim(),
        description: description.trim(),
        tags: splitTags(tags),
      })
      toast.success(t('Character created'))
      setFile(null)
      setName('')
      setDescription('')
      setTags('')
      onOpenChange(false)
      await onCompleted()
    } catch {
      // Shared interceptor handles errors.
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <form onSubmit={submit} className='space-y-4'>
          <DialogHeader>
            <DialogTitle>{t('Create private character')}</DialogTitle>
            <DialogDescription>
              {t(
                'Upload one fictional character image. Real people are not supported.'
              )}
            </DialogDescription>
          </DialogHeader>
          <FormField label={t('Character image')}>
            <Input
              type='file'
              accept='.jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp'
              required
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            />
            <p className='text-muted-foreground text-xs'>
              {t('JPG, PNG, or WebP; maximum 30 MB.')}
            </p>
          </FormField>
          <FormField label={t('Name')}>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={191}
              required
            />
          </FormField>
          <FormField label={t('Description')}>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              maxLength={2000}
              rows={3}
            />
          </FormField>
          <FormField label={t('Tags')}>
            <Input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder={t('Separate tags with commas')}
            />
          </FormField>
          <Alert>
            <HugeiconsIcon icon={Image01Icon} strokeWidth={2} />
            <AlertDescription>
              {t(
                'By uploading, you confirm this image does not depict a real person.'
              )}
            </AlertDescription>
          </Alert>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              disabled={submitting || !file || !name.trim()}
            >
              {submitting ? t('Uploading...') : t('Create character')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function EditPrivateCharacterDialog({
  item,
  onOpenChange,
  onCompleted,
}: {
  item: VirtualCharacter | null
  onOpenChange: (open: boolean) => void
  onCompleted: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!item) return
    const form = new FormData(event.currentTarget)
    setSubmitting(true)
    try {
      await updateVirtualCharacter(item.id, {
        name: String(form.get('name') ?? '').trim(),
        description: String(form.get('description') ?? '').trim(),
        tags: splitTags(String(form.get('tags') ?? '')),
      })
      toast.success(t('Character updated'))
      onOpenChange(false)
      await onCompleted()
    } catch {
      // Shared interceptor handles errors.
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        {item && (
          <form key={item.id} onSubmit={submit} className='space-y-4'>
            <DialogHeader>
              <DialogTitle>{t('Edit character')}</DialogTitle>
              <DialogDescription>
                {t('The source image is not replaced when metadata changes.')}
              </DialogDescription>
            </DialogHeader>
            <FormField label={t('Name')}>
              <Input
                name='name'
                defaultValue={item.name}
                required
                maxLength={191}
              />
            </FormField>
            <FormField label={t('Description')}>
              <Textarea
                name='description'
                defaultValue={item.description}
                rows={3}
                maxLength={2000}
              />
            </FormField>
            <FormField label={t('Tags')}>
              <Input name='tags' defaultValue={item.tags.join(', ')} />
            </FormField>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={submitting}>
                {submitting ? t('Saving...') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function GenerateVideoDialog({
  item,
  models,
  defaultModel,
  onOpenChange,
  onCreated,
}: {
  item: VirtualCharacter | null
  models: string[]
  defaultModel: string
  onOpenChange: (open: boolean) => void
  onCreated: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!item) return
    const form = new FormData(event.currentTarget)
    setSubmitting(true)
    try {
      const response = await createCharacterVideo({
        character_id: item.id,
        model: String(form.get('model') ?? ''),
        prompt: String(form.get('prompt') ?? '').trim(),
        duration: Number(form.get('duration') ?? 5),
        ratio: String(form.get('ratio') ?? '16:9'),
        resolution: String(form.get('resolution') ?? '720p'),
      })
      if (response.error?.message) throw new Error(response.error.message)
      toast.success(t('Video task created'))
      await onCreated()
    } catch (error) {
      toast.error(errorMessage(error, t('Failed to create video task')))
    } finally {
      setSubmitting(false)
    }
  }
  const selectedModel = models.includes(defaultModel)
    ? defaultModel
    : (models[0] ?? '')
  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-xl'>
        {item && (
          <form
            key={`${item.id}-${selectedModel}`}
            onSubmit={submit}
            className='space-y-4'
          >
            <DialogHeader>
              <DialogTitle>
                {t('Create video with {{name}}', { name: item.name })}
              </DialogTitle>
              <DialogDescription>
                {t(
                  'This creates a new video task and does not modify the character image.'
                )}
              </DialogDescription>
            </DialogHeader>
            {models.length === 0 ? (
              <Alert>
                <AlertTitle>{t('No available model')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Your account has no enabled Seedance 2.0 model from the character whitelist.'
                  )}
                </AlertDescription>
              </Alert>
            ) : (
              <>
                <FormField label={t('Prompt')}>
                  <Textarea
                    name='prompt'
                    required
                    rows={5}
                    placeholder={t(
                      'Describe how 图片1中的角色 should move and what scene to create'
                    )}
                  />
                </FormField>
                <div className='grid grid-cols-2 gap-3'>
                  <FormField label={t('Model')}>
                    <NativeSelect
                      name='model'
                      defaultValue={selectedModel}
                      className='w-full'
                    >
                      {models.map((model) => (
                        <NativeSelectOption key={model} value={model}>
                          {model}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </FormField>
                  <FormField label={t('Duration')}>
                    <NativeSelect
                      name='duration'
                      defaultValue='5'
                      className='w-full'
                    >
                      {[4, 5, 6, 8, 10, 12, 15].map((value) => (
                        <NativeSelectOption key={value} value={String(value)}>
                          {t('{{count}} seconds', { count: value })}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </FormField>
                  <FormField label={t('Aspect ratio')}>
                    <NativeSelect
                      name='ratio'
                      defaultValue='16:9'
                      className='w-full'
                    >
                      {['16:9', '9:16', '1:1'].map((value) => (
                        <NativeSelectOption key={value} value={value}>
                          {value}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </FormField>
                  <FormField label={t('Resolution')}>
                    <NativeSelect
                      name='resolution'
                      defaultValue='720p'
                      className='w-full'
                    >
                      <NativeSelectOption value='720p'>720p</NativeSelectOption>
                      <NativeSelectOption value='1080p'>
                        1080p
                      </NativeSelectOption>
                    </NativeSelect>
                  </FormField>
                </div>
              </>
            )}
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                disabled={submitting || models.length === 0}
              >
                {submitting ? t('Submitting...') : t('Create video')}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function PublicCharacterDialog({
  target,
  settings,
  onOpenChange,
  onCompleted,
}: {
  target: VirtualCharacter | 'create' | null
  settings?: VirtualCharacterSettings
  onOpenChange: (open: boolean) => void
  onCompleted: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const item = target && target !== 'create' ? target : null
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const input: PublicVirtualCharacterInput = {
      name: String(form.get('name') ?? '').trim(),
      description: String(form.get('description') ?? '').trim(),
      tags: splitTags(String(form.get('tags') ?? '')),
      cover_url: String(form.get('cover_url') ?? '').trim(),
      asset_id: String(form.get('asset_id') ?? '').trim(),
      public_channel_id: Number(form.get('public_channel_id') ?? 0),
      status: String(form.get('status') ?? 'active') as 'active' | 'offline',
    }
    setSubmitting(true)
    try {
      if (item) await updatePublicVirtualCharacter(item.id, input)
      else await createPublicVirtualCharacter(input)
      toast.success(
        item ? t('Public character updated') : t('Public character created')
      )
      onOpenChange(false)
      await onCompleted()
    } catch {
      // Shared interceptor handles errors.
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-xl'>
        {target && (
          <form
            key={item?.id ?? 'create'}
            onSubmit={submit}
            className='space-y-4'
          >
            <DialogHeader>
              <DialogTitle>
                {item ? t('Edit public character') : t('Add public character')}
              </DialogTitle>
              <DialogDescription>
                {t(
                  'Public characters use a Volc asset:// reference and a fixed single-key channel.'
                )}
              </DialogDescription>
            </DialogHeader>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label={t('Name')}>
                <Input name='name' defaultValue={item?.name} required />
              </FormField>
              <FormField label={t('Asset ID')}>
                <Input
                  name='asset_id'
                  defaultValue={item?.asset_id}
                  placeholder='asset-...'
                  required
                />
              </FormField>
            </div>
            <FormField label={t('Cover URL')}>
              <Input
                name='cover_url'
                type='url'
                defaultValue={item?.cover_url}
                required
              />
            </FormField>
            <FormField label={t('Description')}>
              <Textarea
                name='description'
                defaultValue={item?.description}
                rows={3}
              />
            </FormField>
            <FormField label={t('Tags')}>
              <Input name='tags' defaultValue={item?.tags.join(', ')} />
            </FormField>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label={t('Public channel')}>
                <NativeSelect
                  name='public_channel_id'
                  defaultValue={String(
                    item?.public_channel_id ??
                      settings?.public_channels[0]?.id ??
                      ''
                  )}
                  className='w-full'
                  required
                >
                  {settings?.public_channels.map((channel) => (
                    <NativeSelectOption
                      key={channel.id}
                      value={String(channel.id)}
                    >
                      {channel.name} (#{channel.id})
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </FormField>
              <FormField label={t('Status')}>
                <NativeSelect
                  name='status'
                  defaultValue={item?.status ?? 'active'}
                  className='w-full'
                >
                  <NativeSelectOption value='active'>
                    {t('Active')}
                  </NativeSelectOption>
                  <NativeSelectOption value='offline'>
                    {t('Offline')}
                  </NativeSelectOption>
                </NativeSelect>
              </FormField>
            </div>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                disabled={submitting || !settings?.public_channels.length}
              >
                {submitting ? t('Saving...') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function LibrarySettingsDialog({
  open,
  settings,
  onOpenChange,
  onCompleted,
}: {
  open: boolean
  settings?: VirtualCharacterSettings
  onOpenChange: (open: boolean) => void
  onCompleted: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [importing, setImporting] = useState(false)
  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setSaving(true)
    try {
      await updateVirtualCharacterSettings({
        global_limit: Number(form.get('global_limit') ?? 100),
        models: splitTags(String(form.get('models') ?? '')),
        default_model: String(form.get('default_model') ?? '').trim(),
      })
      const userId = Number(form.get('user_id') ?? 0)
      const userLimit = Number(form.get('user_limit') ?? 0)
      if (userId > 0) await setVirtualCharacterUserLimit(userId, userLimit)
      toast.success(t('Library settings updated'))
      await onCompleted()
    } catch {
      // Shared interceptor handles errors.
    } finally {
      setSaving(false)
    }
  }
  const importFile = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formElement = event.currentTarget
    const form = new FormData(formElement)
    const file = form.get('import_file')
    if (!(file instanceof File) || !file.size) return
    setImporting(true)
    try {
      const channelId = Number(form.get('import_channel') ?? 0)
      const result = await importPublicVirtualCharacters(
        file,
        channelId || undefined
      )
      toast.success(
        t('Imported {{count}} public characters', {
          count: result.data.processed,
        })
      )
      formElement.reset()
      await onCompleted()
    } catch {
      // Shared interceptor handles errors.
    } finally {
      setImporting(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Character library settings')}</DialogTitle>
          <DialogDescription>
            {t(
              'Configure private quotas, Seedance 2.0 models, and public library imports.'
            )}
          </DialogDescription>
        </DialogHeader>
        {settings ? (
          <div className='space-y-6'>
            <form
              key={`${settings.global_limit}-${settings.default_model}`}
              onSubmit={save}
              className='space-y-4'
            >
              <div className='grid grid-cols-2 gap-3'>
                <FormField label={t('Default private quota')}>
                  <Input
                    name='global_limit'
                    type='number'
                    min={1}
                    max={10000}
                    defaultValue={settings.global_limit}
                    required
                  />
                </FormField>
                <FormField label={t('Default model')}>
                  <Input
                    name='default_model'
                    defaultValue={settings.default_model}
                    required
                  />
                </FormField>
              </div>
              <FormField label={t('Seedance 2.0 model whitelist')}>
                <Textarea
                  name='models'
                  defaultValue={settings.models.join(', ')}
                  rows={3}
                  required
                />
                <p className='text-muted-foreground text-xs'>
                  {t('Separate model names with commas.')}
                </p>
              </FormField>
              <div className='rounded-lg border p-3'>
                <p className='mb-3 text-sm font-medium'>
                  {t('Per-account quota override')}
                </p>
                <div className='grid grid-cols-2 gap-3'>
                  <FormField label={t('User ID')}>
                    <Input
                      name='user_id'
                      type='number'
                      min={1}
                      placeholder={t('Leave blank to skip')}
                    />
                  </FormField>
                  <FormField label={t('Quota override')}>
                    <Input
                      name='user_limit'
                      type='number'
                      min={0}
                      max={10000}
                      defaultValue={0}
                    />
                    <p className='text-muted-foreground text-xs'>
                      {t('Use 0 to remove the override.')}
                    </p>
                  </FormField>
                </div>
              </div>
              <div className='flex justify-end'>
                <Button type='submit' disabled={saving}>
                  {saving ? t('Saving...') : t('Save settings')}
                </Button>
              </div>
            </form>
            <form onSubmit={importFile} className='space-y-4 border-t pt-5'>
              <div>
                <p className='text-sm font-medium'>
                  {t('Import public characters')}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Upload JSON or CSV. Existing channel + Asset ID pairs are updated.'
                  )}
                </p>
              </div>
              <div className='grid grid-cols-2 gap-3'>
                <FormField label={t('Import file')}>
                  <Input
                    name='import_file'
                    type='file'
                    accept='.json,.csv,application/json,text/csv'
                    required
                  />
                </FormField>
                <FormField label={t('Default public channel')}>
                  <NativeSelect
                    name='import_channel'
                    defaultValue={String(settings.public_channels[0]?.id ?? '')}
                    className='w-full'
                  >
                    {settings.public_channels.map((channel) => (
                      <NativeSelectOption
                        key={channel.id}
                        value={String(channel.id)}
                      >
                        {channel.name} (#{channel.id})
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </FormField>
              </div>
              <div className='flex justify-end'>
                <Button
                  type='submit'
                  variant='outline'
                  disabled={importing || !settings.public_channels.length}
                >
                  <HugeiconsIcon icon={FileUploadIcon} strokeWidth={2} />
                  {importing ? t('Importing...') : t('Import')}
                </Button>
              </div>
            </form>
          </div>
        ) : (
          <Skeleton className='h-80 w-full' />
        )}
      </DialogContent>
    </Dialog>
  )
}

function TaskHistoryList({ items }: { items: VirtualCharacterTask[] }) {
  const { t } = useTranslation()
  if (!items.length) return <EmptyLibrary tab='history' />
  return (
    <div className='space-y-2'>
      {items.map((item) => {
        const status = item.task?.status ?? item.link_status
        return (
          <Card key={item.task_id} className='py-3'>
            <CardContent className='grid grid-cols-[minmax(220px,1fr)_180px_140px_180px] items-center gap-4'>
              <div className='min-w-0'>
                <p className='truncate font-medium'>{item.character_name}</p>
                <p className='text-muted-foreground truncate font-mono text-xs'>
                  {item.task_id}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>{t('Model')}</p>
                <p className='truncate text-sm'>
                  {item.task?.properties?.origin_model_name ??
                    item.task?.properties?.upstream_model_name ??
                    '-'}
                </p>
              </div>
              <div>
                <Badge variant='outline'>{taskStatusLabel(status, t)}</Badge>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {item.task?.progress ?? ''}
                </p>
              </div>
              <div className='flex justify-end'>
                {item.task?.result_url ? (
                  <Button
                    variant='outline'
                    size='sm'
                    render={
                      <a
                        href={item.task.result_url}
                        target='_blank'
                        rel='noreferrer'
                      />
                    }
                  >
                    <HugeiconsIcon icon={Video01Icon} strokeWidth={2} />
                    {t('Open video')}
                  </Button>
                ) : (
                  <span
                    className={cn(
                      'text-xs',
                      item.task?.fail_reason || item.error
                        ? 'text-destructive'
                        : 'text-muted-foreground'
                    )}
                  >
                    {item.task?.fail_reason || item.error || t('Processing')}
                  </span>
                )}
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}

function CharacterGridSkeleton() {
  return (
    <div className='grid grid-cols-4 gap-4 2xl:grid-cols-5'>
      {Array.from({ length: 8 }).map((_, index) => (
        <Card key={index} className='overflow-hidden p-0'>
          <Skeleton className='aspect-[4/5] w-full rounded-none' />
          <CardContent className='space-y-2 py-4'>
            <Skeleton className='h-5 w-2/3' />
            <Skeleton className='h-4 w-full' />
            <Skeleton className='h-8 w-full' />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function EmptyLibrary({ tab }: { tab: LibraryTab }) {
  const { t } = useTranslation()
  return (
    <div className='text-muted-foreground flex min-h-72 flex-col items-center justify-center rounded-xl border border-dashed'>
      <HugeiconsIcon
        icon={tab === 'history' ? Clock01Icon : AiUserIcon}
        strokeWidth={1.5}
        className='mb-3 size-10'
      />
      <p className='text-foreground font-medium'>
        {tab === 'history'
          ? t('No character tasks yet')
          : t('No characters found')}
      </p>
      <p className='mt-1 text-sm'>
        {tab === 'private'
          ? t('Create your first private fictional character.')
          : t('There is nothing to show on this page.')}
      </p>
    </div>
  )
}

function Pagination({
  page,
  pageSize,
  total,
  onChange,
}: {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)))
  if (total <= pageSize) return null
  return (
    <div className='flex items-center justify-end gap-3'>
      <span className='text-muted-foreground text-xs'>
        {t('Page {{page}} of {{pages}}', { page, pages })}
      </span>
      <Button
        variant='outline'
        size='sm'
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        {t('Previous')}
      </Button>
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

function FormField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className='space-y-1.5'>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className='grid grid-cols-[100px_1fr] gap-3'>
      <span className='text-muted-foreground'>{label}</span>
      <span>{value}</span>
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
    const responseMessage =
      response.response?.data?.error?.message ||
      response.response?.data?.message
    if (responseMessage) return responseMessage
  }
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function statusLabel(
  status: VirtualCharacterStatus,
  t: (key: string) => string
): string {
  const labels: Record<VirtualCharacterStatus, string> = {
    creating: 'Creating',
    active: 'Active',
    blocked: 'Blocked',
    offline: 'Offline',
    deleting: 'Deleting',
    failed: 'Failed',
  }
  return t(labels[status])
}

function validationLabel(
  status: VirtualCharacter['validation_status'],
  t: (key: string) => string
): string {
  return t(
    status === 'accepted'
      ? 'Accepted'
      : status === 'rejected'
        ? 'Rejected'
        : 'Unverified'
  )
}

function taskStatusLabel(status: string, t: (key: string) => string): string {
  const normalized = status.toUpperCase()
  if (normalized === 'SUCCESS') return t('Succeeded')
  if (normalized === 'FAILURE' || normalized === 'FAILED') return t('Failed')
  if (normalized === 'IN_PROGRESS' || normalized === 'ACTIVE')
    return t('Running')
  if (
    normalized === 'SUBMITTED' ||
    normalized === 'QUEUED' ||
    normalized === 'READY' ||
    normalized === 'SUBMITTING'
  )
    return t('Queued')
  return status
}
