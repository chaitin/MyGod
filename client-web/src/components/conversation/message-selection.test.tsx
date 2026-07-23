import * as React from "react"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import {
  ConversationPanel,
  type ConversationPanelForwardMode,
  type ConversationPanelMessage,
} from "@/components/conversation-panel"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

describe("conversation message selection", () => {
  it("opens the message actions from the hover more button", async () => {
    const user = userEvent.setup()
    const onForwardSelected = vi.fn()

    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <SelectionHarness onForwardSelected={onForwardSelected} />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    const messageRow = screen
      .getByText("第一条")
      .closest<HTMLElement>("[data-conversation-message-id]")
    expect(messageRow).not.toBeNull()
    await user.click(
      within(messageRow!).getByRole("button", { name: "更多操作" })
    )

    const menu = screen.getByRole("menu")
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent)
    ).toEqual(["复制", "回复", "转发", "多选"])
    await user.click(within(menu).getByRole("menuitem", { name: "多选" }))
    expect(screen.getByText("已选择 1 条")).toBeInTheDocument()
  })

  it("enters multi-select from the context menu and chooses a forward mode", async () => {
    const user = userEvent.setup()
    const onForwardSelected = vi.fn()

    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <SelectionHarness onForwardSelected={onForwardSelected} />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    await user.pointer({
      keys: "[MouseRight]",
      target: screen.getByText("第一条"),
    })
    await user.click(screen.getByRole("menuitem", { name: "多选" }))

    expect(screen.getByText("已选择 1 条")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "合并转发" })).toBeDisabled()

    await user.click(screen.getByText("第二条"))
    expect(screen.getByText("已选择 2 条")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "合并转发" }))

    expect(onForwardSelected).toHaveBeenCalledWith("merged")
  })

  it("opens a merged chat record card with the requested summary", async () => {
    const user = userEvent.setup()
    const bundleMessage: ConversationPanelMessage = {
      ...createMessage("message-bundle", "unused"),
      body: {
        itemCount: 2,
        items: [
          {
            body: { content: "第一条", type: "text" },
            senderName: "Alice",
            senderType: "user",
            sentAt: "2026-07-13T10:00:00Z",
            summary: "第一条",
          },
          {
            body: {
              itemCount: 1,
              items: [
                {
                  body: { content: "内层消息", type: "text" },
                  senderName: "Carol",
                  senderType: "user",
                  sentAt: "2026-07-13T09:59:00Z",
                  summary: "内层消息",
                },
              ],
              type: "forward_bundle",
            },
            senderName: "Bob",
            senderType: "user",
            sentAt: "2026-07-13T10:01:00Z",
            summary: "[聊天记录] 1 条 - 内层消息",
          },
        ],
        type: "forward_bundle",
      },
    }

    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <ConversationPanel
            conversation={createConversation()}
            currentUserId="user-1"
            draft=""
            historyError={null}
            historyLoading={false}
            historyLoadingBefore={false}
            messages={[bundleMessage]}
            onCancelReply={vi.fn()}
            onDraftChange={vi.fn()}
            onLoadBeforeMessages={vi.fn()}
            onReplyToMessage={vi.fn()}
            onRevokeMessage={vi.fn()}
            onRichTextModeChange={vi.fn()}
            onSendFile={async () => null}
            onSendImage={async () => null}
            onSendMessage={vi.fn()}
            onSendVoice={async () => null}
            replyTarget={null}
            richTextMode={false}
            sending={false}
          />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    const card = screen.getByRole("button", {
      name: /\[聊天记录\] 2 条 - 第一条/,
    })
    await user.click(card)

    const dialog = screen.getByRole("dialog")
    expect(dialog).not.toHaveTextContent("[聊天记录] 2 条 - 第一条")
    expect(dialog).toHaveTextContent("Alice")
    expect(dialog).toHaveTextContent("Bob")

    const nestedCard = screen.getByRole("button", {
      name: /\[聊天记录\] 1 条 - 内层消息/,
    })
    expect(nestedCard.parentElement).toHaveAttribute(
      "data-forward-bundle-item-body"
    )

    await user.click(nestedCard)
    expect(screen.getByRole("dialog")).toHaveTextContent("Carol")
    expect(screen.getByRole("dialog")).toHaveTextContent("内层消息")
  })

  it("renders an unsupported message without hiding supported messages", async () => {
    const user = userEvent.setup()
    const onReply = vi.fn()
    const unsupportedMessage: ConversationPanelMessage = {
      ...createMessage("message-unsupported", "unused"),
      body: { type: "unsupported" },
    }

    render(
      <MemoryRouter>
        <ClientDataContext.Provider value={createClientDataValue()}>
          <ConversationPanel
            conversation={createConversation()}
            currentUserId="user-1"
            draft=""
            historyError={null}
            historyLoading={false}
            historyLoadingBefore={false}
            messages={[
              unsupportedMessage,
              createMessage("message-supported", "后续正常消息"),
            ]}
            onCancelReply={vi.fn()}
            onDraftChange={vi.fn()}
            onLoadBeforeMessages={vi.fn()}
            onReplyToMessage={onReply}
            onRevokeMessage={vi.fn()}
            onRichTextModeChange={vi.fn()}
            onSendFile={async () => null}
            onSendImage={async () => null}
            onSendMessage={vi.fn()}
            onSendVoice={async () => null}
            replyTarget={null}
            richTextMode={false}
            sending={false}
          />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    const unsupported = screen.getByText("暂不支持查看该消息")
    expect(screen.getByText("后续正常消息")).toBeInTheDocument()

    await user.pointer({ keys: "[MouseRight]", target: unsupported })
    expect(
      screen.queryByRole("menuitem", { name: "回复" })
    ).not.toBeInTheDocument()
    expect(onReply).not.toHaveBeenCalled()
  })
})

