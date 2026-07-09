import * as React from "react"
import { useLocation, useNavigate } from "react-router"
import { toast } from "sonner"

import {
  getBrowserNotificationPermission,
  showBrowserMessageNotification,
} from "@/lib/browser-notifications"
import { createConversationMentionLabelResolver } from "@/lib/conversation-mention-labels"
import {
  formatClientMessageBodySummary,
  isClientMessageInitiatedByUser,
  type ClientConversation,
  type ClientMessage,
  type ClientMessageSender,
  type ClientUser,
  type ContactApp,
  type ContactUser,
  normalizeMessageCreatedEventPayload,
} from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { formatMentionTemplateText } from "@/lib/message-mentions"
import { useRealtime } from "@/lib/realtime-context"

const enableNotificationToastId = "enable-browser-message-notifications"
const enableNotificationToastText =
  "收到新消息，左上角点击头像，在设置中可以开启桌面通知"

export function ClientMessageNotificationSync() {
  const location = useLocation()
  const navigate = useNavigate()
  const { subscribeRealtimeEvent } = useRealtime()
  const { contactApps, contacts, conversations, me } = useClientData()
  const contactAppsById = React.useMemo(
    () => new Map(contactApps.map((app) => [app.id, app])),
    [contactApps]
  )
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
        const body = `${senderName}: ${getMessageNotificationSummary({
          appsById: contactAppsById,
          contacts,
          conversation,
          me,
          message,
        })}`

        if (getBrowserNotificationPermission() !== "granted") {
          toast.info(enableNotificationToastText, {
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
          toast.info(enableNotificationToastText, {
            id: enableNotificationToastId,
          })
        }
      } catch {
        // Ignore malformed realtime events. The websocket remains usable.
      }
    })
  }, [
    activeConversationId,
    contactAppsById,
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

function getMessageNotificationSummary({
  appsById,
  contacts,
  conversation,
  me,
  message,
}: {
  appsById: ReadonlyMap<string, ContactApp>
  contacts: ContactUser[]
  conversation: ClientConversation | undefined
  me: ClientUser
  message: ClientMessage
}) {
  const mentionLabelResolver = createConversationMentionLabelResolver({
    appsById,
    contactsById: new Map(contacts.map((contact) => [contact.id, contact])),
    conversation,
    currentUser: me,
  })
  const summary = formatMentionTemplateText(
    formatClientMessageBodySummary(message.body),
    mentionLabelResolver
  )
    .trim()
    .replace(/\s+/g, " ")

  return summary || "收到一条新消息"
}

function formatUserName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name
}
