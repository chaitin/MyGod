import {
  ClientDataRequestError,
  createRequestError,
  normalizeVisibility,
  readJson,
} from "./core"
import {
  normalizeClientMessageBody,
  normalizeMessage,
} from "./message-normalizers"
import type {
  ClientDataFetch,
  ClientDataSuccessEnvelope,
  ClientDataErrorEnvelope,
  ConversationResponse,
  ConversationProjectResponse,
  ConversationMemberResponse,
  ListClientConversationsResponse,
  CreateDirectConversationResponse,
  CreateAppConversationResponse,
  CreateGroupConversationResponse,
  AddGroupConversationMembersResponse,
  GroupConversationActionResponse,
  LeaveGroupConversationResponse,
  DissolveGroupConversationResponse,
  UploadGroupConversationAvatarResponse,
  ClientConversation,
  ClientConversationProject,
  ClientConversationMember,
  CreateGroupConversationInput,
  AddGroupConversationMembersInput,
  UpdateGroupConversationNameInput,
  AddGroupConversationMembersResult,
  GroupConversationActionResult,
  UploadGroupConversationAvatarResult,
  LeaveGroupConversationResult,
  DissolveGroupConversationResult,
  CreateTopicResponse,
  TopicConversationResponse,
  TopicDetailResponse,
  ClientTopicDetail,
  SetConversationPinResponse,
  ConversationPinUpdatedEventPayloadResponse,
  SetConversationMuteResponse,
  DismissConversationResponse,
  RestoreConversationResponse,
  ConversationMuteUpdatedEventPayloadResponse,
} from "./types"

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

export async function setConversationPinned(
  conversationId: string,
  pinned: boolean,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/pin`,
    {
      credentials: "include",
      method: pinned ? "PUT" : "DELETE",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<SetConversationPinResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(
      payload,
      response,
      pinned ? "置顶会话失败" : "取消置顶失败"
    )
  }
  const data = (
    payload as ClientDataSuccessEnvelope<SetConversationPinResponse> | undefined
  )?.data
  if (
    typeof data?.conversation_id !== "string" ||
    data.conversation_id.trim() === "" ||
    typeof data.pinned !== "boolean"
  ) {
    throw new ClientDataRequestError("会话置顶响应格式不正确")
  }
  return {
    conversationId: data.conversation_id,
    pinned: data.pinned,
  }
}

export async function setConversationMuted(
  conversationId: string,
  muted: boolean,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/mute`,
    {
      credentials: "include",
      method: muted ? "PUT" : "DELETE",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<SetConversationMuteResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(
      payload,
      response,
      muted ? "开启消息免打扰失败" : "取消消息免打扰失败"
    )
  }
  const data = (
    payload as
      ClientDataSuccessEnvelope<SetConversationMuteResponse> | undefined
  )?.data
  if (
    typeof data?.conversation_id !== "string" ||
    data.conversation_id.trim() === "" ||
    typeof data.muted !== "boolean"
  ) {
    throw new ClientDataRequestError("会话免打扰响应格式不正确")
  }
  return {
    conversationId: data.conversation_id,
    muted: data.muted,
  }
}

export async function dismissConversation(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}`,
    { credentials: "include", method: "DELETE" }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<DismissConversationResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "删除对话失败")
  }
  const data = (
    payload as
      ClientDataSuccessEnvelope<DismissConversationResponse> | undefined
  )?.data
  if (
    typeof data?.conversation_id !== "string" ||
    data.conversation_id.trim() === ""
  ) {
    throw new ClientDataRequestError("删除对话响应格式不正确")
  }
  return { conversationId: data.conversation_id }
}

export async function restoreConversation(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/restore`,
    { credentials: "include", method: "POST" }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<RestoreConversationResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "恢复对话失败")
  }
  const conversation = (
    payload as
      ClientDataSuccessEnvelope<RestoreConversationResponse> | undefined
  )?.data?.conversation
  return normalizeConversation(conversation)
}

export function normalizeConversationPinUpdatedEventPayload(payload: unknown) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new ClientDataRequestError("会话置顶推送格式不正确")
  }
  const event = payload as ConversationPinUpdatedEventPayloadResponse
  if (
    typeof event.conversation_id !== "string" ||
    event.conversation_id.trim() === "" ||
    typeof event.pinned !== "boolean"
  ) {
    throw new ClientDataRequestError("会话置顶推送格式不正确")
  }
  return {
    conversationId: event.conversation_id,
    pinned: event.pinned,
  }
}

export function normalizeConversationMuteUpdatedEventPayload(payload: unknown) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new ClientDataRequestError("会话免打扰推送格式不正确")
  }
  const event = payload as ConversationMuteUpdatedEventPayloadResponse
  if (
    typeof event.conversation_id !== "string" ||
    event.conversation_id.trim() === "" ||
    typeof event.muted !== "boolean"
  ) {
    throw new ClientDataRequestError("会话免打扰推送格式不正确")
  }
  return {
    conversationId: event.conversation_id,
    muted: event.muted,
  }
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
      app_ids: input.appIds ?? [],
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

