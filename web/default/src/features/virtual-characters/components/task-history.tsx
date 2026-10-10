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
import { Clock01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { VirtualCharacterTask } from '../types'
import { taskStatusLabel } from './utils'

export function TaskHistory({
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
              'Tasks created from official or uploaded characters will appear here.'
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
                  <CardDescription>{item.task_id}</CardDescription>
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
              {(item.references?.length ?? 0) > 1 && (
                <div className='flex flex-wrap gap-2'>
                  {item.references?.map((reference) => (
                    <Badge
                      key={`${item.task_id}-${reference.character_id}`}
                      variant='outline'
                      title={`asset://${reference.provider_asset_id}`}
                    >
                      {reference.character_name}
                    </Badge>
                  ))}
                </div>
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
