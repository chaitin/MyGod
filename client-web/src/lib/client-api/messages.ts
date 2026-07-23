import { ClientDataRequestError, createRequestError, readJson } from "./core"
import type {
  ClientDataFetch,
  ClientDataSuccessEnvelope,
  ClientDataErrorEnvelope,
  ForwardConversationMessagesResponse,
  ListConversationMessagesResponse,
  CreateMessageResponse,
  RevokeConversationMessageResponse,
  MarkConversationReadResponse,
  MessageCreatedEventPayloadResponse,
  MessageUpdatedEventPayloadResponse,
  MessageReactionsUpdatedEventPayloadResponse,
  ListMessageReactionUsersResponse,
  ListMessageReactionSnapshotsResponse,
  SetMessageReactionResponse,
  ConversationRemovedEventPayloadResponse,
  ConversationMemberMentionedEventPayloadResponse,
  ConversationMemberChoiceReceivedEventPayloadResponse,
  MessageChoiceUpdatedEventPayloadResponse,
  SubmitChoiceResponseResponse,
  ListChoiceSnapshotsResponse,
  TopicEventPayloadResponse,
  ReadTemporaryFileURLsResponse,
  ClientMessageBody,
  ClientMessage,
  ListConversationMessagesOptions,
  SendConversationTextMessageInput,
  SendConversationMarkdownMessageInput,
  SendConversationLinkMessageInput,
  SendConversationCardMessageInput,
  SendConversationChartMessageInput,
  SendConversationEntityCardMessageInput,
  SendConversationFileMessageInput,
  SendConversationImageMessageInput,
  SendConversationVoiceMessageInput,
  TemporaryFileReadURL,
  MarkConversationReadOptions,
  MarkConversationReadResult,
  ForwardConversationMessagesInput,
  ForwardConversationMessagesResult,
  MessageReactionsUpdatedEvent,
  MessageReactionSnapshot,
  ClientMessageReactionUser,
  SetMessageReactionInput,
  MessageChoiceUpdatedEvent,
  MessageChoiceSnapshot,
  SubmitChoiceResponseResult,
} from "./types"
import {
  isTemporaryFileReadURLFresh,
  normalizeMarkConversationReadResult,
  normalizeMessage,
  normalizeMessagePage,
  normalizeMessageReactions,
  normalizeMessageReactionUsers,
  normalizeMessageChoiceState,
  normalizeTemporaryFileReadURL,
} from "./message-normalizers"

const temporaryFileReadURLCache = new Map<string, TemporaryFileReadURL>()

export async function submitConversationMessageChoiceResponse(
  conversationId: string,
  messageId: string,
  optionIds: string[],
  fetcher: ClientDataFetch = fetch
): Promise<SubmitChoiceResponseResult> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/choice-response`,
    {
      body: JSON.stringify({ option_ids: optionIds }),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "PUT",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<SubmitChoiceResponseResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "提交选择失败")
  }
  const data = (
    payload as
      ClientDataSuccessEnvelope<SubmitChoiceResponseResponse> | undefined
  )?.data
  if (
    data?.conversation_id !== conversationId ||
    data.message_id !== messageId ||
    typeof data.created !== "boolean" ||
    !data.response?.id ||
    !data.response.created_at ||
    !data.response.user_id ||
    !Array.isArray(data.response.option_ids)
  ) {
    throw new ClientDataRequestError("提交选择响应格式不正确")
  }
  return {
    choice: normalizeMessageChoiceState(data.choice),
    conversationId,
    created: data.created,
    messageId,
    response: {
      createdAt: data.response.created_at,
      id: data.response.id,
      optionIds: [...data.response.option_ids],
      userId: data.response.user_id,
    },
  }
}

export async function listConversationMessageChoiceSnapshots(
  conversationId: string,
  messageIds: string[],
  fetcher: ClientDataFetch = fetch
): Promise<MessageChoiceSnapshot[]> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/choices/query`,
    {
      body: JSON.stringify({ message_ids: messageIds }),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListChoiceSnapshotsResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "同步选择状态失败")
  }
  const data = (
    payload as
      ClientDataSuccessEnvelope<ListChoiceSnapshotsResponse> | undefined
  )?.data
  if (
    data?.conversation_id !== conversationId ||
    !Array.isArray(data.snapshots)
  ) {
    throw new ClientDataRequestError("选择状态快照响应格式不正确")
  }
  const snapshots = data.snapshots.map((snapshot) => {
    if (
      !snapshot?.message_id ||
      (snapshot.status !== "active" &&
        snapshot.status !== "deleted" &&
        snapshot.status !== "revoked") ||
      (snapshot.status === "active" && !snapshot.choice)
    ) {
      throw new ClientDataRequestError("选择状态快照响应格式不正确")
    }
    return {
      choice:
        snapshot.status === "active"
          ? normalizeMessageChoiceState(snapshot.choice)
          : null,
      conversationId,
      messageId: snapshot.message_id,
      status: snapshot.status,
    }
  })
  const requestedMessageIds = [...new Set(messageIds)]
  if (
    snapshots.length !== requestedMessageIds.length ||
    snapshots.some(
      (snapshot, index) => snapshot.messageId !== requestedMessageIds[index]
    )
  ) {
    throw new ClientDataRequestError("选择状态快照响应格式不正确")
  }
  return snapshots
}

