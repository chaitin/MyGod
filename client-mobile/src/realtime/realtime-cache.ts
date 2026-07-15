import type { InfiniteData, QueryClient } from "@tanstack/react-query"

import { fetchConversationMessages } from "@/data/messages-api"
import { normalizeClientMessage } from "@/data/message-normalizer"
import type {
  ClientConversation,
  ClientMessage,
  ClientMessageList,
} from "@/data/models"
import { queryKeys, type ServerTarget } from "@/data/query"
import { realtimeEvents } from "@/realtime/realtime-protocol"

type MessageInfiniteData = InfiniteData<ClientMessageList, number | null>

const CATCH_UP_PAGE_SIZE = 20
const MAX_CATCH_UP_PAGES = 100

export async function applyRealtimeEvent(
  queryClient: QueryClient,
  server: ServerTarget,
  event: string,
  payload: unknown
) {
  if (
    event === realtimeEvents.messageCreated ||
    event === realtimeEvents.messageUpdated
  ) {
    const message = normalizeMessageEventPayload(payload)
    const queryKey = queryKeys.conversationMessages(
      server,
      message.conversationId
    )

    queryClient.setQueryData<MessageInfiniteData>(queryKey, (current) =>
      event === realtimeEvents.messageCreated
        ? appendMessages(current, [message])
        : updateMessage(current, message)
    )

    void queryClient.invalidateQueries(
      {
        exact: true,
        queryKey: queryKeys.conversations(server),
      },
      { cancelRefetch: false }
    )
    return { message }
  }

  if (event === realtimeEvents.conversationRemoved) {
    const conversationId = normalizeConversationRemovedPayload(payload)

    await queryClient.cancelQueries({
      exact: true,
      queryKey: queryKeys.conversations(server),
    })

    queryClient.setQueryData<ClientConversation[]>(
      queryKeys.conversations(server),
      (current) =>
        current?.filter((conversation) => conversation.id !== conversationId)
    )
    queryClient.removeQueries({
      exact: true,
      queryKey: queryKeys.conversationMessages(server, conversationId),
    })
    return {}
  }

  if (event === realtimeEvents.conversationMemberMentioned) {
    const mentioned = normalizeConversationMentionedPayload(payload)

    queryClient.setQueryData<ClientConversation[]>(
      queryKeys.conversations(server),
      (current) =>
        current?.map((conversation) =>
          conversation.id === mentioned.conversationId
            ? {
                ...conversation,
                lastMentionedSeq: Math.max(
                  conversation.lastMentionedSeq,
                  mentioned.lastMentionedSeq
                ),
              }
            : conversation
        )
    )
    return {}
  }

  return {}
}

export async function synchronizeRealtimeData(
  queryClient: QueryClient,
  server: ServerTarget
) {
  const loadedConversationQueries =
    queryClient.getQueriesData<MessageInfiniteData>({
      queryKey: ["server", server.id, server.url, "conversation"],
    })

  await Promise.all([
    queryClient.invalidateQueries(
      {
        exact: true,
        queryKey: queryKeys.conversations(server),
      },
      { cancelRefetch: false }
    ),
    ...loadedConversationQueries.flatMap(([queryKey, data]) => {
      const conversationId = getConversationIdFromMessageQueryKey(queryKey)
      const newestSeq = data ? getNewestMessageSeq(data) : 0

      return conversationId && newestSeq > 0
        ? [
            catchUpConversationMessages(
              queryClient,
              server,
              conversationId,
              newestSeq
            ),
          ]
        : []
    }),
  ])
}

export async function refreshClientDataOnForeground(
  queryClient: QueryClient,
  server: ServerTarget
) {
  await Promise.all([
    queryClient.invalidateQueries(
      {
        exact: true,
        queryKey: queryKeys.contacts(server),
      },
      { cancelRefetch: false }
    ),
    queryClient.invalidateQueries(
      {
        exact: true,
        queryKey: queryKeys.currentUser(server),
      },
      { cancelRefetch: false }
    ),
    synchronizeRealtimeData(queryClient, server),
  ])
}

