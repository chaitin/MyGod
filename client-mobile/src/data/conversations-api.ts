import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import type {
  ClientConversation,
  ClientConversationMember,
  ClientConversationProject,
} from "@/data/models"

type ConversationProjectResponse = {
  avatar?: string
  description?: string
  id?: string
  name?: string
}

type ConversationMemberResponse = {
  avatar?: string
  email?: string
  id?: string
  name?: string
  nickname?: string
  phone?: string
  role?: string
  type?: string
}

type ConversationResponse = {
  avatar?: string
  created_at?: string
  id?: string
  last_message_at?: string | null
  last_message_id?: string | null
  last_message_seq?: number
  last_message_summary?: string
  last_mentioned_seq?: number
  last_read_seq?: number
  member_count?: number
  members?: ConversationMemberResponse[]
  name?: string
  projects?: ConversationProjectResponse[]
  type?: string
  unread_count?: number
  visibility?: string
}

type ConversationsResponse = {
  conversations?: ConversationResponse[]
}

export async function fetchConversations(
  serverUrl: string,
  options: { fetcher?: ApiFetch; signal?: AbortSignal } = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<
    ConversationsResponse
  >("/api/client/conversations", {
    errorMessage: "加载会话列表失败",
    method: "GET",
    signal: options.signal,
  })

  if (!data || !Array.isArray(data.conversations)) {
    throw new ApiRequestError("会话列表响应格式不正确")
  }

  return data.conversations.map(normalizeConversation)
}

function normalizeConversation(
  conversation: ConversationResponse
): ClientConversation {
  if (!conversation.created_at || !conversation.id || !conversation.name) {
    throw new ApiRequestError("会话列表响应格式不正确")
  }

  const normalized: ClientConversation = {
    avatar: conversation.avatar ?? "",
    createdAt: conversation.created_at,
    id: conversation.id,
    lastMessageAt: conversation.last_message_at ?? null,
    lastMessageId: conversation.last_message_id ?? null,
    lastMessageSeq: conversation.last_message_seq ?? 0,
    lastMessageSummary: conversation.last_message_summary ?? "",
    lastMentionedSeq: conversation.last_mentioned_seq ?? 0,
    lastReadSeq: conversation.last_read_seq ?? 0,
    memberCount: conversation.member_count ?? 0,
    name: conversation.name,
    type: normalizeConversationType(conversation.type),
    unreadCount: conversation.unread_count ?? 0,
    visibility: conversation.visibility === "public" ? "public" : "private",
  }

  if (conversation.members) {
    normalized.members = conversation.members.map(normalizeConversationMember)
  }

  if (conversation.projects) {
    normalized.projects = conversation.projects.map(
      normalizeConversationProject
    )
  }

  return normalized
}

function normalizeConversationMember(
  member: ConversationMemberResponse
): ClientConversationMember {
  const type = member.type === "app" ? "app" : "user"

  if (!member.id || !member.name || (type === "user" && !member.email)) {
    throw new ApiRequestError("会话成员响应格式不正确")
  }

  return {
    avatar: member.avatar ?? "",
    email: member.email ?? "",
    id: member.id,
    name: member.name,
    nickname: member.nickname ?? "",
    phone: member.phone ?? "",
    role:
      member.role === "owner" || member.role === "admin"
        ? member.role
        : "member",
    type,
  }
}

function normalizeConversationProject(
  project: ConversationProjectResponse
): ClientConversationProject {
  if (!project.id || !project.name) {
    throw new ApiRequestError("会话关联项目响应格式不正确")
  }

  return {
    avatar: project.avatar ?? "",
    description: project.description ?? "",
    id: project.id,
    name: project.name,
  }
}

function normalizeConversationType(type: string | undefined) {
  if (type === "direct" || type === "app") {
    return type
  }

  return "group"
}
