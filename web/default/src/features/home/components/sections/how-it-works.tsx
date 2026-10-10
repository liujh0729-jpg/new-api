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
'use client'

import { Link } from '@tanstack/react-router'
import {
  Analytics01Icon,
  ApiGatewayIcon,
  ArrowRight01Icon,
  DashboardBrowsingIcon,
  Route01Icon,
  Wallet02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useReducedMotion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'
import { HeroTerminalDemo } from '../hero-terminal-demo'

export function HowItWorks() {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()

  const operations = [
    {
      title: t('Smart routing'),
      description: t('Balance providers, regions, and fallback channels.'),
      icon: Route01Icon,
    },
    {
      title: t('Usage analytics'),
      description: t('See tokens, latency, and spend in one place.'),
      icon: Analytics01Icon,
    },
    {
      title: t('Unified billing'),
      description: t('Manage model pricing and user quota centrally.'),
      icon: Wallet02Icon,
    },
  ]

  return (
    <section className='bg-[var(--home-bg)] px-4 py-20 text-[var(--home-text)] sm:px-6 md:py-28'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mx-auto mb-12 max-w-4xl text-center md:mb-16'>
          <h2 className='text-3xl leading-[1.03] font-semibold tracking-[-0.05em] text-balance md:text-5xl'>
            {t('Native AI operations, ready to scale')}
          </h2>
          <p className='mx-auto mt-4 max-w-[60ch] text-sm leading-relaxed text-[var(--home-text-muted)] md:text-base'>
            {t(
              'Connect models once, then control routing, access, usage, and cost from the same platform.'
            )}
          </p>
        </AnimateInView>

        <div className='grid grid-cols-1 gap-2.5 lg:grid-cols-12'>
          <AnimateInView className='lg:col-span-7'>
            <article className='home-surface-card flex h-full min-h-[520px] flex-col rounded-2xl p-6 md:p-8'>
              <div className='mb-8 flex items-start gap-4'>
                <div className='flex size-11 shrink-0 items-center justify-center rounded-xl bg-[var(--home-icon-bg)] text-[var(--home-accent)]'>
                  <HugeiconsIcon icon={ApiGatewayIcon} strokeWidth={1.7} />
                </div>
                <div>
                  <h3 className='text-2xl font-semibold tracking-[-0.03em]'>
                    {t('One API surface')}
                  </h3>
                  <p className='mt-2 max-w-[50ch] text-sm leading-relaxed text-[var(--home-text-muted)]'>
                    {t(
                      'Keep familiar request formats while changing providers behind the route.'
                    )}
                  </p>
                </div>
              </div>
              <div className='mt-auto'>
                <HeroTerminalDemo />
              </div>
            </article>
          </AnimateInView>

          <AnimateInView className='lg:col-span-5' delay={70}>
            <article className='group relative min-h-[520px] overflow-hidden rounded-2xl bg-[#17181c]'>
              <video
                aria-label={t(
                  'AI media routed through a unified model workflow'
                )}
                src='/seedance-style.mp4'
                poster='/seedance-style.webp'
                autoPlay={!shouldReduceMotion}
                controls={Boolean(shouldReduceMotion)}
                muted
                loop
                playsInline
                preload='metadata'
                className='absolute inset-0 h-full w-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:scale-[1.025]'
              />
              <div className='home-media-scrim absolute inset-0' />
              <div className='absolute inset-x-0 bottom-0 p-7 md:p-9'>
                <h3 className='text-2xl font-semibold tracking-[-0.03em] md:text-3xl'>
                  {t('Change providers without changing your product')}
                </h3>
                <p className='mt-3 max-w-[42ch] text-sm leading-relaxed text-white/70'>
                  {t(
                    'Fallback rules keep traffic moving when an upstream is busy.'
                  )}
                </p>
              </div>
            </article>
          </AnimateInView>

          <AnimateInView className='h-full lg:col-span-5' delay={100}>
            <article className='flex h-full min-h-[430px] flex-col rounded-2xl bg-[var(--home-accent)] p-7 text-[#1a1015] md:p-9'>
              <div className='flex size-11 items-center justify-center rounded-xl bg-[#1a1015]/10'>
                <HugeiconsIcon icon={Wallet02Icon} strokeWidth={1.7} />
              </div>
              <h3 className='mt-7 text-3xl leading-[1.04] font-semibold tracking-[-0.04em]'>
                {t('Billing that follows the model')}
              </h3>
              <p className='mt-3 max-w-[42ch] text-sm leading-relaxed text-[#2d1a22]/75'>
                {t(
                  'Price every model accurately, enforce quota, and settle usage from one ledger.'
                )}
              </p>
              <div className='mt-auto grid gap-4 pt-9'>
                {operations.map((operation) => (
                  <div
                    key={operation.title}
                    className='grid grid-cols-[auto_1fr] items-start gap-3 border-t border-[#1a1015]/15 pt-4'
                  >
                    <HugeiconsIcon
                      icon={operation.icon}
                      size={19}
                      strokeWidth={1.7}
                    />
                    <div>
                      <p className='text-sm font-semibold'>{operation.title}</p>
                      <p className='mt-1 text-xs leading-relaxed text-[#2d1a22]/70'>
                        {operation.description}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </article>
          </AnimateInView>

          <AnimateInView className='h-full lg:col-span-7' delay={130}>
            <article className='home-console-card flex h-full min-h-[430px] flex-col rounded-2xl p-7 md:p-9'>
              <div className='flex size-11 items-center justify-center rounded-xl bg-[var(--home-icon-bg)] text-[var(--home-accent)]'>
                <HugeiconsIcon icon={DashboardBrowsingIcon} strokeWidth={1.7} />
              </div>
              <h3 className='mt-7 max-w-xl text-3xl leading-[1.04] font-semibold tracking-[-0.04em] md:text-4xl'>
                {t('See every request clearly')}
              </h3>
              <p className='mt-3 max-w-[54ch] text-sm leading-relaxed text-[var(--home-text-muted)]'>
                {t(
                  'Trace latency, errors, tokens, and cost without stitching together separate tools.'
                )}
              </p>
              <div className='mt-auto flex justify-end pt-10'>
                <Link
                  to='/dashboard'
                  className='group inline-flex items-center gap-2 text-sm font-semibold text-[var(--home-text)] underline-offset-4 hover:underline'
                >
                  {t('Open console')}
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    size={17}
                    strokeWidth={2}
                    className='transition-transform group-hover:translate-x-0.5'
                  />
                </Link>
              </div>
            </article>
          </AnimateInView>
        </div>
      </div>
    </section>
  )
}