async function catchUpConversationMessages(
  queryClient: QueryClient,
  server: ServerTarget,
  conversationId: string,
  initialAfterSeq: number
) {
  let afterSeq = initialAfterSeq

  for (let pageIndex = 0; pageIndex < MAX_CATCH_UP_PAGES; pageIndex += 1) {
    const result = await fetchConversationMessages(
      server.url,
      conversationId,
      { afterSeq, limit: CATCH_UP_PAGE_SIZE }
    )

    if (result.messages.length > 0) {
      queryClient.setQueryData<MessageInfiniteData>(
        queryKeys.conversationMessages(server, conversationId),
        (current) => appendMessages(current, result.messages)
      )
    }

    if (!result.page.hasMoreAfter) {
      return
    }

    const nextAfterSeq = result.messages.reduce(
      (newest, message) => Math.max(newest, message.seq),
      afterSeq
    )
    if (nextAfterSeq <= afterSeq) {
      break
    }
    afterSeq = nextAfterSeq
  }

  // An unusually large gap is cheaper and safer to reload than to leave a
  // partially synchronized cache behind.
  await queryClient.invalidateQueries({
    exact: true,
    queryKey: queryKeys.conversationMessages(server, conversationId),
  })
}

function appendMessages(
  current: MessageInfiniteData | undefined,
  incoming: ClientMessage[]
) {
  if (!current || current.pages.length === 0 || incoming.length === 0) {
    return current
  }

  const incomingIds = new Set(incoming.map((message) => message.id))
  const pages = current.pages.map((page) => ({
    ...page,
    messages: page.messages.filter((message) => !incomingIds.has(message.id)),
  }))
  const firstPage = pages[0]
  const messages = mergeMessages([...firstPage.messages, ...incoming])

  pages[0] = {
    ...firstPage,
    messages,
    page: {
      ...firstPage.page,
      hasMoreAfter: false,
      newestSeq: Math.max(
        firstPage.page.newestSeq,
        ...incoming.map((message) => message.seq)
      ),
    },
  }

  return { ...current, pages }
}

function updateMessage(
  current: MessageInfiniteData | undefined,
  incoming: ClientMessage
) {
  if (!current) {
    return current
  }

  let found = false
  const pages = current.pages.map((page) => ({
    ...page,
    messages: page.messages.map((message) => {
      if (message.id !== incoming.id) {
        return message
      }

      found = true
      return incoming
    }),
  }))

  return found ? { ...current, pages } : current
}

function mergeMessages(messages: ClientMessage[]) {
  const messagesById = new Map<string, ClientMessage>()
  for (const message of messages) {
    messagesById.set(message.id, message)
  }

  return Array.from(messagesById.values()).sort(
    (left, right) => right.seq - left.seq
  )
}

function getNewestMessageSeq(data: MessageInfiniteData) {
  let newestSeq = 0
  for (const page of data.pages) {
    for (const message of page.messages) {
      newestSeq = Math.max(newestSeq, message.seq)
    }
  }
  return newestSeq
}

function getConversationIdFromMessageQueryKey(queryKey: readonly unknown[]) {
  return queryKey.length === 6 &&
    queryKey[0] === "server" &&
    queryKey[3] === "conversation" &&
    typeof queryKey[4] === "string" &&
    queryKey[5] === "messages"
    ? queryKey[4]
    : null
}

function normalizeMessageEventPayload(payload: unknown) {
  const value = asRecord(payload)
  if (!value || !("message" in value)) {
    throw new Error("实时消息格式不正确")
  }
  return normalizeClientMessage(value.message)
}

function normalizeConversationRemovedPayload(payload: unknown) {
  const value = asRecord(payload)
  if (!value || typeof value.conversation_id !== "string") {
    throw new Error("实时会话移除事件格式不正确")
  }
  return value.conversation_id
}

function normalizeConversationMentionedPayload(payload: unknown) {
  const value = asRecord(payload)
  if (
    !value ||
    typeof value.conversation_id !== "string" ||
    typeof value.last_mentioned_seq !== "number" ||
    !Number.isFinite(value.last_mentioned_seq) ||
    value.last_mentioned_seq <= 0
  ) {
    throw new Error("实时会话提醒事件格式不正确")
  }

  return {
    conversationId: value.conversation_id,
    lastMentionedSeq: value.last_mentioned_seq,
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}
