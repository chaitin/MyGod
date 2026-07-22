import type {
  ClientContacts,
  ClientConversation,
  ClientMessage,
  ClientMessageBody,
  ClientMessageReaction,
  ClientUser,
} from "@/data/models"
import type { AttachmentResourceReference } from "@/data/resources"
import { getContactDisplayName } from "@/domain/contacts/contact-display"
import type { EntityReference } from "@/domain/entities/entity-profile"
import {
  formatMentionTemplateText,
  type MessageMentionLabelResolver,
} from "@/domain/messages/message-mentions"

export {
  formatMentionTemplateText,
  type MessageMentionLabelResolver,
} from "@/domain/messages/message-mentions"

export type PresentedMessage = {
  author: string
  avatar: string
  body: ClientMessageBody
  createdAt: string
  delegatedByName: string
  id: string
  reactions: ClientMessageReaction[]
  replyTo?: {
    author: string
    summary: string
  }
  role: "me" | "other" | "system"
  sender: EntityReference | null
  topic?: {
    archived: boolean
    conversationId: string
    recentReplies: {
      author: string
      avatar: string
      id: string
      summary: string
      time: string
    }[]
  }
}

const messageTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  hour: "2-digit",
  hour12: false,
  minute: "2-digit",
})

export function buildPresentedMessages({
  contacts,
  conversation,
  currentUser,
  messages,
  resolveMentionLabel,
}: {
  contacts: ClientContacts
  conversation: ClientConversation
  currentUser: ClientUser
  messages: ClientMessage[]
  resolveMentionLabel: MessageMentionLabelResolver
}): PresentedMessage[] {
  const usersById = new Map(
    contacts.users.map((user) => [user.id.toLowerCase(), user] as const)
  )
  const appsById = new Map(
    contacts.apps.map((app) => [app.id.toLowerCase(), app] as const)
  )
  const messagesById = new Map(messages.map((message) => [message.id, message]))

  return messages.map((message) => {
    const role =
      message.body.type === "system_event" || message.sender.type === "system"
        ? "system"
        : message.sender.type === "user" && message.sender.id === currentUser.id
          ? "me"
          : "other"
    const replyMessage = message.replyToMessageId
      ? messagesById.get(message.replyToMessageId)
      : undefined

    return {
      author: getMessageAuthor(
        message,
        conversation,
        currentUser,
        usersById,
        appsById
      ),
      avatar: getMessageAvatar(
        message,
        conversation,
        currentUser,
        usersById,
        appsById
      ),
      body: message.body,
      createdAt: message.createdAt,
      delegatedByName: message.delegatedBy?.name ?? "",
      id: message.id,
      reactions: message.reactions,
      replyTo: message.replyTo
        ? {
            author: getReplyAuthor(
              message.replyTo.sender,
              conversation,
              currentUser,
              usersById,
              appsById
            ),
            summary: formatMentionTemplateText(
              message.replyTo.summary,
              resolveMentionLabel
            ),
          }
        : replyMessage
          ? {
              author: getMessageAuthor(
                replyMessage,
                conversation,
                currentUser,
                usersById,
                appsById
              ),
              summary: formatClientMessageBodySummary(
                replyMessage.body,
                resolveMentionLabel
              ),
            }
          : undefined,
      role,
      sender:
        message.sender.type === "system"
          ? null
          : { id: message.sender.id, type: message.sender.type },
      topic: message.topic
        ? {
            archived: message.topic.archived,
            conversationId: message.topic.conversationId,
            recentReplies: message.topic.recentReplies.map((reply) => ({
              author: getTopicReplyAuthor(
                reply.sender,
                conversation,
                currentUser,
                usersById,
                appsById
              ),
              avatar: getTopicReplyAvatar(
                reply.sender,
                conversation,
                currentUser,
                usersById,
                appsById
              ),
              id: reply.id,
              summary: formatMentionTemplateText(
                reply.summary,
                resolveMentionLabel
              ),
              time: formatMessageTime(reply.createdAt),
            })),
          }
        : undefined,
    }
  })
}

export function createMessageMentionLabelResolver({
  contacts,
  conversation,
  currentUser,
}: {
  contacts: ClientContacts
  conversation: ClientConversation
  currentUser: ClientUser
}): MessageMentionLabelResolver {
  const userLabels = new Map(
    contacts.users.map(
      (user) => [user.id.toLowerCase(), getContactDisplayName(user)] as const
    )
  )
  const appLabels = new Map(
    contacts.apps.map((app) => [app.id.toLowerCase(), app.name] as const)
  )
  userLabels.set(currentUser.id.toLowerCase(), getContactDisplayName(currentUser))

  for (const member of conversation.members ?? []) {
    const labels = member.type === "app" ? appLabels : userLabels
    if (!labels.has(member.id.toLowerCase())) {
      labels.set(
        member.id.toLowerCase(),
        member.nickname.trim() || member.name.trim()
      )
    }
  }

  return ({ id, type }) => {
    if (type === "all") return "所有人"
    return (type === "app" ? appLabels : userLabels).get(id.toLowerCase())
  }
}

