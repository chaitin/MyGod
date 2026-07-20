import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { MessageBubble } from "@/components/conversation/conversation-message"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"
import type { ConversationPanelMessage } from "@/lib/conversation-panel-types"

describe("conversation topic preview", () => {
  it("shows the three recent replies inside the source-message bubble", async () => {
    const onOpenTopic = vi.fn()
    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <MessageBubble
            conversation={createConversation()}
            currentUserId="user-1"
            mentionLabelResolver={() => undefined}
            message={createMessage()}
            onInsertMention={vi.fn()}
            onOpenTopic={onOpenTopic}
            onRevoke={vi.fn()}
          />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    expect(screen.getAllByText("Alice")).toHaveLength(2)
    expect(screen.getByText("：第一条回复")).toBeVisible()
    expect(screen.getByText("Bob")).toBeVisible()
    expect(screen.getByText("：第二条回复")).toBeVisible()
    expect(screen.getByText("茉莉")).toBeVisible()
    expect(screen.getByText("：第三条回复")).toBeVisible()
    expect(screen.getByText("10:03")).toBeVisible()

    await userEvent.click(
      screen.getByRole("button", { name: "查看话题最近回复" })
    )
    expect(onOpenTopic).toHaveBeenCalledWith("topic-1")
    onOpenTopic.mockClear()

    await userEvent.click(screen.getByRole("button", { name: "查看话题" }))
    expect(onOpenTopic).toHaveBeenCalledWith("topic-1")
  })

  it("uses the same entry label for closed topics", () => {
    const message = createMessage()
    message.topic!.archived = true

    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <MessageBubble
            conversation={createConversation()}
            currentUserId="user-1"
            mentionLabelResolver={() => undefined}
            message={message}
            onInsertMention={vi.fn()}
            onOpenTopic={vi.fn()}
            onRevoke={vi.fn()}
          />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    expect(screen.getByRole("button", { name: "查看话题" })).toBeVisible()
    expect(screen.queryByText("查看已关闭话题")).not.toBeInTheDocument()
  })
})

function createMessage(): ConversationPanelMessage {
  return {
    author: "Alice",
    avatar: "",
    body: { content: "我们讨论一下", type: "text" },
    canRevoke: false,
    delegatedByName: "",
    id: "message-1",
    mentionTarget: null,
    role: "other",
    senderAppId: null,
    senderAppProfile: null,
    senderUserId: "user-2",
    time: "10:00",
    topic: {
      archived: false,
      conversationId: "topic-1",
      recentReplies: [
        {
          author: "Alice",
          avatar: "",
          id: "reply-1",
          summary: "第一条回复",
          time: "10:01",
        },
        {
          author: "Bob",
          avatar: "",
          id: "reply-2",
          summary: "第二条回复",
          time: "10:02",
        },
        {
          author: "茉莉",
          avatar: "",
          id: "reply-3",
          summary: "第三条回复",
          time: "10:03",
        },
      ],
    },
  }
}

function createConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-20T10:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 1,
    lastMessageSummary: "我们讨论一下",
    lastMentionedSeq: 0,
    lastReadSeq: 1,
    memberCount: 2,
    name: "测试会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
  }
}

function createClientDataValue(): ClientDataContextValue {
  return {
    contacts: [],
    me: {
      avatar: "",
      createdAt: "2026-07-20T10:00:00Z",
      email: "me@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "我",
      nickname: "",
      phone: "",
      status: "active",
    },
    openDirectConversation: vi.fn(),
  } as unknown as ClientDataContextValue
}
