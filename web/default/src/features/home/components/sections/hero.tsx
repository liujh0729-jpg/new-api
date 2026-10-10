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

import type { PointerEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRight01Icon, PlayIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  motion,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
} from 'motion/react'
import { useTranslation } from 'react-i18next'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Button } from '@/components/ui/button'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const reveal = {
  hidden: { opacity: 0, y: 24 },
  visible: { opacity: 1, y: 0 },
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()
  const shouldReduceMotion = useReducedMotion()
  const pointerX = useMotionValue(0)
  const pointerY = useMotionValue(0)
  const hoverProgress = useMotionValue(0)
  const spotlightTargetX = useMotionValue(0)
  const spotlightTargetY = useMotionValue(0)
  const trackX = useSpring(useTransform(pointerX, [-1, 1], [34, -34]), {
    stiffness: 92,
    damping: 22,
    mass: 0.82,
  })
  const trackY = useSpring(useTransform(pointerY, [-1, 1], [-9, 9]), {
    stiffness: 92,
    damping: 22,
    mass: 0.82,
  })
  const trackRotateX = useSpring(useTransform(pointerY, [-1, 1], [3.4, -3.4]), {
    stiffness: 92,
    damping: 22,
    mass: 0.82,
  })
  const trackRotateY = useSpring(useTransform(pointerX, [-1, 1], [7.2, -7.2]), {
    stiffness: 92,
    damping: 22,
    mass: 0.82,
  })
  const trackScale = useSpring(
    useTransform(hoverProgress, [0, 1], [1, 1.018]),
    {
      stiffness: 120,
      damping: 24,
      mass: 0.7,
    }
  )
  const focusX = useSpring(useTransform(pointerX, [-1, 1], [14, -14]), {
    stiffness: 132,
    damping: 20,
    mass: 0.58,
  })
  const focusY = useSpring(useTransform(pointerY, [-1, 1], [-4, 4]), {
    stiffness: 132,
    damping: 20,
    mass: 0.58,
  })
  const focusRotateY = useSpring(useTransform(pointerX, [-1, 1], [2.6, -2.6]), {
    stiffness: 132,
    damping: 20,
    mass: 0.58,
  })
  const focusScale = useSpring(
    useTransform(hoverProgress, [0, 1], [1, 1.038]),
    {
      stiffness: 138,
      damping: 21,
      mass: 0.58,
    }
  )
  const lightX = useSpring(useTransform(pointerX, [-1, 1], [110, -110]), {
    stiffness: 78,
    damping: 25,
  })
  const lightY = useSpring(useTransform(pointerY, [-1, 1], [34, -34]), {
    stiffness: 78,
    damping: 25,
  })
  const spotlightX = useSpring(spotlightTargetX, {
    stiffness: 185,
    damping: 25,
    mass: 0.42,
  })
  const spotlightY = useSpring(spotlightTargetY, {
    stiffness: 185,
    damping: 25,
    mass: 0.42,
  })
  const spotlightOpacity = useSpring(
    useTransform(hoverProgress, [0, 1], [0, 0.94]),
    {
      stiffness: 170,
      damping: 24,
      mass: 0.42,
    }
  )
  const spotlightScale = useSpring(
    useTransform(hoverProgress, [0, 1], [0.72, 1]),
    {
      stiffness: 170,
      damping: 24,
      mass: 0.42,
    }
  )
  const primaryTarget = props.isAuthenticated ? '/dashboard' : '/sign-up'
  const primaryLabel = props.isAuthenticated
    ? t('Open console')
    : t('Start building')

  const updateHeroDepth = (event: PointerEvent<HTMLElement>) => {
    if (shouldReduceMotion || event.pointerType === 'touch') return
    const bounds = event.currentTarget.getBoundingClientRect()
    const localX = event.clientX - bounds.left
    const localY = event.clientY - bounds.top
    hoverProgress.set(1)
    spotlightTargetX.set(localX)
    spotlightTargetY.set(localY)
    pointerX.set((localX / bounds.width - 0.5) * 2)
    pointerY.set((localY / bounds.height - 0.5) * 2)
  }

  const resetHeroDepth = () => {
    hoverProgress.set(0)
    pointerX.set(0)
    pointerY.set(0)
  }

  return (
    <section className='home-hero-stage relative flex min-h-[100dvh] flex-col overflow-hidden px-4 pt-20 text-[var(--home-text)] sm:px-6 lg:pt-24'>
      <motion.div
        initial={shouldReduceMotion ? false : 'hidden'}
        animate='visible'
        transition={{ staggerChildren: shouldReduceMotion ? 0 : 0.08 }}
        className='relative mx-auto flex w-full max-w-5xl flex-col items-center text-center'
      >
        <motion.p
          variants={reveal}
          transition={{ duration: 0.55, ease: [0.16, 1, 0.3, 1] }}
          className='mb-4 text-xs font-semibold tracking-[0.14em] text-[var(--home-accent)] uppercase'
        >
          {t('Seedance 2.5 is now available')}
        </motion.p>
        <motion.h1
          variants={reveal}
          transition={{ duration: 0.65, ease: [0.16, 1, 0.3, 1] }}
          className='home-hero-title max-w-4xl leading-[0.98] font-semibold tracking-[-0.06em] text-balance'
        >
          <span>{t('Now,')}</span>
          <span className='home-hero-title-rest inline-block'>
            {t('connect to the world you imagine.')}
          </span>
        </motion.h1>
        <motion.p
          variants={reveal}
          transition={{ duration: 0.65, ease: [0.16, 1, 0.3, 1] }}
          className='mt-5 max-w-[56ch] text-sm leading-relaxed text-[var(--home-text-muted)] sm:text-base'
        >
          {t(
            'Route, monitor, and bill every model through one production-ready API on {{systemName}}.',
            { systemName }
          )}
        </motion.p>
        <motion.div
          variants={reveal}
          transition={{ duration: 0.65, ease: [0.16, 1, 0.3, 1] }}
          className='mt-7 flex flex-wrap items-center justify-center gap-3'
        >
          <Button
            size='lg'
            className='group/button h-11 rounded-full bg-[var(--home-primary-button-bg)] px-5 text-[var(--home-primary-button-fg)] hover:bg-[var(--home-primary-button-hover)] active:scale-[0.98]'
            render={<Link to={primaryTarget} />}
          >
            {primaryLabel}
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              className='transition-transform group-hover/button:translate-x-0.5'
              strokeWidth={2}
            />
          </Button>
          <Button
            size='lg'
            variant='outline'
            className='h-11 rounded-full border-[var(--home-border-strong)] bg-[var(--home-secondary-button)] px-5 text-[var(--home-text)] hover:bg-[var(--home-secondary-button-hover)] active:scale-[0.98]'
            render={<Link to='/pricing' />}
          >
            {t('Explore models')}
            <HugeiconsIcon icon={PlayIcon} strokeWidth={1.8} />
          </Button>
        </motion.div>
      </motion.div>

      <motion.figure
        initial={
          shouldReduceMotion ? false : { opacity: 0, y: 24, scale: 0.97 }
        }
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{
          duration: 1,
          delay: shouldReduceMotion ? 0 : 0.18,
          ease: [0.16, 1, 0.3, 1],
        }}
        className='home-gateway-visual relative mx-auto mt-8 w-full max-w-7xl flex-1 sm:mt-9'
        onPointerEnter={updateHeroDepth}
        onPointerMove={updateHeroDepth}
        onPointerLeave={resetHeroDepth}
      >
        <motion.div
          aria-hidden='true'
          className='home-gateway-depth-light absolute inset-x-[12%] bottom-[4%] h-[58%] rounded-[50%]'
          style={shouldReduceMotion ? undefined : { x: lightX, y: lightY }}
        />
        <motion.div
          className='home-gateway-motion absolute inset-x-0 bottom-0 mx-auto h-full max-h-[54dvh] w-full'
          style={
            shouldReduceMotion
              ? undefined
              : {
                  x: trackX,
                  y: trackY,
                  rotateX: trackRotateX,
                  rotateY: trackRotateY,
                  scale: trackScale,
                }
          }
        >
          <img
            src='/home-ai-gateway.webp'
            alt={t('AI models converging through a unified API gateway')}
            width={1672}
            height={941}
            fetchPriority='high'
            draggable={false}
            className='h-full w-full object-contain object-bottom'
          />
        </motion.div>
        <motion.div
          aria-hidden='true'
          className='home-gateway-focus absolute inset-x-0 bottom-0 mx-auto h-full max-h-[54dvh] w-full'
          style={
            shouldReduceMotion
              ? undefined
              : {
                  x: focusX,
                  y: focusY,
                  rotateY: focusRotateY,
                  scale: focusScale,
                }
          }
        >
          <img
            src='/home-ai-gateway.webp'
            alt=''
            width={1672}
            height={941}
            draggable={false}
            className='home-gateway-focus-image h-full w-full object-contain object-bottom'
          />
        </motion.div>
        <motion.div
          aria-hidden='true'
          className='home-gateway-spotlight-anchor absolute top-0 left-0'
          style={
            shouldReduceMotion
              ? { opacity: 0 }
              : {
                  x: spotlightX,
                  y: spotlightY,
                  opacity: spotlightOpacity,
                  scale: spotlightScale,
                }
          }
        >
          <div className='home-gateway-spotlight' />
        </motion.div>
      </motion.figure>
    </section>
  )
}