export function formatClientMessageBodySummary(
  body: ClientMessageBody,
  resolveMentionLabel: MessageMentionLabelResolver
): string {
  if (body.type === "text") {
    return formatMentionTemplateText(body.content, resolveMentionLabel)
  }
  if (body.type === "markdown") {
    return formatMentionTemplateText(
      formatMarkdownAsPlainText(body.content),
      resolveMentionLabel
    )
  }
  if (body.type === "link") return `[链接] ${body.title || body.url}`
  if (body.type === "card") return `[卡片] ${body.title}`
  if (body.type === "chart") return `[图表] ${body.title}`
  if (body.type === "file") return `[文件] ${body.name}`
  if (body.type === "image") return "[图片]"
  if (body.type === "voice") {
    const summary = `[语音] ${formatVoiceDuration(body.durationMS)}`
    return body.transcript ? `${summary} - ${body.transcript}` : summary
  }
  if (body.type === "forward_bundle") {
    return `[聊天记录] ${body.itemCount} 条`
  }
  if (body.type === "revoked") return "该消息已被撤回"
  if (body.type === "unsupported") return "暂不支持查看该消息"
  if (body.event === "message_revoked") {
    return `${body.actor.displayName} 撤回了一条消息`
  }
  if (body.event === "topic_closed") {
    return `${body.actor.displayName} 已将话题关闭`
  }
  if (body.event === "group_avatar_updated") {
    return `${body.actor.displayName} 修改了群头像`
  }
  if (body.event === "group_visibility_changed") {
    return body.visibility === "public"
      ? `${body.actor.displayName} 将当前群设置为公开群`
      : `${body.actor.displayName} 将当前群设为私有群`
  }
  if (body.event === "group_member_joined") {
    return `${body.actor.displayName} 加入群聊`
  }
  if (body.event === "group_member_left") {
    return `${body.actor.displayName} 已退出群聊`
  }
  if (body.event === "group_member_removed") {
    return `${body.actor.displayName} 已将 ${body.target.displayName} 移出群聊`
  }
  if (body.event === "group_name_updated") {
    return `${body.actor.displayName} 修改群聊名称为 ${body.name}`
  }
  if (body.event === "group_members_invited") {
    return `${body.inviter.displayName} 邀请 ${body.invitees
      .map((invitee) => invitee.displayName)
      .join("、")} 加入群聊`
  }

  return "系统消息"
}

export function collectMessageResources(messages: ClientMessage[]) {
  const resources = new Map<string, AttachmentResourceReference>()

  for (const message of messages) {
    collectBodyResources(message.body, resources)
  }

  return Array.from(resources.values()).sort((left, right) =>
    left.fileId.localeCompare(right.fileId)
  )
}

