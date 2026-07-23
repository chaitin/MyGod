import {
  ClientDataRequestError,
  formatClientMessageBodySummary,
  type ClientConversation,
  type ClientMessage,
  type ClientMessageChoiceState,
  type ClientMessagePage,
  type MessageChoiceSnapshot,
  type MessageReactionsUpdatedEvent,
} from "@/lib/client-data-api"
import type { ClientConversationMessageState } from "@/lib/client-data-context"

export const messagePageLimit = 20
export const conversationMessageRetentionLimit = 300

export const emptyConversationMessageState: ClientConversationMessageState = {
  error: null,
  loaded: false,
  loading: false,
  loadingBefore: false,
  messages: [],
  page: null,
  sending: false,
}

export function getMessageSummary(message: ClientMessage) {
  return formatClientMessageBodySummary(message.body)
}

export function applyMessageReactionsUpdate(
  message: ClientMessage,
  event: MessageReactionsUpdatedEvent,
  currentUserId: string
) {
  if (
    message.id !== event.messageId ||
    message.conversationId !== event.conversationId ||
    message.body.type === "revoked" ||
    (message.reactionVersion ?? 0) >= event.reactionVersion ||
    event.reactionVersion > (message.reactionVersion ?? 0) + 1
  ) {
    return message
  }
  const previousByText = new Map(
    (message.reactions ?? []).map((reaction) => [reaction.text, reaction])
  )
  return {
    ...message,
    reactionVersion: event.reactionVersion,
    reactions: event.reactions.map((reaction) => ({
      ...reaction,
      reactedByMe:
        event.actorUserId === currentUserId && event.actorText === reaction.text
          ? event.actorReacted
          : (previousByText.get(reaction.text)?.reactedByMe ?? false),
    })),
  }
}

export function applyMessageReactionSnapshot(
  message: ClientMessage,
  snapshot: {
    conversationId: string
    messageId: string
    reactionVersion: number
    reactions: ClientMessage["reactions"]
  }
) {
  if (
    message.id !== snapshot.messageId ||
    message.conversationId !== snapshot.conversationId ||
    message.body.type === "revoked" ||
    message.reactionVersion > snapshot.reactionVersion
  ) {
    return message
  }
  return {
    ...message,
    reactionVersion: snapshot.reactionVersion,
    reactions: snapshot.reactions,
  }
}

export function applyMessageChoiceState(
  message: ClientMessage,
  choice: ClientMessageChoiceState
) {
  if (
    message.body.type !== "choice" ||
    message.body.options.length !== choice.options.length
  ) {
    return message
  }

  const previous = message.choice
  if (previous) {
    if (previous.responseCount > choice.responseCount) {
      if (previous.myOptionIds.length === 0 && choice.myOptionIds.length > 0) {
        return {
          ...message,
          choice: { ...previous, myOptionIds: choice.myOptionIds },
        }
      }
      return message
    }
    if (
      previous.responseCount === choice.responseCount &&
      previous.myOptionIds.length > 0 &&
      choice.myOptionIds.length === 0
    ) {
      return message
    }
  }

  return { ...message, choice }
}

export function applyMessageChoiceSnapshot(
  message: ClientMessage,
  snapshot: MessageChoiceSnapshot
): ClientMessage | null {
  if (
    message.id !== snapshot.messageId ||
    message.conversationId !== snapshot.conversationId
  ) {
    return message
  }
  if (snapshot.status === "deleted") {
    return null
  }
  if (snapshot.status === "revoked") {
    return {
      ...message,
      body: { type: "revoked" },
      choice: undefined,
      reactions: [],
    }
  }
  if (!snapshot.choice) {
    return message
  }
  return applyMessageChoiceState(message, snapshot.choice)
}

export function createConversationMessageState(): ClientConversationMessageState {
  return {
    error: null,
    loaded: false,
    loading: false,
    loadingBefore: false,
    messages: [],
    page: null,
    sending: false,
  }
}

export function compactConversationMessageState(
  state: ClientConversationMessageState,
  limit = conversationMessageRetentionLimit
): ClientConversationMessageState {
  if (state.messages.length <= limit) {
    return state
  }

  const messages = state.messages.slice(-limit)
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    ...state,
    messages,
    page: {
      hasMoreAfter: state.page?.hasMoreAfter ?? false,
      hasMoreBefore: true,
      limit: state.page?.limit ?? messagePageLimit,
      newestSeq: lastMessage?.seq ?? 0,
      oldestSeq: firstMessage?.seq ?? 0,
    },
  }
}

