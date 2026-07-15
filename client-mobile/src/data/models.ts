export type AppInfo = {
  appName: string
  authenticated: boolean
  organizationName: string
}

export type AuthenticatedUser = {
  email: string
  id: string
  name: string
}

export type ClientUser = {
  avatar: string
  createdAt: string
  email: string
  id: string
  lastOnlineAt: string | null
  name: string
  nickname: string
  phone: string
  status: "active" | "disabled"
}

export type ContactUser = {
  avatar: string
  email: string
  id: string
  lastOnlineAt: string | null
  name: string
  nickname: string
  online: boolean
  phone: string
  type: "user"
}

export type ContactApp = {
  avatar: string
  description: string
  id: string
  name: string
  online: boolean
  type: "app"
}

export type ContactGroupAvatarMember = {
  avatar: string
  name: string
  nickname: string
  role: "owner" | "admin" | "member"
}

export type ContactGroup = {
  avatar: string
  avatarMembers: ContactGroupAvatarMember[]
  id: string
  joined: boolean
  memberCount: number
  name: string
  type: "group"
  visibility: "private" | "public"
}

export type ClientContacts = {
  apps: ContactApp[]
  groups: ContactGroup[]
  users: ContactUser[]
}

export type ClientConversationProject = {
  avatar: string
  description: string
  id: string
  name: string
}

export type ClientConversationMember = {
  avatar: string
  email: string
  id: string
  name: string
  nickname: string
  phone: string
  role: "owner" | "admin" | "member"
  type: "user" | "app"
}

export type ClientConversation = {
  avatar: string
  createdAt: string
  id: string
  lastMessageAt: string | null
  lastMessageId: string | null
  lastMessageSeq: number
  lastMessageSummary: string
  lastMentionedSeq: number
  lastReadSeq: number
  memberCount: number
  members?: ClientConversationMember[]
  name: string
  projects?: ClientConversationProject[]
  type: "direct" | "group" | "app"
  unreadCount: number
  visibility: "private" | "public"
}

export type ClientMessageSender = {
  id: string
  type: "user" | "app" | "system"
}

export type ClientMessageReplyTo = {
  id: string
  sender: {
    id: string
    name: string
    type: "user" | "app" | "system"
  }
  seq: number
  summary: string
}

export type ClientTextMessageBody = {
  content: string
  type: "text"
}

export type ClientMarkdownMessageBody = {
  content: string
  type: "markdown"
}

export type ClientLinkMessageBody = {
  title: string
  type: "link"
  url: string
}

export type ClientCardMessageBody = {
  description: string
  title: string
  type: "card"
  url: string
}

export type ClientChartMessageBody = {
  chartType: "line" | "bar" | "pie" | "radar"
  data: Record<string, unknown>
  description: string
  title: string
  type: "chart"
}

export type ClientFileMessageBody = {
  fileId: string
  name: string
  sizeBytes: number
  type: "file"
}

export type ClientImageMessageBody = {
  fileId: string
  height?: number
  type: "image"
  width?: number
}

export type ClientVoiceMessageBody = {
  contentType: string
  durationMS: number
  fileId: string
  sizeBytes: number
  transcript: string
  type: "voice"
}

export type ClientForwardableMessageBody =
  | ClientTextMessageBody
  | ClientMarkdownMessageBody
  | ClientLinkMessageBody
  | ClientCardMessageBody
  | ClientChartMessageBody
  | ClientFileMessageBody
  | ClientImageMessageBody
  | ClientVoiceMessageBody
  | ClientForwardBundleMessageBody

export type ClientForwardBundleMessageBody = {
  itemCount: number
  items: {
    body: ClientForwardableMessageBody
    senderName: string
    senderType: "user" | "app"
    sentAt: string
    summary: string
  }[]
  type: "forward_bundle"
}

export type ClientSystemEventUserRef = {
  displayName: string
  id: string
}

export type ClientSystemEventMessageBody =
  | {
      event: "group_members_invited"
      invitees: ClientSystemEventUserRef[]
      inviter: ClientSystemEventUserRef
      type: "system_event"
    }
  | {
      actor: ClientSystemEventUserRef
      event:
        | "group_avatar_updated"
        | "group_member_joined"
        | "group_member_left"
        | "message_revoked"
      type: "system_event"
    }
  | {
      actor: ClientSystemEventUserRef
      event: "group_visibility_changed"
      type: "system_event"
      visibility: "private" | "public"
    }
  | {
      actor: ClientSystemEventUserRef
      event: "group_member_removed"
      target: ClientSystemEventUserRef
      type: "system_event"
    }
  | {
      actor: ClientSystemEventUserRef
      event: "group_name_updated"
      name: string
      type: "system_event"
    }

export type ClientMessageBody =
  | ClientForwardableMessageBody
  | ClientSystemEventMessageBody
  | { type: "revoked" }
  | { type: "unsupported" }

export type ClientMessage = {
  body: ClientMessageBody
  clientMessageId: string
  conversationId: string
  createdAt: string
  delegatedBy?: {
    id: string
    name: string
    type: "user" | "app"
  }
  id: string
  replyTo?: ClientMessageReplyTo
  replyToMessageId?: string
  revokedAt?: string
  revokedByUserId?: string
  sender: ClientMessageSender
  seq: number
}

export type ClientMessagePage = {
  hasMoreAfter: boolean
  hasMoreBefore: boolean
  limit: number
  newestSeq: number
  oldestSeq: number
}

export type ClientMessageList = {
  messages: ClientMessage[]
  page: ClientMessagePage
}

export type TemporaryFileReadUrl = {
  expiresAt: string
  fileId: string
  url: string
}
