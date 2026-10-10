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
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '../src/features/playground/constants'
import { buildChatCompletionPayload } from '../src/features/playground/lib/payload-builder'
import type {
  Message,
  ParameterEnabled,
  PlaygroundConfig,
} from '../src/features/playground/types'

const messages: Message[] = [
  {
    key: 'user-1',
    from: 'user',
    versions: [{ id: 'version-1', content: 'hello' }],
  },
]

function config(overrides: Partial<PlaygroundConfig> = {}): PlaygroundConfig {
  return { ...DEFAULT_CONFIG, model: 'qwythos-9b', ...overrides }
}

function parameters(
  overrides: Partial<ParameterEnabled> = {}
): ParameterEnabled {
  return { ...DEFAULT_PARAMETER_ENABLED, ...overrides }
}

describe('Playground chat completion payload', () => {
  test('omits penalties and think with fresh defaults', () => {
    const payload = buildChatCompletionPayload(
      messages,
      config(),
      parameters()
    )

    expect(payload.frequency_penalty).toBeUndefined()
    expect(payload.presence_penalty).toBeUndefined()
    expect(payload.think).toBeUndefined()
  })

  test('preserves explicitly enabled zero penalties', () => {
    const payload = buildChatCompletionPayload(
      messages,
      config({ frequency_penalty: 0, presence_penalty: 0 }),
      parameters({ frequency_penalty: true, presence_penalty: true })
    )

    expect(payload.frequency_penalty).toBe(0)
    expect(payload.presence_penalty).toBe(0)
  })

  test('maps the reasoning control to an explicit think boolean', () => {
    const enabled = buildChatCompletionPayload(
      messages,
      config({ thinking_mode: 'enabled' }),
      parameters()
    )
    const disabled = buildChatCompletionPayload(
      messages,
      config({ thinking_mode: 'disabled' }),
      parameters()
    )

    expect(enabled.think).toBe(true)
    expect(disabled.think).toBe(false)
  })
})
