import { useCallback, useRef } from "react"

import type { ClientConversationMessageState } from "@/lib/client-data-context"
import { compactConversationMessageState } from "@/lib/client-data-state"

export function useConversationMessageRetention() {
  const viewTokensRef = useRef<Map<string, Set<symbol>>>(new Map())

  const registerConversationMessageView = useCallback(
    (conversationId: string) => {
      if (!conversationId) {
        return () => undefined
      }

      const token = Symbol(conversationId)
      const currentTokens =
        viewTokensRef.current.get(conversationId) ?? new Set<symbol>()
      currentTokens.add(token)
      viewTokensRef.current.set(conversationId, currentTokens)

      return () => {
        const tokens = viewTokensRef.current.get(conversationId)
        if (!tokens) {
          return
        }
        tokens.delete(token)
        if (tokens.size === 0) {
          viewTokensRef.current.delete(conversationId)
        }
      }
    },
    []
  )

  const applyConversationMessageRetention = useCallback(
    (conversationId: string, state: ClientConversationMessageState) => {
      return viewTokensRef.current.has(conversationId)
        ? state
        : compactConversationMessageState(state)
    },
    []
  )

  return {
    applyConversationMessageRetention,
    registerConversationMessageView,
  }
}
