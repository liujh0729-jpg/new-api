import { Award } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { UserProfile } from '../types'

interface MembershipCardProps {
  profile: UserProfile | null
  loading: boolean
}

function formatDiscount(multiplierPPM: number): string {
  const value = multiplierPPM / 100_000
  return `${Number(value.toFixed(2))}`
}

export function MembershipCard({ profile, loading }: MembershipCardProps) {
  const { t } = useTranslation()
  const membership = profile?.membership

  return (
    <Card>
      <CardHeader className='pb-3'>
        <CardTitle className='flex items-center gap-2 text-base'>
          <Award className='text-primary size-4' />
          {t('Membership')}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='flex items-end justify-between gap-3'>
          <div>
            <div className='text-lg font-semibold'>
              {loading
                ? t('Loading...')
                : membership?.display_name || t('Normal user')}
            </div>
            <div className='text-muted-foreground font-mono text-xs'>
              {membership?.code || 'NORMAL'}
            </div>
          </div>
          <div className='text-right'>
            <div className='text-primary text-2xl font-bold tabular-nums'>
              {t('{{discount}} discount', {
                discount: formatDiscount(
                  membership?.multiplier_ppm ?? 1_000_000
                ),
              })}
            </div>
            <div className='text-muted-foreground text-xs'>
              {t('Applies after the group ratio')}
            </div>
          </div>
        </div>
        <p className='text-muted-foreground border-t pt-3 text-xs leading-relaxed'>
          {membership?.ends_at
            ? t('Valid until {{date}}', {
                date: new Date(membership.ends_at * 1000).toLocaleString(),
              })
            : t('No expiration date')}
        </p>
      </CardContent>
    </Card>
  )
}
