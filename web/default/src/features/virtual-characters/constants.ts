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
export const VIRTUAL_CHARACTER_NATIONALITIES = [
  '中国',
  '美国',
  '日本',
  '韩国',
  '英国',
  '法国',
  '印度',
  '巴西',
] as const

export const VIRTUAL_CHARACTER_GENDERS = ['男', '女'] as const

export const VIRTUAL_CHARACTER_AGE_BANDS = [
  { value: '0-20', labelKey: '0-20 years' },
  { value: '20-40', labelKey: '20-40 years' },
  { value: '40-60', labelKey: '40-60 years' },
  { value: '60-80', labelKey: '60-80 years' },
  { value: '80-100', labelKey: '80-100 years' },
] as const

export const VIRTUAL_CHARACTER_NATIONALITY_LABEL_KEYS: Record<
  (typeof VIRTUAL_CHARACTER_NATIONALITIES)[number],
  string
> = {
  中国: 'China',
  美国: 'United States',
  日本: 'Japan',
  韩国: 'South Korea',
  英国: 'United Kingdom',
  法国: 'France',
  印度: 'India',
  巴西: 'Brazil',
}

export const VIRTUAL_CHARACTER_GENDER_LABEL_KEYS: Record<
  (typeof VIRTUAL_CHARACTER_GENDERS)[number],
  string
> = {
  男: 'Male',
  女: 'Female',
}
