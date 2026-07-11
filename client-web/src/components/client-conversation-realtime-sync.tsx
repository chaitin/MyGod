import * as React from "react"
import { matchPath, useLocation, useNavigate } from "react-router"

import {
  normalizeConversationMemberMentionedEventPayload,
  normalizeConversationRemovedEventPayload,
  normalizeMessageCreatedEventPayload,
  normalizeMessageUpdatedEventPayload,
} from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { useRealtime } from "@/lib/realtime-context"

export function ClientConversationRealtimeSync() {
  const location = useLocation()
  const navigate = useNavigate()
  const { ready: realtimeReady, subscribeRealtimeEvent } = useRealtime()
  const {
    handleIncomingConversationMessage,
    handleIncomingConversationMessageUpdate,
    refreshConversations,
    removeConversation,
    syncLoadedConversationMessages,
    updateConversationLastMentionedSeq,
  } = useClientData()
  const hasSeenRealtimeReadyRef = React.useRef(realtimeReady)
  const previousRealtimeReadyRef = React.useRef(realtimeReady)
  const activeConversationId = React.useMemo(
    () =>
      matchPath("/chat/:conversationId", location.pathname)?.params
        .conversationId ?? "",
    [location.pathname]
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
            message.body.event === "group_member_left" ||
            message.body.event === "group_member_removed")
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
    return subscribeRealtimeEvent("message.updated", (payload) => {
      try {
        const message = normalizeMessageUpdatedEventPayload(payload)
        handleIncomingConversationMessageUpdate(message)
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [handleIncomingConversationMessageUpdate, subscribeRealtimeEvent])

  React.useEffect(() => {
    return subscribeRealtimeEvent("conversation.removed", (payload) => {
      try {
        const event = normalizeConversationRemovedEventPayload(payload)
        removeConversation(event.conversationId)
        if (activeConversationId === event.conversationId) {
          navigate("/chat", { replace: true })
        }
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [
    activeConversationId,
    navigate,
    removeConversation,
    subscribeRealtimeEvent,
  ])

  React.useEffect(() => {
    return subscribeRealtimeEvent(
      "conversation.member_mentioned",
      (payload) => {
        try {
          const event =
            normalizeConversationMemberMentionedEventPayload(payload)
          updateConversationLastMentionedSeq(
            event.conversationId,
            event.lastMentionedSeq
          )
        } catch {
          // Ignore malformed realtime events. The websocket remains usable.
        }
      }
    )
  }, [subscribeRealtimeEvent, updateConversationLastMentionedSeq])

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
