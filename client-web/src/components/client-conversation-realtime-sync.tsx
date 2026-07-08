import * as React from "react"
import { useLocation } from "react-router"

import { normalizeMessageCreatedEventPayload } from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { useRealtime } from "@/lib/realtime-context"

export function ClientConversationRealtimeSync() {
  const location = useLocation()
  const { ready: realtimeReady, subscribeRealtimeEvent } = useRealtime()
  const {
    handleIncomingConversationMessage,
    refreshConversations,
    syncLoadedConversationMessages,
  } = useClientData()
  const hasSeenRealtimeReadyRef = React.useRef(realtimeReady)
  const previousRealtimeReadyRef = React.useRef(realtimeReady)
  const activeConversationId = React.useMemo(
    () => new URLSearchParams(location.search).get("conversation_id") ?? "",
    [location.search]
  )

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.created", (payload) => {
      try {
        const message = normalizeMessageCreatedEventPayload(payload)
        handleIncomingConversationMessage(message, {
          activeConversationId,
          visible: document.visibilityState === "visible",
        })
        if (
          message.body.type === "system_event" &&
          (message.body.event === "group_avatar_updated" ||
            message.body.event === "group_name_updated" ||
            message.body.event === "group_member_left")
        ) {
          void refreshConversations().catch(() => undefined)
        }
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [
    activeConversationId,
    handleIncomingConversationMessage,
    refreshConversations,
    subscribeRealtimeEvent,
  ])

  React.useEffect(() => {
    const wasReady = previousRealtimeReadyRef.current
    previousRealtimeReadyRef.current = realtimeReady

    if (!realtimeReady || wasReady) {
      return
    }

    if (hasSeenRealtimeReadyRef.current) {
      void refreshConversations().catch(() => undefined)
    }
    hasSeenRealtimeReadyRef.current = true
    syncLoadedConversationMessages()
  }, [realtimeReady, refreshConversations, syncLoadedConversationMessages])

  return null
}
