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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

interface RouteDemo {
  id: string
  labelKey: string
  endpoint: string
  body: string[]
}

const ROUTE_DEMOS: RouteDemo[] = [
  {
    id: 'chat',
    labelKey: 'Chat completions',
    endpoint: '/v1/chat/completions',
    body: [
      '"model": "gpt-5",',
      '"messages": [{',
      '  "role": "user",',
      '  "content": "Plan a product launch"',
      '}],',
      '"stream": true',
    ],
  },
  {
    id: 'video',
    labelKey: 'Video generation',
    endpoint: '/v1/videos',
    body: [
      '"model": "seedance-2.5",',
      '"prompt": "A cinematic city after rain",',
      '"resolution": "1080p",',
      '"duration": 8,',
      '"generate_audio": true',
    ],
  },
]

export function HeroTerminalDemo() {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState(ROUTE_DEMOS[0].id)
  const activeRoute =
    ROUTE_DEMOS.find((route) => route.id === activeId) ?? ROUTE_DEMOS[0]

  return (
    <div className='home-api-surface overflow-hidden rounded-2xl border text-[var(--home-code-text)]'>
      <div
        role='tablist'
        aria-label={t('Compatible API routes')}
        className='flex items-center gap-1 border-b border-[var(--home-border)] px-2 pt-2'
      >
        {ROUTE_DEMOS.map((route) => {
          const isActive = route.id === activeId
          return (
            <button
              key={route.id}
              type='button'
              role='tab'
              aria-selected={isActive}
              onClick={() => setActiveId(route.id)}
              className={cn(
                'min-w-0 rounded-t-lg border-b-2 px-3 py-2.5 text-left text-xs font-medium transition-colors active:translate-y-px',
                isActive
                  ? 'border-[var(--home-accent)] bg-[var(--home-code-tab-active)] text-[var(--home-code-text)]'
                  : 'border-transparent text-[var(--home-code-muted)] hover:bg-[var(--home-code-tab-hover)] hover:text-[var(--home-code-text)]'
              )}
            >
              <span className='truncate'>{t(route.labelKey)}</span>
            </button>
          )
        })}
      </div>

      <div className='grid 2xl:grid-cols-[0.62fr_1.38fr]'>
        <div className='border-b border-[var(--home-border)] px-5 py-5 2xl:border-r 2xl:border-b-0'>
          <p className='mb-2 text-xs font-medium text-[var(--home-code-muted)]'>
            {t('Endpoint')}
          </p>
          <code className='font-mono text-xs leading-relaxed break-all text-[var(--home-code-text)]'>
            POST {activeRoute.endpoint}
          </code>
        </div>

        <div className='min-w-0 px-5 py-5'>
          <p className='mb-2 text-xs font-medium text-[var(--home-code-muted)]'>
            {t('Request body')}
          </p>
          <pre
            key={activeRoute.id}
            className='landing-animate-fade-in overflow-x-auto font-mono text-[11px] leading-5 text-[var(--home-code-muted)] sm:text-xs'
          >
            <code>
              <span className='text-[var(--home-accent)]'>{'{'}</span>
              {'\n'}
              {activeRoute.body.map((line) => (
                <span
                  key={line}
                  className='block pl-4 text-[var(--home-code-line)]'
                >
                  {line}
                </span>
              ))}
              <span className='text-[var(--home-accent)]'>{'}'}</span>
            </code>
          </pre>
        </div>
      </div>
    </div>
  )
}