export function formatMarkdownAsPlainText(content: string) {
  return content
    .replace(/```[\s\S]*?```/g, (block) =>
      block.replace(/^```[^\n]*\n?/, "").replace(/```$/, "").trim()
    )
    .replace(/`([^`]*)`/g, "$1")
    .replace(/!\[([^\]]*)]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^>\s?/gm, "")
    .replace(/^\s*[-*+]\s+/gm, "• ")
    .replace(/^\s*\d+[.)]\s+/gm, "")
    .replace(/[*_~]+/g, "")
    .trim()
}

export function formatFileSize(sizeBytes: number) {
  if (sizeBytes < 1_024) return `${sizeBytes} B`
  if (sizeBytes < 1_024 * 1_024) return `${(sizeBytes / 1_024).toFixed(1)} KB`
  if (sizeBytes < 1_024 * 1_024 * 1_024) {
    return `${(sizeBytes / (1_024 * 1_024)).toFixed(1)} MB`
  }
  return `${(sizeBytes / (1_024 * 1_024 * 1_024)).toFixed(1)} GB`
}

export function formatVoiceDuration(durationMs: number) {
  const totalSeconds = Math.max(1, Math.ceil(durationMs / 1_000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
}

function collectBodyResources(
  body: ClientMessageBody,
  resources: Map<string, AttachmentResourceReference>
) {
  if (body.type === "file") {
    resources.set(body.fileId, {
      expectedSizeBytes: body.sizeBytes,
      fileId: body.fileId,
      fileName: body.name,
      kind: "file",
      type: "attachment",
    })
  } else if (body.type === "image") {
    resources.set(body.fileId, {
      fileId: body.fileId,
      fileName: "image.webp",
      kind: "image",
      mimeType: "image/webp",
      type: "attachment",
    })
  } else if (body.type === "voice") {
    resources.set(body.fileId, {
      expectedSizeBytes: body.sizeBytes,
      fileId: body.fileId,
      fileName: "voice.webm",
      kind: "voice",
      mimeType: body.contentType,
      type: "attachment",
    })
  }
}

function formatMessageTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "" : messageTimeFormatter.format(date)
}

const messageTimeMarkerThresholdMs = 60 * 60 * 1_000

export function shouldShowMessageTimeMarker(
  olderCreatedAt: string,
  newerCreatedAt: string
) {
  const olderTime = new Date(olderCreatedAt).getTime()
  const newerTime = new Date(newerCreatedAt).getTime()

  if (Number.isNaN(olderTime) || Number.isNaN(newerTime)) return false
  return newerTime - olderTime > messageTimeMarkerThresholdMs
}

export function formatMessageTimeMarker(value: string, now = new Date()) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""

  const time = `${padMessageTimePart(date.getHours())}:${padMessageTimePart(date.getMinutes())}`
  if (isSameLocalMessageDay(date, now)) return time

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

function getMessageAuthor(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: ClientUser,
  usersById: ReadonlyMap<string, ClientContacts["users"][number]>,
  appsById: ReadonlyMap<string, ClientContacts["apps"][number]>
) {
  if (message.sender.type === "system") return "系统"
  if (message.sender.type === "app") {
    return (
      appsById.get(message.sender.id.toLowerCase())?.name ||
      conversation.members?.find((member) => member.id === message.sender.id)?.name ||
      (conversation.type === "app" ? conversation.name : "应用")
    )
  }
  if (message.sender.id === currentUser.id) return getContactDisplayName(currentUser)
  const user = usersById.get(message.sender.id.toLowerCase())
  if (user) return getContactDisplayName(user)
  return conversation.type === "direct" ? conversation.name : "成员"
}

function getMessageAvatar(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: ClientUser,
  usersById: ReadonlyMap<string, ClientContacts["users"][number]>,
  appsById: ReadonlyMap<string, ClientContacts["apps"][number]>
) {
  if (message.sender.type === "system") return ""
  if (message.sender.type === "app") {
    return (
      appsById.get(message.sender.id.toLowerCase())?.avatar ||
      conversation.members?.find((member) => member.id === message.sender.id)?.avatar ||
      (conversation.type === "app" ? conversation.avatar : "")
    )
  }
  if (message.sender.id === currentUser.id) return currentUser.avatar
  return (
    usersById.get(message.sender.id.toLowerCase())?.avatar ||
    (conversation.type === "direct" ? conversation.avatar : "")
  )
}

function getReplyAuthor(
  sender: NonNullable<ClientMessage["replyTo"]>["sender"],
  conversation: ClientConversation,
  currentUser: ClientUser,
  usersById: ReadonlyMap<string, ClientContacts["users"][number]>,
  appsById: ReadonlyMap<string, ClientContacts["apps"][number]>
) {
  if (sender.type === "system") return "系统"
  if (sender.type === "app") {
    return sender.name || appsById.get(sender.id.toLowerCase())?.name || "应用"
  }
  if (sender.id === currentUser.id) return getContactDisplayName(currentUser)
  const user = usersById.get(sender.id.toLowerCase())
  return user
    ? getContactDisplayName(user)
    : sender.name || (conversation.type === "direct" ? conversation.name : "成员")
}

function getTopicReplyAuthor(
  sender: NonNullable<ClientMessage["topic"]>["recentReplies"][number]["sender"],
  conversation: ClientConversation,
  currentUser: ClientUser,
  usersById: ReadonlyMap<string, ClientContacts["users"][number]>,
  appsById: ReadonlyMap<string, ClientContacts["apps"][number]>
) {
  if (sender.type === "app") {
    return (
      appsById.get(sender.id.toLowerCase())?.name ||
      conversation.members?.find((member) => member.id === sender.id)?.name ||
      "应用"
    )
  }
  if (sender.id === currentUser.id) return getContactDisplayName(currentUser)
  const user = usersById.get(sender.id.toLowerCase())
  if (user) return getContactDisplayName(user)
  return conversation.type === "direct" ? conversation.name : "成员"
}

function getTopicReplyAvatar(
  sender: NonNullable<ClientMessage["topic"]>["recentReplies"][number]["sender"],
  conversation: ClientConversation,
  currentUser: ClientUser,
  usersById: ReadonlyMap<string, ClientContacts["users"][number]>,
  appsById: ReadonlyMap<string, ClientContacts["apps"][number]>
) {
  if (sender.type === "app") {
    return (
      appsById.get(sender.id.toLowerCase())?.avatar ||
      conversation.members?.find((member) => member.id === sender.id)?.avatar ||
      ""
    )
  }
  if (sender.id === currentUser.id) return currentUser.avatar
  return (
    usersById.get(sender.id.toLowerCase())?.avatar ||
    (conversation.type === "direct" ? conversation.avatar : "")
  )
}