export async function setConversationMessageReaction(
  conversationId: string,
  messageId: string,
  input: SetMessageReactionInput,
  fetcher: ClientDataFetch = fetch
): Promise<MessageReactionSnapshot> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/reactions`,
    {
      body: JSON.stringify({ reacted: input.reacted, text: input.text }),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "PUT",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<SetMessageReactionResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "更新消息表情失败")
  }
  const data = (
    payload as ClientDataSuccessEnvelope<SetMessageReactionResponse> | undefined
  )?.data
  if (
    !data?.conversation_id ||
    !data.message_id ||
    !Number.isSafeInteger(data.reaction_version) ||
    (data.reaction_version ?? -1) < 0
  ) {
    throw new ClientDataRequestError("消息表情响应格式不正确")
  }
  return {
    conversationId: data.conversation_id,
    messageId: data.message_id,
    reactionVersion: data.reaction_version!,
    reactions: normalizeMessageReactions(data.reactions),
  }
}

export async function listConversationMessageReactionSnapshots(
  conversationId: string,
  messageIds: string[],
  fetcher: ClientDataFetch = fetch
): Promise<MessageReactionSnapshot[]> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/reactions/query`,
    {
      body: JSON.stringify({ message_ids: messageIds }),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListMessageReactionSnapshotsResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "同步消息表情失败")
  }
  const data = (
    payload as
      | ClientDataSuccessEnvelope<ListMessageReactionSnapshotsResponse>
      | undefined
  )?.data
  if (
    data?.conversation_id !== conversationId ||
    !Array.isArray(data.snapshots)
  ) {
    throw new ClientDataRequestError("消息表情快照响应格式不正确")
  }
  const snapshots = data.snapshots.map((snapshot) => {
    if (
      !snapshot?.message_id ||
      !Number.isSafeInteger(snapshot.reaction_version) ||
      (snapshot.reaction_version ?? -1) < 0
    ) {
      throw new ClientDataRequestError("消息表情快照响应格式不正确")
    }
    return {
      conversationId: data.conversation_id!,
      messageId: snapshot.message_id,
      reactionVersion: snapshot.reaction_version!,
      reactions: normalizeMessageReactions(snapshot.reactions),
    }
  })
  const requestedMessageIds = [...new Set(messageIds)]
  if (
    snapshots.length !== requestedMessageIds.length ||
    snapshots.some(
      (snapshot, index) => snapshot.messageId !== requestedMessageIds[index]
    )
  ) {
    throw new ClientDataRequestError("消息表情快照响应格式不正确")
  }
  return snapshots
}

export async function listConversationMessageReactionUsers(
  conversationId: string,
  messageId: string,
  text: string,
  fetcher: ClientDataFetch = fetch
): Promise<ClientMessageReactionUser[]> {
  const searchParams = new URLSearchParams({ text })
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/reactions/users?${searchParams.toString()}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ListMessageReactionUsersResponse>
  >(response)
  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载表情参与者失败")
  }
  const data = (
    payload as
      ClientDataSuccessEnvelope<ListMessageReactionUsersResponse> | undefined
  )?.data
  if (
    data?.conversation_id !== conversationId ||
    data.message_id !== messageId ||
    data.text !== text
  ) {
    throw new ClientDataRequestError("消息表情参与者响应格式不正确")
  }
  return normalizeMessageReactionUsers(data.users)
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

