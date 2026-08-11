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
import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Add01Icon,
  RefreshIcon,
  Settings01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import {
  getAvailableVideoModels,
  getValidationSession,
  getVirtualCharacterConfig,
  getVirtualCharacterHistory,
  getVirtualCharacterSettings,
  listVirtualCharacters,
  virtualCharacterQueryKeys,
} from './api'
import { CharacterCard } from './components/character-card'
import { CharacterDetailDialog } from './components/character-detail-dialog'
import {
  CharacterImagePreviewDialog,
  type CharacterImagePreview,
} from './components/character-image-preview-dialog'
import { CreateRealPersonDialog } from './components/create-real-person-dialog'
import { CreateVirtualCharacterDialog } from './components/create-virtual-character-dialog'
import { DeleteCharacterDialog } from './components/delete-character-dialog'
import { GenerateDialog } from './components/generate-dialog'
import { SettingsDialog } from './components/settings-dialog'
import { TaskHistory } from './components/task-history'
import { CharacterGridSkeleton, Pagination } from './components/ui-bits'
import { ValidationDialog } from './components/validation-dialog'
import {
  VIRTUAL_CHARACTER_AGE_BANDS,
  VIRTUAL_CHARACTER_GENDER_LABEL_KEYS,
  VIRTUAL_CHARACTER_GENDERS,
  VIRTUAL_CHARACTER_NATIONALITIES,
  VIRTUAL_CHARACTER_NATIONALITY_LABEL_KEYS,
} from './constants'
import type {
  VirtualCharacter,
  VirtualCharacterValidationSession,
} from './types'

type LibraryTab = 'public' | 'private' | 'history'
const CHARACTER_PAGE_SIZE = 12

