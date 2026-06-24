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
import { useCallback } from 'react'
import { SSE } from 'sse.js'
import { getCommonHeaders } from '@/lib/api'
import { API_ENDPOINTS, ERROR_MESSAGES } from '../constants'
import type { ChatCompletionRequest, ChatCompletionChunk } from '../types'

let activeStreamSource: SSE | null = null
let activeStreamComplete = false
const SSE_READY_STATE_CLOSED = 2

type SSEErrorEvent = Event & { data?: string; responseCode?: number }
type SSEReadyStateEvent = Event & { readyState?: number }

function closeStreamSource(source: SSE): void {
  if (activeStreamSource === source) {
    activeStreamSource = null
  }

  source.close()
}

/**
 * Hook for handling streaming chat completion requests
 */
export function useStreamRequest() {
  const sendStreamRequest = useCallback(
    (
      payload: ChatCompletionRequest,
      onUpdate: (type: 'reasoning' | 'content', chunk: string) => void,
      onComplete: () => void,
      onError: (error: string, errorCode?: string) => void
    ) => {
      if (activeStreamSource) {
        activeStreamComplete = true
        closeStreamSource(activeStreamSource)
      }

      const source = new SSE(API_ENDPOINTS.CHAT_COMPLETIONS, {
        headers: getCommonHeaders(),
        method: 'POST',
        payload: JSON.stringify(payload),
        start: false,
      })

      activeStreamSource = source
      activeStreamComplete = false
      let hasReceivedStreamMessage = false

      const closeSource = () => {
        closeStreamSource(source)
      }

      const completeStream = () => {
        if (!activeStreamComplete && activeStreamSource === source) {
          activeStreamComplete = true
          closeSource()
          onComplete()
        }
      }

      const handleError = (errorMessage: string, errorCode?: string) => {
        if (!activeStreamComplete && activeStreamSource === source) {
          activeStreamComplete = true
          onError(errorMessage, errorCode)
          closeSource()
        }
      }

      source.addEventListener('message', (e: MessageEvent) => {
        if (activeStreamSource !== source) return

        if (e.data === '[DONE]') {
          completeStream()
          return
        }

        try {
          const chunk = JSON.parse(e.data) as ChatCompletionChunk & {
            error?: { message?: string; code?: string }
          }
          if (chunk.error) {
            handleError(
              chunk.error.message || ERROR_MESSAGES.API_REQUEST_ERROR,
              chunk.error.code
            )
            return
          }

          hasReceivedStreamMessage = true
          const delta = chunk.choices?.[0]?.delta

          if (delta) {
            if (delta.reasoning_content) {
              onUpdate('reasoning', delta.reasoning_content)
            }
            if (delta.content) {
              onUpdate('content', delta.content)
            }
          }
        } catch (error) {
          // eslint-disable-next-line no-console
          console.error('Failed to parse SSE message:', error)
          handleError(ERROR_MESSAGES.PARSE_ERROR)
        }
      })

      source.addEventListener(
        'readystatechange',
        (e: SSEReadyStateEvent) => {
          if (activeStreamSource !== source) return
          if (activeStreamComplete || e.readyState !== SSE_READY_STATE_CLOSED) {
            return
          }

          if (hasReceivedStreamMessage) {
            completeStream()
            return
          }

          handleError(ERROR_MESSAGES.CONNECTION_CLOSED)
        }
      )

      source.addEventListener(
        'error',
        (e: SSEErrorEvent) => {
          if (activeStreamSource !== source) return

          // Only handle errors if stream didn't complete normally
          if (source.readyState !== 2) {
            // eslint-disable-next-line no-console
            console.error('SSE Error:', e)
            let errorMessage =
              e.data ||
              (e.responseCode
                ? `HTTP ${e.responseCode}: ${ERROR_MESSAGES.CONNECTION_CLOSED}`
                : ERROR_MESSAGES.API_REQUEST_ERROR)
            let errorCode: string | undefined
            if (e.data) {
              try {
                const parsed = JSON.parse(e.data) as {
                  error?: { message?: string; code?: string }
                }
                if (parsed?.error) {
                  errorMessage = parsed.error.message || errorMessage
                  errorCode = parsed.error.code || undefined
                }
              } catch {
                // not JSON, use raw string
              }
            }
            if (!errorCode && e.responseCode === 402) {
              errorCode = 'payment_required'
            }
            handleError(errorMessage, errorCode)
          }
        }
      )

      try {
        source.stream()
      } catch (error: unknown) {
        // eslint-disable-next-line no-console
        console.error('Failed to start SSE stream:', error)
        onError(ERROR_MESSAGES.STREAM_START_ERROR)
        if (activeStreamSource === source) {
          activeStreamSource = null
        }
      }
    },
    []
  )

  const stopStream = useCallback(() => {
    if (activeStreamSource) {
      activeStreamComplete = true
      closeStreamSource(activeStreamSource)
    }
  }, [])

  const isStreaming = activeStreamSource !== null

  return {
    sendStreamRequest,
    stopStream,
    isStreaming,
  }
}
