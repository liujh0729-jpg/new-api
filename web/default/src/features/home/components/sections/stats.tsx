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

interface StatsProps {
  className?: string
}

const MODEL_NAMES = [
  'ap-deepseek-v4-flash',
  'ap-doubao-seed-2.1-turbo',
  'ap-glm-5.3-flash',
  'ap-gpt-5.6-luna',
  'ap-kimi-k3',
  'ap-qwen3.8-max',
  'ap-agnes-2.5-flash',
  'AP Seedance-2.5 标准版',
  'AP Seedance-2.0 VIP',
  'AP Seedance-2.0 轻量版',
]

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const facts = [
    { value: '40+', label: t('Upstream AI providers') },
    { value: '100+', label: t('Models with unified billing') },
    { value: '1', label: t('Compatible API surface') },
    { value: t('Live'), label: t('Usage and cost visibility') },
  ]

  return (
    <section className='home-stats overflow-hidden border-y border-[var(--home-border)] bg-[var(--home-bg)] text-[var(--home-text)]'>
      <div className='home-model-marquee border-b border-[var(--home-border)] py-4'>
        <div
          className='home-model-marquee-track'
          aria-label={t('Supported AI providers and model families')}
        >
          {[0, 1].map((group) => (
            <div
              key={group}
              className='home-model-marquee-group'
              aria-hidden={group === 1}
            >
              {MODEL_NAMES.map((name) => (
                <span key={name} className='home-model-marquee-item'>
                  {name}
                </span>
              ))}
            </div>
          ))}
        </div>
      </div>

      <dl className='mx-auto grid max-w-7xl grid-cols-2 gap-x-5 gap-y-8 px-4 py-9 sm:px-6 md:grid-cols-4 md:gap-0 md:py-11'>
        {facts.map((fact) => (
          <div
            key={fact.label}
            className='flex flex-col text-left md:border-l md:border-[var(--home-border)] md:px-8 md:first:border-l-0 md:first:pl-0'
          >
            <dd className='text-2xl font-semibold tracking-[-0.04em] md:text-3xl'>
              {fact.value}
            </dd>
            <dt className='mt-1.5 max-w-[21ch] text-xs leading-relaxed text-[var(--home-text-subtle)]'>
              {fact.label}
            </dt>
          </div>
        ))}
      </dl>
    </section>
  )
}
