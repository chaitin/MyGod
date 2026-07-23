import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { SendCardDialog } from "@/components/conversation/send-card-dialog"
import type { ClientCardMessageBody } from "@/lib/client-data-api"

const mocks = vi.hoisted(() => ({
  longConversationName:
    "这是一个特别特别长并且必须在对话框中截断的会话名称".repeat(4),
  conversations: [
    {
      avatar: "",
      createdAt: "2026-07-14T08:00:00Z",
      id: "conversation-1",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastChoiceSeq: 0,
      lastMentionedSeq: 0,
      lastReadSeq: 0,
      memberCount: 2,
      name: "设计群",
      type: "group",
      unreadCount: 0,
      visibility: "private",
    },
    {
      avatar: "",
      createdAt: "2026-07-14T08:00:00Z",
      id: "conversation-2",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastChoiceSeq: 0,
      lastMentionedSeq: 0,
      lastReadSeq: 0,
      memberCount: 2,
      name: "Alice",
      type: "direct",
      unreadCount: 0,
      visibility: "private",
    },
    {
      avatar: "",
      createdAt: "2026-07-14T08:00:00Z",
      id: "conversation-3",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastChoiceSeq: 0,
      lastMentionedSeq: 0,
      lastReadSeq: 0,
      memberCount: 2,
      name: "这是一个特别特别长并且必须在对话框中截断的会话名称".repeat(4),
      type: "direct",
      unreadCount: 0,
      visibility: "private",
    },
  ],
  sendConversationCard: vi.fn(),
}))

vi.mock("@/lib/client-data-context", () => ({
  useClientData: () => ({
    conversations: mocks.conversations,
    sendConversationCard: mocks.sendConversationCard,
  }),
}))

describe("SendCardDialog", () => {
  it("selects one conversation and sends the card", async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    const card = createCard()
    mocks.sendConversationCard.mockReset()
    mocks.sendConversationCard.mockResolvedValue({
      id: "message-1",
    })

    render(
      <MemoryRouter>
        <SendCardDialog card={card} onOpenChange={onOpenChange} open />
      </MemoryRouter>
    )

    const sendButton = screen.getByRole("button", { name: "发送" })
    expect(sendButton).toBeDisabled()
    expect(screen.getByText(mocks.longConversationName)).toBeVisible()

    await user.click(screen.getByRole("radio", { name: "设计群" }))
    await user.click(screen.getByRole("radio", { name: "Alice" }))
    expect(screen.getByRole("radio", { name: "设计群" })).not.toBeChecked()
    expect(screen.getByRole("radio", { name: "Alice" })).toBeChecked()
    expect(sendButton).toBeEnabled()
    await user.click(sendButton)

    await waitFor(() => {
      expect(mocks.sendConversationCard).toHaveBeenCalledWith(
        "conversation-2",
        card
      )
    })
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})

function createCard(): ClientCardMessageBody {
  return {
    description: "任务说明",
    title: "任务标题",
    type: "card",
    url: "/projects/project-1?taskId=task-1",
  }
}
