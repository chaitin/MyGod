import { describe, expect, it } from "vitest"

import type {
  ClientConversation,
  ClientConversationMember,
} from "@/lib/client-data-api"
import {
  createConversationSearchIndex,
  searchConversationIndex,
} from "@/lib/conversation-search"
import { createPinyinSearchTokens } from "@/lib/pinyin-search"

describe("conversation search", () => {
  it("creates original, full, spaced, and initial pinyin tokens", () => {
    expect(createPinyinSearchTokens(["张三"])).toEqual(
      expect.arrayContaining(["张三", "zhangsan", "zhang san", "zs"])
    )
  })

  it.each(["张三", "zhangsan", "zhang san", "zs"])(
    "finds a Chinese conversation name with %s",
    (keyword) => {
      const conversation = createConversation({ name: "张三" })
      const results = search([conversation], keyword)

      expect(results.map((result) => result.conversation.id)).toEqual([
        conversation.id,
      ])
      expect(results[0]?.matchedField?.kind).toBe("conversation_name")
    }
  )

  it("searches the other user in a direct chat and excludes the current user", () => {
    const conversation = createConversation({
      members: [
        createMember({
          email: "owner-only@example.com",
          id: "current-user",
          name: "当前用户独有姓名",
        }),
        createMember({
          email: "Zhang.San@Example.com",
          name: "张三",
          nickname: "小张",
          phone: "13800138000",
        }),
      ],
      name: "项目搭档",
      type: "direct",
    })

    expect(search([conversation], "xiaozhang")[0]?.matchedField?.kind).toBe(
      "member_nickname"
    )
    expect(
      search([conversation], "zhang.san@EXAMPLE")[0]?.matchedField?.kind
    ).toBe("member_email")
    expect(search([conversation], "1380013")[0]?.matchedField?.kind).toBe(
      "member_phone"
    )
    expect(search([conversation], "当前用户独有姓名")).toHaveLength(0)
    expect(search([conversation], "owner-only")).toHaveLength(0)
  })

  it("searches user and app members in a group chat", () => {
    const conversation = createConversation({
      members: [
        createMember({ id: "member-1", name: "李四", nickname: "小李" }),
        createMember({
          id: "app-1",
          name: "会议助手",
          nickname: "纪要机器人",
          type: "app",
        }),
      ],
      name: "产品讨论群",
      type: "group",
    })

    expect(search([conversation], "ls")[0]?.matchedField?.kind).toBe(
      "member_name"
    )
    expect(search([conversation], "jiyaojiqiren")[0]?.matchedField?.kind).toBe(
      "app_name"
    )
  })

  it("does not index app members as direct-chat people", () => {
    const conversation = createConversation({
      members: [createMember({ id: "app-1", name: "私聊机器人", type: "app" })],
      name: "智能助理",
      type: "direct",
    })

    expect(search([conversation], "私聊机器人")).toHaveLength(0)
  })

  it("keeps one result per conversation and retains its best match", () => {
    const conversation = createConversation({
      members: [createMember({ email: "alpha@example.com", name: "Alpha" })],
      name: "Alpha",
      type: "group",
    })

    const results = search([conversation], "alpha")

    expect(results).toHaveLength(1)
    expect(results[0]?.matchedField?.kind).toBe("conversation_name")
    expect(results[0]?.matchQuality).toBe("exact")
  })

  it("ranks quality, then field priority, then recent activity", () => {
    const conversations = [
      createConversation({
        id: "contains-name",
        lastMessageAt: "2026-07-14T12:00:00Z",
        name: "团队 Alpha 讨论",
      }),
      createConversation({
        id: "exact-member-new",
        lastMessageAt: "2026-07-14T11:00:00Z",
        members: [createMember({ id: "member-2", name: "alpha" })],
        name: "新会话",
        type: "group",
      }),
      createConversation({
        id: "exact-member-old",
        lastMessageAt: "2026-07-14T10:00:00Z",
        members: [createMember({ id: "member-3", name: "alpha" })],
        name: "旧会话",
        type: "group",
      }),
      createConversation({
        id: "prefix-conversation",
        lastMessageAt: "2026-07-14T09:00:00Z",
        name: "Alphabet",
      }),
    ]

    expect(
      search(conversations, "alpha").map((result) => result.conversation.id)
    ).toEqual([
      "exact-member-new",
      "exact-member-old",
      "prefix-conversation",
      "contains-name",
    ])
  })

  it("shows the first 8 conversations in the provided order", () => {
    const conversations = Array.from({ length: 10 }, (_, index) =>
      createConversation({
        id: `conversation-${index}`,
        lastMessageAt: new Date(Date.UTC(2026, 6, 1, index)).toISOString(),
      })
    )

    const results = search(conversations, "  ")

    expect(results).toHaveLength(8)
    expect(results[0]?.conversation.id).toBe("conversation-0")
    expect(results[7]?.conversation.id).toBe("conversation-7")
    expect(results.every((result) => result.matchedField === null)).toBe(true)
  })

  it("reuses search fields when only conversation activity changes", () => {
    const conversation = createConversation({
      members: [createMember({ name: "张三", nickname: "小张" })],
      name: "产品群",
      type: "group",
    })
    const index = createConversationSearchIndex([conversation], "current-user")
    const updatedConversation = {
      ...conversation,
      lastMessageAt: "2026-07-14T12:00:00Z",
      lastMessageSummary: "新消息",
      lastChoiceSeq: 0,
    }

    const updatedIndex = createConversationSearchIndex(
      [updatedConversation],
      "current-user",
      index
    )

    expect(updatedIndex[0]?.fields).toBe(index[0]?.fields)
    expect(updatedIndex[0]?.conversation).toBe(updatedConversation)
    expect(searchConversationIndex(updatedIndex, "xz")).toHaveLength(1)
  })

  it("rebuilds search fields when searchable member data changes", () => {
    const conversation = createConversation({
      members: [createMember({ name: "张三" })],
      type: "group",
    })
    const index = createConversationSearchIndex([conversation], "current-user")
    const updatedConversation = {
      ...conversation,
      members: [createMember({ name: "李四" })],
    }

    const updatedIndex = createConversationSearchIndex(
      [updatedConversation],
      "current-user",
      index
    )

    expect(updatedIndex[0]?.fields).not.toBe(index[0]?.fields)
    expect(searchConversationIndex(updatedIndex, "zs")).toHaveLength(0)
    expect(searchConversationIndex(updatedIndex, "ls")).toHaveLength(1)
  })

  it("limits non-empty results to 20", () => {
    const conversations = Array.from({ length: 25 }, (_, index) =>
      createConversation({ id: `conversation-${index}`, name: `项目 ${index}` })
    )

    expect(search(conversations, "项目")).toHaveLength(20)
  })
})

function search(conversations: ClientConversation[], keyword: string) {
  return searchConversationIndex(
    createConversationSearchIndex(conversations, "current-user"),
    keyword
  )
}

function createConversation(
  overrides: Partial<ClientConversation> = {}
): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-01T00:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 0,
    members: [],
    name: "普通会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
    ...overrides,
    lastMessageSender: overrides.lastMessageSender ?? null,
  }
}

function createMember(
  overrides: Partial<ClientConversationMember> = {}
): ClientConversationMember {
  return {
    avatar: "",
    email: "member@example.com",
    id: "member-1",
    name: "成员",
    nickname: "",
    phone: "",
    role: "member",
    type: "user",
    ...overrides,
  }
}
