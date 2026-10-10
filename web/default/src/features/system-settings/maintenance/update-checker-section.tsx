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
import { useState } from 'react'
import { ExternalLinkIcon, RefreshCcwIcon, AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { SettingsSection } from '../components/settings-section'

type GitCommit = {
  sha: string
  author: string
  date: string
  message: string
}

type GitStatus = {
  is_repo: boolean
  branch?: string
  remote?: string
  remote_url?: string
  local_head?: string
  remote_head?: string
  commits_behind?: number
  commits_ahead?: number
  dirty?: boolean
  latest_local?: GitCommit
  latest_remote?: GitCommit
  incoming?: GitCommit[]
  app_version?: string
  fetched_at?: number
}

type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

type ApplyResult = {
  output?: string
  new_head?: string
  branch?: string
  remote?: string
  restart_required?: boolean
}

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

export function UpdateCheckerSection({
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [applying, setApplying] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [status, setStatus] = useState<GitStatus | null>(null)

  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const version = currentVersion || t('Unknown')

  const handleCheckUpdates = async () => {
    setChecking(true)
    try {
      const res = await api.get<ApiResponse<GitStatus>>(
        '/api/option/update_check',
        { params: { t: Date.now(), force: 1 } }
      )
      const payload = res.data
      if (!payload?.success || !payload.data) {
        toast.error(payload?.message || t('Failed to check for updates'))
        return
      }
      const data = payload.data
      setStatus(data)
      if (!data.is_repo) {
        toast.error(data ? t('Current deployment is not a git repository') : t('Failed to check for updates'))
        return
      }
      const behind = data.commits_behind ?? 0
      if (behind === 0 && !data.dirty) {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: data.remote_head?.slice(0, 7) || version,
          })
        )
        return
      }
      setDialogOpen(true)
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to check for updates')
      toast.error(message)
    } finally {
      setChecking(false)
    }
  }

  const handleApplyUpdate = async () => {
    if (!status) return
    setApplying(true)
    try {
      const res = await api.post<ApiResponse<ApplyResult>>(
        '/api/option/update_apply',
        {}
      )
      const payload = res.data
      if (!payload?.success) {
        toast.error(payload?.message || t('Update failed'))
        return
      }
      toast.success(
        payload?.message ||
          t('Code updated. Rebuild and restart the service to apply.')
      )
      setDialogOpen(false)
      setStatus(null)
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Update failed')
      toast.error(message)
    } finally {
      setApplying(false)
    }
  }

  const openRemote = () => {
    if (status?.remote_url) {
      window.open(status.remote_url, '_blank', 'noopener,noreferrer')
    }
  }

  const behind = status?.commits_behind ?? 0
  const ahead = status?.commits_ahead ?? 0
  const incoming = status?.incoming ?? []

  return (
    <>
      <SettingsSection
        title={t('System maintenance')}
        description={t(
          'Compare local code with the GitHub repository and pull updates.'
        )}
      >
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Current version')}
              </div>
              <div className='text-lg font-semibold'>{version}</div>
            </div>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Uptime since')}
              </div>
              <div className='text-lg font-semibold'>{uptime}</div>
            </div>
          </div>

          {status?.is_repo && (
            <div className='text-muted-foreground space-y-1 text-sm'>
              <div>
                {t('Branch')}: <span className='font-mono'>{status.branch}</span>
                {' · '}
                {t('Remote')}: <span className='font-mono'>{status.remote}</span>
              </div>
              <div>
                {t('Behind')}:{' '}
                <span className='font-mono'>{behind}</span>, {t('Ahead')}:{' '}
                <span className='font-mono'>{ahead}</span>
                {status.dirty ? ` · ${t('Working tree has changes')}` : ''}
              </div>
            </div>
          )}

          <div className='flex flex-wrap items-center gap-3'>
            <Button onClick={handleCheckUpdates} disabled={checking}>
              {checking ? (
                t('Checking updates...')
              ) : (
                <>
                  <RefreshCcwIcon className='me-2 h-4 w-4' />
                  {t('Check for updates')}
                </>
              )}
            </Button>
            {status?.remote_url && (
              <Button variant='link' onClick={openRemote}>
                <ExternalLinkIcon className='me-1 h-4 w-4' />
                {status.remote_url}
              </Button>
            )}
          </div>
        </div>
      </SettingsSection>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='max-h-[80vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>
              {behind > 0
                ? t('{{count}} new commits on GitHub', { count: behind })
                : t('Repository status')}
            </DialogTitle>
            <DialogDescription>
              {t('Branch')} {status?.branch} · {t('Remote')} {status?.remote}
              {status?.fetched_at
                ? ` · ${formatTimestampToDate(status.fetched_at, 'seconds')}`
                : ''}
            </DialogDescription>
          </DialogHeader>

          <div className='space-y-4'>
            {status?.dirty && (
              <div className='border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-200 flex items-start gap-2 rounded-lg border p-3 text-sm'>
                <AlertTriangle className='mt-0.5 size-4 shrink-0' />
                {t(
                  'Local working tree has uncommitted changes; update is blocked to protect them.'
                )}
              </div>
            )}

            {ahead > 0 && (
              <div className='text-muted-foreground text-sm'>
                {t(
                  'Local branch has {{count}} commits not pushed to the remote.',
                  { count: ahead }
                )}
              </div>
            )}

            {behind === 0 && !status?.dirty && (
              <p className='text-muted-foreground text-sm'>
                {t('You are up to date with the remote branch.')}
              </p>
            )}

            {incoming.length > 0 && (
              <div className='space-y-2'>
                <div className='text-muted-foreground text-xs font-medium uppercase tracking-wide'>
                  {t('Incoming commits')}
                </div>
                <ul className='divide-y rounded-lg border'>
                  {incoming.map((c) => (
                    <li key={c.sha} className='p-3 text-sm'>
                      <div className='flex items-start justify-between gap-3'>
                        <div className='min-w-0 flex-1'>
                          <div className='truncate font-medium'>
                            {c.message || '(no message)'}
                          </div>
                          <div className='text-muted-foreground mt-1 flex flex-wrap gap-2 text-xs'>
                            <span className='font-mono'>
                              {c.sha.slice(0, 7)}
                            </span>
                            <span>{c.author}</span>
                            {c.date && (
                              <span>
                                {formatTimestampToDate(
                                  new Date(c.date).getTime(),
                                  'milliseconds'
                                )}
                              </span>
                            )}
                          </div>
                        </div>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setDialogOpen(false)}
            >
              {t('Close')}
            </Button>
            <Button
              type='button'
              onClick={handleApplyUpdate}
              disabled={applying || behind === 0 || status?.dirty}
            >
              {applying
                ? t('Updating...')
                : behind > 0
                  ? t('Pull latest from GitHub')
                  : t('Nothing to pull')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