function SelectionHarness({
  onForwardSelected,
}: {
  onForwardSelected: (mode: ConversationPanelForwardMode) => void
}) {
  const [active, setActive] = React.useState(false)
  const [selectedMessageIds, setSelectedMessageIds] = React.useState<
    Set<string>
  >(() => new Set())

  function toggle(message: ConversationPanelMessage) {
    setSelectedMessageIds((current) => {
      const next = new Set(current)
      if (next.has(message.id)) {
        next.delete(message.id)
      } else {
        next.add(message.id)
      }
      return next
    })
  }

  return (
    <ConversationPanel
      conversation={createConversation()}
      currentUserId="user-1"
      draft=""
      historyError={null}
      historyLoading={false}
      historyLoadingBefore={false}
      messageSelection={{ active, selectedMessageIds }}
      messages={[
        createMessage("message-1", "第一条"),
        createMessage("message-2", "第二条"),
      ]}
      onCancelMessageSelection={() => {
        setActive(false)
        setSelectedMessageIds(new Set())
      }}
      onCancelReply={vi.fn()}
      onDraftChange={vi.fn()}
      onForwardSelectedMessages={onForwardSelected}
      onLoadBeforeMessages={vi.fn()}
      onReplyToMessage={vi.fn()}
      onRevokeMessage={vi.fn()}
      onRichTextModeChange={vi.fn()}
      onSendFile={async () => null}
      onSendImage={async () => null}
      onSendMessage={vi.fn()}
      onSendVoice={async () => null}
      onStartMessageSelection={(message) => {
        setActive(true)
        setSelectedMessageIds(new Set([message.id]))
      }}
      onToggleMessageSelection={toggle}
      replyTarget={null}
      richTextMode={false}
      sending={false}
    />
  )
}

function createConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-13T10:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 2,
    lastMessageSender: null,
    lastMessageSummary: "第二条",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 2,
    memberCount: 2,
    name: "测试会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
  }
}

function createMessage(id: string, content: string): ConversationPanelMessage {
  return {
    author: "Alice",
    avatar: "",
    body: { content, type: "text" },
    canRevoke: false,
    createdAt: "2026-07-13T10:00:00Z",
    delegatedByName: "",
    id,
    mentionTarget: null,
    reactionVersion: 0,
    reactions: [],
    role: "other",
    senderAppId: null,
    senderAppProfile: null,
    senderUserId: "user-2",
    time: "10:00",
  }
}

function createClientDataValue(): ClientDataContextValue {
  return {
    contacts: [],
    me: {
      avatar: "",
      createdAt: "2026-07-13T10:00:00Z",
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
