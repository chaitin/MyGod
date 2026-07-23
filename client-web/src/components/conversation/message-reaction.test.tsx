import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { MessageBubble } from "@/components/conversation/conversation-message"
import type { ClientConversation } from "@/lib/client-data-api"
import type { ConversationPanelMessage } from "@/lib/conversation-panel-types"

const reactionMocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
}))

vi.mock("@/lib/client-data-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/client-data-api")>(
    "@/lib/client-data-api"
  )
  return {
    ...actual,
    listConversationMessageReactionUsers: reactionMocks.listUsers,
  }
})
vi.mock("@/components/user-profile-popover", () => ({
  UserProfilePopover: ({
    children,
    triggerAriaLabel,
  }: {
    children: ReactNode
    triggerAriaLabel?: string
  }) => <button aria-label={triggerAriaLabel}>{children}</button>,
}))
vi.mock("@/components/app-profile-popover", () => ({
  AppProfilePopover: ({ children }: { children: ReactNode }) => children,
}))

describe("MessageBubble reactions", () => {
  it("uses the translucent background for reactions on current-user messages", () => {
    renderBubble({
      messageOverrides: {
        role: "me",
        senderUserId: "user-1",
      },
      onSetReaction: vi.fn(),
    })

    expect(
      screen
        .getByRole("button", { name: "移除表情 自定义文本" })
        .closest('[data-slot="message-reaction-chip"]')
    ).toHaveClass("bg-background/70")
  })

  it("renders arbitrary text, toggles chips, and shares the full expression picker", async () => {
    reactionMocks.listUsers.mockResolvedValue([
      { id: "user-11", name: "完整用户甲" },
      { id: "user-12", name: "完整用户乙" },
    ])
    const onSetReaction = vi.fn().mockResolvedValue(undefined)
    renderBubble({ onSetReaction })

    const reactionToggle = screen.getByRole("button", {
      name: "移除表情 自定义文本",
    })
    const reactionChip = reactionToggle.closest<HTMLDivElement>(
      '[data-slot="message-reaction-chip"]'
    )
    expect(reactionChip).toHaveClass("bg-background/70")
    expect(reactionChip).toHaveTextContent(
      "自定义文本李昌志, 朱文磊, 王彪, 赵一, 钱二, 孙三, 周四, 吴五, 郑六, 王七等 16 人"
    )
    expect(
      screen.getByRole("button", { name: "李昌志资料" })
    ).toBeInTheDocument()
    const participantCount = screen.getByRole("button", {
      name: "查看表情 自定义文本 的 16 位参与者",
    })
    expect(participantCount).toHaveTextContent("16")

    fireEvent.click(participantCount)
    await waitFor(() =>
      expect(reactionMocks.listUsers).toHaveBeenCalledWith(
        "conversation-1",
        "message-1",
        "自定义文本"
      )
    )
    expect(
      await screen.findByRole("button", { name: "完整用户甲资料" })
    ).toBeInTheDocument()

    const addButton = screen.getByRole("button", { name: "添加表情" })
    const bubbleLine = addButton.closest('[data-slot="message-bubble-line"]')
    const hoverActions = addButton.closest(
      '[data-slot="message-hover-actions"]'
    )
    expect(bubbleLine).not.toBeNull()
    expect(hoverActions).not.toBeNull()
    expect(bubbleLine).toContainElement(screen.getByText("hello"))
    expect(bubbleLine).toContainElement(reactionChip)
    expect(hoverActions).toContainElement(
      screen.getByRole("button", { name: "更多操作" })
    )
    expect(addButton.className).toBe(
      screen.getByRole("button", { name: "更多操作" }).className
    )
    expect(
      addButton.compareDocumentPosition(
        screen.getByRole("button", { name: "更多操作" })
      ) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    const messageBubble = screen
      .getByText("hello")
      .closest<HTMLElement>("[data-message-action-trigger]")
    expect(messageBubble).toContainElement(reactionChip)

    fireEvent.click(reactionToggle)
    await waitFor(() =>
      expect(onSetReaction).toHaveBeenCalledWith(
        expect.objectContaining({ id: "message-1" }),
        "自定义文本",
        false
      )
    )

    fireEvent.click(addButton)
    const frequentSection = screen.getByRole("region", { name: "常用" })
    const allSection = screen.getByRole("region", { name: "所有表情" })
    expect(within(frequentSection).getAllByRole("button")).toHaveLength(8)
    expect(within(allSection).getAllByRole("button")).toHaveLength(64)
    fireEvent.click(
      within(allSection).getByRole("button", { name: "庆祝礼花" })
    )
    await waitFor(() =>
      expect(onSetReaction).toHaveBeenCalledWith(
        expect.objectContaining({ id: "message-1" }),
        "🎉",
        true
      )
    )
  })

  it("hides the picker in read-only topics but lets users remove their own reaction", () => {
    const onSetReaction = vi.fn().mockResolvedValue(undefined)
    renderBubble({ canReply: false, onSetReaction })

    expect(
      screen.queryByRole("button", { name: "添加表情" })
    ).not.toBeInTheDocument()
    const ownReaction = screen.getByRole("button", {
      name: "移除表情 自定义文本",
    })
    expect(ownReaction).toBeEnabled()
    fireEvent.click(ownReaction)
    expect(onSetReaction).toHaveBeenCalledWith(
      expect.objectContaining({ id: "message-1" }),
      "自定义文本",
      false
    )
  })

  it("does not offer new reactions on revoked messages", () => {
    const onSetReaction = vi.fn().mockResolvedValue(undefined)
    renderBubble({
      messageOverrides: { body: { type: "revoked" }, reactions: [] },
      onSetReaction,
    })

    expect(
      screen.queryByRole("button", { name: "添加表情" })
    ).not.toBeInTheDocument()
  })
})

function renderBubble({
  canReply = true,
  messageOverrides,
  onSetReaction,
}: {
  canReply?: boolean
  messageOverrides?: Partial<ConversationPanelMessage>
  onSetReaction: (
    message: ConversationPanelMessage,
    text: string,
    reacted: boolean
  ) => Promise<void>
}) {
  const message: ConversationPanelMessage = {
    author: "Alice",
    avatar: "",
    body: { content: "hello", type: "text" },
    canRevoke: false,
    createdAt: "2026-07-21T00:00:00Z",
    delegatedByName: "",
    id: "message-1",
    mentionTarget: null,
    reactionVersion: 1,
    reactions: [
      {
        count: 16,
        reactedByMe: true,
        text: "自定义文本",
        users: [
          { id: "user-1", name: "李昌志" },
          { id: "user-2", name: "朱文磊" },
          { id: "user-3", name: "王彪" },
          { id: "user-4", name: "赵一" },
          { id: "user-5", name: "钱二" },
          { id: "user-6", name: "孙三" },
          { id: "user-7", name: "周四" },
          { id: "user-8", name: "吴五" },
          { id: "user-9", name: "郑六" },
          { id: "user-10", name: "王七" },
        ],
      },
    ],
    role: "other",
    senderAppId: null,
    senderAppProfile: null,
    senderUserId: "user-2",
    time: "08:00",
    ...messageOverrides,
  }
  const conversation = {
    id: "conversation-1",
    name: "Alice",
    type: "direct",
  } as ClientConversation
  return render(
    <MessageBubble
      canReply={canReply}
      conversation={conversation}
      currentUserId="user-1"
      mentionLabelResolver={() => undefined}
      message={message}
      onInsertMention={() => undefined}
      onRevoke={() => undefined}
      onSetReaction={onSetReaction}
    />
  )
}
