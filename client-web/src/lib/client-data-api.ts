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
  contacts?: ContactUserResponse[]
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

type CreateGroupConversationResponse = {
  conversation?: ConversationResponse
}

type AddGroupConversationMembersResponse = {
  conversation?: ConversationResponse
  message?: MessageResponse | null
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
  event?: string
  invitees?: SystemEventUserRefResponse[]
  inviter?: SystemEventUserRefResponse
  type?: "system_event"
}

type GroupAvatarUpdatedSystemEventBodyResponse = {
  actor?: SystemEventUserRefResponse
  event?: string
  type?: "system_event"
}

type MessageBodyResponse =
  | TextMessageBodyResponse
  | FileMessageBodyResponse
  | ImageMessageBodyResponse
  | GroupMembersInvitedSystemEventBodyResponse
  | GroupAvatarUpdatedSystemEventBodyResponse

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

export type ClientMessageBody =
  | ClientTextMessageBody
  | ClientFileMessageBody
  | ClientImageMessageBody
  | ClientGroupMembersInvitedSystemEventBody
  | ClientGroupAvatarUpdatedSystemEventBody

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

export type AddGroupConversationMembersResult = {
  conversation: ClientConversation
  message: ClientMessage | null
}

export type UploadGroupConversationAvatarResult = {
  conversation: ClientConversation
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
  const response = await fetcher("/api/client/contacts/users", {
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

  const contacts = (
    payload as ClientDataSuccessEnvelope<ListClientContactsResponse> | undefined
  )?.data?.contacts

  if (!contacts) {
    throw new ClientDataRequestError("通讯录响应格式不正确")
  }

  return contacts.map(normalizeContactUser)
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

  if (body.type === "file") {
    return `[文件] ${body.name}`
  }

  if (body.type === "image") {
    return "[图片]"
  }

  if (body.event === "group_avatar_updated") {
    return `${body.actor.displayName} 修改了群头像`
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

function normalizeSystemEventMessageBody(
  body:
    | GroupMembersInvitedSystemEventBodyResponse
    | GroupAvatarUpdatedSystemEventBodyResponse
):
  | ClientGroupMembersInvitedSystemEventBody
  | ClientGroupAvatarUpdatedSystemEventBody {
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