export function mergeConversationMessages(
  currentMessages: ClientMessage[],
  nextMessages: ClientMessage[]
) {
  if (nextMessages.length === 0) {
    return currentMessages
  }

  const normalizedNextMessages = deduplicateAndSortMessages(nextMessages)
  if (currentMessages.length === 0) {
    return normalizedNextMessages
  }

  const currentMessageIds = new Set<string>()
  let currentMessagesAreSortedAndUnique = true

  for (let index = 0; index < currentMessages.length; index += 1) {
    const message = currentMessages[index]
    if (currentMessageIds.has(message.id)) {
      currentMessagesAreSortedAndUnique = false
      break
    }
    currentMessageIds.add(message.id)

    const previousMessage = currentMessages[index - 1]
    if (previousMessage && compareMessages(previousMessage, message) > 0) {
      currentMessagesAreSortedAndUnique = false
      break
    }
  }

  const overlapsCurrentMessages = normalizedNextMessages.some((message) =>
    currentMessageIds.has(message.id)
  )

  if (currentMessagesAreSortedAndUnique && !overlapsCurrentMessages) {
    const firstCurrentMessage = currentMessages[0]
    const lastCurrentMessage = currentMessages[currentMessages.length - 1]
    const firstNextMessage = normalizedNextMessages[0]
    const lastNextMessage =
      normalizedNextMessages[normalizedNextMessages.length - 1]

    if (compareMessages(lastCurrentMessage, firstNextMessage) <= 0) {
      return [...currentMessages, ...normalizedNextMessages]
    }

    if (compareMessages(lastNextMessage, firstCurrentMessage) < 0) {
      return [...normalizedNextMessages, ...currentMessages]
    }
  }

  return deduplicateAndSortMessages([
    ...currentMessages,
    ...normalizedNextMessages,
  ])
}

function deduplicateAndSortMessages(messages: ClientMessage[]) {
  const messagesById = new Map<string, ClientMessage>()

  for (const message of messages) {
    const existing = messagesById.get(message.id)
    messagesById.set(
      message.id,
      existing?.topic && !message.topic
        ? { ...message, topic: existing.topic }
        : message
    )
  }

  return Array.from(messagesById.values()).sort(compareMessages)
}

function compareMessages(messageA: ClientMessage, messageB: ClientMessage) {
  if (messageA.seq !== messageB.seq) {
    return messageA.seq - messageB.seq
  }

  return messageA.createdAt.localeCompare(messageB.createdAt)
}

export function updatePageWithMessage(
  page: ClientMessagePage | null,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: false,
    hasMoreBefore: page?.hasMoreBefore ?? false,
    limit: page?.limit ?? messagePageLimit,
    newestSeq: lastMessage?.seq ?? 0,
    oldestSeq: firstMessage?.seq ?? 0,
  }
}

export function mergePageWithBeforeResult(
  currentPage: ClientMessagePage | null,
  resultPage: ClientMessagePage,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: currentPage?.hasMoreAfter ?? resultPage.hasMoreAfter,
    hasMoreBefore: resultPage.hasMoreBefore,
    limit: resultPage.limit,
    newestSeq: lastMessage?.seq ?? currentPage?.newestSeq ?? 0,
    oldestSeq: firstMessage?.seq ?? resultPage.oldestSeq,
  }
}

export function mergePageWithAfterResult(
  currentPage: ClientMessagePage | null,
  resultPage: ClientMessagePage,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: resultPage.hasMoreAfter,
    hasMoreBefore: currentPage?.hasMoreBefore ?? resultPage.hasMoreBefore,
    limit: resultPage.limit,
    newestSeq: lastMessage?.seq ?? resultPage.newestSeq,
    oldestSeq: firstMessage?.seq ?? currentPage?.oldestSeq ?? 0,
  }
}

export function getNewestMessageSeq(state: ClientConversationMessageState) {
  const lastMessage = state.messages[state.messages.length - 1]

  return Math.max(state.page?.newestSeq ?? 0, lastMessage?.seq ?? 0)
}

const builtinAssistantAppId = "00000000-0000-0000-0000-000000000001"

export function orderConversations(conversations: ClientConversation[]) {
  return [...conversations].sort((left, right) => {
    const leftIsBuiltinAssistant = isBuiltinAssistantConversation(left)
    const rightIsBuiltinAssistant = isBuiltinAssistantConversation(right)
    if (leftIsBuiltinAssistant !== rightIsBuiltinAssistant) {
      return leftIsBuiltinAssistant ? -1 : 1
    }

    const leftPinned = Boolean(left.pinned)
    const rightPinned = Boolean(right.pinned)
    if (leftPinned !== rightPinned) {
      return leftPinned ? -1 : 1
    }

    const leftActivity = getConversationActivityTimestamp(left)
    const rightActivity = getConversationActivityTimestamp(right)
    if (leftActivity !== rightActivity) {
      return rightActivity - leftActivity
    }

    return left.id < right.id ? -1 : left.id > right.id ? 1 : 0
  })
}

export function isBuiltinAssistantConversation(
  conversation: ClientConversation
) {
  return (
    conversation.type === "app" &&
    conversation.members?.some(
      (member) => member.type === "app" && member.id === builtinAssistantAppId
    ) === true
  )
}

function getConversationActivityTimestamp(conversation: ClientConversation) {
  const timestamp = Date.parse(
    conversation.lastMessageAt ?? conversation.createdAt
  )

  return Number.isNaN(timestamp) ? Number.NEGATIVE_INFINITY : timestamp
}

export function getClientDataErrorMessage(
  error: unknown,
  fallbackMessage: string
) {
  if (error instanceof ClientDataRequestError) {
    return error.message
  }

  return fallbackMessage
}
