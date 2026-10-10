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
import { Link } from '@tanstack/react-router'
import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { AnimateInView } from '@/components/animate-in-view'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  if (props.isAuthenticated) return null

  return (
    <section className='bg-[var(--home-bg)] px-4 pt-6 pb-20 sm:px-6 md:pb-28'>
      <AnimateInView
        animation='scale-in'
        className='relative mx-auto flex min-h-[520px] max-w-7xl items-center justify-center overflow-hidden rounded-2xl text-center text-[#f6f6f2]'
      >
        <img
          src='/home-ai-gateway.webp'
          alt=''
          aria-hidden='true'
          width={1672}
          height={941}
          loading='lazy'
          className='absolute inset-0 h-full w-full object-cover'
        />
        <div className='absolute inset-0 bg-[#08090b]/72' />
        <div className='home-cta-vignette absolute inset-0' />

        <div className='relative mx-auto max-w-3xl px-6 py-16 md:px-10'>
          <h2 className='text-3xl leading-[1.03] font-semibold tracking-[-0.05em] text-balance md:text-5xl'>
            {t('Build on every model with one integration')}
          </h2>
          <p className='mx-auto mt-4 max-w-[54ch] text-sm leading-relaxed text-[#b2b4bc] md:text-base'>
            {t(
              'Start with a compatible API, transparent usage, and production controls from day one.'
            )}
          </p>
          <div className='mt-8 flex flex-wrap justify-center gap-3'>
            <Button
              size='lg'
              className='group/button h-11 rounded-full bg-[#f6f6f2] px-5 text-[#101114] hover:bg-white active:scale-[0.98]'
              render={<Link to='/sign-up' />}
            >
              {t('Start building')}
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                className='transition-transform group-hover/button:translate-x-0.5'
                strokeWidth={2}
              />
            </Button>
            <Button
              size='lg'
              variant='outline'
              className='h-11 rounded-full border-white/20 bg-white/5 px-5 text-[#f6f6f2] hover:border-white/35 hover:bg-white/10 hover:text-white active:scale-[0.98]'
              render={<Link to='/docs' />}
            >
              {t('Read docs')}
            </Button>
          </div>
        </div>
      </AnimateInView>
    </section>
  )
}
