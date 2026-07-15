import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import {
  normalizeClientMessage,
  normalizeClientMessagePage,
  normalizeTemporaryFileReadUrl,
} from "@/data/message-normalizer"
import type {
  ClientMessageList,
  TemporaryFileReadUrl,
} from "@/data/models"

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

export async function fetchTemporaryFileReadUrls(
  serverUrl: string,
  fileIds: string[],
  options: ApiOptions = {}
): Promise<TemporaryFileReadUrl[]> {
  if (fileIds.length === 0) return []

  const data = await createApiClient(serverUrl, options.fetcher).request<{
    urls?: unknown[]
  }>("/api/client/temporary-files/read-urls", {
    body: JSON.stringify({ file_ids: Array.from(new Set(fileIds)) }),
    errorMessage: "获取文件访问地址失败",
    headers: { "Content-Type": "application/json" },
    method: "POST",
    signal: options.signal,
  })

  if (!Array.isArray(data?.urls)) {
    throw new ApiRequestError("文件访问地址响应格式不正确")
  }

  return data.urls.map(normalizeTemporaryFileReadUrl)
}