export async function forwardConversationMessages(
  sourceConversationId: string,
  input: ForwardConversationMessagesInput,
  fetcher: ClientDataFetch = fetch
): Promise<ForwardConversationMessagesResult> {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(sourceConversationId)}/messages/forward`,
    {
      body: JSON.stringify({
        client_forward_id: input.clientForwardId,
        message_ids: input.messageIds,
        mode: input.mode,
        target_conversation_ids: input.targetConversationIds,
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
    | ClientDataSuccessEnvelope<ForwardConversationMessagesResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "转发消息失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<ForwardConversationMessagesResponse> | undefined
  )?.data
  if (
    typeof data?.sent_count !== "number" ||
    typeof data.failed_count !== "number" ||
    !Array.isArray(data.results)
  ) {
    throw new ClientDataRequestError("转发消息响应格式不正确")
  }

  return {
    failedCount: data.failed_count,
    results: data.results.map((result) => {
      if (
        !result.conversation_id ||
        (result.status !== "sent" && result.status !== "failed")
      ) {
        throw new ClientDataRequestError("转发消息响应格式不正确")
      }

      if (result.status === "sent") {
        if (!Array.isArray(result.messages)) {
          throw new ClientDataRequestError("转发消息响应格式不正确")
        }
        return {
          conversationId: result.conversation_id,
          messages: result.messages.map(normalizeMessage),
          status: "sent" as const,
        }
      }

      if (!result.error?.code || !result.error.message) {
        throw new ClientDataRequestError("转发消息响应格式不正确")
      }
      return {
        conversationId: result.conversation_id,
        error: {
          code: result.error.code,
          message: result.error.message,
        },
        messages: [],
        status: "failed" as const,
      }
    }),
    sentCount: data.sent_count,
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
        reply_to_message_id: input.replyToMessageId,
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
        reply_to_message_id: input.replyToMessageId,
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
        reply_to_message_id: input.replyToMessageId,
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

export async function sendConversationCardMessage(
  conversationId: string,
  input: SendConversationCardMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        reply_to_message_id: input.replyToMessageId,
        body: {
          description: input.description,
          title: input.title,
          type: "card",
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
    throw createRequestError(payload, response, "发送卡片失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationChartMessage(
  conversationId: string,
  input: SendConversationChartMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        reply_to_message_id: input.replyToMessageId,
        body: {
          chart_type: input.chart.chartType,
          data: input.chart.data,
          description: input.chart.description,
          title: input.chart.title,
          type: "chart",
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
    throw createRequestError(payload, response, "发送图表失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function sendConversationEntityCardMessage(
  conversationId: string,
  input: SendConversationEntityCardMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      body: JSON.stringify({
        client_message_id: input.clientMessageId,
        reply_to_message_id: input.replyToMessageId,
        body: {
          entity_id: input.entityId,
          entity_type: input.entityType,
          type: "entity_card",
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
    throw createRequestError(payload, response, "发送对象卡片失败")
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
  if (input.replyToMessageId) {
    formData.set("reply_to_message_id", input.replyToMessageId)
  }
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
  if (input.replyToMessageId) {
    formData.set("reply_to_message_id", input.replyToMessageId)
  }
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

export async function sendConversationVoiceMessage(
  conversationId: string,
  input: SendConversationVoiceMessageInput,
  fetcher: ClientDataFetch = fetch
) {
  const formData = new FormData()
  formData.set("client_message_id", input.clientMessageId)
  formData.set("duration_ms", String(input.durationMS))
  if (input.replyToMessageId) {
    formData.set("reply_to_message_id", input.replyToMessageId)
  }
  formData.set("voice", input.voice, "voice-message.webm")

  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/voices`,
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
    throw createRequestError(payload, response, "发送语音失败")
  }

  const message = (
    payload as ClientDataSuccessEnvelope<CreateMessageResponse> | undefined
  )?.data?.message

  return normalizeMessage(message)
}

