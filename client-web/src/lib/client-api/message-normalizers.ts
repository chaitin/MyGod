import { ClientDataRequestError, normalizeVisibility } from "./core"
import { normalizeChartMessageBody } from "./chart-message-normalizer"
import type {
  MessageDelegatedByResponse,
  MessageReplyToResponse,
  MessageTopicReplyResponse,
  SystemEventUserRefResponse,
  GroupMembersInvitedSystemEventBodyResponse,
  GroupAvatarUpdatedSystemEventBodyResponse,
  GroupVisibilityChangedSystemEventBodyResponse,
  GroupMemberJoinedSystemEventBodyResponse,
  GroupMemberLeftSystemEventBodyResponse,
  GroupMemberRemovedSystemEventBodyResponse,
  GroupNameUpdatedSystemEventBodyResponse,
  MessageRevokedSystemEventBodyResponse,
  TopicClosedSystemEventBodyResponse,
  MessageBodyResponse,
  MessageReactionUserResponse,
  MessageResponse,
  MessagePageResponse,
  MarkConversationReadResponse,
  TemporaryFileReadURLResponse,
  ClientMessageDelegatedBy,
  ClientMessageReplyTo,
  ClientImageMessageBody,
  ClientVoiceMessageBody,
  ClientForwardBundleMessageBody,
  ClientForwardableMessageBody,
  ClientSystemEventUserRef,
  ClientGroupMembersInvitedSystemEventBody,
  ClientGroupAvatarUpdatedSystemEventBody,
  ClientGroupVisibilityChangedSystemEventBody,
  ClientGroupMemberJoinedSystemEventBody,
  ClientGroupMemberLeftSystemEventBody,
  ClientGroupMemberRemovedSystemEventBody,
  ClientGroupNameUpdatedSystemEventBody,
  ClientMessageRevokedSystemEventBody,
  ClientTopicClosedSystemEventBody,
  ClientMessageBody,
  ClientMessage,
  ClientMessageReaction,
  ClientMessagePage,
  ClientMessageTopicReply,
  TemporaryFileReadURL,
  MarkConversationReadResult,
} from "./types"

const temporaryFileReadURLCacheSafetyWindowMs = 5 * 60 * 1000
const maxForwardBundleDepth = 5
const maxForwardBundleLeafCount = 50

export function normalizeMarkConversationReadResult(
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

export function normalizeMessage(
  message: MessageResponse | undefined
): ClientMessage {
  const senderType = normalizeMessageSenderType(message?.sender?.type)
  const senderId = message?.sender?.id ?? ""
  const revokedAt = message?.revoked_at
  if (
    !message?.conversation_id ||
    !message.created_at ||
    !message.id ||
    !message.sender ||
    (senderType !== "system" && !senderId) ||
    typeof message.seq !== "number" ||
    (revokedAt !== undefined && typeof revokedAt !== "string")
  ) {
    throw new ClientDataRequestError("消息响应格式不正确")
  }

  const normalized: ClientMessage = {
    body: revokedAt
      ? { type: "revoked" }
      : normalizeClientMessageBody(message.body),
    clientMessageId: message.client_message_id ?? "",
    conversationId: message.conversation_id,
    createdAt: message.created_at,
    id: message.id,
    reactionVersion: normalizeReactionVersion(message.reaction_version),
    reactions: normalizeMessageReactions(message.reactions),
    sender: {
      id: senderId,
      type: senderType,
    },
    seq: message.seq,
  }
  const delegatedBy = normalizeMessageDelegatedBy(message.delegated_by)
  if (delegatedBy) {
    normalized.delegatedBy = delegatedBy
  }
  const replyTo = normalizeMessageReplyTo(message.reply_to)
  if (replyTo) {
    normalized.replyTo = replyTo
  }
  if (message.topic) {
    if (!message.topic.conversation_id) {
      throw new ClientDataRequestError("消息话题信息响应格式不正确")
    }
    normalized.topic = {
      archived: Boolean(message.topic.archived),
      conversationId: message.topic.conversation_id,
      recentReplies: (message.topic.recent_replies ?? []).map(
        normalizeMessageTopicReply
      ),
    }
  }
  if (message.reply_to_message_id) {
    normalized.replyToMessageId = message.reply_to_message_id
  }
  if (revokedAt) {
    normalized.revokedAt = revokedAt
    if (message.revoked_by_user_id) {
      normalized.revokedByUserId = message.revoked_by_user_id
    }
  }

  return normalized
}

export function normalizeMessageReactions(
  reactions: MessageResponse["reactions"]
): ClientMessageReaction[] {
  if (reactions == null) {
    return []
  }
  if (!Array.isArray(reactions)) {
    throw new ClientDataRequestError("消息表情响应格式不正确")
  }
  return reactions.map((reaction) => {
    if (
      typeof reaction?.text !== "string" ||
      reaction.text === "" ||
      typeof reaction.count !== "number" ||
      !Number.isInteger(reaction.count) ||
      reaction.count <= 0 ||
      (reaction.reacted_by_me !== undefined &&
        typeof reaction.reacted_by_me !== "boolean")
    ) {
      throw new ClientDataRequestError("消息表情响应格式不正确")
    }
    return {
      count: reaction.count,
      reactedByMe: Boolean(reaction.reacted_by_me),
      text: reaction.text,
      users: normalizeMessageReactionUsers(reaction.users),
    }
  })
}

export function normalizeMessageReactionUsers(
  value: MessageReactionUserResponse[] | undefined
) {
  if (value === undefined) return []
  if (
    !Array.isArray(value) ||
    value.some(
      (user) =>
        typeof user?.id !== "string" ||
        user.id.trim() === "" ||
        typeof user.name !== "string" ||
        user.name.trim() === ""
    )
  ) {
    throw new ClientDataRequestError("消息表情响应格式不正确")
  }
  return value.map((user) => ({ id: user.id!, name: user.name! }))
}

function normalizeReactionVersion(value: number | undefined) {
  if (value === undefined) {
    return 0
  }
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new ClientDataRequestError("消息表情版本格式不正确")
  }
  return value
}

