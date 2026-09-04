import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, Plus, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import {
  archiveMembershipLevel,
  createMembershipLevel,
  getMembershipLevels,
  updateMembershipLevel,
} from './api'
import { LevelDialog } from './components/level-dialog'
import type {
  MembershipLevel,
  MembershipLevelDraft,
  MembershipLevelInput,
} from './types'

function formatMultiplier(ppm: number) {
  return Number((ppm / 1_000_000).toFixed(6))
}

function createLevelDraft(level: MembershipLevel | null): MembershipLevelDraft {
  return {
    code: level?.code ?? '',
    display_name: level?.display_name ?? '',
    multiplier: String((level?.multiplier_ppm ?? 1_000_000) / 1_000_000),
    rank: String(level?.rank ?? 0),
    sort_order: String(level?.sort_order ?? 0),
    enabled: level?.enabled ?? true,
  }
}

export function MembershipManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingLevel, setEditingLevel] = useState<MembershipLevel | null>(null)
  const [levelDraft, setLevelDraft] = useState<MembershipLevelDraft>(() =>
    createLevelDraft(null)
  )
  const [levelDialogOpen, setLevelDialogOpen] = useState(false)
  const [archiveTarget, setArchiveTarget] = useState<MembershipLevel | null>(
    null
  )

  const levelsQuery = useQuery({
    queryKey: ['membership-levels'],
    queryFn: () => getMembershipLevels(false),
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['membership-levels'] })

  const saveMutation = useMutation({
    mutationFn: async (input: MembershipLevelInput) => {
      if (editingLevel) {
        const { code: _code, ...updates } = input
        return updateMembershipLevel(editingLevel.id, updates)
      }
      return createMembershipLevel(input)
    },
    onSuccess: async () => {
      toast.success(t('Membership level saved'))
      setLevelDialogOpen(false)
      await refresh()
    },
    onError: (error) => toast.error(error.message),
  })

  const archiveMutation = useMutation({
    mutationFn: archiveMembershipLevel,
    onSuccess: async () => {
      toast.success(t('Membership level archived'))
      setArchiveTarget(null)
      await refresh()
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Membership Management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Manage independent membership levels and global billing discounts.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <div className='flex gap-2'>
          <Button variant='outline' onClick={refresh}>
            <RefreshCw className='size-4' />
            {t('Refresh')}
          </Button>
          <Button
            onClick={() => {
              setEditingLevel(null)
              setLevelDraft(createLevelDraft(null))
              setLevelDialogOpen(true)
            }}
          >
            <Plus className='size-4' />
            {t('Add level')}
          </Button>
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle className='text-base'>
                {t('Membership levels')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Code')}</TableHead>
                    <TableHead>{t('Display name')}</TableHead>
                    <TableHead>{t('Multiplier')}</TableHead>
                    <TableHead>{t('Discount')}</TableHead>
                    <TableHead>{t('Rank')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(levelsQuery.data?.data ?? []).map((level) => (
                    <TableRow key={level.id}>
                      <TableCell className='font-mono'>{level.code}</TableCell>
                      <TableCell>{level.display_name}</TableCell>
                      <TableCell className='font-mono'>
                        {formatMultiplier(level.multiplier_ppm)}x
                      </TableCell>
                      <TableCell>
                        {t('{{discount}} discount', {
                          discount: Number(
                            (level.multiplier_ppm / 100_000).toFixed(2)
                          ),
                        })}
                      </TableCell>
                      <TableCell>{level.rank}</TableCell>
                      <TableCell>
                        {level.enabled ? t('Enabled') : t('Disabled')}
                      </TableCell>
                      <TableCell className='space-x-2 text-right'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => {
                            setEditingLevel(level)
                            setLevelDraft(createLevelDraft(level))
                            setLevelDialogOpen(true)
                          }}
                        >
                          {t('Edit')}
                        </Button>
                        {!level.is_default && (
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => setArchiveTarget(level)}
                          >
                            <Archive className='size-4' />
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>

      <LevelDialog
        open={levelDialogOpen}
        level={editingLevel}
        draft={levelDraft}
        submitting={saveMutation.isPending}
        onOpenChange={setLevelDialogOpen}
        onDraftChange={setLevelDraft}
        onSubmit={(input) => saveMutation.mutate(input)}
      />
      <ConfirmDialog
        open={Boolean(archiveTarget)}
        onOpenChange={(open) => !open && setArchiveTarget(null)}
        title={t('Archive membership level')}
        desc={t(
          'Existing grant history is preserved, but this level stops applying immediately.'
        )}
        destructive
        isLoading={archiveMutation.isPending}
        handleConfirm={() =>
          archiveTarget && archiveMutation.mutate(archiveTarget.id)
        }
      />
    </SectionPageLayout>
  )
}
