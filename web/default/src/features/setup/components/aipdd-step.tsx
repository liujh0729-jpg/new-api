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
import type { UseFormReturn } from 'react-hook-form'
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { PasswordInput } from '@/components/password-input'
import type { SetupFormValues } from '../types'

interface AipddStepProps {
  form: UseFormReturn<SetupFormValues>
}

export function AipddStep({ form }: AipddStepProps) {
  const { t } = useTranslation()

  return (
    <div className='grid gap-4'>
      <Alert>
        <AlertDescription className='flex items-start gap-2'>
          <KeyRound className='mt-0.5 size-4 text-primary' />
          {t(
            'Provide your AIPDD API key to automatically configure models and channels. This key will be saved to the system.'
          )}
        </AlertDescription>
      </Alert>

      <FormField
        control={form.control}
        name='aipddKey'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('AIPDD API Key')}</FormLabel>
            <FormControl>
              <PasswordInput
                {...field}
                placeholder={t('Enter your AIPDD API key')}
                autoComplete='off'
                onChange={(event) => {
                  form.clearErrors('aipddKey')
                  field.onChange(event)
                }}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
