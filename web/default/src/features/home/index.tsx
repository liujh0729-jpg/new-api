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
import { useAuthStore } from '@/stores/auth-store'
import { Markdown } from '@/components/ui/markdown'
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { CTA, Features, Hero, HowItWorks, Stats } from './components'
import './home.css'
import { useHomePageContent } from './hooks'

function HomeSkeleton() {
  return (
    <main className='home-landing min-h-[100dvh] bg-[var(--home-bg)] px-6 pt-24 text-[var(--home-text)]'>
      <div className='mx-auto flex max-w-5xl flex-col items-center py-12 text-center'>
        <Skeleton className='h-4 w-44 rounded-full bg-[var(--home-skeleton)]' />
        <Skeleton className='mt-5 h-16 w-full max-w-3xl rounded-xl bg-[var(--home-skeleton)]' />
        <Skeleton className='mt-4 h-5 w-full max-w-xl rounded-lg bg-[var(--home-skeleton)]' />
        <div className='mt-7 flex gap-3'>
          <Skeleton className='h-11 w-32 rounded-full bg-[var(--home-skeleton)]' />
          <Skeleton className='h-11 w-32 rounded-full bg-[var(--home-skeleton)]' />
        </div>
        <Skeleton className='mt-10 aspect-[16/8] w-full rounded-2xl bg-[var(--home-skeleton)]' />
      </div>
    </main>
  )
}

export function Home() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user
  const { content, isLoaded, isUrl } = useHomePageContent()

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <HomeSkeleton />
      </PublicLayout>
    )
  }

  if (content) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='overflow-x-hidden'>
          {isUrl ? (
            <iframe
              src={content}
              className='min-h-[calc(100dvh-4rem)] w-full border-none pt-16'
              title={t('Custom Home Page')}
            />
          ) : (
            <div className='container mx-auto py-8'>
              <Markdown className='custom-home-content'>{content}</Markdown>
            </div>
          )}
        </main>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main className='home-landing overflow-x-clip'>
        <Hero isAuthenticated={isAuthenticated} />
        <Stats />
        <Features />
        <HowItWorks />
        <CTA isAuthenticated={isAuthenticated} />
      </main>
      <Footer className='home-footer' />
    </PublicLayout>
  )
}
