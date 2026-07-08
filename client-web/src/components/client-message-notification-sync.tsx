import * as React from "react"
import { useLocation, useNavigate } from "react-router"
import { toast } from "sonner"

import {
  getBrowserNotificationPermission,
  showBrowserMessageNotification,
} from "@/lib/browser-notifications"
import {
  formatClientMessageBodySummary,
  isClientMessageInitiatedByUser,
  type ClientConversation,
  type ClientMessage,
  type ClientMessageSender,
  type ClientUser,
  type ContactUser,
  normalizeMessageCreatedEventPayload,
} from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { useRealtime } from "@/lib/realtime-context"

const enableNotificationToastId = "enable-browser-message-notifications"

export function ClientMessageNotificationSync() {
  const location = useLocation()
  const navigate = useNavigate()
  const { subscribeRealtimeEvent } = useRealtime()
  const { contacts, conversations, me } = useClientData()
  const activeConversationId = React.useMemo(
    () => new URLSearchParams(location.search).get("conversation_id") ?? "",
    [location.search]
  )

  React.useEffect(() => {
    return subscribeRealtimeEvent("message.created", (payload) => {
      try {
        const message = normalizeMessageCreatedEventPayload(payload)
        if (isClientMessageInitiatedByUser(message, me.id)) {
          return
        }
        if (
          document.visibilityState === "visible" &&
          message.conversationId === activeConversationId
        ) {
          return
        }

        const conversation = conversations.find(
          (currentConversation) =>
            currentConversation.id === message.conversationId
        )
        const senderName = getMessageNotificationSenderName({
          contacts,
          conversation,
          me,
          sender: message.sender,
        })
        const body = `${senderName}: ${getMessageNotificationSummary(message)}`

        if (getBrowserNotificationPermission() !== "granted") {
          toast.info("收到新消息，可在设置中开启桌面通知", {
            id: enableNotificationToastId,
          })
          return
        }

        const notified = showBrowserMessageNotification({
          body,
          tag: message.id,
          title: "收到新消息",
          onClick: () => {
            window.focus()
            navigate(
              `/chat?conversation_id=${encodeURIComponent(message.conversationId)}`
            )
          },
        })
        if (!notified) {
          toast.info("收到新消息，可在设置中开启桌面通知", {
            id: enableNotificationToastId,
          })
        }
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [
    activeConversationId,
    contacts,
    conversations,
    me,
    navigate,
    subscribeRealtimeEvent,
  ])

  return null
}

function getMessageNotificationSenderName({
  contacts,
  conversation,
  me,
  sender,
}: {
  contacts: ContactUser[]
  conversation: ClientConversation | undefined
  me: ClientUser
  sender: ClientMessageSender
}) {
  if (sender.type === "system") {
    return "系统"
  }

  if (sender.type === "app") {
    return conversation?.name ?? "应用"
  }

  if (sender.id === me.id) {
    return formatUserName(me)
  }

  const contact = contacts.find(
    (currentContact) => currentContact.id === sender.id
  )
  if (contact) {
    return formatUserName(contact)
  }

  const member = conversation?.members?.find(
    (currentMember) => currentMember.id === sender.id
  )
  if (member) {
    return formatUserName(member)
  }

  if (conversation?.type === "direct") {
    return conversation.name
  }

  return "未知用户"
}

function getMessageNotificationSummary(message: ClientMessage) {
  const summary = formatClientMessageBodySummary(message.body)
    .trim()
    .replace(/\s+/g, " ")

  return summary || "收到一条新消息"
}

function formatUserName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name
}