function normalizeMessageTopicReply(
  reply: MessageTopicReplyResponse
): ClientMessageTopicReply {
  const senderType = normalizeMessageSenderType(reply?.sender?.type)
  const senderId = reply?.sender?.id ?? ""
  if (
    !reply?.id ||
    !reply.created_at ||
    !reply.sender ||
    !senderId ||
    (senderType !== "user" && senderType !== "app") ||
    typeof reply.summary !== "string"
  ) {
    throw new ClientDataRequestError("话题回复摘要响应格式不正确")
  }
  return {
    createdAt: reply.created_at,
    id: reply.id,
    sender: { id: senderId, type: senderType },
    summary: reply.summary,
  }
}

export function normalizeClientMessageBody(
  body: MessageBodyResponse | undefined
): ClientMessageBody {
  try {
    return normalizeMessageBody(body)
  } catch {
    return { type: "unsupported" }
  }
}

function normalizeMessageDelegatedBy(
  delegatedBy: MessageDelegatedByResponse | null | undefined
): ClientMessageDelegatedBy | undefined {
  if (!delegatedBy) {
    return undefined
  }
  if (
    !delegatedBy.id ||
    !delegatedBy.name ||
    (delegatedBy.type !== "user" && delegatedBy.type !== "app")
  ) {
    throw new ClientDataRequestError("消息代发信息响应格式不正确")
  }

  return {
    id: delegatedBy.id,
    name: delegatedBy.name,
    type: delegatedBy.type,
  }
}

function normalizeMessageReplyTo(
  replyTo: MessageReplyToResponse | null | undefined
): ClientMessageReplyTo | undefined {
  if (!replyTo) {
    return undefined
  }

  const senderType = normalizeMessageSenderType(replyTo.sender?.type)
  const senderId = replyTo.sender?.id ?? ""
  if (
    !replyTo.id ||
    !replyTo.sender ||
    (senderType !== "system" && !senderId) ||
    typeof replyTo.sender.name !== "string" ||
    typeof replyTo.seq !== "number" ||
    typeof replyTo.summary !== "string"
  ) {
    throw new ClientDataRequestError("消息引用信息响应格式不正确")
  }

  return {
    id: replyTo.id,
    sender: {
      id: senderId,
      name: replyTo.sender.name,
      type: senderType,
    },
    seq: replyTo.seq,
    summary: replyTo.summary,
  }
}

