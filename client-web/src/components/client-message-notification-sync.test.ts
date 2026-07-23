import { describe, expect, it } from "vitest"

import type { ClientConversation, ClientMessage } from "@/lib/client-data-api"
import { shouldSuppressMessageNotification } from "@/lib/message-notification-policy"

describe("message notification suppression", () => {
  it("suppresses muted and system messages", () => {
    const message = createMessage()
    expect(
      shouldSuppressMessageNotification({
        conversation: createConversation({ notificationMuted: true }),
        currentUserId: "user-1",
        eventNotificationMuted: false,
        message,
      })
    ).toBe(true)
    expect(
      shouldSuppressMessageNotification({
        conversation: undefined,
        currentUserId: "user-1",
        eventNotificationMuted: true,
        message,
      })
    ).toBe(true)
    expect(
      shouldSuppressMessageNotification({
        conversation: createConversation(),
        currentUserId: "user-1",
        eventNotificationMuted: false,
        message: createMessage({ sender: { id: "", type: "system" } }),
      })
    ).toBe(true)
  })

  it("allows an ordinary message from another sender", () => {
    expect(
      shouldSuppressMessageNotification({
        conversation: createConversation(),
        currentUserId: "user-1",
        eventNotificationMuted: false,
        message: createMessage(),
      })
    ).toBe(false)
  })
})

function createMessage(overrides: Partial<ClientMessage> = {}): ClientMessage {
  return {
    body: { content: "新消息", type: "text" },
    clientMessageId: "",
    conversationId: "conversation-1",
    createdAt: "2026-07-22T00:00:00Z",
    id: "message-1",
    reactions: [],
    reactionVersion: 0,
    replyToMessageId: "",
    revokedByUserId: "",
    sender: { id: "user-2", type: "user" },
    seq: 1,
    ...overrides,
  }
}

function createConversation(
  overrides: Partial<ClientConversation> = {}
): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-22T00:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSender: null,
    lastMessageSeq: 0,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    name: "会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
    ...overrides,
  }
}
