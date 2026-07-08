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
import { nanoid } from 'nanoid'
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
  DEFAULT_VIDEO_DURATION,
  STORAGE_KEYS,
  normalizeLTXVideoSizeForModel,
  normalizeVideoDurationForModel,
  normalizeVideoResolutionForModel,
} from '../constants'
import type {
  PlaygroundConfig,
  ParameterEnabled,
  Message,
  PlaygroundConversation,
  PlaygroundConversationState,
  PlaygroundMode,
} from '../types'
import { sanitizeMessagesOnLoad } from './message-utils'

const CONVERSATION_STATE_VERSION = 1 as const
const CONVERSATION_TITLE_MAX_LENGTH = 60
export const PLAYGROUND_CONVERSATION_STATE_EVENT =
  'playground:conversation-state'

export interface PlaygroundConversationStateEventDetail {
  storageKey: string
  state: PlaygroundConversationState
}

interface LoadConversationStateOptions {
  sanitizeMessages?: boolean
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function getConversationStorageKey(
  userId?: number | string | null
): string {
  const suffix = userId === undefined || userId === null ? 'anonymous' : userId
  return `${STORAGE_KEYS.CONVERSATIONS_PREFIX}_user_${suffix}`
}

function truncateTitle(title: string): string {
  if (title.length <= CONVERSATION_TITLE_MAX_LENGTH) return title
  return `${title.slice(0, CONVERSATION_TITLE_MAX_LENGTH - 3).trimEnd()}...`
}

export function getConversationTitle(
  messages: Message[],
  mode: PlaygroundMode
): string {
  const firstUserText =
    messages
      .find((message) => message.from === 'user')
      ?.versions?.[0]?.content?.replace(/\s+/g, ' ')
      .trim() || ''

  if (firstUserText) return truncateTitle(firstUserText)
  if (mode === 'image') return 'Image conversation'
  if (mode === 'video') return 'Video conversation'
  return 'New conversation'
}

export function createPlaygroundConversation(
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled,
  messages: Message[] = []
): PlaygroundConversation {
  const now = Date.now()
  return {
    id: nanoid(),
    title: getConversationTitle(messages, config.mode),
    config,
    parameterEnabled,
    messages,
    createdAt: now,
    updatedAt: now,
  }
}

function createDefaultConversationState(
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled
): PlaygroundConversationState {
  const conversation = createPlaygroundConversation(config, parameterEnabled)
  return {
    version: CONVERSATION_STATE_VERSION,
    activeConversationId: conversation.id,
    conversations: [conversation],
  }
}

function normalizeConversation(
  value: unknown,
  configFallback: PlaygroundConfig,
  parameterFallback: ParameterEnabled,
  options: LoadConversationStateOptions
): PlaygroundConversation | null {
  if (!isObject(value)) return null

  const id = typeof value.id === 'string' && value.id ? value.id : nanoid()
  const config = (
    isObject(value.config)
      ? { ...DEFAULT_CONFIG, ...configFallback, ...value.config }
      : configFallback
  ) as PlaygroundConfig
  config.video_resolution = normalizeVideoResolutionForModel(
    config.model || '',
    config.video_resolution
  )
  config.video_size = normalizeLTXVideoSizeForModel(
    config.model || '',
    config.video_size
  )
  const parameterEnabled = (
    isObject(value.parameterEnabled)
      ? {
          ...DEFAULT_PARAMETER_ENABLED,
          ...parameterFallback,
          ...value.parameterEnabled,
        }
      : parameterFallback
  ) as ParameterEnabled
  const rawMessages = Array.isArray(value.messages)
    ? (value.messages as Message[])
    : []
  const messages =
    options.sanitizeMessages === false
      ? rawMessages
      : sanitizeMessagesOnLoad(rawMessages)
  const createdAt =
    typeof value.createdAt === 'number' && Number.isFinite(value.createdAt)
      ? value.createdAt
      : Date.now()
  const updatedAt =
    typeof value.updatedAt === 'number' && Number.isFinite(value.updatedAt)
      ? value.updatedAt
      : createdAt
  const title =
    typeof value.title === 'string' && value.title.trim()
      ? value.title.trim()
      : getConversationTitle(messages, config.mode)

  return {
    id,
    title,
    config,
    parameterEnabled,
    messages,
    createdAt,
    updatedAt,
  }
}

function normalizeConversationState(
  value: unknown,
  configFallback: PlaygroundConfig,
  parameterFallback: ParameterEnabled,
  options: LoadConversationStateOptions
): PlaygroundConversationState | null {
  if (!isObject(value) || !Array.isArray(value.conversations)) return null

  const conversations = value.conversations
    .map((conversation) =>
      normalizeConversation(
        conversation,
        configFallback,
        parameterFallback,
        options
      )
    )
    .filter(
      (conversation): conversation is PlaygroundConversation =>
        conversation !== null
    )

  if (conversations.length === 0) return null

  const activeConversationId =
    typeof value.activeConversationId === 'string' &&
    conversations.some(
      (conversation) => conversation.id === value.activeConversationId
    )
      ? value.activeConversationId
      : [...conversations].sort((a, b) => b.updatedAt - a.updatedAt)[0].id

  return {
    version: CONVERSATION_STATE_VERSION,
    activeConversationId,
    conversations,
  }
}

/**
 * Load playground config from localStorage
 */
export function loadConfig(): Partial<PlaygroundConfig> {
  try {
    const saved = localStorage.getItem(STORAGE_KEYS.CONFIG)
    if (saved) {
      const config = JSON.parse(saved) as Partial<PlaygroundConfig>
      if (
        config.video_duration === 15 &&
        !localStorage.getItem(STORAGE_KEYS.VIDEO_DURATION_DEFAULT_MIGRATED)
      ) {
        const migrated = {
          ...config,
          video_duration: DEFAULT_VIDEO_DURATION,
        }
        localStorage.setItem(STORAGE_KEYS.CONFIG, JSON.stringify(migrated))
        localStorage.setItem(
          STORAGE_KEYS.VIDEO_DURATION_DEFAULT_MIGRATED,
          'true'
        )
        return migrated
      }
      if (typeof config.video_duration === 'number') {
        config.video_duration = normalizeVideoDurationForModel(
          config.model || '',
          config.video_duration
        )
      }
      if (typeof config.video_resolution === 'string') {
        config.video_resolution = normalizeVideoResolutionForModel(
          config.model || '',
          config.video_resolution
        )
      }
      if (typeof config.video_size === 'string') {
        config.video_size = normalizeLTXVideoSizeForModel(
          config.model || '',
          config.video_size
        )
      }
      return config
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load config:', error)
  }
  return {}
}

/**
 * Save playground config to localStorage
 */
export function saveConfig(config: Partial<PlaygroundConfig>): void {
  try {
    localStorage.setItem(STORAGE_KEYS.CONFIG, JSON.stringify(config))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save config:', error)
  }
}

/**
 * Load parameter enabled state from localStorage
 */
export function loadParameterEnabled(): Partial<ParameterEnabled> {
  try {
    const saved = localStorage.getItem(STORAGE_KEYS.PARAMETER_ENABLED)
    if (saved) {
      return JSON.parse(saved)
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load parameter enabled:', error)
  }
  return {}
}

/**
 * Save parameter enabled state to localStorage
 */
export function saveParameterEnabled(
  parameterEnabled: Partial<ParameterEnabled>
): void {
  try {
    localStorage.setItem(
      STORAGE_KEYS.PARAMETER_ENABLED,
      JSON.stringify(parameterEnabled)
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save parameter enabled:', error)
  }
}

/**
 * Load messages from localStorage
 */
export function loadMessages(): Message[] | null {
  try {
    const saved = localStorage.getItem(STORAGE_KEYS.MESSAGES)
    if (saved) {
      const parsed: Message[] = JSON.parse(saved)
      const sanitized = sanitizeMessagesOnLoad(parsed)
      // Persist sanitized result to avoid re-sanitizing on subsequent loads
      if (sanitized !== parsed) {
        saveMessages(sanitized)
      }
      return sanitized
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load messages:', error)
  }
  return null
}

/**
 * Save messages to localStorage
 */
export function saveMessages(messages: Message[]): void {
  try {
    localStorage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save messages:', error)
  }
}

/**
 * Load user-scoped playground conversations from localStorage.
 */
export function loadConversationState(
  userId: number | string | null | undefined,
  configFallback: PlaygroundConfig,
  parameterFallback: ParameterEnabled,
  options: LoadConversationStateOptions = {}
): PlaygroundConversationState {
  try {
    const saved = localStorage.getItem(getConversationStorageKey(userId))
    if (saved) {
      const normalized = normalizeConversationState(
        JSON.parse(saved),
        configFallback,
        parameterFallback,
        options
      )
      if (normalized) return normalized
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground conversations:', error)
  }

  return createDefaultConversationState(configFallback, parameterFallback)
}

/**
 * Save user-scoped playground conversations to localStorage.
 */
export function saveConversationState(
  userId: number | string | null | undefined,
  state: PlaygroundConversationState
): void {
  try {
    const storageKey = getConversationStorageKey(userId)
    localStorage.setItem(storageKey, JSON.stringify(state))
    localStorage.removeItem(STORAGE_KEYS.MESSAGES)
    window.dispatchEvent(
      new CustomEvent<PlaygroundConversationStateEventDetail>(
        PLAYGROUND_CONVERSATION_STATE_EVENT,
        {
          detail: {
            storageKey,
            state,
          },
        }
      )
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground conversations:', error)
  }
}

/**
 * Clear all playground data
 */
export function clearPlaygroundData(): void {
  try {
    localStorage.removeItem(STORAGE_KEYS.CONFIG)
    localStorage.removeItem(STORAGE_KEYS.PARAMETER_ENABLED)
    localStorage.removeItem(STORAGE_KEYS.MESSAGES)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to clear playground data:', error)
  }
}
