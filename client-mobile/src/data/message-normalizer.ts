import { ApiRequestError } from "@/data/api-client"
import type {
  ClientForwardableMessageBody,
  ClientMessage,
  ClientMessageBody,
  ClientMessagePage,
  ClientMessageReplyTo,
  ClientSystemEventMessageBody,
  ClientSystemEventUserRef,
  TemporaryFileReadUrl,
} from "@/data/models"

const MAX_FORWARD_BUNDLE_DEPTH = 5
const MAX_FORWARD_BUNDLE_ITEMS = 50

export function normalizeClientMessage(value: unknown): ClientMessage {
  const message = asRecord(value)
  const sender = asRecord(message?.sender)
  const senderType = normalizeSenderType(sender?.type)
  const senderId = asString(sender?.id) ?? ""
  const conversationId = asString(message?.conversation_id)
  const createdAt = asString(message?.created_at)
  const id = asString(message?.id)
  const seq = asNumber(message?.seq)
  const revokedAt = asString(message?.revoked_at)

  if (
    !message ||
    !conversationId ||
    !createdAt ||
    !id ||
    !sender ||
    (senderType !== "system" && !senderId) ||
    seq === undefined
  ) {
    throw new ApiRequestError("消息响应格式不正确")
  }

  const normalized: ClientMessage = {
    body: revokedAt
      ? { type: "revoked" }
      : normalizeMessageBodyOrUnsupported(message.body),
    clientMessageId: asString(message.client_message_id) ?? "",
    conversationId,
    createdAt,
    id,
    sender: {
      id: senderId,
      type: senderType,
    },
    seq,
  }
  const delegatedBy = normalizeDelegatedBy(message.delegated_by)
  const replyTo = normalizeReplyTo(message.reply_to)
  const replyToMessageId = asString(message.reply_to_message_id)

  if (delegatedBy) {
    normalized.delegatedBy = delegatedBy
  }
  if (replyTo) {
    normalized.replyTo = replyTo
  }
  if (replyToMessageId) {
    normalized.replyToMessageId = replyToMessageId
  }
  if (revokedAt) {
    normalized.revokedAt = revokedAt
    const revokedByUserId = asString(message.revoked_by_user_id)
    if (revokedByUserId) {
      normalized.revokedByUserId = revokedByUserId
    }
  }

  return normalized
}

export function normalizeClientMessagePage(value: unknown): ClientMessagePage {
  const page = asRecord(value)
  const limit = asNumber(page?.limit)
  const newestSeq = asNumber(page?.newest_seq)
  const oldestSeq = asNumber(page?.oldest_seq)

  if (!page || limit === undefined || newestSeq === undefined || oldestSeq === undefined) {
    throw new ApiRequestError("消息列表响应格式不正确")
  }

  return {
    hasMoreAfter: Boolean(page.has_more_after),
    hasMoreBefore: Boolean(page.has_more_before),
    limit,
    newestSeq,
    oldestSeq,
  }
}

export function normalizeTemporaryFileReadUrl(value: unknown): TemporaryFileReadUrl {
  const item = asRecord(value)
  const expiresAt = asString(item?.expires_at)
  const fileId = asString(item?.file_id)
  const url = asString(item?.url)

  if (!item || !expiresAt || !fileId || !url) {
    throw new ApiRequestError("文件访问地址响应格式不正确")
  }

  return { expiresAt, fileId, url }
}

function normalizeMessageBodyOrUnsupported(value: unknown): ClientMessageBody {
  try {
    return normalizeMessageBody(value)
  } catch {
    return { type: "unsupported" }
  }
}

function normalizeMessageBody(
  value: unknown,
  forwardBundleDepth = 0
): ClientMessageBody {
  const body = asRecord(value)
  const type = asString(body?.type)

  if (!body || !type) {
    throw new Error("invalid message body")
  }

  if (type === "text" || type === "markdown") {
    const content = asString(body.content)
    if (content === undefined) throw new Error("invalid message body")
    return { content, type }
  }

  if (type === "link") {
    const title = asString(body.title)
    const url = asString(body.url)
    if (title === undefined || url === undefined) throw new Error("invalid message body")
    return { title, type, url }
  }

  if (type === "card") {
    const description = asString(body.description)
    const title = asString(body.title)
    const url = asString(body.url)
    if (description === undefined || title === undefined || url === undefined) {
      throw new Error("invalid message body")
    }
    return { description, title, type, url }
  }

  if (type === "chart") {
    const chartType = asString(body.chart_type)
    const data = asRecord(body.data)
    const description = asString(body.description)
    const title = asString(body.title)
    if (
      !data ||
      (chartType !== "line" &&
        chartType !== "bar" &&
        chartType !== "pie" &&
        chartType !== "radar") ||
      description === undefined ||
      title === undefined
    ) {
      throw new Error("invalid message body")
    }
    return { chartType, data, description, title, type }
  }

  if (type === "file") {
    const fileId = asString(body.file_id)
    const name = asString(body.name)
    const sizeBytes = asNumber(body.size_bytes)
    if (!fileId || name === undefined || sizeBytes === undefined || sizeBytes < 0) {
      throw new Error("invalid message body")
    }
    return { fileId, name, sizeBytes, type }
  }

  if (type === "image") {
    const fileId = asString(body.file_id)
    if (!fileId) throw new Error("invalid message body")
    const image: Extract<ClientMessageBody, { type: "image" }> = { fileId, type }
    const width = asNumber(body.width)
    const height = asNumber(body.height)
    if (width !== undefined && width > 0) image.width = width
    if (height !== undefined && height > 0) image.height = height
    return image
  }

  if (type === "voice") {
    const contentType = asString(body.content_type)
    const durationMS = asNumber(body.duration_ms)
    const fileId = asString(body.file_id)
    const sizeBytes = asNumber(body.size_bytes)
    if (
      !contentType ||
      !durationMS ||
      durationMS < 0 ||
      durationMS > 60_000 ||
      !fileId ||
      !sizeBytes ||
      sizeBytes < 0
    ) {
      throw new Error("invalid message body")
    }
    return {
      contentType,
      durationMS,
      fileId,
      sizeBytes,
      transcript: asString(body.transcript)?.trim() ?? "",
      type,
    }
  }

  if (type === "forward_bundle") {
    return normalizeForwardBundle(body, forwardBundleDepth)
  }

  if (type === "system_event") {
    return normalizeSystemEvent(body)
  }

  throw new Error("invalid message body")
}

