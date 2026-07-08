type ClientDataFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type ClientDataSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type ClientDataErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type ClientUserResponse = {
  avatar?: string
  created_at?: string
  email?: string
  id?: string
  last_online_at?: string | null
  name?: string
  nickname?: string
  phone?: string
  status?: string
}

type CurrentClientUserResponse = {
  user?: ClientUserResponse
}

type UploadCurrentClientAvatarResponse = {
  user?: ClientUserResponse
}

type UpdateCurrentClientUserInput = {
  avatar?: string
  nickname?: string
}

type ContactUserResponse = {
  avatar?: string
  email?: string
  id?: string
  last_online_at?: string | null
  name?: string
  nickname?: string
  online?: boolean
  phone?: string
  type?: string
}

type ListClientContactsResponse = {
  apps?: ContactAppResponse[]
  groups?: ContactGroupResponse[]
  users?: ContactUserResponse[]
}

type ContactAppResponse = {
  avatar?: string
  description?: string
  id?: string
  name?: string
  online?: boolean
  type?: string
}

type ContactGroupResponse = {
  avatar?: string
  id?: string
  joined?: boolean
  member_count?: number
  name?: string
  type?: string
  visibility?: string
}

type ConversationResponse = {
  avatar?: string
  created_at?: string
  id?: string
  last_message_at?: string | null
  last_message_id?: string | null
  last_message_seq?: number
  last_message_summary?: string
  last_read_seq?: number
  member_count?: number
  members?: ConversationMemberResponse[]
  name?: string
  type?: string
  unread_count?: number
  visibility?: string
}

type ConversationMemberResponse = {
  avatar?: string
  email?: string
  id?: string
  name?: string
  nickname?: string
  phone?: string
  role?: string
}

type ListClientConversationsResponse = {
  conversations?: ConversationResponse[]
}

type CreateDirectConversationResponse = {
  conversation?: ConversationResponse
  created?: boolean
}

type CreateAppConversationResponse = {
  conversation?: ConversationResponse
  created?: boolean
}

type CreateGroupConversationResponse = {
  conversation?: ConversationResponse
}

type AddGroupConversationMembersResponse = {
  conversation?: ConversationResponse
  message?: MessageResponse | null
}

type GroupConversationActionResponse = {
  conversation?: ConversationResponse
  message?: MessageResponse | null
}

type LeaveGroupConversationResponse = {
  conversation_id?: string
  message?: MessageResponse
}

type UploadGroupConversationAvatarResponse = {
  conversation?: ConversationResponse
  message?: MessageResponse
}

type MessageSenderResponse = {
  id?: string
  type?: string
}

type TextMessageBodyResponse = {
  content?: string
  type?: "text"
}

type MarkdownMessageBodyResponse = {
  content?: string
  type?: "markdown"
}

type LinkMessageBodyResponse = {
  title?: string
  type?: "link"
  url?: string
}

type FileMessageBodyResponse = {
  file_id?: string
  name?: string
  size_bytes?: number
  type?: "file"
}

type ImageMessageBodyResponse = {
  file_id?: string
  type?: "image"
}

type SystemEventUserRefResponse = {
  display_name?: string
  id?: string
}

type GroupMembersInvitedSystemEventBodyResponse = {
  event?: "group_members_invited"
  invitees?: SystemEventUserRefResponse[]
  inviter?: SystemEventUserRefResponse
  type?: "system_event"
}

type GroupAvatarUpdatedSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: "group_avatar_updated"
  type?: "system_event"
}

type GroupVisibilityChangedSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: "group_visibility_changed"
  type?: "system_event"
  visibility?: string
}

type GroupMemberJoinedSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: "group_member_joined"
  type?: "system_event"
}

type GroupMemberLeftSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: "group_member_left"
  type?: "system_event"
}

type GroupNameUpdatedSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: "group_name_updated"
  name?: string
  type?: "system_event"
}

type MessageBodyResponse =
  | TextMessageBodyResponse
  | MarkdownMessageBodyResponse
  | LinkMessageBodyResponse
  | FileMessageBodyResponse
  | ImageMessageBodyResponse
  | GroupMembersInvitedSystemEventBodyResponse
  | GroupAvatarUpdatedSystemEventBodyResponse
  | GroupVisibilityChangedSystemEventBodyResponse
  | GroupMemberJoinedSystemEventBodyResponse
  | GroupMemberLeftSystemEventBodyResponse
  | GroupNameUpdatedSystemEventBodyResponse

