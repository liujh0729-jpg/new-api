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
import { describe, expect, test } from 'bun:test'
import { normalizePlaygroundError } from '../src/features/playground/lib/error-utils'

const MODEL_NOT_OPEN_MESSAGE =
  'The upstream account has not activated the selected model. Ask an administrator to enable it in the provider console, or switch to another available model.'
const translate = (key: string) => `translated:${key}`

describe('normalizePlaygroundError', () => {
  test('extracts ModelNotOpen from a wrapped task error', () => {
    const error = {
      response: {
        data: {
          code: 'fail_to_fetch_task',
          message:
            '{"error":{"code":"ModelNotOpen","message":"任务处理失败，请稍后重试","param":"","type":"Not Found"}}',
        },
      },
    }

    expect(normalizePlaygroundError(error, translate)).toEqual({
      code: 'ModelNotOpen',
      message: `translated:${MODEL_NOT_OPEN_MESSAGE}`,
    })
  })

  test('recognizes a provider model activation message without an error code', () => {
    const error = {
      message: 'The current account has not activated the model service.',
    }

    expect(normalizePlaygroundError(error, translate)).toEqual({
      code: undefined,
      message: `translated:${MODEL_NOT_OPEN_MESSAGE}`,
    })
  })

  test('keeps unrelated upstream errors unchanged', () => {
    const error = { code: 'InternalError', message: 'Upstream unavailable' }

    expect(normalizePlaygroundError(error, translate)).toEqual({
      code: 'InternalError',
      message: 'Upstream unavailable',
    })
  })
})
