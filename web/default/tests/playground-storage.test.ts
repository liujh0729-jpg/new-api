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
import { afterEach, describe, expect, test } from 'bun:test'
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '../src/features/playground/constants'
import {
  createPlaygroundConversation,
  getConversationStorageKey,
  loadConversationState,
  saveConversationState,
} from '../src/features/playground/lib/storage'

const originalLocalStorage = Object.getOwnPropertyDescriptor(
  globalThis,
  'localStorage'
)

function installLocalStorage(storage: Partial<Storage>): void {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage,
  })
}

afterEach(() => {
  if (originalLocalStorage) {
    Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
  } else {
    Reflect.deleteProperty(globalThis, 'localStorage')
  }
})

describe('Playground conversation storage', () => {
  test('loads legacy conversations with automatic reasoning mode', () => {
    const { thinking_mode: _thinkingMode, ...legacyConfig } = DEFAULT_CONFIG
    const legacyConversation = {
      ...createPlaygroundConversation(
        DEFAULT_CONFIG,
        DEFAULT_PARAMETER_ENABLED
      ),
      config: legacyConfig,
    }
    const storedState = {
      version: 1,
      activeConversationId: legacyConversation.id,
      conversations: [legacyConversation],
    }
    installLocalStorage({
      getItem: (key) =>
        key === getConversationStorageKey('legacy')
          ? JSON.stringify(storedState)
          : null,
    })

    const loaded = loadConversationState(
      'legacy',
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )

    expect(loaded.conversations[0]?.config.thinking_mode).toBe('auto')
  })

  test('normalizes an unsupported AP Seedance resolution from history', () => {
    const conversation = {
      ...createPlaygroundConversation(
        DEFAULT_CONFIG,
        DEFAULT_PARAMETER_ENABLED
      ),
      config: {
        ...DEFAULT_CONFIG,
        mode: 'video' as const,
        model: 'AP Seedance-2.0 高性价比版',
        video_resolution: '720p',
      },
    }
    const storedState = {
      version: 1,
      activeConversationId: conversation.id,
      conversations: [conversation],
    }
    installLocalStorage({
      getItem: (key) =>
        key === getConversationStorageKey('seedance-history')
          ? JSON.stringify(storedState)
          : null,
    })

    const loaded = loadConversationState(
      'seedance-history',
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )

    expect(loaded.conversations[0]?.config.video_resolution).toBe('1080p')
  })

  test('reports quota failures without throwing', () => {
    installLocalStorage({
      setItem: () => {
        throw new DOMException('quota exceeded', 'QuotaExceededError')
      },
    })
    const conversation = createPlaygroundConversation(
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )

    const saved = saveConversationState('quota-user', {
      version: 1,
      activeConversationId: conversation.id,
      conversations: [conversation],
    })

    expect(saved).toBe(false)
  })
})
