import { File } from "expo-file-system"

import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import {
  normalizeClientMessage,
  normalizeClientMessagePage,
  normalizeMessageReactions,
  normalizeReactionVersion,
} from "@/data/message-normalizer"
import type {
  ClientMessageList,
  MessageReactionSnapshot,
} from "@/data/models"
import type { ClientMessageUpload } from "@/data/message-upload"

type ApiOptions = {
  fetcher?: ApiFetch
  signal?: AbortSignal
}

export async function fetchConversationMessages(
  serverUrl: string,
  conversationId: string,
  input: { afterSeq?: number; beforeSeq?: number; limit?: number } = {},
  options: ApiOptions = {}
): Promise<ClientMessageList> {
  const search = new URLSearchParams({ limit: String(input.limit ?? 20) })
  if (input.beforeSeq !== undefined) search.set("before_seq", String(input.beforeSeq))
  if (input.afterSeq !== undefined) search.set("after_seq", String(input.afterSeq))

  const data = await createApiClient(serverUrl, options.fetcher).request<{
    messages?: unknown[]
    page?: unknown
  }>(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages?${search.toString()}`,
    {
      errorMessage: "加载消息失败",
      method: "GET",
      signal: options.signal,
    }
  )

  if (!Array.isArray(data?.messages) || !data.page) {
    throw new ApiRequestError("消息列表响应格式不正确")
  }

  return {
    messages: data.messages.map(normalizeClientMessage),
    page: normalizeClientMessagePage(data.page),
  }
}

export async function setConversationMessageReaction(
  serverUrl: string,
  conversationId: string,
  messageId: string,
  input: { reacted: boolean; text: string },
  options: ApiOptions = {}
): Promise<MessageReactionSnapshot> {
  const data = await createApiClient(serverUrl, options.fetcher).request<{
    conversation_id?: unknown
    message_id?: unknown
    reaction_version?: unknown
    reactions?: unknown
  }>(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/reactions`,
    {
      body: JSON.stringify({ reacted: input.reacted, text: input.text }),
      errorMessage: "更新消息表情失败",
      headers: { "Content-Type": "application/json" },
      method: "PUT",
      signal: options.signal,
    }
  )

  return normalizeReactionSnapshot(data, conversationId, messageId, true)
}

