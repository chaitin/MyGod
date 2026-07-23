import * as React from "react"
import { matchPath, useLocation, useNavigate } from "react-router"

import {
  normalizeConversationPinUpdatedEventPayload,
  normalizeConversationMuteUpdatedEventPayload,
  normalizeConversationMemberMentionedEventPayload,
  normalizeConversationMemberChoiceReceivedEventPayload,
  normalizeConversationRemovedEventPayload,
  normalizeMessageCreatedEventPayload,
  normalizeMessageUpdatedEventPayload,
  normalizeMessageReactionsUpdatedEventPayload,
  normalizeMessageChoiceUpdatedEventPayload,
  normalizeTopicEventPayload,
} from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { useRealtime } from "@/lib/realtime-context"

export function ClientConversationRealtimeSync() {
  const location = useLocation()
  const navigate = useNavigate()
  const { ready: realtimeReady, subscribeRealtimeEvent } = useRealtime()
  const {
    foregroundConversationId,
    handleIncomingConversationMessage,
    handleIncomingConversationMessageUpdate,
    handleIncomingMessageChoiceUpdate,
    handleIncomingMessageReactionsUpdate,
    refreshConversations,
    removeConversation,
    syncLoadedConversationMessages,
    updateConversationLastMentionedSeq,
    updateConversationLastChoiceSeq,
    updateConversationMuted,
    updateConversationPinned,
    updateMessageTopic,
  } = useClientData()
  const hasSeenRealtimeReadyRef = React.useRef(realtimeReady)
  const previousRealtimeReadyRef = React.useRef(realtimeReady)
  const activeConversationId = React.useMemo(
    () =>
      matchPath("/chat/:conversationId", location.pathname)?.params
        .conversationId ?? "",
    [location.pathname]
  )
  const visibleConversationId = foregroundConversationId || activeConversationId

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.created", (payload) => {
      try {
        const message = normalizeMessageCreatedEventPayload(payload)
        handleIncomingConversationMessage(message, {
          activeConversationId: visibleConversationId,
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
    handleIncomingConversationMessage,
    refreshConversations,
    subscribeRealtimeEvent,
    visibleConversationId,
  ])

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.updated", (payload) => {
      try {
        const message = normalizeMessageUpdatedEventPayload(payload)
        handleIncomingConversationMessageUpdate(message)
        if (message.body.type === "revoked") {
          void refreshConversations().catch(() => undefined)
        }
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [
    handleIncomingConversationMessageUpdate,
    refreshConversations,
    subscribeRealtimeEvent,
  ])

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.reactions_updated", (payload) => {
      try {
        handleIncomingMessageReactionsUpdate(
          normalizeMessageReactionsUpdatedEventPayload(payload)
        )
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [handleIncomingMessageReactionsUpdate, subscribeRealtimeEvent])

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.choice_updated", (payload) => {
      try {
        handleIncomingMessageChoiceUpdate(
          normalizeMessageChoiceUpdatedEventPayload(payload)
        )
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [handleIncomingMessageChoiceUpdate, subscribeRealtimeEvent])

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
    return subscribeRealtimeEvent("conversation.restored", (payload) => {
      try {
        normalizeConversationRemovedEventPayload(payload)
        void refreshConversations().catch(() => undefined)
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [refreshConversations, subscribeRealtimeEvent])

  React.useEffect(() => {
    return subscribeRealtimeEvent("conversation.mute_updated", (payload) => {
      try {
        const event = normalizeConversationMuteUpdatedEventPayload(payload)
        updateConversationMuted(event.conversationId, event.muted)
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [subscribeRealtimeEvent, updateConversationMuted])

  React.useEffect(() => {
    return subscribeRealtimeEvent("conversation.pin_updated", (payload) => {
      try {
        const event = normalizeConversationPinUpdatedEventPayload(payload)
        updateConversationPinned(event.conversationId, event.pinned)
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [subscribeRealtimeEvent, updateConversationPinned])

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
    return subscribeRealtimeEvent(
      "conversation.member_choice_received",
      (payload) => {
        try {
          const event =
            normalizeConversationMemberChoiceReceivedEventPayload(payload)
          updateConversationLastChoiceSeq(
            event.conversationId,
            event.lastChoiceSeq
          )
        } catch {
          // Ignore malformed realtime events. The websocket remains usable.
        }
      }
    )
  }, [subscribeRealtimeEvent, updateConversationLastChoiceSeq])

  React.useEffect(() => {
    const handleTopicEvent = (payload: unknown) => {
      try {
        const event = normalizeTopicEventPayload(payload)
        updateMessageTopic?.(
          event.parentConversationId,
          event.sourceMessageId,
          {
            archived: event.archived,
            conversationId: event.conversationId,
          }
        )
        void refreshConversations().catch(() => undefined)
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    }
    const unsubscribers = [
      subscribeRealtimeEvent("topic.created", handleTopicEvent),
      subscribeRealtimeEvent("topic.participated", handleTopicEvent),
      subscribeRealtimeEvent("topic.archived", handleTopicEvent),
    ]
    return () => {
      for (const unsubscribe of unsubscribers) {
        unsubscribe()
      }
    }
  }, [refreshConversations, subscribeRealtimeEvent, updateMessageTopic])

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
