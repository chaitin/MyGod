import type {
  ConversationPanelAppProfile,
  ConversationPanelMentionTarget,
  ConversationPanelMessage,
  ConversationPanelReplyTarget,
} from "@/lib/conversation-panel-types"
import {
  getConversationAppAvatar,
  getConversationAppDisplayName,
} from "@/lib/conversation-app-profile"
import {
  formatClientMessageBodySummary,
  type ClientConversation,
  type ClientMessage,
  type ClientUser,
  type ContactApp,
  type ContactUser,
} from "@/lib/client-data-api"
import {
  formatMentionTemplateText,
  type MentionLabelResolver,
} from "@/lib/message-mentions"

export function toConversationPanelMessage(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>,
  messagesById: ReadonlyMap<string, ClientMessage>,
  mentionLabelResolver: MentionLabelResolver
): ConversationPanelMessage {
  const fromMe =
    message.sender.type === "user" && message.sender.id === currentUser.id
  const role =
    message.sender.type === "system" ? "system" : fromMe ? "me" : "other"

  return {
    author: getMessageAuthor(
      message,
      conversation,
      currentUser,
      contactsById,
      appsById
    ),
    avatar: getMessageAvatar(
      message,
      conversation,
      currentUser,
      contactsById,
      appsById
    ),
    body: message.body,
    choice: message.choice,
    canRevoke: canRevokeMessage(message, conversation, currentUser.id),
    createdAt: message.createdAt,
    delegatedByName: message.delegatedBy?.name ?? "",
    id: message.id,
    mentionTarget: getMessageMentionTarget(message, mentionLabelResolver),
    reactionVersion: message.reactionVersion ?? 0,
    reactions: message.reactions ?? [],
    replyTo: getMessageReplyTarget(
      message,
      conversation,
      currentUser,
      contactsById,
      appsById,
      messagesById,
      mentionLabelResolver
    ),
    role,
    senderAppId: message.sender.type === "app" ? message.sender.id : null,
    senderAppProfile: getMessageAppProfile(message, conversation, appsById),
    senderUserId: message.sender.type === "user" ? message.sender.id : null,
    time: formatConversationMessageTime(message.createdAt),
    topic: getMessageTopic(
      message,
      conversation,
      currentUser,
      contactsById,
      appsById,
      mentionLabelResolver
    ),
  }
}

function getMessageTopic(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>,
  mentionLabelResolver: MentionLabelResolver
) {
  if (!message.topic) {
    return undefined
  }
  return {
    archived: message.topic.archived,
    conversationId: message.topic.conversationId,
    recentReplies: message.topic.recentReplies.map((reply) => ({
      author: getTopicReplyAuthor(
        reply.sender,
        conversation,
        currentUser,
        contactsById,
        appsById
      ),
      avatar: getTopicReplyAvatar(
        reply.sender,
        conversation,
        currentUser,
        contactsById,
        appsById
      ),
      id: reply.id,
      summary: formatMentionTemplateText(reply.summary, mentionLabelResolver),
      time: formatTopicReplyTime(reply.createdAt),
    })),
  }
}

function formatTopicReplyTime(createdAt: string) {
  const date = new Date(createdAt)
  if (Number.isNaN(date.getTime())) {
    return ""
  }
  return `${padMessageTimePart(date.getHours())}:${padMessageTimePart(date.getMinutes())}`
}

function getTopicReplyAuthor(
  sender: NonNullable<
    ClientMessage["topic"]
  >["recentReplies"][number]["sender"],
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>
) {
  if (sender.type === "app") {
    return getConversationAppDisplayName(conversation, sender.id, appsById)
  }
  if (sender.id === currentUser.id) {
    return formatMessageUserName(currentUser)
  }
  const contact = contactsById.get(sender.id)
  if (contact) {
    return formatMessageUserName(contact)
  }
  return conversation.type === "direct" ? conversation.name : "成员"
}

function getTopicReplyAvatar(
  sender: NonNullable<
    ClientMessage["topic"]
  >["recentReplies"][number]["sender"],
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>
) {
  if (sender.type === "app") {
    return getConversationAppAvatar(conversation, sender.id, appsById)
  }
  if (sender.id === currentUser.id) {
    return currentUser.avatar
  }
  return (
    contactsById.get(sender.id)?.avatar ||
    (conversation.type === "direct" ? conversation.avatar : "")
  )
}

export function formatConversationMessageSummary(
  body: ClientMessage["body"],
  mentionLabelResolver: MentionLabelResolver
) {
  return formatMentionTemplateText(
    formatClientMessageBodySummary(body),
    mentionLabelResolver
  )
}

export function formatConversationMessageTime(
  createdAt: string,
  now = new Date()
) {
  const date = new Date(createdAt)

  if (Number.isNaN(date.getTime())) {
    return ""
  }

  const time = `${padMessageTimePart(date.getHours())}:${padMessageTimePart(date.getMinutes())}`
  if (isSameLocalMessageDay(date, now)) {
    return time
  }

  const monthAndDay = `${padMessageTimePart(date.getMonth() + 1)}/${padMessageTimePart(date.getDate())}`
  if (date.getFullYear() === now.getFullYear()) {
    return `${monthAndDay} ${time}`
  }

  return `${date.getFullYear()}/${monthAndDay} ${time}`
}

