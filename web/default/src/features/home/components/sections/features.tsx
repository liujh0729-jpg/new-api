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
import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useReducedMotion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { AnimateInView } from '@/components/animate-in-view'

interface FeaturesProps {
  className?: string
}

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()

  const capabilities = [
    t('Text, image, video, and audio references'),
    t('Native audio and cinematic camera control'),
    t('Editing and extension in one workflow'),
  ]

  return (
    <section className='home-model-showcase px-4 py-20 text-[#111217] sm:px-6 md:py-28'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mx-auto mb-10 max-w-4xl text-center md:mb-14'>
          <h2 className='text-3xl leading-[1.02] font-semibold tracking-[-0.05em] text-balance md:text-5xl lg:text-6xl'>
            {t('Use the right model for every workload')}
          </h2>
          <p className='mx-auto mt-4 max-w-[60ch] text-sm leading-relaxed text-[#312c38]/75 md:text-base'>
            {t(
              'Move from language to image and video without rebuilding your integration.'
            )}
          </p>
          <div className='mt-7 flex flex-wrap items-center justify-center gap-3'>
            <Button
              className='h-10 rounded-full bg-[#111217] px-5 text-white hover:bg-[#24252b] active:scale-[0.98]'
              render={<Link to='/pricing' />}
            >
              {t('Explore models')}
            </Button>
            <Button
              variant='outline'
              className='h-10 rounded-full border-[#211b27]/20 bg-white/10 px-5 text-[#111217] hover:bg-white/35 active:scale-[0.98]'
              render={<Link to='/docs' />}
            >
              {t('Read docs')}
            </Button>
          </div>
        </AnimateInView>

        <div className='grid grid-cols-1 gap-2.5 lg:grid-cols-12'>
          <AnimateInView className='lg:col-span-5'>
            <article className='flex h-full min-h-[390px] flex-col rounded-2xl bg-[#f4f1f4] p-7 shadow-[0_24px_70px_-45px_rgba(54,12,42,0.35)] md:p-9'>
              <div>
                <p className='text-sm font-medium text-[#5f5563]'>
                  Seedance 2.5
                </p>
                <h3 className='mt-2 max-w-md text-3xl leading-[1.04] font-semibold tracking-[-0.04em] md:text-4xl'>
                  {t('Production video from multimodal references')}
                </h3>
                <p className='mt-4 max-w-[48ch] text-sm leading-relaxed text-[#5f5962]'>
                  {t(
                    'Generate, edit, and extend coherent video through the same task API.'
                  )}
                </p>
              </div>

              <dl className='mt-auto grid grid-cols-3 border-t border-[#17131b]/10 pt-7'>
                <div>
                  <dd className='text-lg font-semibold'>1080p</dd>
                  <dt className='mt-1 text-xs text-[#6a626c]'>{t('Output')}</dt>
                </div>
                <div className='border-l border-[#17131b]/10 pl-5'>
                  <dd className='text-lg font-semibold'>30s</dd>
                  <dt className='mt-1 text-xs text-[#6a626c]'>
                    {t('Duration')}
                  </dt>
                </div>
                <div className='border-l border-[#17131b]/10 pl-5'>
                  <dd className='text-lg font-semibold'>50</dd>
                  <dt className='mt-1 text-xs text-[#6a626c]'>
                    {t('References')}
                  </dt>
                </div>
              </dl>
            </article>
          </AnimateInView>

          <AnimateInView className='lg:col-span-7' delay={80}>
            <article className='relative min-h-[390px] overflow-hidden rounded-2xl bg-[#111217] text-white'>
              <video
                aria-label={t('Cinematic AI video generation sample')}
                src='/seedance-cinematic.mp4'
                poster='/seedance-cinematic.webp'
                autoPlay={!shouldReduceMotion}
                controls={Boolean(shouldReduceMotion)}
                muted
                loop
                playsInline
                preload='metadata'
                className='absolute inset-0 h-full w-full object-cover'
              />
              <div className='home-media-scrim absolute inset-0' />
              <div className='absolute inset-x-0 bottom-0 p-7 md:p-9'>
                <h3 className='max-w-lg text-2xl leading-tight font-semibold tracking-[-0.03em] md:text-3xl'>
                  {t('Keep character, motion, and atmosphere consistent')}
                </h3>
                <p className='mt-3 max-w-[46ch] text-sm leading-relaxed text-white/70'>
                  {t(
                    'One model carries the creative direction across every shot.'
                  )}
                </p>
              </div>
            </article>
          </AnimateInView>

          <AnimateInView className='lg:col-span-12' delay={120}>
            <article className='relative min-h-[430px] overflow-hidden rounded-2xl bg-[#10161c] text-white md:min-h-[520px]'>
              <video
                aria-label={t('Long-form AI video story sample')}
                src='/seedance-story.mp4'
                poster='/seedance-story.webp'
                autoPlay={!shouldReduceMotion}
                controls={Boolean(shouldReduceMotion)}
                muted
                loop
                playsInline
                preload='metadata'
                className='absolute inset-0 h-full w-full object-cover'
              />
              <div className='home-media-scrim home-media-scrim-strong absolute inset-0' />
              <div className='absolute inset-y-0 left-0 flex max-w-xl flex-col justify-center p-7 md:p-12'>
                <h3 className='text-3xl leading-[1.06] font-semibold tracking-[-0.04em] md:text-5xl'>
                  {t('A complete creative workflow')}
                </h3>
                <div className='mt-8 grid gap-4'>
                  {capabilities.map((capability) => (
                    <div
                      key={capability}
                      className='border-l border-white/30 pl-4 text-sm font-medium text-white/85 md:text-base'
                    >
                      {capability}
                    </div>
                  ))}
                </div>
              </div>
            </article>
          </AnimateInView>
        </div>

        <div className='mt-8 flex justify-end'>
          <Link
            to='/pricing'
            className='group inline-flex items-center gap-2 text-sm font-semibold text-[#171218] underline-offset-4 hover:underline'
          >
            {t('Explore models')}
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              size={17}
              strokeWidth={2}
              className='transition-transform group-hover:translate-x-0.5'
            />
          </Link>
        </div>
      </div>
    </section>
  )
}
