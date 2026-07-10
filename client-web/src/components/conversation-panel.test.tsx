import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import {
  ConversationPanel,
  type ConversationPanelMessage,
} from "@/components/conversation-panel"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

describe("ConversationPanel", () => {
  it("refocuses the composer textarea when a reply target is selected", async () => {
    const { rerender } = render(
      <ConversationPanel
        conversation={createConversation("conversation-1")}
        currentUserId="user-1"
        draft=""
        historyError={null}
        historyLoading={false}
        historyLoadingBefore={false}
        messages={[]}
        onCancelReply={vi.fn()}
        onDraftChange={vi.fn()}
        onLoadBeforeMessages={vi.fn()}
        onReplyToMessage={vi.fn()}
        onRevokeMessage={vi.fn()}
        onRichTextModeChange={vi.fn()}
        onSendFile={async () => null}
        onSendImage={async () => null}
        onSendMessage={vi.fn()}
        replyTarget={null}
        richTextMode={false}
        sending={false}
      />
    )

    const composer = screen.getByPlaceholderText("输入消息")
    const sendButton = screen.getByRole("button", { name: "发送消息" })

    await waitFor(() => expect(composer).toHaveFocus())
    sendButton.focus()
    expect(sendButton).toHaveFocus()

    rerender(
      <ConversationPanel
        conversation={createConversation("conversation-1")}
        currentUserId="user-1"
        draft=""
        historyError={null}
        historyLoading={false}
        historyLoadingBefore={false}
        messages={[]}
        onCancelReply={vi.fn()}
        onDraftChange={vi.fn()}
        onLoadBeforeMessages={vi.fn()}
        onReplyToMessage={vi.fn()}
        onRevokeMessage={vi.fn()}
        onRichTextModeChange={vi.fn()}
        onSendFile={async () => null}
        onSendImage={async () => null}
        onSendMessage={vi.fn()}
        replyTarget={{
          author: "李四",
          id: "message-1",
          summary: "收到",
        }}
        richTextMode={false}
        sending={false}
      />
    )

    await waitFor(() => expect(composer).toHaveFocus())
  })

  it("does not send when Enter belongs to an IME interaction", () => {
    const onSendMessage = vi.fn()

    render(
      <ConversationPanel
        conversation={createConversation("conversation-1")}
        currentUserId="user-1"
        draft="nihao"
        historyError={null}
        historyLoading={false}
        historyLoadingBefore={false}
        messages={[]}
        onCancelReply={vi.fn()}
        onDraftChange={vi.fn()}
        onLoadBeforeMessages={vi.fn()}
        onReplyToMessage={vi.fn()}
        onRevokeMessage={vi.fn()}
        onRichTextModeChange={vi.fn()}
        onSendFile={async () => null}
        onSendImage={async () => null}
        onSendMessage={onSendMessage}
        replyTarget={null}
        richTextMode={false}
        sending={false}
      />
    )

    const composer = screen.getByPlaceholderText("输入消息")
    const compositionKeyDownNotCanceled = fireEvent.keyDown(composer, {
      code: "Enter",
      isComposing: true,
      key: "Enter",
    })
    const processKeyDownNotCanceled = fireEvent.keyDown(composer, {
      code: "Enter",
      key: "Enter",
      keyCode: 229,
    })

    expect(compositionKeyDownNotCanceled).toBe(true)
    expect(processKeyDownNotCanceled).toBe(true)
    expect(onSendMessage).not.toHaveBeenCalled()
  })

  it("opens the app profile popover from an app message avatar", async () => {
    const user = userEvent.setup()
    const openAppConversation = vi.fn()

    render(
      <MemoryRouter>
        <ClientDataContext.Provider
          value={createClientDataValue({
            contactApps: [
              {
                avatar: "/assets/apps/assistant.webp",
                description: "企业助手",
                id: "app-1",
                name: "智能助手",
                online: true,
                type: "app",
              },
            ],
            openAppConversation,
          })}
        >
          <ConversationPanel
            conversation={createConversation("conversation-1")}
            currentUserId="user-1"
            draft=""
            historyError={null}
            historyLoading={false}
            historyLoadingBefore={false}
            messages={[
              createAppPanelMessage({
                appId: "app-1",
                avatar: "/assets/apps/assistant.webp",
                author: "智能助手",
              }),
            ]}
            onCancelReply={vi.fn()}
            onDraftChange={vi.fn()}
            onLoadBeforeMessages={vi.fn()}
            onReplyToMessage={vi.fn()}
            onRevokeMessage={vi.fn()}
            onRichTextModeChange={vi.fn()}
            onSendFile={async () => null}
            onSendImage={async () => null}
            onSendMessage={vi.fn()}
            replyTarget={null}
            richTextMode={false}
            sending={false}
          />
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    await user.click(screen.getByRole("button", { name: "智能助手资料" }))

    expect(await screen.findByText("企业助手")).toBeInTheDocument()
    expect(screen.getByText("类型")).toBeInTheDocument()
    expect(screen.getByText("应用")).toBeInTheDocument()
    expect(screen.getByText("状态")).toBeInTheDocument()
    expect(screen.getByText("在线")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "发消息" })).toBeInTheDocument()
  })
})

function createConversation(id: string): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-09T00:00:00Z",
    id,
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSummary: "",
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    members: [],
    name: "测试会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
  }
}

function createAppPanelMessage({
  appId,
  author,
  avatar,
}: {
  appId: string
  author: string
  avatar: string
}): ConversationPanelMessage {
  return {
    author,
    avatar,
    body: {
      content: "应用消息",
      type: "text",
    },
    canRevoke: false,
    delegatedByName: "",
    id: "message-1",
    mentionTarget: null,
    role: "other",
    senderAppId: appId,
    senderAppProfile: {
      avatar,
      description: "",
      id: appId,
      name: author,
      online: false,
    },
    senderUserId: null,
    time: "10:00",
  }
}

function createClientDataValue(
  overrides: Partial<ClientDataContextValue> = {}
): ClientDataContextValue {
  const value: Partial<ClientDataContextValue> = {
    contactApps: [],
    contactGroups: [],
    contacts: [],
    contactsError: null,
    contactsLoading: false,
    contactsRefreshing: false,
    conversations: [],
    me: {
      avatar: "",
      createdAt: "2026-07-09T00:00:00Z",
      email: "me@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "张三",
      nickname: "",
      phone: "",
      status: "active",
    },
    meError: null,
    meLoading: false,
    meRefreshing: false,
    openAppConversation: vi.fn(),
    openDirectConversation: vi.fn(),
    ...overrides,
  }

  return value as ClientDataContextValue
}