function isSameLocalMessageDay(date: Date, otherDate: Date) {
  return (
    date.getFullYear() === otherDate.getFullYear() &&
    date.getMonth() === otherDate.getMonth() &&
    date.getDate() === otherDate.getDate()
  )
}

function padMessageTimePart(value: number) {
  return String(value).padStart(2, "0")
}

function canRevokeMessage(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUserId: string
) {
  if (
    message.sender.type === "system" ||
    message.body.type === "revoked" ||
    message.body.type === "unsupported" ||
    conversation.topic?.archived
  ) {
    return false
  }
  if (message.sender.type === "user" && message.sender.id === currentUserId) {
    return true
  }
  if (
    conversation.type !== "group" &&
    conversation.topic?.parentConversationType !== "group"
  ) {
    return false
  }

  const currentMember = conversation.members?.find(
    (member) => member.id === currentUserId
  )

  return currentMember?.role === "owner" || currentMember?.role === "admin"
}

function getMessageMentionTarget(
  message: ClientMessage,
  mentionLabelResolver: MentionLabelResolver
): ConversationPanelMentionTarget | null {
  if (message.sender.type !== "user" && message.sender.type !== "app") {
    return null
  }

  const label = mentionLabelResolver({
    id: message.sender.id,
    type: message.sender.type,
  })?.trim()
  if (!label) {
    return null
  }

  return {
    id: message.sender.id,
    label,
    targetType: message.sender.type,
  }
}

function getMessageReplyTarget(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>,
  messagesById: ReadonlyMap<string, ClientMessage>,
  mentionLabelResolver: MentionLabelResolver
): ConversationPanelReplyTarget | undefined {
  if (message.replyTo) {
    return {
      id: message.replyTo.id,
      author: getReplyToSenderAuthor(
        message.replyTo.sender,
        conversation,
        currentUser,
        contactsById,
        appsById
      ),
      summary: formatMentionTemplateText(
        message.replyTo.summary,
        mentionLabelResolver
      ),
    }
  }

  if (!message.replyToMessageId) {
    return undefined
  }

  const replyMessage = messagesById.get(message.replyToMessageId)
  if (!replyMessage) {
    return undefined
  }

  return {
    id: replyMessage.id,
    author: getMessageAuthor(
      replyMessage,
      conversation,
      currentUser,
      contactsById,
      appsById
    ),
    summary: formatConversationMessageSummary(
      replyMessage.body,
      mentionLabelResolver
    ),
  }
}

function getReplyToSenderAuthor(
  sender: NonNullable<ClientMessage["replyTo"]>["sender"],
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>
) {
  if (sender.type === "system") {
    return "系统"
  }

  if (sender.type === "app") {
    return (
      sender.name ||
      getConversationAppDisplayName(conversation, sender.id, appsById)
    )
  }

  if (sender.id === currentUser.id) {
    return formatMessageUserName(currentUser)
  }

  const contact = contactsById.get(sender.id)
  if (contact) {
    return formatMessageUserName(contact)
  }

  return (
    sender.name || (conversation.type === "direct" ? conversation.name : "成员")
  )
}

function getMessageAuthor(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>
) {
  if (message.sender.type === "system") {
    return "系统"
  }

  if (message.sender.type === "app") {
    return getConversationAppDisplayName(
      conversation,
      message.sender.id,
      appsById
    )
  }

  if (message.sender.type === "user" && message.sender.id === currentUser.id) {
    return formatMessageUserName(currentUser)
  }

  if (message.sender.type === "user") {
    const contact = contactsById.get(message.sender.id)
    if (contact) {
      return formatMessageUserName(contact)
    }
  }

  if (message.sender.type === "user" && conversation.type === "direct") {
    return conversation.name
  }

  return "成员"
}

function formatMessageUserName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name
}

function getMessageAvatar(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id">,
  contactsById: ReadonlyMap<string, ContactUser>,
  appsById: ReadonlyMap<string, ContactApp>
) {
  if (message.sender.type === "user" && message.sender.id === currentUser.id) {
    return currentUser.avatar
  }

  if (message.sender.type === "user") {
    return (
      contactsById.get(message.sender.id)?.avatar ||
      (conversation.type === "direct" ? conversation.avatar : "")
    )
  }

  if (message.sender.type === "app") {
    return getConversationAppAvatar(conversation, message.sender.id, appsById)
  }

  return ""
}

function getMessageAppProfile(
  message: ClientMessage,
  conversation: ClientConversation,
  appsById: ReadonlyMap<string, ContactApp>
): ConversationPanelAppProfile | null {
  if (message.sender.type !== "app") {
    return null
  }

  const contactApp = appsById.get(message.sender.id)

  return {
    avatar: getConversationAppAvatar(conversation, message.sender.id, appsById),
    description: contactApp?.description ?? "",
    id: message.sender.id,
    name: getConversationAppDisplayName(
      conversation,
      message.sender.id,
      appsById
    ),
    online: contactApp?.online ?? false,
  }
}
