import { describe, expect, it } from "vitest"

import type { ClientConversation, ClientMessage } from "@/lib/client-data-api"
import {
  compactConversationMessageState,
  createConversationMessageState,
  mergeConversationMessages,
  orderConversations,
} from "@/lib/client-data-state"

describe("compactConversationMessageState", () => {
  it("keeps the newest messages and preserves the older-page boundary", () => {
    const messages = Array.from({ length: 305 }, (_, index) =>
      createMessage(`message-${index + 1}`, index + 1)
    )
    const state = {
      ...createConversationMessageState(),
      loaded: true,
      messages,
      page: {
        hasMoreAfter: false,
        hasMoreBefore: false,
        limit: 20,
        newestSeq: 305,
        oldestSeq: 1,
      },
    }

    const compacted = compactConversationMessageState(state)

    expect(compacted.messages).toHaveLength(300)
    expect(compacted.messages[0].seq).toBe(6)
    expect(compacted.messages.at(-1)?.seq).toBe(305)
    expect(compacted.page).toEqual({
      hasMoreAfter: false,
      hasMoreBefore: true,
      limit: 20,
      newestSeq: 305,
      oldestSeq: 6,
    })
  })

  it("keeps the same state when it is already within the limit", () => {
    const state = {
      ...createConversationMessageState(),
      messages: [createMessage("message-1", 1)],
    }

    expect(compactConversationMessageState(state)).toBe(state)
  })
})

describe("mergeConversationMessages", () => {
  it("appends newer messages in sequence order", () => {
    const current = [createMessage("message-1", 1)]
    const next = [createMessage("message-3", 3), createMessage("message-2", 2)]

    expect(
      mergeConversationMessages(current, next).map(({ id }) => id)
    ).toEqual(["message-1", "message-2", "message-3"])
  })

  it("prepends an older page in sequence order", () => {
    const current = [
      createMessage("message-3", 3),
      createMessage("message-4", 4),
    ]
    const next = [createMessage("message-2", 2), createMessage("message-1", 1)]

    expect(
      mergeConversationMessages(current, next).map(({ id }) => id)
    ).toEqual(["message-1", "message-2", "message-3", "message-4"])
  })

  it("replaces an existing message with its newest representation", () => {
    const current = [createMessage("message-1", 1, "旧内容")]
    const updated = createMessage("message-1", 1, "新内容")

    expect(mergeConversationMessages(current, [updated])).toEqual([updated])
  })

  it("deduplicates messages within an incoming page", () => {
    const first = createMessage("message-1", 1, "旧内容")
    const latest = createMessage("message-1", 1, "新内容")

    expect(mergeConversationMessages([], [first, latest])).toEqual([latest])
  })

  it("falls back to a full merge for overlapping sequence ranges", () => {
    const current = [
      createMessage("message-1", 1),
      createMessage("message-3", 3),
    ]
    const next = [createMessage("message-4", 4), createMessage("message-2", 2)]

    expect(
      mergeConversationMessages(current, next).map(({ id }) => id)
    ).toEqual(["message-1", "message-2", "message-3", "message-4"])
  })

  it("uses creation time to order messages with the same sequence", () => {
    const later = createMessage("message-2", 1, "", "2026-07-14T10:01:00Z")
    const earlier = createMessage("message-1", 1, "", "2026-07-14T10:00:00Z")

    expect(
      mergeConversationMessages([later], [earlier]).map(({ id }) => id)
    ).toEqual(["message-1", "message-2"])
  })
})

describe("orderConversations", () => {
  it("pins only the built-in assistant and orders every other conversation by activity", () => {
    const assistant = createConversation("assistant", "app", "2026-07-01", [
      createAppMember("00000000-0000-0000-0000-000000000001"),
    ])
    const regularApp = createConversation("regular-app", "app", "2026-07-18")
    const activeGroup = createConversation(
      "active-group",
      "group",
      "2026-07-20"
    )
    const direct = createConversation("direct", "direct", "2026-07-19")

    expect(
      orderConversations([regularApp, assistant, direct, activeGroup]).map(
        ({ id }) => id
      )
    ).toEqual(["assistant", "active-group", "direct", "regular-app"])
  })

  it("does not pin a group that contains the built-in assistant", () => {
    const recentApp = createConversation("recent-app", "app", "2026-07-20")
    const oldGroup = createConversation("old-group", "group", "2026-07-01", [
      createAppMember("00000000-0000-0000-0000-000000000001"),
    ])

    expect(
      orderConversations([oldGroup, recentApp]).map(({ id }) => id)
    ).toEqual(["recent-app", "old-group"])
  })

  it("orders pinned conversations by activity ahead of unpinned conversations", () => {
    const assistant = createConversation("assistant", "app", "2026-07-01", [
      createAppMember("00000000-0000-0000-0000-000000000001"),
    ])
    const olderPinned = {
      ...createConversation("older-pinned", "group", "2026-07-18"),
      pinned: true,
    }
    const recentPinned = {
      ...createConversation("recent-pinned", "direct", "2026-07-19"),
      pinned: true,
    }
    const newestUnpinned = createConversation(
      "newest-unpinned",
      "group",
      "2026-07-20"
    )

    expect(
      orderConversations([
        newestUnpinned,
        olderPinned,
        assistant,
        recentPinned,
      ]).map(({ id }) => id)
    ).toEqual(["assistant", "recent-pinned", "older-pinned", "newest-unpinned"])
  })
})

function createMessage(
  id: string,
  seq: number,
  content = id,
  createdAt = `2026-07-14T10:00:${String(seq).padStart(2, "0")}Z`
): ClientMessage {
  return {
    body: { content, type: "text" },
    clientMessageId: `client-${id}`,
    conversationId: "conversation-1",
    createdAt,
    id,
    reactionVersion: 0,
    reactions: [],
    sender: { id: "user-1", type: "user" },
    seq,
  }
}

function createConversation(
  id: string,
  type: ClientConversation["type"],
  activityDate: string,
  members?: ClientConversation["members"]
): ClientConversation {
  return {
    avatar: "",
    createdAt: `${activityDate}T08:00:00Z`,
    id,
    lastMessageAt: `${activityDate}T09:00:00Z`,
    lastMessageId: `message-${id}`,
    lastMessageSeq: 1,
    lastMessageSender: null,
    lastMessageSummary: id,
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 1,
    memberCount: members?.length ?? 2,
    members,
    name: id,
    type,
    unreadCount: 0,
    visibility: "private",
  }
}

function createAppMember(id: string) {
  return {
    avatar: "",
    email: "",
    id,
    name: "App",
    nickname: "",
    phone: "",
    role: "member" as const,
    type: "app" as const,
  }
}
