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
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'

export function ToggleField({
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

export function CharacterGridSkeleton() {
  return (
    <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6'>
      {[0, 1, 2, 3, 4, 5].map((item) => (
        <Card key={item}>
          <Skeleton className='aspect-[3/4] w-full' />
          <CardContent className='flex flex-col gap-1.5 px-2.5 pt-2.5 pb-2.5'>
            <Skeleton className='h-3.5 w-1/2' />
            <Skeleton className='h-3 w-full' />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

export function Pagination({
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
