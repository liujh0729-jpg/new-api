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
import { useState, useCallback, useEffect, useMemo } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  createPlaygroundConversation,
  getConversationTitle,
  getConversationStorageKey,
  loadConfig,
  loadConversationState,
  loadParameterEnabled,
  PLAYGROUND_CONVERSATION_STATE_EVENT,
  saveConfig,
  saveConversationState,
  saveParameterEnabled,
  type PlaygroundConversationStateEventDetail,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
  PlaygroundConversation,
  PlaygroundConversationState,
} from '../types'

type MessagesUpdater = Message[] | ((prev: Message[]) => Message[])
type ConversationStateUpdater =
  | PlaygroundConversationState
  | ((prev: PlaygroundConversationState) => PlaygroundConversationState)

function loadDefaultConfig(): PlaygroundConfig {
  return { ...DEFAULT_CONFIG, ...loadConfig() }
}

function loadDefaultParameterEnabled(): ParameterEnabled {
  return { ...DEFAULT_PARAMETER_ENABLED, ...loadParameterEnabled() }
}

function loadInitialConversationState(
  userId?: number | string | null,
  sanitizeMessages = true
): PlaygroundConversationState {
  return loadConversationState(
    userId,
    loadDefaultConfig(),
    loadDefaultParameterEnabled(),
    { sanitizeMessages }
  )
}

function getActiveConversation(
  state: PlaygroundConversationState
): PlaygroundConversation {
  return (
    state.conversations.find(
      (conversation) => conversation.id === state.activeConversationId
    ) || state.conversations[0]!
  )
}

function sortConversations(
  conversations: PlaygroundConversation[]
): PlaygroundConversation[] {
  return [...conversations].sort((a, b) => b.updatedAt - a.updatedAt)
}

function replaceConversation(
  state: PlaygroundConversationState,
  conversation: PlaygroundConversation
): PlaygroundConversationState {
  return {
    ...state,
    conversations: state.conversations.map((item) =>
      item.id === conversation.id ? conversation : item
    ),
  }
}

/**
 * Main state management hook for playground
 */
export function usePlaygroundState() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [conversationState, setConversationState] =
    useState<PlaygroundConversationState>(() =>
      loadInitialConversationState(userId)
    )
  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])

  useEffect(() => {
    setConversationState(loadInitialConversationState(userId))
  }, [userId])

  useEffect(() => {
    const storageKey = getConversationStorageKey(userId)
    const handleConversationStateChange = (event: Event) => {
      const detail = (
        event as CustomEvent<PlaygroundConversationStateEventDetail>
      ).detail
      if (detail?.storageKey !== storageKey) return
      setConversationState(detail.state)
    }

    window.addEventListener(
      PLAYGROUND_CONVERSATION_STATE_EVENT,
      handleConversationStateChange
    )
    return () => {
      window.removeEventListener(
        PLAYGROUND_CONVERSATION_STATE_EVENT,
        handleConversationStateChange
      )
    }
  }, [userId])

  const persistConversationState = useCallback(
    (updater: ConversationStateUpdater) => {
      const currentState = loadInitialConversationState(userId, false)
      const nextState =
        typeof updater === 'function' ? updater(currentState) : updater
      saveConversationState(userId, nextState)
      setConversationState(nextState)
    },
    [userId]
  )

  const activeConversation = getActiveConversation(conversationState)
  const config = activeConversation.config
  const parameterEnabled = activeConversation.parameterEnabled
  const messages = activeConversation.messages
  const conversations = useMemo(
    () => sortConversations(conversationState.conversations),
    [conversationState.conversations]
  )

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      persistConversationState((prev) => {
        const active = getActiveConversation(prev)
        const updatedConfig = { ...active.config, [key]: value }
        const updatedConversation = {
          ...active,
          config: updatedConfig,
          title: getConversationTitle(active.messages, updatedConfig.mode),
          updatedAt: Date.now(),
        }

        saveConfig(updatedConfig)
        return replaceConversation(prev, updatedConversation)
      })
    },
    [persistConversationState]
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      persistConversationState((prev) => {
        const active = getActiveConversation(prev)
        const updatedParameterEnabled = {
          ...active.parameterEnabled,
          [key]: value,
        }
        const updatedConversation = {
          ...active,
          parameterEnabled: updatedParameterEnabled,
          updatedAt: Date.now(),
        }

        saveParameterEnabled(updatedParameterEnabled)
        return replaceConversation(prev, updatedConversation)
      })
    },
    [persistConversationState]
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: MessagesUpdater) => {
      persistConversationState((prev) => {
        const active = getActiveConversation(prev)
        const nextMessages =
          typeof updater === 'function' ? updater(active.messages) : updater
        const updatedConversation = {
          ...active,
          messages: nextMessages,
          title: getConversationTitle(nextMessages, active.config.mode),
          updatedAt: Date.now(),
        }

        return replaceConversation(prev, updatedConversation)
      })
    },
    [persistConversationState]
  )

  // Clear all messages
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  const createConversation = useCallback(() => {
    persistConversationState((prev) => {
      const active = getActiveConversation(prev)
      if (active.messages.length === 0) return prev

      const conversation = createPlaygroundConversation(
        loadDefaultConfig(),
        loadDefaultParameterEnabled()
      )
      return {
        ...prev,
        activeConversationId: conversation.id,
        conversations: [conversation, ...prev.conversations],
      }
    })
  }, [persistConversationState])

  const selectConversation = useCallback(
    (conversationId: string) => {
      persistConversationState((prev) => {
        if (
          prev.activeConversationId === conversationId ||
          !prev.conversations.some(
            (conversation) => conversation.id === conversationId
          )
        ) {
          return prev
        }

        return {
          ...prev,
          activeConversationId: conversationId,
        }
      })
    },
    [persistConversationState]
  )

  const deleteConversation = useCallback(
    (conversationId: string) => {
      persistConversationState((prev) => {
        const remaining = prev.conversations.filter(
          (conversation) => conversation.id !== conversationId
        )

        if (remaining.length === prev.conversations.length) return prev

        if (remaining.length === 0) {
          const conversation = createPlaygroundConversation(
            loadDefaultConfig(),
            loadDefaultParameterEnabled()
          )
          return {
            ...prev,
            activeConversationId: conversation.id,
            conversations: [conversation],
          }
        }

        const activeConversationId =
          prev.activeConversationId === conversationId
            ? sortConversations(remaining)[0]!.id
            : prev.activeConversationId

        return {
          ...prev,
          activeConversationId,
          conversations: remaining,
        }
      })
    },
    [persistConversationState]
  )

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    persistConversationState((prev) => {
      const active = getActiveConversation(prev)
      const updatedConversation = {
        ...active,
        config: DEFAULT_CONFIG,
        parameterEnabled: DEFAULT_PARAMETER_ENABLED,
        title: getConversationTitle(active.messages, DEFAULT_CONFIG.mode),
        updatedAt: Date.now(),
      }

      saveConfig(DEFAULT_CONFIG)
      saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
      return replaceConversation(prev, updatedConversation)
    })
  }, [persistConversationState])

  return {
    // State
    activeConversationId: conversationState.activeConversationId,
    conversations,
    config,
    parameterEnabled,
    messages,
    models,
    groups,

    // Setters
    setModels,
    setGroups,

    // Actions
    updateConfig,
    updateParameterEnabled,
    updateMessages,
    clearMessages,
    createConversation,
    selectConversation,
    deleteConversation,
    resetConfig,
  }
}
