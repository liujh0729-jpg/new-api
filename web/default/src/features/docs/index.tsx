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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { PublicLayout } from '@/components/layout'

const PUBLIC_DOCS_URL =
  'https://s.apifox.cn/fea0b520-e6d9-489c-ae5e-109391c771dd/9376375m0'

export function Docs() {
  const { t } = useTranslation()

  useEffect(() => {
    window.location.replace(PUBLIC_DOCS_URL)
  }, [])

  return (
    <PublicLayout>
      <main className='flex min-h-[50vh] items-center justify-center px-4 py-16'>
        <a
          className='text-primary font-medium underline underline-offset-4'
          href={PUBLIC_DOCS_URL}
          rel='noopener noreferrer'
        >
          {t('Read docs')}
        </a>
      </main>
    </PublicLayout>
  )
}
