import type {
  ClientContacts,
  ClientConversation,
  ClientMessage,
  ClientMessageBody,
  ClientUser,
} from "@/data/models"
import { getContactDisplayName } from "@/features/contacts/contact-directory-model"

export type MessageMentionLabelResolver = (target: {
  id: string
  type: "all" | "app" | "user"
}) => string | undefined

export type PresentedMessage = {
  author: string
  avatar: string
  body: ClientMessageBody
  delegatedByName: string
  id: string
  replyTo?: {
    author: string
    summary: string
  }
  role: "me" | "other" | "system"
  time: string
}

const mentionTokenPattern =
  /\{\(@(?:(user)\/(all)|(user|app)\/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}/g
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
      message.sender.type === "system"
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
      delegatedByName: message.delegatedBy?.name ?? "",
      id: message.id,
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
      time: formatMessageTime(message.createdAt),
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

export function formatMentionTemplateText(
  content: string,
  resolveLabel: MessageMentionLabelResolver
) {
  return content.replace(
    mentionTokenPattern,
    (
      token,
      _allType: string | undefined,
      allId: string | undefined,
      targetType: "app" | "user" | undefined,
      targetId: string | undefined
    ) => {
      if (allId === "all") return "@所有人"
      if (!targetType || !targetId) return token
      const label = resolveLabel({ id: targetId, type: targetType })?.trim()
      return label ? `@${label}` : targetType === "app" ? "@应用" : "@用户"
    }
  )
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

export function collectMessageFileIds(messages: ClientMessage[]) {
  const fileIds = new Set<string>()

  for (const message of messages) {
    collectBodyFileIds(message.body, fileIds)
  }

  return Array.from(fileIds)
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

function collectBodyFileIds(body: ClientMessageBody, fileIds: Set<string>) {
  if (body.type === "file" || body.type === "image" || body.type === "voice") {
    fileIds.add(body.fileId)
  } else if (body.type === "forward_bundle") {
    for (const item of body.items) collectBodyFileIds(item.body, fileIds)
  }
}

function formatMessageTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "" : messageTimeFormatter.format(date)
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