export async function revokeConversationMessage(
  conversationId: string,
  messageId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/revoke`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<RevokeConversationMessageResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "撤回消息失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<RevokeConversationMessageResponse> | undefined
  )?.data
  if (!data?.message || !data.system_message) {
    throw new ClientDataRequestError("撤回消息响应格式不正确")
  }

  return {
    message: normalizeMessage(data.message),
    systemMessage: normalizeMessage(data.system_message),
  }
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

export function normalizeMessageCreatedEventNotificationMuted(
  payload: unknown
) {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("消息推送格式不正确")
  }

  return Boolean(
    (payload as MessageCreatedEventPayloadResponse).notification_muted
  )
}

export function normalizeMessageUpdatedEventPayload(
  payload: unknown
): ClientMessage {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("消息更新推送格式不正确")
  }

  return normalizeMessage(
    (payload as MessageUpdatedEventPayloadResponse).message
  )
}

export function normalizeMessageReactionsUpdatedEventPayload(
  payload: unknown
): MessageReactionsUpdatedEvent {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("消息表情推送格式不正确")
  }
  const value = payload as MessageReactionsUpdatedEventPayloadResponse
  if (
    typeof value.actor_reacted !== "boolean" ||
    typeof value.actor_text !== "string" ||
    !value.actor_text ||
    typeof value.actor_user_id !== "string" ||
    !value.actor_user_id ||
    typeof value.conversation_id !== "string" ||
    !value.conversation_id ||
    typeof value.message_id !== "string" ||
    !value.message_id ||
    !Number.isSafeInteger(value.reaction_version) ||
    (value.reaction_version ?? -1) < 0 ||
    !Array.isArray(value.reactions)
  ) {
    throw new ClientDataRequestError("消息表情推送格式不正确")
  }
  const reactions = value.reactions.map((reaction) => {
    if (
      typeof reaction?.text !== "string" ||
      !reaction.text ||
      typeof reaction.count !== "number" ||
      !Number.isInteger(reaction.count) ||
      reaction.count <= 0
    ) {
      throw new ClientDataRequestError("消息表情推送格式不正确")
    }
    return {
      count: reaction.count,
      text: reaction.text,
      users: normalizeMessageReactionUsers(reaction.users),
    }
  })
  return {
    actorReacted: value.actor_reacted,
    actorText: value.actor_text,
    actorUserId: value.actor_user_id,
    conversationId: value.conversation_id,
    messageId: value.message_id,
    reactionVersion: value.reaction_version!,
    reactions,
  }
}

export function normalizeConversationRemovedEventPayload(payload: unknown) {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("会话移除推送格式不正确")
  }

  const conversationId = (payload as ConversationRemovedEventPayloadResponse)
    .conversation_id
  if (typeof conversationId !== "string" || conversationId.trim() === "") {
    throw new ClientDataRequestError("会话移除推送格式不正确")
  }

  return {
    conversationId,
  }
}

export function normalizeConversationMemberMentionedEventPayload(
  payload: unknown
) {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("会话提醒推送格式不正确")
  }

  const event = payload as ConversationMemberMentionedEventPayloadResponse
  if (
    typeof event.conversation_id !== "string" ||
    event.conversation_id.trim() === "" ||
    typeof event.last_mentioned_seq !== "number"
  ) {
    throw new ClientDataRequestError("会话提醒推送格式不正确")
  }

  return {
    conversationId: event.conversation_id,
    lastMentionedSeq: event.last_mentioned_seq,
  }
}

export function normalizeConversationMemberChoiceReceivedEventPayload(
  payload: unknown
) {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("选择消息提醒推送格式不正确")
  }

  const event = payload as ConversationMemberChoiceReceivedEventPayloadResponse
  if (
    typeof event.conversation_id !== "string" ||
    event.conversation_id.trim() === "" ||
    typeof event.last_choice_seq !== "number"
  ) {
    throw new ClientDataRequestError("选择消息提醒推送格式不正确")
  }

  return {
    conversationId: event.conversation_id,
    lastChoiceSeq: event.last_choice_seq,
  }
}

export function normalizeMessageChoiceUpdatedEventPayload(
  payload: unknown
): MessageChoiceUpdatedEvent {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("选择消息更新推送格式不正确")
  }

  const event = payload as MessageChoiceUpdatedEventPayloadResponse
  if (
    typeof event.conversation_id !== "string" ||
    event.conversation_id.trim() === "" ||
    typeof event.message_id !== "string" ||
    event.message_id.trim() === "" ||
    typeof event.actor_user_id !== "string" ||
    event.actor_user_id.trim() === "" ||
    !Array.isArray(event.actor_option_ids) ||
    !event.actor_option_ids.every(
      (optionId) => typeof optionId === "string" && optionId !== ""
    )
  ) {
    throw new ClientDataRequestError("选择消息更新推送格式不正确")
  }

  return {
    actorOptionIds: [...event.actor_option_ids],
    actorUserId: event.actor_user_id,
    choice: normalizeMessageChoiceState(event.choice),
    conversationId: event.conversation_id,
    messageId: event.message_id,
  }
}

export function normalizeTopicEventPayload(payload: unknown) {
  if (!isObject(payload)) {
    throw new ClientDataRequestError("话题推送格式不正确")
  }
  const event = payload as TopicEventPayloadResponse
  if (
    !event.conversation_id ||
    !event.parent_conversation_id ||
    !event.source_message_id
  ) {
    throw new ClientDataRequestError("话题推送格式不正确")
  }
  return {
    archived: Boolean(event.archived),
    conversationId: event.conversation_id,
    parentConversationId: event.parent_conversation_id,
    sourceMessageId: event.source_message_id,
  }
}

export function formatClientMessageBodySummary(body: ClientMessageBody) {
  if (body.type === "text") {
    return body.content
  }

  if (body.type === "markdown") {
    return formatMarkdownMessageSummary(body.content)
  }

  if (body.type === "choice") {
    const content =
      body.contentType === "markdown"
        ? formatMarkdownMessageSummary(body.content)
        : body.content
    return `[选择] ${content}`
  }

  if (body.type === "link") {
    return `[链接] ${body.title}`
  }

  if (body.type === "card") {
    return `[卡片] ${body.title}`
  }

  if (body.type === "chart") {
    return `[图表] ${body.title}`
  }

  if (body.type === "file") {
    return `[文件] ${body.name}`
  }

  if (body.type === "image") {
    return "[图片]"
  }

  if (body.type === "voice") {
    const summary = `[语音] ${formatVoiceMessageDuration(body.durationMS)}`

    return body.transcript ? `${summary} - ${body.transcript}` : summary
  }

  if (body.type === "forward_bundle") {
    const preview = truncateForwardBundleSummary(body.items[0]?.summary ?? "")
    return `[聊天记录] ${body.itemCount} 条 - ${preview || "消息"}`
  }

  if (body.type === "revoked") {
    return "该消息已被撤回"
  }

  if (body.type === "unsupported") {
    return "暂不支持查看该消息"
  }

  if (body.event === "message_revoked") {
    return `${body.actor.displayName} 撤回了一条消息`
  }

  if (body.event === "topic_closed") {
    return `${body.actor.displayName} 已将话题关闭`
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

  if (body.event === "group_member_removed") {
    return `${body.actor.displayName} 已将 ${body.target.displayName} 移出群聊`
  }

  if (body.event === "group_name_updated") {
    return `${body.actor.displayName} 修改群聊名称为 ${body.name}`
  }

  return `${body.inviter.displayName} 邀请 ${body.invitees
    .map((invitee) => invitee.displayName)
    .join(",")} 加入群聊`
}

function truncateForwardBundleSummary(content: string) {
  const characters = Array.from(content.trim())
  if (characters.length <= 100) {
    return characters.join("")
  }

  return `${characters.slice(0, 100).join("").trim()}…`
}

function formatVoiceMessageDuration(durationMS: number) {
  const totalSeconds = Math.ceil(durationMS / 1_000)

  return `${String(Math.floor(totalSeconds / 60)).padStart(2, "0")}:${String(totalSeconds % 60).padStart(2, "0")}`
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
    message.body.event === "group_member_removed" ||
    message.body.event === "group_name_updated" ||
    message.body.event === "message_revoked" ||
    message.body.event === "topic_closed"
  ) {
    return message.body.actor.id === userId
  }

  return message.body.inviter.id === userId
}

function isDefined<T>(value: T | null | undefined): value is T {
  return value !== null && value !== undefined
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}
