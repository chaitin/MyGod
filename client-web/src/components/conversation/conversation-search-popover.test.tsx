import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ConversationSearchPopover } from "@/components/conversation/conversation-search-popover"
import type {
  ClientConversation,
  ClientConversationMember,
} from "@/lib/client-data-api"

describe("ConversationSearchPopover", () => {
  it("opens on focus and preserves the conversation-list order", async () => {
    const user = userEvent.setup()

    renderSearch([
      createConversation({
        id: "newer",
        lastMessageAt: "2026-07-14T10:00:00Z",
        name: "最近会话",
      }),
      createConversation({
        id: "older",
        lastMessageAt: "2026-07-14T09:00:00Z",
        name: "较早会话",
      }),
    ])

    const input = screen.getByRole("combobox", { name: "搜索消息" })
    await user.click(input)

    expect(input).toHaveFocus()
    expect(input).toHaveAttribute("aria-expanded", "true")
    expect(screen.getByRole("listbox", { name: "搜索会话结果" })).toBeVisible()
    expect(screen.getAllByRole("option")[0]).toHaveTextContent("最近会话")
  })

  it("shows and searches a topic with its parent conversation name", async () => {
    const user = userEvent.setup()
    renderSearch([
      createConversation({
        name: "发布计划",
        topic: {
          archived: false,
          parentConversationId: "parent-1",
          parentConversationName: "产品群",
          parentConversationType: "group",
          participating: true,
          sourceMessageId: "message-1",
          sourceMessageSeq: 1,
          sourceSender: {
            avatar: "/avatars/alice.webp",
            id: "user-2",
            name: "Alice",
            type: "user",
          },
        },
        type: "topic",
      }),
    ])

    await user.type(
      screen.getByRole("combobox", { name: "搜索消息" }),
      "产品群"
    )

    expect(
      screen.getByRole("option", { name: /发布计划 - 产品群/ })
    ).toBeVisible()
  })

  it("searches by pinyin and selects a result with the mouse", async () => {
    const user = userEvent.setup()
    const onSelectConversation = vi.fn()
    renderSearch(
      [
        createConversation({
          id: "direct-zhang",
          members: [
            createMember({ id: "current-user", name: "当前用户" }),
            createMember({ id: "zhang", name: "张三", nickname: "小张" }),
          ],
          name: "产品搭档",
        }),
      ],
      onSelectConversation
    )

    const input = screen.getByRole("combobox", { name: "搜索消息" })
    await user.type(input, "xz")

    const result = screen.getByRole("option", { name: /产品搭档/ })
    expect(result).toHaveTextContent("匹配成员：小张")

    await user.click(result)

    expect(onSelectConversation).toHaveBeenCalledWith("direct-zhang")
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument()
    expect(input).toHaveValue("")
  })

  it("supports arrow navigation and Enter selection", async () => {
    const user = userEvent.setup()
    const onSelectConversation = vi.fn()
    renderSearch(
      [
        createConversation({ id: "first", name: "项目一" }),
        createConversation({ id: "second", name: "项目二" }),
      ],
      onSelectConversation
    )

    const input = screen.getByRole("combobox", { name: "搜索消息" })
    await user.type(input, "项目")

    const options = screen.getAllByRole("option")
    expect(options[0]).toHaveAttribute("aria-selected", "true")

    await user.keyboard("{ArrowDown}")
    expect(options[1]).toHaveAttribute("aria-selected", "true")
    expect(input).toHaveAttribute("aria-activedescendant", options[1].id)

    await user.keyboard("{Enter}")

    expect(onSelectConversation).toHaveBeenCalledWith("second")
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument()
  })

  it("closes and clears on Escape and outside click", async () => {
    const user = userEvent.setup()
    render(
      <>
        <ConversationSearchPopover
          conversations={[createConversation({ name: "测试会话" })]}
          currentUserId="current-user"
          getConversationDescription={getConversationDescription}
          onSelectConversation={vi.fn()}
        />
        <button type="button">外部按钮</button>
      </>
    )

    const input = screen.getByRole("combobox", { name: "搜索消息" })
    await user.type(input, "测试")
    await user.keyboard("{Escape}")

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument()
    expect(input).toHaveValue("")

    await user.type(input, "测试")
    await user.click(screen.getByRole("button", { name: "外部按钮" }))

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument()
    expect(input).toHaveValue("")
  })

  it("shows an empty state without changing the underlying conversations", async () => {
    const user = userEvent.setup()
    renderSearch([createConversation({ name: "设计讨论" })])

    await user.type(
      screen.getByRole("combobox", { name: "搜索消息" }),
      "不存在"
    )

    expect(screen.getByText("未找到相关会话")).toBeInTheDocument()
    expect(screen.queryByRole("option")).not.toBeInTheDocument()
  })
})

function renderSearch(
  conversations: ClientConversation[],
  onSelectConversation = vi.fn()
) {
  return render(
    <ConversationSearchPopover
      conversations={conversations}
      currentUserId="current-user"
      getConversationDescription={getConversationDescription}
      onSelectConversation={onSelectConversation}
    />
  )
}

function getConversationDescription(conversation: ClientConversation) {
  return conversation.lastMessageSummary.trim() || "暂无消息"
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
    lastMessageSummary: "暂无消息",
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