function normalizeForwardBundle(
  body: Record<string, unknown>,
  depth: number
): Extract<ClientMessageBody, { type: "forward_bundle" }> {
  const itemCount = asNumber(body.item_count)
  const items = Array.isArray(body.items) ? body.items : null

  if (
    depth >= MAX_FORWARD_BUNDLE_DEPTH ||
    !itemCount ||
    itemCount > MAX_FORWARD_BUNDLE_ITEMS ||
    !items ||
    items.length !== itemCount
  ) {
    throw new Error("invalid message body")
  }

  return {
    itemCount,
    items: items.map((value) => {
      const item = asRecord(value)
      const senderName = asString(item?.sender_name)?.trim()
      const senderType = asString(item?.sender_type)
      const sentAt = asString(item?.sent_at)
      const summary = asString(item?.summary)
      const itemBody = normalizeMessageBody(item?.body, depth + 1)

      if (
        !item ||
        !senderName ||
        (senderType !== "user" && senderType !== "app") ||
        !sentAt ||
        summary === undefined ||
        !isForwardableBody(itemBody)
      ) {
        throw new Error("invalid message body")
      }

      return { body: itemBody, senderName, senderType, sentAt, summary }
    }),
    type: "forward_bundle",
  }
}

function normalizeSystemEvent(
  body: Record<string, unknown>
): ClientSystemEventMessageBody {
  const event = asString(body.event)

  if (event === "group_members_invited") {
    const inviter = normalizeSystemUser(body.inviter)
    const invitees = Array.isArray(body.invitees)
      ? body.invitees.map(normalizeSystemUser)
      : null
    if (!inviter || !invitees || invitees.some((invitee) => !invitee)) {
      throw new Error("invalid message body")
    }
    return {
      event,
      invitees: invitees as ClientSystemEventUserRef[],
      inviter,
      type: "system_event",
    }
  }

  const actor = normalizeSystemUser(body.actor)
  if (!actor) throw new Error("invalid message body")

  if (
    event === "group_avatar_updated" ||
    event === "group_member_joined" ||
    event === "group_member_left" ||
    event === "message_revoked"
  ) {
    return { actor, event, type: "system_event" }
  }

  if (event === "group_visibility_changed") {
    return {
      actor,
      event,
      type: "system_event",
      visibility: body.visibility === "public" ? "public" : "private",
    }
  }

  if (event === "group_member_removed") {
    const target = normalizeSystemUser(body.target)
    if (!target) throw new Error("invalid message body")
    return { actor, event, target, type: "system_event" }
  }

  if (event === "group_name_updated") {
    const name = asString(body.name)
    if (name === undefined) throw new Error("invalid message body")
    return { actor, event, name, type: "system_event" }
  }

  throw new Error("invalid message body")
}

function normalizeSystemUser(value: unknown): ClientSystemEventUserRef | null {
  const user = asRecord(value)
  const displayName = asString(user?.display_name)
  const id = asString(user?.id)
  return user && displayName && id ? { displayName, id } : null
}

function normalizeDelegatedBy(value: unknown) {
  if (value === undefined || value === null) return undefined
  const delegated = asRecord(value)
  const id = asString(delegated?.id)
  const name = asString(delegated?.name)
  const type = asString(delegated?.type)
  if (!delegated || !id || !name || (type !== "user" && type !== "app")) {
    throw new ApiRequestError("消息代发信息响应格式不正确")
  }
  return { id, name, type: type as "user" | "app" }
}

function normalizeReplyTo(value: unknown): ClientMessageReplyTo | undefined {
  if (value === undefined || value === null) return undefined
  const reply = asRecord(value)
  const sender = asRecord(reply?.sender)
  const id = asString(reply?.id)
  const senderId = asString(sender?.id) ?? ""
  const senderName = asString(sender?.name)
  const senderType = normalizeSenderType(sender?.type)
  const seq = asNumber(reply?.seq)
  const summary = asString(reply?.summary)
  if (
    !reply ||
    !sender ||
    !id ||
    (senderType !== "system" && !senderId) ||
    senderName === undefined ||
    seq === undefined ||
    summary === undefined
  ) {
    throw new ApiRequestError("消息引用信息响应格式不正确")
  }
  return {
    id,
    sender: { id: senderId, name: senderName, type: senderType },
    seq,
    summary,
  }
}

function isForwardableBody(
  body: ClientMessageBody
): body is ClientForwardableMessageBody {
  return body.type !== "system_event" && body.type !== "revoked" && body.type !== "unsupported"
}

function normalizeSenderType(value: unknown): "user" | "app" | "system" {
  return value === "app" || value === "system" ? value : "user"
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function asString(value: unknown) {
  return typeof value === "string" ? value : undefined
}

function asNumber(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}