function normalizeMessageBody(
  body: MessageBodyResponse | undefined,
  forwardBundleDepth = 0
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
    body?.type === "card" &&
    typeof body.title === "string" &&
    typeof body.description === "string" &&
    typeof body.url === "string"
  ) {
    return {
      description: body.description,
      title: body.title,
      type: "card",
      url: body.url,
    }
  }

  if (body?.type === "chart") {
    return normalizeChartMessageBody(body)
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
    const normalizedImage: ClientImageMessageBody = {
      fileId: body.file_id,
      type: "image",
    }

    if (isPositiveFiniteNumber(body.width)) {
      normalizedImage.width = body.width
    }
    if (isPositiveFiniteNumber(body.height)) {
      normalizedImage.height = body.height
    }

    return normalizedImage
  }

  if (
    body?.type === "voice" &&
    typeof body.file_id === "string" &&
    typeof body.duration_ms === "number" &&
    body.duration_ms > 0 &&
    body.duration_ms <= 60_000 &&
    typeof body.size_bytes === "number" &&
    body.size_bytes > 0 &&
    typeof body.content_type === "string" &&
    body.content_type === "audio/webm"
  ) {
    const normalizedVoice: ClientVoiceMessageBody = {
      contentType: body.content_type,
      durationMS: body.duration_ms,
      fileId: body.file_id,
      sizeBytes: body.size_bytes,
      transcript:
        typeof body.transcript === "string" ? body.transcript.trim() : "",
      type: "voice",
    }

    return normalizedVoice
  }

  if (
    body?.type === "forward_bundle" &&
    forwardBundleDepth < maxForwardBundleDepth &&
    typeof body.item_count === "number" &&
    body.item_count > 0 &&
    body.item_count <= maxForwardBundleLeafCount &&
    Array.isArray(body.items) &&
    body.items.length === body.item_count
  ) {
    const normalizedBundle: ClientForwardBundleMessageBody = {
      itemCount: body.item_count,
      items: body.items.map((item) => {
        if (
          !item ||
          !item.sender_name?.trim() ||
          (item.sender_type !== "user" && item.sender_type !== "app") ||
          !item.sent_at ||
          typeof item.summary !== "string"
        ) {
          throw new ClientDataRequestError("消息响应格式不正确")
        }

        const normalizedBody = normalizeMessageBody(
          item.body,
          forwardBundleDepth + 1
        )
        if (!isForwardableMessageBody(normalizedBody)) {
          throw new ClientDataRequestError("消息响应格式不正确")
        }

        return {
          body: normalizedBody,
          senderName: item.sender_name,
          senderType: item.sender_type,
          sentAt: item.sent_at,
          summary: item.summary,
        }
      }),
      type: "forward_bundle",
    }

    if (
      countForwardBundleLeaves(normalizedBundle) > maxForwardBundleLeafCount
    ) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return normalizedBundle
  }

  if (body?.type === "system_event") {
    return normalizeSystemEventMessageBody(body)
  }

  throw new ClientDataRequestError("消息响应格式不正确")
}

function isForwardableMessageBody(
  body: ClientMessageBody
): body is ClientForwardableMessageBody {
  return (
    body.type === "text" ||
    body.type === "markdown" ||
    body.type === "link" ||
    body.type === "card" ||
    body.type === "chart" ||
    body.type === "file" ||
    body.type === "image" ||
    body.type === "voice" ||
    body.type === "forward_bundle"
  )
}

function countForwardBundleLeaves(body: ClientForwardableMessageBody): number {
  if (body.type !== "forward_bundle") {
    return 1
  }

  return body.items.reduce(
    (count, item) => count + countForwardBundleLeaves(item.body),
    0
  )
}

function isPositiveFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
}

function normalizeSystemEventMessageBody(
  body:
    | GroupMembersInvitedSystemEventBodyResponse
    | GroupAvatarUpdatedSystemEventBodyResponse
    | GroupVisibilityChangedSystemEventBodyResponse
    | GroupMemberJoinedSystemEventBodyResponse
    | GroupMemberLeftSystemEventBodyResponse
    | GroupMemberRemovedSystemEventBodyResponse
    | GroupNameUpdatedSystemEventBodyResponse
    | MessageRevokedSystemEventBodyResponse
    | TopicClosedSystemEventBodyResponse
):
  | ClientGroupMembersInvitedSystemEventBody
  | ClientGroupAvatarUpdatedSystemEventBody
  | ClientGroupVisibilityChangedSystemEventBody
  | ClientGroupMemberJoinedSystemEventBody
  | ClientGroupMemberLeftSystemEventBody
  | ClientGroupMemberRemovedSystemEventBody
  | ClientGroupNameUpdatedSystemEventBody
  | ClientMessageRevokedSystemEventBody
  | ClientTopicClosedSystemEventBody {
  if (body.event === "topic_closed") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "topic_closed",
      type: "system_event",
    }
  }

  if (body.event === "message_revoked") {
    if (!("actor" in body) || !isSystemEventUserRefResponse(body.actor)) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "message_revoked",
      type: "system_event",
    }
  }

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

  if (body.event === "group_member_removed") {
    if (
      !("actor" in body) ||
      !isSystemEventUserRefResponse(body.actor) ||
      !("target" in body) ||
      !isSystemEventUserRefResponse(body.target)
    ) {
      throw new ClientDataRequestError("消息响应格式不正确")
    }

    return {
      actor: normalizeSystemEventUserRef(body.actor),
      event: "group_member_removed",
      target: normalizeSystemEventUserRef(body.target),
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

export function normalizeTemporaryFileReadURL(
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

export function isTemporaryFileReadURLFresh(
  item: TemporaryFileReadURL,
  now: number
) {
  const expiresAt = Date.parse(item.expiresAt)

  return (
    Number.isFinite(expiresAt) &&
    expiresAt - temporaryFileReadURLCacheSafetyWindowMs > now
  )
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

export function normalizeMessagePage(
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