export async function fetchConversationMessageReactionSnapshots(
  serverUrl: string,
  conversationId: string,
  messageIds: string[],
  options: ApiOptions = {}
): Promise<MessageReactionSnapshot[]> {
  const uniqueMessageIds = [...new Set(messageIds)]
  const data = await createApiClient(serverUrl, options.fetcher).request<{
    conversation_id?: unknown
    snapshots?: unknown
  }>(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/reactions/query`,
    {
      body: JSON.stringify({ message_ids: uniqueMessageIds }),
      errorMessage: "同步消息表情失败",
      headers: { "Content-Type": "application/json" },
      method: "POST",
      signal: options.signal,
    }
  )

  if (
    data?.conversation_id !== conversationId ||
    !Array.isArray(data.snapshots)
  ) {
    throw new ApiRequestError("消息表情快照响应格式不正确")
  }

  const snapshots = data.snapshots.map((value, index) =>
    normalizeReactionSnapshot(
      value,
      conversationId,
      uniqueMessageIds[index] ?? ""
    )
  )
  if (
    snapshots.length !== uniqueMessageIds.length ||
    snapshots.some(
      (snapshot, index) => snapshot.messageId !== uniqueMessageIds[index]
    )
  ) {
    throw new ApiRequestError("消息表情快照响应格式不正确")
  }

  return snapshots
}

export async function sendConversationTextMessage(
  serverUrl: string,
  conversationId: string,
  input: { clientMessageId: string; content: string },
  options: ApiOptions = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<{
    message?: unknown
  }>(`/api/client/conversations/${encodeURIComponent(conversationId)}/messages`, {
    body: JSON.stringify({
      body: { content: input.content, type: "text" },
      client_message_id: input.clientMessageId,
    }),
    errorMessage: "发送消息失败",
    headers: { "Content-Type": "application/json" },
    method: "POST",
    signal: options.signal,
  })

  if (!data?.message) {
    throw new ApiRequestError("发送消息响应格式不正确")
  }

  return normalizeClientMessage(data.message)
}

export function sendConversationFileMessage(
  serverUrl: string,
  conversationId: string,
  input: { clientMessageId: string; file: ClientMessageUpload },
  options: ApiOptions = {}
) {
  return sendConversationUploadMessage(
    serverUrl,
    conversationId,
    {
      clientMessageId: input.clientMessageId,
      fieldName: "file",
      path: "files",
      upload: input.file,
    },
    "发送文件失败",
    options
  )
}

export function sendConversationImageMessage(
  serverUrl: string,
  conversationId: string,
  input: { clientMessageId: string; image: ClientMessageUpload },
  options: ApiOptions = {}
) {
  return sendConversationUploadMessage(
    serverUrl,
    conversationId,
    {
      clientMessageId: input.clientMessageId,
      fieldName: "image",
      path: "images",
      upload: input.image,
    },
    "发送图片失败",
    options
  )
}

export function sendConversationVoiceMessage(
  serverUrl: string,
  conversationId: string,
  input: {
    clientMessageId: string
    durationMS: number
    voice: ClientMessageUpload
  },
  options: ApiOptions = {}
) {
  return sendConversationUploadMessage(
    serverUrl,
    conversationId,
    {
      clientMessageId: input.clientMessageId,
      extraFields: { duration_ms: String(input.durationMS) },
      fieldName: "voice",
      path: "voices",
      upload: input.voice,
    },
    "发送语音失败",
    options
  )
}

export async function markConversationRead(
  serverUrl: string,
  conversationId: string,
  upToSeq: number,
  options: ApiOptions = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<{
    conversation_id?: string
    last_read_seq?: number
    unread_count?: number
  }>(`/api/client/conversations/${encodeURIComponent(conversationId)}/read`, {
    body: JSON.stringify({ up_to_seq: upToSeq }),
    errorMessage: "标记会话已读失败",
    headers: { "Content-Type": "application/json" },
    method: "POST",
    signal: options.signal,
  })

  if (
    !data?.conversation_id ||
    typeof data.last_read_seq !== "number" ||
    typeof data.unread_count !== "number"
  ) {
    throw new ApiRequestError("标记会话已读响应格式不正确")
  }

  return {
    conversationId: data.conversation_id,
    lastReadSeq: data.last_read_seq,
    unreadCount: data.unread_count,
  }
}

async function sendConversationUploadMessage(
  serverUrl: string,
  conversationId: string,
  input: {
    clientMessageId: string
    extraFields?: Record<string, string>
    fieldName: "file" | "image" | "voice"
    path: "files" | "images" | "voices"
    upload: ClientMessageUpload
  },
  errorMessage: string,
  options: ApiOptions
) {
  const formData = new FormData()
  const file = new File(input.upload.uri)

  formData.set("client_message_id", input.clientMessageId)
  for (const [name, value] of Object.entries(input.extraFields ?? {})) {
    formData.set(name, value)
  }
  if (input.fieldName === "voice") {
    formData.set(
      input.fieldName,
      createTypedFilePart(file, input.upload)
    )
  } else {
    formData.set(input.fieldName, file, input.upload.name)
  }

  const data = await createApiClient(serverUrl, options.fetcher).request<{
    message?: unknown
  }>(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${input.path}`,
    {
      body: formData,
      errorMessage,
      method: "POST",
      signal: options.signal,
      timeoutMs: 120_000,
    }
  )

  if (!data?.message) {
    throw new ApiRequestError(`${errorMessage}：响应格式不正确`)
  }

  return normalizeClientMessage(data.message)
}

function createTypedFilePart(
  file: File,
  upload: ClientMessageUpload
): Blob {
  // Expo Fetch accepts file-like values with bytes(), name and type. Android
  // otherwise classifies .webm as video/webm instead of the required audio MIME.
  return {
    bytes: () => file.bytes(),
    name: upload.name,
    type: upload.mimeType,
  } as unknown as Blob
}

function normalizeReactionSnapshot(
  value: unknown,
  expectedConversationId: string,
  expectedMessageId: string,
  requireConversationId = false
): MessageReactionSnapshot {
  const snapshot = asRecord(value)
  const responseConversationId = asString(snapshot?.conversation_id)
  const messageId = asString(snapshot?.message_id)
  if (
    !snapshot ||
    (requireConversationId && responseConversationId !== expectedConversationId) ||
    (responseConversationId !== undefined &&
      responseConversationId !== expectedConversationId) ||
    messageId !== expectedMessageId ||
    !Number.isSafeInteger(snapshot.reaction_version) ||
    (snapshot.reaction_version as number) < 0
  ) {
    throw new ApiRequestError("消息表情快照响应格式不正确")
  }

  return {
    conversationId: expectedConversationId,
    messageId,
    reactionVersion: normalizeReactionVersion(snapshot.reaction_version),
    reactions: normalizeMessageReactions(snapshot.reactions),
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function asString(value: unknown) {
  return typeof value === "string" ? value : undefined
}