export async function createConversationTopic(
  conversationId: string,
  messageId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/topic`,
    { credentials: "include", method: "POST" }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<CreateTopicResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "创建话题失败")
  }
  const data = (payload as ClientDataSuccessEnvelope<CreateTopicResponse>)?.data
  if (!data?.conversation) {
    throw new ClientDataRequestError("创建话题响应格式不正确")
  }
  return {
    conversation: normalizeConversation(data.conversation),
    created: Boolean(data.created),
  }
}

export async function getConversationTopic(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<ClientTopicDetail> {
  const response = await fetcher(
    `/api/client/conversations/topics/${encodeURIComponent(conversationId)}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<TopicDetailResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载话题失败")
  }
  return normalizeTopicDetail(
    (payload as ClientDataSuccessEnvelope<TopicDetailResponse>)?.data
  )
}

export async function participateConversationTopic(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
) {
  return mutateConversationTopic(
    conversationId,
    "participate",
    "参与话题失败",
    fetcher
  )
}

export async function archiveConversationTopic(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
) {
  return mutateConversationTopic(
    conversationId,
    "archive",
    "关闭话题失败",
    fetcher
  )
}

async function mutateConversationTopic(
  conversationId: string,
  action: "participate" | "archive",
  fallbackMessage: string,
  fetcher: ClientDataFetch
) {
  const response = await fetcher(
    `/api/client/conversations/topics/${encodeURIComponent(conversationId)}/${action}`,
    { credentials: "include", method: "POST" }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<TopicConversationResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, fallbackMessage)
  }
  const conversation = (
    payload as ClientDataSuccessEnvelope<TopicConversationResponse>
  )?.data?.conversation
  if (!conversation) {
    throw new ClientDataRequestError(`${fallbackMessage}响应格式不正确`)
  }
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

