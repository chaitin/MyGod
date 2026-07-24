import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { MessageBubble } from "@/components/conversation/conversation-message"
import type { ClientConversation } from "@/lib/client-data-api"
import type { ConversationPanelMessage } from "@/lib/conversation-panel-types"

vi.mock("@/components/user-profile-popover", () => ({
  UserProfilePopover: ({ children }: { children: ReactNode }) => children,
}))
vi.mock("@/components/app-profile-popover", () => ({
  AppProfilePopover: ({ children }: { children: ReactNode }) => children,
}))

describe("MessageBubble choice messages", () => {
  it("submits a single answer from the original bubble", async () => {
    const onRespondToChoice = vi.fn().mockResolvedValue(undefined)
    renderChoice({ onRespondToChoice })

    expect(screen.getByText("请选择项目")).toBeInTheDocument()
    expect(
      screen.getByText("请选择项目").closest('[data-slot="choice-message"]')
    ).toHaveClass("w-120", "max-w-full")
    expect(screen.queryByText("3 人已回答")).not.toBeInTheDocument()
    expect(screen.queryByText("2")).not.toBeInTheDocument()
    expect(screen.queryByText("1")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "提交" })).toHaveClass("w-full")
    expect(screen.getByRole("button", { name: "提交" })).toHaveAttribute(
      "data-variant",
      "outline"
    )
    fireEvent.click(screen.getByRole("radio", { name: "项目 B" }))
    fireEvent.click(screen.getByRole("button", { name: "提交" }))

    await waitFor(() =>
      expect(onRespondToChoice).toHaveBeenCalledWith(
        expect.objectContaining({ id: "message-1" }),
        ["project-b"]
      )
    )
  })

  it("keeps an existing answer selected and disables another submission", () => {
    renderChoice({
      messageOverrides: {
        choice: {
          myOptionIds: ["project-a"],
          options: [
            { id: "project-a", responseCount: 3 },
            { id: "project-b", responseCount: 1 },
          ],
          responseCount: 4,
        },
      },
      onRespondToChoice: vi.fn(),
    })

    expect(screen.getByRole("radio", { name: "项目 A" })).toBeChecked()
    expect(
      screen.queryByRole("button", { name: "已提交" })
    ).not.toBeInTheDocument()
    expect(screen.queryByText("4 人已回答")).not.toBeInTheDocument()
  })

  it("shows response counts in group conversations", () => {
    renderChoice({
      conversationOverrides: { type: "group" },
      messageOverrides: { role: "me" },
      onRespondToChoice: vi.fn(),
    })

    expect(screen.getByText("2").closest('[data-slot="badge"]')).toHaveClass(
      "bg-background/70"
    )
  })

  it.each(["direct", "app"] as const)(
    "hides response counts in %s conversations",
    (type) => {
      renderChoice({
        conversationOverrides: { type },
        onRespondToChoice: vi.fn(),
      })

      expect(screen.queryByText("2")).not.toBeInTheDocument()
      expect(screen.queryByText("1")).not.toBeInTheDocument()
    }
  )

  it.each([
    ["direct", false],
    ["app", false],
    ["group", true],
  ] as const)(
    "%s parent conversation controls topic response counts",
    (parentConversationType, showsCounts) => {
      renderChoice({
        conversationOverrides: {
          type: "topic",
          topic: {
            archived: false,
            parentConversationId: "parent-1",
            parentConversationName: "父会话",
            parentConversationType,
            participating: true,
            sourceMessageId: "source-1",
            sourceMessageSeq: 1,
            sourceSender: {
              avatar: "",
              id: "user-2",
              name: "Bob",
              type: "user",
            },
          },
        },
        onRespondToChoice: vi.fn(),
      })

      if (showsCounts) {
        expect(screen.getByText("2")).toBeInTheDocument()
        expect(screen.getByText("1")).toBeInTheDocument()
      } else {
        expect(screen.queryByText("2")).not.toBeInTheDocument()
        expect(screen.queryByText("1")).not.toBeInTheDocument()
      }
    }
  )
})

function renderChoice({
  conversationOverrides,
  messageOverrides,
  onRespondToChoice,
}: {
  conversationOverrides?: Partial<ClientConversation>
  messageOverrides?: Partial<ConversationPanelMessage>
  onRespondToChoice: (
    message: ConversationPanelMessage,
    optionIds: string[]
  ) => Promise<void>
}) {
  const message: ConversationPanelMessage = {
    author: "茉莉",
    avatar: "",
    body: {
      type: "choice",
      contentType: "text",
      content: "请选择项目",
      selection: "single",
      options: [
        { id: "project-a", label: "项目 A" },
        { id: "project-b", label: "项目 B" },
      ],
    },
    choice: {
      myOptionIds: [],
      options: [
        { id: "project-a", responseCount: 2 },
        { id: "project-b", responseCount: 1 },
      ],
      responseCount: 3,
    },
    canRevoke: false,
    createdAt: "2026-07-23T10:00:00Z",
    delegatedByName: "",
    id: "message-1",
    mentionTarget: null,
    reactionVersion: 0,
    reactions: [],
    role: "other",
    senderAppId: "app-1",
    senderAppProfile: null,
    senderUserId: null,
    time: "18:00",
    ...messageOverrides,
  }
  const conversation = {
    id: "conversation-1",
    name: "茉莉",
    type: "app",
    ...conversationOverrides,
  } as ClientConversation
  return render(
    <MessageBubble
      conversation={conversation}
      currentUserId="user-1"
      mentionLabelResolver={() => undefined}
      message={message}
      onInsertMention={() => undefined}
      onRespondToChoice={onRespondToChoice}
    />
  )
}
