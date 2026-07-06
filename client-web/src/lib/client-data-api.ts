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
  member_count?: number
  name?: string
  type?: string
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

type MessageSenderResponse = {
  id?: string
  type?: string
}

type TextMessageBodyResponse = {
  content?: string
  type?: string
}

type MessageResponse = {
  body?: TextMessageBodyResponse
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

type MessageCreatedEventPayloadResponse = {
  message?: MessageResponse
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
  memberCount: number
  name: string
  type: "direct" | "group" | "app"
}

export type ClientMessageSender = {
  id: string
  type: "user" | "app" | "system"
}

export type ClientTextMessageBody = {
  content: string
  type: "text"
}

export type ClientMessage = {
  body: ClientTextMessageBody
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

export type CreateGroupConversationInput = {
  memberIds: string[]
  name: string
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
      | ClientDataSuccessEnvelope<ListConversationMessagesResponse>
      | undefined
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

  return {
    avatar: conversation.avatar ?? "",
    createdAt: conversation.created_at,
    id: conversation.id,
    lastMessageAt: conversation.last_message_at ?? null,
    lastMessageId: conversation.last_message_id ?? null,
    lastMessageSeq: conversation.last_message_seq ?? 0,
    lastMessageSummary: conversation.last_message_summary ?? "",
    memberCount: conversation.member_count ?? 0,
    name: conversation.name,
    type: normalizeConversationType(conversation.type),
  }
}

function normalizeConversationType(type: string | undefined) {
  if (type === "direct" || type === "app") {
    return type
  }

  return "group"
}

function normalizeMessage(message: MessageResponse | undefined): ClientMessage {
  if (
    !message?.conversation_id ||
    !message.created_at ||
    !message.id ||
    !message.sender?.id ||
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
      id: message.sender.id,
      type: normalizeMessageSenderType(message.sender.type),
    },
    seq: message.seq,
  }
}

function normalizeMessageBody(
  body: TextMessageBodyResponse | undefined
): ClientTextMessageBody {
  if (body?.type !== "text" || typeof body.content !== "string") {
    throw new ClientDataRequestError("消息响应格式不正确")
  }

  return {
    content: body.content,
    type: "text",
  }
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