type MessageResponse = {
  body?: MessageBodyResponse
  client_message_id?: string
  conversation_id?: string
  created_at?: string
  id?: string
  sender?: MessageSenderResponse
  seq?: number
}

type MessagePageResponse = {
  has_more_after?: boolean
  has_more_before?: boolean
  limit?: number
  newest_seq?: number
  oldest_seq?: number
}

type ListConversationMessagesResponse = {
  messages?: MessageResponse[]
  page?: MessagePageResponse
}

type CreateMessageResponse = {
  message?: MessageResponse
}

type MarkConversationReadResponse = {
  conversation_id?: string
  last_read_seq?: number
  unread_count?: number
}

type MessageCreatedEventPayloadResponse = {
  message?: MessageResponse
}

type TemporaryFileReadURLResponse = {
  expires_at?: string
  file_id?: string
  url?: string
}

type ReadTemporaryFileURLsResponse = {
  urls?: TemporaryFileReadURLResponse[]
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

export type ContactGroup = {
  avatar: string
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

export type ClientConversation = {
  avatar: string
  createdAt: string
  id: string
  lastMessageAt: string | null
  lastMessageId: string | null
  lastMessageSeq: number
  lastMessageSummary: string
  lastReadSeq: number
  memberCount: number
  members?: ClientConversationMember[]
  name: string
  type: "direct" | "group" | "app"
  unreadCount: number
  visibility: "private" | "public"
}

export type ClientConversationMember = {
  avatar: string
  email: string
  id: string
  name: string
  nickname: string
  phone: string
  role: "owner" | "admin" | "member"
  type: "user"
}

export type ClientMessageSender = {
  id: string
  type: "user" | "app" | "system"
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

export type ClientFileMessageBody = {
  fileId: string
  name: string
  sizeBytes: number
  type: "file"
}

export type ClientImageMessageBody = {
  fileId: string
  type: "image"
}

export type ClientSystemEventUserRef = {
  displayName: string
  id: string
}

export type ClientGroupMembersInvitedSystemEventBody = {
  event: "group_members_invited"
  invitees: ClientSystemEventUserRef[]
  inviter: ClientSystemEventUserRef
  type: "system_event"
}

export type ClientGroupAvatarUpdatedSystemEventBody = {
  actor: ClientSystemEventUserRef
  event: "group_avatar_updated"
  type: "system_event"
}

export type ClientGroupVisibilityChangedSystemEventBody = {
  actor: ClientSystemEventUserRef
  event: "group_visibility_changed"
  type: "system_event"
  visibility: "private" | "public"
}

export type ClientGroupMemberJoinedSystemEventBody = {
  actor: ClientSystemEventUserRef
  event: "group_member_joined"
  type: "system_event"
}

export type ClientGroupMemberLeftSystemEventBody = {
  actor: ClientSystemEventUserRef
  event: "group_member_left"
  type: "system_event"
}

export type ClientGroupNameUpdatedSystemEventBody = {
  actor: ClientSystemEventUserRef
  event: "group_name_updated"
  name: string
  type: "system_event"
}

export type ClientMessageBody =
  | ClientTextMessageBody
  | ClientMarkdownMessageBody
  | ClientLinkMessageBody
  | ClientFileMessageBody
  | ClientImageMessageBody
  | ClientGroupMembersInvitedSystemEventBody
  | ClientGroupAvatarUpdatedSystemEventBody
  | ClientGroupVisibilityChangedSystemEventBody
  | ClientGroupMemberJoinedSystemEventBody
  | ClientGroupMemberLeftSystemEventBody
  | ClientGroupNameUpdatedSystemEventBody

export type ClientMessage = {
  body: ClientMessageBody
  clientMessageId: string
  conversationId: string
  createdAt: string
  id: string
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

export type ListConversationMessagesOptions = {
  afterSeq?: number
  beforeSeq?: number
  limit?: number
}

export type SendConversationTextMessageInput = {
  clientMessageId: string
  content: string
}

export type SendConversationMarkdownMessageInput = {
  clientMessageId: string
  content: string
}

export type SendConversationLinkMessageInput = {
  clientMessageId: string
  url: string
}

export type SendConversationFileMessageInput = {
  clientMessageId: string
  file: File
}

export type SendConversationImageMessageInput = {
  clientMessageId: string
  image: File
}

export type TemporaryFileReadURL = {
  expiresAt: string
  fileId: string
  url: string
}

const temporaryFileReadURLCache = new Map<string, TemporaryFileReadURL>()
const temporaryFileReadURLCacheSafetyWindowMs = 5 * 60 * 1000

export type MarkConversationReadOptions = {
  upToSeq?: number
}

export type MarkConversationReadResult = {
  conversationId: string
  lastReadSeq: number
  unreadCount: number
}

export type CreateGroupConversationInput = {
  memberIds: string[]
  name: string
}

export type AddGroupConversationMembersInput = {
  memberIds: string[]
}

export type UpdateGroupConversationNameInput = {
  name: string
}

export type AddGroupConversationMembersResult = {
  conversation: ClientConversation
  message: ClientMessage | null
}

export type GroupConversationActionResult = {
  conversation: ClientConversation
  message: ClientMessage | null
}

export type UploadGroupConversationAvatarResult = {
  conversation: ClientConversation
  message: ClientMessage
}

export type LeaveGroupConversationResult = {
  conversationId: string
  message: ClientMessage
}

export class ClientDataRequestError extends Error {
  code?: string
  status?: number

  constructor(message: string, options?: { code?: string; status?: number }) {
    super(message)
    this.name = "ClientDataRequestError"
    this.code = options?.code
    this.status = options?.status
  }
}

export async function getCurrentClientUser(fetcher: ClientDataFetch = fetch) {
  const response = await fetcher("/api/client/me", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<CurrentClientUserResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载当前用户失败")
  }

  const user = (
    payload as ClientDataSuccessEnvelope<CurrentClientUserResponse> | undefined
  )?.data?.user

  return normalizeClientUser(user)
}

export async function updateCurrentClientUser(
  input: UpdateCurrentClientUserInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher("/api/client/me", {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "PATCH",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<CurrentClientUserResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "更新个人信息失败")
  }

  const user = (
    payload as ClientDataSuccessEnvelope<CurrentClientUserResponse> | undefined
  )?.data?.user

  return normalizeClientUser(user)
}

export async function uploadCurrentClientAvatar(
  file: File,
  fetcher: ClientDataFetch = fetch
) {
  const formData = new FormData()
  formData.set("file", file)

  const response = await fetcher("/api/client/me/avatar", {
    body: formData,
    credentials: "include",
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<UploadCurrentClientAvatarResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "上传头像失败")
  }

  const user = (
    payload as
      ClientDataSuccessEnvelope<UploadCurrentClientAvatarResponse> | undefined
  )?.data?.user

  return normalizeClientUser(user)
}

export async function listClientContacts(fetcher: ClientDataFetch = fetch) {
  const response = await fetcher("/api/client/contacts", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListClientContactsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载通讯录失败")
  }

  const data = (
    payload as ClientDataSuccessEnvelope<ListClientContactsResponse> | undefined
  )?.data

  if (
    !data ||
    !Array.isArray(data.apps) ||
    !Array.isArray(data.groups) ||
    !Array.isArray(data.users)
  ) {
    throw new ClientDataRequestError("通讯录响应格式不正确")
  }

  return {
    apps: data.apps.map(normalizeContactApp),
    groups: data.groups.map(normalizeContactGroup),
    users: data.users.map(normalizeContactUser),
  }
}

export async function listClientConversations(
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher("/api/client/conversations", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListClientConversationsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载会话列表失败")
  }

  const conversations = (
    payload as
      ClientDataSuccessEnvelope<ListClientConversationsResponse> | undefined
  )?.data?.conversations

  if (!conversations) {
    throw new ClientDataRequestError("会话列表响应格式不正确")
  }

  return conversations.map(normalizeConversation)
}

export async function createDirectConversation(
  userId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher("/api/client/conversations/direct", {
    body: JSON.stringify({
      user_id: userId,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<CreateDirectConversationResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "创建一对一会话失败")
  }

  const conversation = (
    payload as
      ClientDataSuccessEnvelope<CreateDirectConversationResponse> | undefined
  )?.data?.conversation

  return normalizeConversation(conversation)
}

export async function openAppConversation(
  appId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher("/api/client/conversations/apps", {
    body: JSON.stringify({
      app_id: appId,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<CreateAppConversationResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "创建应用会话失败")
  }

  const conversation = (
    payload as
      ClientDataSuccessEnvelope<CreateAppConversationResponse> | undefined
  )?.data?.conversation

  return normalizeConversation(conversation)
}

export async function createGroupConversation(
  input: CreateGroupConversationInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher("/api/client/conversations/groups", {
    body: JSON.stringify({
      member_ids: input.memberIds,
      name: input.name,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<CreateGroupConversationResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "创建群聊失败")
  }

  const conversation = (
    payload as
      ClientDataSuccessEnvelope<CreateGroupConversationResponse> | undefined
  )?.data?.conversation

  return normalizeConversation(conversation)
}

export async function joinGroupConversation(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<GroupConversationActionResult> {
  return postGroupConversationAction(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/join`,
    "加入群聊失败",
    fetcher
  )
}

export async function setGroupConversationPublic(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<GroupConversationActionResult> {
  return postGroupConversationAction(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/public`,
    "设置公开群失败",
    fetcher
  )
}

export async function setGroupConversationPrivate(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<GroupConversationActionResult> {
  return postGroupConversationAction(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/private`,
    "取消公开群失败",
    fetcher
  )
}

export async function updateGroupConversationName(
  conversationId: string,
  input: UpdateGroupConversationNameInput,
  fetcher: ClientDataFetch = fetch
): Promise<GroupConversationActionResult> {
  const response = await fetcher(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/name`,
    {
      body: JSON.stringify({
        name: input.name,
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<GroupConversationActionResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "修改群聊名称失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<GroupConversationActionResponse> | undefined
  )?.data

  if (!data?.conversation) {
    throw new ClientDataRequestError("修改群聊名称响应格式不正确")
  }

  return {
    conversation: normalizeConversation(data.conversation),
    message: data.message ? normalizeMessage(data.message) : null,
  }
}

export async function leaveGroupConversation(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<LeaveGroupConversationResult> {
  const response = await fetcher(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/leave`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<LeaveGroupConversationResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "退出群聊失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<LeaveGroupConversationResponse> | undefined
  )?.data

  if (!data?.conversation_id || !data.message) {
    throw new ClientDataRequestError("退出群聊响应格式不正确")
  }

  return {
    conversationId: data.conversation_id,
    message: normalizeMessage(data.message),
  }
}

async function postGroupConversationAction(
  url: string,
  fallbackMessage: string,
  fetcher: ClientDataFetch
): Promise<GroupConversationActionResult> {
  const response = await fetcher(url, {
    credentials: "include",
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<GroupConversationActionResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, fallbackMessage)
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<GroupConversationActionResponse> | undefined
  )?.data

  if (!data?.conversation) {
    throw new ClientDataRequestError("群聊操作响应格式不正确")
  }

  return {
    conversation: normalizeConversation(data.conversation),
    message: data.message ? normalizeMessage(data.message) : null,
  }
}

export async function addGroupConversationMembers(
  conversationId: string,
  input: AddGroupConversationMembersInput,
  fetcher: ClientDataFetch = fetch
): Promise<AddGroupConversationMembersResult> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/members`,
    {
      body: JSON.stringify({
        member_ids: input.memberIds,
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<AddGroupConversationMembersResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "添加群聊成员失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<AddGroupConversationMembersResponse> | undefined
  )?.data

  if (!data?.conversation) {
    throw new ClientDataRequestError("添加群聊成员响应格式不正确")
  }

  return {
    conversation: normalizeConversation(data.conversation),
    message: data.message ? normalizeMessage(data.message) : null,
  }
}

export async function uploadGroupConversationAvatar(
  conversationId: string,
  file: File,
  fetcher: ClientDataFetch = fetch
): Promise<UploadGroupConversationAvatarResult> {
  const formData = new FormData()
  formData.set("file", file)

  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/avatar`,
    {
      body: formData,
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<UploadGroupConversationAvatarResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "上传群头像失败")
  }

  const data = (
    payload as
      | ClientDataSuccessEnvelope<UploadGroupConversationAvatarResponse>
      | undefined
  )?.data

  if (!data?.conversation || !data.message) {
    throw new ClientDataRequestError("上传群头像响应格式不正确")
  }

  return {
    conversation: normalizeConversation(data.conversation),
    message: normalizeMessage(data.message),
  }
}

export async function listConversationMessages(
  conversationId: string,
  options: ListConversationMessagesOptions = {},
  fetcher: ClientDataFetch = fetch
) {
  const searchParams = new URLSearchParams()
  searchParams.set("limit", String(options.limit ?? 20))
  if (options.beforeSeq !== undefined) {
    searchParams.set("before_seq", String(options.beforeSeq))
  }
  if (options.afterSeq !== undefined) {
    searchParams.set("after_seq", String(options.afterSeq))
  }

  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(
      conversationId
    )}/messages?${searchParams.toString()}`,
    {
      credentials: "include",
      method: "GET",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListConversationMessagesResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载消息失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<ListConversationMessagesResponse> | undefined
  )?.data

  if (!data?.messages || !data.page) {
    throw new ClientDataRequestError("消息列表响应格式不正确")
  }

  return {
    messages: data.messages.map(normalizeMessage),
    page: normalizeMessagePage(data.page),
  }
}

export async function sendConversationTextMessage(
  conversationId: string,
  input: SendConversationTextMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        body: {
          type: "text",
          content: input.content,
        },
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送消息失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationMarkdownMessage(
  conversationId: string,
  input: SendConversationMarkdownMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        body: {
          type: "markdown",
          content: input.content,
        },
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送富文本消息失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationLinkMessage(
  conversationId: string,
  input: SendConversationLinkMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        body: {
          type: "link",
          url: input.url,
        },
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送链接失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationFileMessage(
  conversationId: string,
  input: SendConversationFileMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const formData = new FormData()
  formData.set("client_message_id", input.clientMessageId)
  formData.set("file", input.file)

  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/files`,
    {
      body: formData,
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送文件失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationImageMessage(
  conversationId: string,
  input: SendConversationImageMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const formData = new FormData()
  formData.set("client_message_id", input.clientMessageId)
  formData.set("image", input.image)

  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/images`,
    {
      body: formData,
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送图片失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function readTemporaryFileURLs(
  fileIds: string[],
  fetcher: ClientDataFetch = fetch
): Promise<TemporaryFileReadURL[]> {
  if (fileIds.length === 0) {
    return []
  }

  const now = Date.now()
  const urlsByID = new Map<string, TemporaryFileReadURL>()
  const missingFileIDs: string[] = []

  for (const fileId of new Set(fileIds)) {
    const cachedURL = temporaryFileReadURLCache.get(fileId)

    if (cachedURL && isTemporaryFileReadURLFresh(cachedURL, now)) {
      urlsByID.set(fileId, cachedURL)
      continue
    }

    temporaryFileReadURLCache.delete(fileId)
    missingFileIDs.push(fileId)
  }

  if (missingFileIDs.length === 0) {
    return fileIds.map((fileId) => urlsByID.get(fileId)).filter(isDefined)
  }

  const response = await fetcher("/api/client/temporary-files/read-urls", {
    body: JSON.stringify({
      file_ids: missingFileIDs,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ReadTemporaryFileURLsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "获取文件下载地址失败")
  }

  const urls = (
    payload as
      ClientDataSuccessEnvelope<ReadTemporaryFileURLsResponse> | undefined
  )?.data?.urls

  if (!Array.isArray(urls)) {
    throw new ClientDataRequestError("文件下载地址响应格式不正确")
  }

  for (const url of urls.map(normalizeTemporaryFileReadURL)) {
    temporaryFileReadURLCache.set(url.fileId, url)
    urlsByID.set(url.fileId, url)
  }

  const orderedURLs = fileIds.map((fileId) => urlsByID.get(fileId))
  if (orderedURLs.some((url) => !url)) {
    throw new ClientDataRequestError("文件下载地址响应格式不正确")
  }

  return orderedURLs.filter(isDefined)
}

export async function markConversationRead(
  conversationId: string,
  options: MarkConversationReadOptions = {},
  fetcher: ClientDataFetch = fetch
): Promise<MarkConversationReadResult> {
  const body =
    options.upToSeq === undefined
      ? {}
      : {
          up_to_seq: options.upToSeq,
        }
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/read`,
    {
      body: JSON.stringify(body),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<MarkConversationReadResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "标记会话已读失败")
  }

  return normalizeMarkConversationReadResult(
    (
      payload as
        ClientDataSuccessEnvelope<MarkConversationReadResponse> | undefined
    )?.data
  )
}

export function normalizeMessageCreatedEventPayload(
  payload: unknown
): ClientMessage {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("消息推送格式不正确")
  }

  return normalizeMessage(
    (payload as MessageCreatedEventPayloadResponse).message
  )
}

export function formatClientMessageBodySummary(body: ClientMessageBody) {
  if (body.type === "text") {
    return body.content
  }

  if (body.type === "markdown") {
    return formatMarkdownMessageSummary(body.content)
  }

  if (body.type === "link") {
    return `[链接] ${body.title}`
  }

  if (body.type === "file") {
    return `[文件] ${body.name}`
  }

  if (body.type === "image") {
    return "[图片]"
  }

  if (body.event === "group_avatar_updated") {
    return `${body.actor.displayName} 修改了群头像`
  }

  if (body.event === "group_visibility_changed") {
    if (body.visibility === "public") {
      return `${body.actor.displayName} 将当前群设置为公开群`
    }

    return `${body.actor.displayName} 将当前群设为私有群`
  }

  if (body.event === "group_member_joined") {
    return `${body.actor.displayName} 加入群聊`
  }

  if (body.event === "group_member_left") {
    return `${body.actor.displayName} 已退出群聊`
  }

  if (body.event === "group_name_updated") {
    return `${body.actor.displayName} 修改群聊名称为 ${body.name}`
  }

  return `${body.inviter.displayName} 邀请 ${body.invitees
    .map((invitee) => invitee.displayName)
    .join(",")} 加入群聊`
}

export function isClientMessageInitiatedByUser(
  message: ClientMessage,
  userId: string
) {
  if (message.sender.type === "user") {
    return message.sender.id === userId
  }

  if (message.body.type !== "system_event") {
    return false
  }

  if (message.body.event === "group_avatar_updated") {
    return message.body.actor.id === userId
  }

  if (
    message.body.event === "group_visibility_changed" ||
    message.body.event === "group_member_joined" ||
    message.body.event === "group_member_left" ||
    message.body.event === "group_name_updated"
  ) {
    return message.body.actor.id === userId
  }

  return message.body.inviter.id === userId
}

function normalizeClientUser(user: ClientUserResponse | undefined): ClientUser {
  if (!user?.created_at || !user.email || !user.id || !user.name) {
    throw new ClientDataRequestError("当前用户响应格式不正确")
  }

  return {
    avatar: user.avatar ?? "",
    createdAt: user.created_at,
    email: user.email,
    id: user.id,
    lastOnlineAt: user.last_online_at ?? null,
    name: user.name,
    nickname: user.nickname ?? "",
    phone: user.phone ?? "",
    status: user.status === "disabled" ? "disabled" : "active",
  }
}

function normalizeContactUser(
  contact: ContactUserResponse | undefined
): ContactUser {
  if (!contact?.email || !contact.id || !contact.name) {
    throw new ClientDataRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: contact.avatar ?? "",
    email: contact.email,
    id: contact.id,
    lastOnlineAt: contact.last_online_at ?? null,
    name: contact.name,
    nickname: contact.nickname ?? "",
    online: Boolean(contact.online),
    phone: contact.phone ?? "",
    type: "user",
  }
}

function normalizeContactApp(app: ContactAppResponse | undefined): ContactApp {
  if (!app?.id || !app.name) {
    throw new ClientDataRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: app.avatar ?? "",
    description: app.description ?? "",
    id: app.id,
    name: app.name,
    online: Boolean(app.online),
    type: "app",
  }
}

function normalizeContactGroup(
  group: ContactGroupResponse | undefined
): ContactGroup {
  if (!group?.id || !group.name) {
    throw new ClientDataRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: group.avatar ?? "",
    id: group.id,
    joined: Boolean(group.joined),
    memberCount: group.member_count ?? 0,
    name: group.name,
    type: "group",
    visibility: normalizeVisibility(group.visibility),
  }
}

function normalizeConversation(
  conversation: ConversationResponse | undefined
): ClientConversation {
  if (!conversation?.created_at || !conversation.id || !conversation.name) {
    throw new ClientDataRequestError("会话列表响应格式不正确")
  }

  const normalizedConversation: ClientConversation = {
    avatar: conversation.avatar ?? "",
    createdAt: conversation.created_at,
    id: conversation.id,
    lastMessageAt: conversation.last_message_at ?? null,
    lastMessageId: conversation.last_message_id ?? null,
    lastMessageSeq: conversation.last_message_seq ?? 0,
    lastMessageSummary: conversation.last_message_summary ?? "",
    lastReadSeq: conversation.last_read_seq ?? 0,
    memberCount: conversation.member_count ?? 0,
    name: conversation.name,
    type: normalizeConversationType(conversation.type),
    unreadCount: conversation.unread_count ?? 0,
    visibility: normalizeVisibility(conversation.visibility),
  }

  if (conversation.members) {
    normalizedConversation.members = conversation.members.map(
      normalizeConversationMember
    )
  }

  return normalizedConversation
}

function normalizeMarkConversationReadResult(
  result: MarkConversationReadResponse | undefined
): MarkConversationReadResult {
  if (
    !result?.conversation_id ||
    typeof result.last_read_seq !== "number" ||
    typeof result.unread_count !== "number"
  ) {
    throw new ClientDataRequestError("标记会话已读响应格式不正确")
  }

  return {
    conversationId: result.conversation_id,
    lastReadSeq: result.last_read_seq,
    unreadCount: result.unread_count,
  }
}

function normalizeConversationMember(
  member: ConversationMemberResponse | undefined
): ClientConversationMember {
  if (!member?.email || !member.id || !member.name) {
    throw new ClientDataRequestError("会话成员响应格式不正确")
  }

  return {
    avatar: member.avatar ?? "",
    email: member.email,
    id: member.id,
    name: member.name,
    nickname: member.nickname ?? "",
    phone: member.phone ?? "",
    role: normalizeConversationMemberRole(member.role),
    type: "user",
  }
}

function normalizeConversationMemberRole(role: string | undefined) {
  if (role === "owner" || role === "admin") {
    return role
  }

  return "member"
}

function normalizeConversationType(type: string | undefined) {
  if (type === "direct" || type === "app") {
    return type
  }

  return "group"
}

function normalizeMessage(message: MessageResponse | undefined): ClientMessage {
  const senderType = normalizeMessageSenderType(message?.sender?.type)
  const senderId = message?.sender?.id ?? ""
  if (
    !message?.conversation_id ||
    !message.created_at ||
    !message.id ||
    !message.sender ||
    (senderType !== "system" && !senderId) ||
    typeof message.seq !== "number"
  ) {
    throw new ClientDataRequestError("消息响应格式不正确")
  }

  return {
    body: normalizeMessageBody(message.body),
    clientMessageId: message.client_message_id ?? "",
    conversationId: message.conversation_id,
    createdAt: message.created_at,
    id: message.id,
    sender: {
      id: senderId,
      type: senderType,
    },
    seq: message.seq,
  }
}

function normalizeMessageBody(
  body: MessageBodyResponse | undefined
): ClientMessageBody {
  if (body?.type === "text" && typeof body.content === "string") {
    return {
      content: body.content,
      type: "text",
    }
  }

  if (body?.type === "markdown" && typeof body.content === "string") {
    return {
      content: body.content,
      type: "markdown",
    }
  }

  if (
    body?.type === "link" &&
    typeof body.url === "string" &&
    typeof body.title === "string"
  ) {
    return {
      title: body.title,
      type: "link",
      url: body.url,
    }
  }

  if (
    body?.type === "file" &&
    typeof body.file_id === "string" &&
    typeof body.name === "string" &&
    typeof body.size_bytes === "number" &&
    body.size_bytes >= 0
  ) {
    return {
      fileId: body.file_id,
      name: body.name,
      sizeBytes: body.size_bytes,
      type: "file",
    }
  }

  if (body?.type === "image" && typeof body.file_id === "string") {
    return {
      fileId: body.file_id,
      type: "image",
    }
  }

  if (body?.type === "system_event") {
    return normalizeSystemEventMessageBody(body)
  }

  throw new ClientDataRequestError("消息响应格式不正确")
}

function formatMarkdownMessageSummary(content: string) {
  return content
    .replace(/```[\s\S]*?```/g, (block) =>
      block
        .replace(/^```[^\n]*\n?/, "")
        .replace(/```$/, "")
        .trim()
    )
    .replace(/`([^`]*)`/g, "$1")
    .replace(/!\[[^\]]*]\([^)]*\)/g, "")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^>\s?/gm, "")
    .replace(/^\s*[-*+]\s+/gm, "")
    .replace(/^\s*\d+[.)]\s+/gm, "")
    .replace(/[*_~]+/g, "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .join("\n")
}

function normalizeSystemEventMessageBody(
  body:
    | GroupMembersInvitedSystemEventBodyResponse
    | GroupAvatarUpdatedSystemEventBodyResponse
    | GroupVisibilityChangedSystemEventBodyResponse
    | GroupMemberJoinedSystemEventBodyResponse
    | GroupMemberLeftSystemEventBodyResponse
    | GroupNameUpdatedSystemEventBodyResponse
):
  | ClientGroupMembersInvitedSystemEventBody
  | ClientGroupAvatarUpdatedSystemEventBody
  | ClientGroupVisibilityChangedSystemEventBody
  | ClientGroupMemberJoinedSystemEventBody
  | ClientGroupMemberLeftSystemEventBody
  | ClientGroupNameUpdatedSystemEventBody {
  if (body.event === "group_avatar_updated") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_avatar_updated",
      type: "system_event",
    }
  }

  if (body.event === "group_visibility_changed") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_visibility_changed",
      type: "system_event",
      visibility: normalizeVisibility(body.visibility),
    }
  }

  if (body.event === "group_member_joined") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_member_joined",
      type: "system_event",
    }
  }

  if (body.event === "group_member_left") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_member_left",
      type: "system_event",
    }
  }

  if (body.event === "group_name_updated") {
    if (
      !("actor" in body) ||
      !isSystemEventUserRefResponse(body.actor) ||
      typeof body.name !== "string"
    ) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_name_updated",
      name: body.name,
      type: "system_event",
    }
  }

  if (
    body.event !== "group_members_invited" ||
    !("inviter" in body) ||
    !isSystemEventUserRefResponse(body.inviter) ||
    !("invitees" in body) ||
    !Array.isArray(body.invitees)
  ) {
    throw new ClientDataRequestError("消息响应格式不正确")
  }

  return {
    event: "group_members_invited",
    invitees: body.invitees.map(normalizeSystemEventUserRef),
    inviter: normalizeSystemEventUserRef(body.inviter),
    type: "system_event",
  }
}

function normalizeVisibility(value: string | undefined) {
  if (value === "public") {
    return "public"
  }

  return "private"
}

function normalizeSystemEventUserRef(
  userRef: SystemEventUserRefResponse
): ClientSystemEventUserRef {
  if (!isSystemEventUserRefResponse(userRef)) {
    throw new ClientDataRequestError("消息响应格式不正确")
  }

  return {
    displayName: userRef.display_name,
    id: userRef.id,
  }
}

function normalizeTemporaryFileReadURL(
  item: TemporaryFileReadURLResponse
): TemporaryFileReadURL {
  if (!item.file_id || !item.url || !item.expires_at) {
    throw new ClientDataRequestError("文件下载地址响应格式不正确")
  }

  return {
    expiresAt: item.expires_at,
    fileId: item.file_id,
    url: item.url,
  }
}

function isTemporaryFileReadURLFresh(item: TemporaryFileReadURL, now: number) {
  const expiresAt = Date.parse(item.expiresAt)

  return (
    Number.isFinite(expiresAt) &&
    expiresAt - temporaryFileReadURLCacheSafetyWindowMs > now
  )
}

function isDefined<T>(value: T | null | undefined): value is T {
  return value !== null && value !== undefined
}

function isSystemEventUserRefResponse(
  userRef: SystemEventUserRefResponse | undefined
): userRef is Required<SystemEventUserRefResponse> {
  return Boolean(userRef?.id && userRef.display_name)
}

function normalizeMessageSenderType(type: string | undefined) {
  if (type === "app" || type === "system") {
    return type
  }

  return "user"
}

function normalizeMessagePage(
  page: MessagePageResponse | undefined
): ClientMessagePage {
  if (
    !page ||
    typeof page.limit !== "number" ||
    typeof page.oldest_seq !== "number" ||
    typeof page.newest_seq !== "number"
  ) {
    throw new ClientDataRequestError("消息列表响应格式不正确")
  }

  return {
    hasMoreAfter: Boolean(page.has_more_after),
    hasMoreBefore: Boolean(page.has_more_before),
    limit: page.limit,
    newestSeq: page.newest_seq,
    oldestSeq: page.oldest_seq,
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function createRequestError(
  payload:
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<unknown> | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as ClientDataErrorEnvelope | undefined)?.error

  return new ClientDataRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
      status: response.status,
    }
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
