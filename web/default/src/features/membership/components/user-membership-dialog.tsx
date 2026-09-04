import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  createUserMembership,
  getMembershipLevels,
  getUserMemberships,
  revokeUserMembership,
} from '../api'

type UserMembershipDialogProps = {
  open: boolean
  user: { id: number; username: string }
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

function toUnix(value: string): number {
  if (!value) return 0
  const milliseconds = new Date(value).getTime()
  return Number.isFinite(milliseconds) ? Math.floor(milliseconds / 1000) : 0
}

function formatTime(timestamp: number): string {
  return timestamp > 0 ? new Date(timestamp * 1000).toLocaleString() : '—'
}

export function UserMembershipDialog(props: UserMembershipDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [levelId, setLevelId] = useState('')
  const [startsAt, setStartsAt] = useState('')
  const [endsAt, setEndsAt] = useState('')
  const [note, setNote] = useState('')

  const levelsQuery = useQuery({
    queryKey: ['membership-levels'],
    queryFn: () => getMembershipLevels(false),
    enabled: props.open,
  })
  const historyQuery = useQuery({
    queryKey: ['user-memberships', props.user.id],
    queryFn: () => getUserMemberships(props.user.id),
    enabled: props.open,
  })
  const levels = useMemo(
    () =>
      (levelsQuery.data?.data ?? []).filter(
        (level) => level.enabled && !level.is_default
      ),
    [levelsQuery.data]
  )

  const selectedLevelId = levelId || String(levels[0]?.id ?? '')

  const refresh = async () => {
    await queryClient.invalidateQueries({
      queryKey: ['user-memberships', props.user.id],
    })
    props.onSuccess?.()
  }

  const grantMutation = useMutation({
    mutationFn: () =>
      createUserMembership({
        user_id: props.user.id,
        membership_level_id: Number(selectedLevelId),
        starts_at: toUnix(startsAt),
        ends_at: toUnix(endsAt),
        note,
      }),
    onSuccess: async () => {
      toast.success(t('Membership granted'))
      setEndsAt('')
      setNote('')
      await refresh()
    },
    onError: (error) => toast.error(error.message),
  })

  const revokeMutation = useMutation({
    mutationFn: revokeUserMembership,
    onSuccess: async () => {
      toast.success(t('Membership revoked'))
      await refresh()
    },
    onError: (error) => toast.error(error.message),
  })

  const current = historyQuery.data?.data.current
  const grants = historyQuery.data?.data.grants ?? []

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] max-w-4xl overflow-y-auto'>
        <DialogHeader>
          <DialogTitle>{t('Manage Membership')}</DialogTitle>
          <DialogDescription>
            {props.user.username} · {t('Current')}:{' '}
            {current?.display_name || t('Normal user')}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-3 rounded-lg border p-3 md:grid-cols-2'>
          <div className='grid gap-2'>
            <Label>{t('Membership level')}</Label>
            <Select
              items={levels.map((level) => ({
                value: String(level.id),
                label: level.display_name,
              }))}
              value={selectedLevelId}
              onValueChange={(value) => setLevelId(value ?? '')}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Select a level')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {levels.map((level) => (
                    <SelectItem key={level.id} value={String(level.id)}>
                      {level.display_name} · {level.multiplier_ppm / 1_000_000}x
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='membership-note'>{t('Note')}</Label>
            <Input
              id='membership-note'
              value={note}
              maxLength={500}
              onChange={(event) => setNote(event.target.value)}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='membership-start'>{t('Starts at')}</Label>
            <Input
              id='membership-start'
              type='datetime-local'
              value={startsAt}
              onChange={(event) => setStartsAt(event.target.value)}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='membership-end'>{t('Ends at')}</Label>
            <Input
              id='membership-end'
              type='datetime-local'
              value={endsAt}
              onChange={(event) => setEndsAt(event.target.value)}
            />
          </div>
          <div className='md:col-span-2 md:text-right'>
            <Button
              disabled={!selectedLevelId || grantMutation.isPending}
              onClick={() => grantMutation.mutate()}
            >
              {t('Grant membership')}
            </Button>
          </div>
        </div>

        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Level')}</TableHead>
                <TableHead>{t('Period')}</TableHead>
                <TableHead>{t('Source')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {grants.map((grant) => (
                <TableRow key={grant.id}>
                  <TableCell>
                    {grant.level?.display_name || grant.membership_level_id}
                  </TableCell>
                  <TableCell className='text-xs'>
                    {formatTime(grant.starts_at)} —{' '}
                    {grant.ends_at
                      ? formatTime(grant.ends_at)
                      : t('No expiration')}
                  </TableCell>
                  <TableCell>{t(grant.source)}</TableCell>
                  <TableCell>{t(grant.status)}</TableCell>
                  <TableCell className='text-right'>
                    {grant.status === 'active' && (
                      <Button
                        size='sm'
                        variant='destructive'
                        disabled={revokeMutation.isPending}
                        onClick={() => revokeMutation.mutate(grant.id)}
                      >
                        {t('Revoke')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