export function VirtualCharacters() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userRole = useAuthStore((state) => state.auth.user?.role ?? 0)
  const isAdmin = userRole >= ROLE.ADMIN
  const [tab, setTab] = useState<LibraryTab>('public')
  const [publicPage, setPublicPage] = useState(1)
  const [privatePage, setPrivatePage] = useState(1)
  const [historyPage, setHistoryPage] = useState(1)
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [nationality, setNationality] = useState('')
  const [gender, setGender] = useState('')
  const [ageBand, setAgeBand] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [createOpen, setCreateOpen] = useState(false)
  const [createVirtualOpen, setCreateVirtualOpen] = useState(false)
  const [validation, setValidation] =
    useState<VirtualCharacterValidationSession | null>(null)
  const [detailID, setDetailID] = useState<number | null>(null)
  const [imagePreview, setImagePreview] =
    useState<CharacterImagePreview | null>(null)
  const [generateTarget, setGenerateTarget] = useState<{
    character: VirtualCharacter
  } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<VirtualCharacter | null>(
    null
  )
  const [settingsOpen, setSettingsOpen] = useState(false)

  const publicListParams = {
    scope: 'public' as const,
    page: publicPage,
    pageSize: CHARACTER_PAGE_SIZE,
    keyword,
    nationality,
    gender,
    ageBand,
  }
  const privateListParams = {
    scope: 'private' as const,
    page: privatePage,
    pageSize: CHARACTER_PAGE_SIZE,
    keyword,
    status: statusFilter === 'all' ? '' : statusFilter,
  }

  const publicQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list(publicListParams),
    queryFn: () => listVirtualCharacters(publicListParams),
  })
  const privateQuery = useQuery({
    queryKey: virtualCharacterQueryKeys.list(privateListParams),
    queryFn: () => listVirtualCharacters(privateListParams),
    refetchInterval: (query) =>
      query.state.data?.data.page.items.some(
        (item) => item.status === 'creating'
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

  const currentPage = (() => {
    if (tab === 'public') return publicQuery.data?.data.page
    if (tab === 'private') return privateQuery.data?.data.page
    return historyQuery.data?.data.page
  })()
  const characters =
    tab === 'public'
      ? (publicQuery.data?.data.page.items ?? [])
      : (privateQuery.data?.data.page.items ?? [])
  const loading = (() => {
    if (tab === 'public') return publicQuery.isLoading
    if (tab === 'private') return privateQuery.isLoading
    return historyQuery.isLoading
  })()

  const applyKeywordSearch = () => {
    setKeyword(keywordInput.trim())
    setPublicPage(1)
    setPrivatePage(1)
  }

  const resetFilters = () => {
    setKeywordInput('')
    setKeyword('')
    setNationality('')
    setGender('')
    setAgeBand('')
    setStatusFilter('all')
    setPublicPage(1)
    setPrivatePage(1)
  }

  const refreshAll = async () => {
    await queryClient.invalidateQueries({
      queryKey: virtualCharacterQueryKeys.all,
    })
  }

  return (
    <>
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
              disabled={
                !configQuery.data?.data.virtual_enabled ||
                (privateQuery.data?.data.used ?? 0) >=
                  (privateQuery.data?.data.limit ?? 0)
              }
              onClick={() => setCreateVirtualOpen(true)}
            >
              <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
              {t('Upload character')}
            </Button>
          )}
          {/* Real-person create entry is intentionally omitted: the backend hard-disables
              real_person_enabled until that flow ships. CreateRealPersonDialog /
              ValidationDialog stay mounted for ?validation_session= deep links and
              onRetry; restore a gated Actions button when the API flag is live again. */}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <Tabs
              value={tab}
              onValueChange={(value) => setTab(value as LibraryTab)}
            >
              <TabsList>
                <TabsTrigger value='public'>
                  {t('Public characters')}
                </TabsTrigger>
                <TabsTrigger value='private'>{t('My characters')}</TabsTrigger>
                <TabsTrigger value='history'>{t('Task history')}</TabsTrigger>
              </TabsList>
            </Tabs>

            <div className='flex flex-wrap items-center gap-2'>
              {tab !== 'history' && (
                <>
                  <Input
                    className='max-w-sm'
                    value={keywordInput}
                    onChange={(event) => setKeywordInput(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        applyKeywordSearch()
                      }
                    }}
                    placeholder={t('Search by name or tag')}
                  />
                  {tab === 'public' && (
                    <>
                      <NativeSelect
                        value={nationality}
                        onChange={(event) => {
                          setNationality(event.target.value)
                          setPublicPage(1)
                        }}
                      >
                        <NativeSelectOption value=''>
                          {t('All nationalities')}
                        </NativeSelectOption>
                        {VIRTUAL_CHARACTER_NATIONALITIES.map((item) => (
                          <NativeSelectOption key={item} value={item}>
                            {t(VIRTUAL_CHARACTER_NATIONALITY_LABEL_KEYS[item])}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                      <NativeSelect
                        value={gender}
                        onChange={(event) => {
                          setGender(event.target.value)
                          setPublicPage(1)
                        }}
                      >
                        <NativeSelectOption value=''>
                          {t('All genders')}
                        </NativeSelectOption>
                        {VIRTUAL_CHARACTER_GENDERS.map((item) => (
                          <NativeSelectOption key={item} value={item}>
                            {t(VIRTUAL_CHARACTER_GENDER_LABEL_KEYS[item])}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                      <NativeSelect
                        value={ageBand}
                        onChange={(event) => {
                          setAgeBand(event.target.value)
                          setPublicPage(1)
                        }}
                      >
                        <NativeSelectOption value=''>
                          {t('All age ranges')}
                        </NativeSelectOption>
                        {VIRTUAL_CHARACTER_AGE_BANDS.map((band) => (
                          <NativeSelectOption
                            key={band.value}
                            value={band.value}
                          >
                            {t(band.labelKey)}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                    </>
                  )}
                  {tab === 'private' && (
                    <NativeSelect
                      value={statusFilter}
                      onChange={(event) => {
                        setStatusFilter(event.target.value)
                        setPrivatePage(1)
                      }}
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
                  )}
                  <Button size='sm' onClick={applyKeywordSearch}>
                    {t('Search')}
                  </Button>
                  <Button variant='outline' size='sm' onClick={resetFilters}>
                    {t('Reset')}
                  </Button>
                </>
              )}
              <Button variant='outline' size='sm' onClick={refreshAll}>
                <HugeiconsIcon icon={RefreshIcon} data-icon='inline-start' />
                {t('Refresh')}
              </Button>
              {tab === 'private' && privateQuery.data && (
                <Badge variant='outline'>
                  {t('{{used}} of {{limit}} characters', {
                    used: privateQuery.data.data.used ?? 0,
                    limit: privateQuery.data.data.limit ?? 0,
                  })}
                </Badge>
              )}
            </div>

            {tab === 'public' &&
              configQuery.isSuccess &&
              !configQuery.data?.data.official_enabled && (
                <Alert variant='destructive'>
                  <AlertTitle>
                    {t('Official character library is disabled')}
                  </AlertTitle>
                  <AlertDescription>
                    {t(
                      'An administrator must enable the character library and configure the Volc provider first.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
            {tab === 'private' &&
              configQuery.isSuccess &&
              !configQuery.data?.data.virtual_enabled && (
                <Alert variant='destructive'>
                  <AlertTitle>{t('Character library is disabled')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'An administrator must enable the character library and configure the Volc provider first.'
                    )}
                  </AlertDescription>
                </Alert>
              )}

            {(() => {
              if (loading) return <CharacterGridSkeleton />
              if (tab === 'history') {
                return (
                  <TaskHistory
                    items={historyQuery.data?.data.page.items ?? []}
                    outputNotice={historyQuery.data?.data.output_notice}
                    onRefresh={() => historyQuery.refetch()}
                  />
                )
              }
              if (characters.length === 0) {
                return (
                  <Card>
                    <CardHeader>
                      <CardTitle>{t('No characters found')}</CardTitle>
                      <CardDescription>
                        {tab === 'public'
                          ? t(
                              'The authoritative official catalog has no matching active characters.'
                            )
                          : t('Create your first private virtual character.')}
                      </CardDescription>
                    </CardHeader>
                  </Card>
                )
              }
              return (
                <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6'>
                  {characters.map((item) => (
                    <CharacterCard
                      key={item.id}
                      item={item}
                      onOpen={() => setDetailID(item.id)}
                      onPreview={setImagePreview}
                      onGenerate={() => setGenerateTarget({ character: item })}
                      onDelete={() => setDeleteTarget(item)}
                    />
                  ))}
                </div>
              )
            })()}

            {currentPage && currentPage.total > currentPage.page_size && (
              <Pagination
                page={currentPage.page}
                total={currentPage.total}
                pageSize={currentPage.page_size}
                onChange={(page) => {
                  if (tab === 'public') setPublicPage(page)
                  else if (tab === 'private') setPrivatePage(page)
                  else setHistoryPage(page)
                }}
              />
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CreateVirtualCharacterDialog
        open={createVirtualOpen}
        onOpenChange={setCreateVirtualOpen}
        onCreated={refreshAll}
      />
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
      />
      <CharacterImagePreviewDialog
        preview={imagePreview}
        onClose={() => setImagePreview(null)}
      />
      <GenerateDialog
        target={generateTarget}
        models={userModelsQuery.data ?? []}
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
    </>
  )
}