export async function dissolveGroupConversation(
  conversationId: string,
  fetcher: ClientDataFetch = fetch
): Promise<DissolveGroupConversationResult> {
  const response = await fetcher(
    `/api/client/conversations/groups/${encodeURIComponent(conversationId)}`,
    {
      credentials: "include",
      method: "DELETE",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<DissolveGroupConversationResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "解散群聊失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<DissolveGroupConversationResponse> | undefined
  )?.data

  if (!data?.conversation_id) {
    throw new ClientDataRequestError("解散群聊响应格式不正确")
  }

  return {
    conversationId: data.conversation_id,
  }
}

export async function removeGroupConversationMember(
  conversationId: string,
  memberId: string,
  memberTypeOrFetcher: "user" | "app" | ClientDataFetch = "user",
  fetcher: ClientDataFetch = fetch
): Promise<GroupConversationActionResult> {
  const memberType =
    typeof memberTypeOrFetcher === "function" ? "user" : memberTypeOrFetcher
  const activeFetcher =
    typeof memberTypeOrFetcher === "function" ? memberTypeOrFetcher : fetcher
  const url =
    memberType === "user"
      ? `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/members/${encodeURIComponent(memberId)}`
      : `/api/client/conversations/groups/${encodeURIComponent(conversationId)}/members/${encodeURIComponent(memberType)}/${encodeURIComponent(memberId)}`

  const response = await activeFetcher(url, {
    credentials: "include",
    method: "DELETE",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<GroupConversationActionResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "移出群聊成员失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<GroupConversationActionResponse> | undefined
  )?.data

  if (!data?.conversation) {
    throw new ClientDataRequestError("移出群聊成员响应格式不正确")
  }

  return {
    conversation: normalizeConversation(data.conversation),
    message: data.message ? normalizeMessage(data.message) : null,
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
        app_ids: input.appIds ?? [],
        member_ids: input.memberIds ?? [],
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

function normalizeConversation(
  conversation: ConversationResponse | undefined
): ClientConversation {
  if (!conversation?.created_at || !conversation.id || !conversation.name) {
    throw new ClientDataRequestError("会话列表响应格式不正确")
  }

  const normalizedConversation: ClientConversation = {
    avatar: conversation.avatar ?? "",
    canSend: conversation.can_send !== false,
    createdAt: conversation.created_at,
    id: conversation.id,
    lastMessageAt: conversation.last_message_at ?? null,
    lastMessageId: conversation.last_message_id ?? null,
    lastMessageSeq: conversation.last_message_seq ?? 0,
    lastMessageSender: normalizeConversationLastMessageSender(
      conversation.last_message_sender
    ),
    lastMessageSummary: conversation.last_message_summary ?? "",
    lastChoiceSeq: conversation.last_choice_seq ?? 0,
    lastMentionedSeq: conversation.last_mentioned_seq ?? 0,
    lastReadSeq: conversation.last_read_seq ?? 0,
    memberCount: conversation.member_count ?? 0,
    name: conversation.name,
    type: normalizeConversationType(conversation.type),
    unreadCount: conversation.unread_count ?? 0,
    visibility: normalizeVisibility(conversation.visibility),
  }

  if (typeof conversation.notification_muted === "boolean") {
    normalizedConversation.notificationMuted = conversation.notification_muted
  }

  if (typeof conversation.pinned === "boolean") {
    normalizedConversation.pinned = conversation.pinned
  }

  if (conversation.members) {
    normalizedConversation.members = conversation.members.map(
      normalizeConversationMember
    )
  }

  if (conversation.projects) {
    normalizedConversation.projects = conversation.projects.map(
      normalizeConversationProject
    )
  }

  if (conversation.topic) {
    const topic = conversation.topic
    if (
      !topic.parent_conversation_id ||
      !topic.parent_conversation_name ||
      !topic.source_message_id ||
      !topic.source_sender?.id ||
      !topic.source_sender.name ||
      typeof topic.source_message_seq !== "number"
    ) {
      throw new ClientDataRequestError("会话话题信息响应格式不正确")
    }
    normalizedConversation.topic = {
      archived: Boolean(topic.archived),
      parentConversationId: topic.parent_conversation_id,
      parentConversationName: topic.parent_conversation_name,
      parentConversationType: normalizeParentConversationType(
        topic.parent_conversation_type
      ),
      participating: Boolean(topic.participating),
      sourceMessageId: topic.source_message_id,
      sourceMessageSeq: topic.source_message_seq,
      sourceSender: {
        avatar: topic.source_sender.avatar ?? "",
        id: topic.source_sender.id,
        name: topic.source_sender.name,
        type: normalizeTopicSourceSenderType(topic.source_sender.type),
      },
    }
  }

  return normalizedConversation
}

function normalizeConversationLastMessageSender(
  sender: ConversationResponse["last_message_sender"]
): ClientConversation["lastMessageSender"] {
  if (!sender) {
    return null
  }

  return {
    id: sender.id ?? "",
    name: sender.name ?? "",
    nickname: sender.nickname ?? "",
    type:
      sender.type === "app" || sender.type === "system" ? sender.type : "user",
  }
}

function normalizeConversationProject(
  project: ConversationProjectResponse | undefined
): ClientConversationProject {
  if (!project?.id || !project.name) {
    throw new ClientDataRequestError("会话关联项目响应格式不正确")
  }

  return {
    avatar: project.avatar ?? "",
    description: project.description ?? "",
    id: project.id,
    name: project.name,
  }
}

function normalizeConversationMember(
  member: ConversationMemberResponse | undefined
): ClientConversationMember {
  const memberType = member?.type === "app" ? "app" : "user"
  if (!member?.id || !member.name || (memberType === "user" && !member.email)) {
    throw new ClientDataRequestError("会话成员响应格式不正确")
  }

  return {
    avatar: member.avatar ?? "",
    email: member.email ?? "",
    id: member.id,
    name: member.name,
    nickname: member.nickname ?? "",
    phone: member.phone ?? "",
    role: normalizeConversationMemberRole(member.role),
    type: memberType,
  }
}

function normalizeConversationMemberRole(role: string | undefined) {
  if (role === "owner" || role === "admin") {
    return role
  }

  return "member"
}

function normalizeConversationType(type: string | undefined) {
  if (type === "direct" || type === "app" || type === "topic") {
    return type
  }

  return "group"
}

function normalizeParentConversationType(type: string | undefined) {
  if (type === "direct" || type === "app") {
    return type
  }
  return "group"
}

function normalizeTopicSourceSenderType(type: string | undefined) {
  if (type === "user" || type === "app") {
    return type
  }
  throw new ClientDataRequestError("话题来源消息发送者响应格式不正确")
}

function normalizeTopicDetail(
  detail: TopicDetailResponse | undefined
): ClientTopicDetail {
  const parent = detail?.parent_conversation
  const source = detail?.source_message
  const senderType = source?.sender?.type
  if (
    !detail?.conversation ||
    !parent?.id ||
    !parent.name ||
    !source?.created_at ||
    !source.id ||
    !source.sender?.id ||
    !source.sender.name ||
    (senderType !== "user" && senderType !== "app") ||
    typeof source.seq !== "number" ||
    typeof source.summary !== "string"
  ) {
    throw new ClientDataRequestError("话题详情响应格式不正确")
  }
  return {
    canArchive: Boolean(detail.can_archive),
    canParticipate: Boolean(detail.can_participate),
    conversation: normalizeConversation(detail.conversation),
    parentConversation: {
      id: parent.id,
      name: parent.name,
      type: normalizeParentConversationType(parent.type),
    },
    sourceMessage: {
      body: source.revoked_at
        ? { type: "revoked" }
        : normalizeClientMessageBody(source.body),
      createdAt: source.created_at,
      id: source.id,
      revokedAt: source.revoked_at ?? null,
      sender: {
        avatar: source.sender.avatar ?? "",
        id: source.sender.id,
        name: source.sender.name,
        type: senderType,
      },
      seq: source.seq,
      summary: source.summary,
    },
  }
}
