import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  ConversationPanel,
  type ConversationPanelMessage,
} from "@/components/conversation-panel"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

const mocks = vi.hoisted(() => ({
  copyTemporaryImageToClipboard: vi.fn(),
  readTemporaryFileURLs: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock("@/lib/image-clipboard", () => ({
  copyTemporaryImageToClipboard: mocks.copyTemporaryImageToClipboard,
}))

vi.mock("@/lib/client-data-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/client-data-api")>()

  return {
    ...actual,
    readTemporaryFileURLs: mocks.readTemporaryFileURLs,
  }
})

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}))

describe("conversation image copy", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.readTemporaryFileURLs.mockResolvedValue([
      {
        expiresAt: "2026-07-17T12:00:00Z",
        fileId: "file-1",
        url: "https://example.com/image.png",
      },
    ])
  })

  it("copies an image from the message action menu and reports success", async () => {
    const user = userEvent.setup()
    mocks.copyTemporaryImageToClipboard.mockResolvedValue(undefined)
    renderImageConversation()

    await openImageMessageActionMenu()
    const copyAction = await screen.findByRole("menuitem", { name: "复制" })
    expect(copyAction).not.toHaveAttribute("data-disabled")

    await user.click(copyAction)

    await waitFor(() => {
      expect(mocks.copyTemporaryImageToClipboard).toHaveBeenCalledWith("file-1")
      expect(mocks.toastSuccess).toHaveBeenCalledWith("图片已复制")
    })
    expect(mocks.toastError).not.toHaveBeenCalled()
  })

  it("reports an error when copying the image fails", async () => {
    const user = userEvent.setup()
    mocks.copyTemporaryImageToClipboard.mockRejectedValue(
      new Error("clipboard unavailable")
    )
    renderImageConversation()

    await openImageMessageActionMenu()
    await user.click(await screen.findByRole("menuitem", { name: "复制" }))

    await waitFor(() => {
      expect(mocks.toastError).toHaveBeenCalledWith("图片复制失败")
    })
    expect(mocks.toastSuccess).not.toHaveBeenCalled()
  })
})

function renderImageConversation() {
  return render(
    <MemoryRouter>
      <ClientDataContext.Provider value={createClientDataValue()}>
        <ConversationPanel
          conversation={createConversation()}
          currentUserId="user-1"
          draft=""
          historyError={null}
          historyLoading={false}
          historyLoadingBefore={false}
          messages={[createImageMessage()]}
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
}

async function openImageMessageActionMenu() {
  const image = await screen.findByRole("button", { name: "预览图片" })
  const messageActionTrigger = image.closest("[data-message-action-trigger]")
  if (!messageActionTrigger) {
    throw new Error("missing message action trigger")
  }

  fireEvent.contextMenu(messageActionTrigger)
}

function createImageMessage(): ConversationPanelMessage {
  return {
    author: "Alice",
    avatar: "",
    body: {
      fileId: "file-1",
      height: 120,
      type: "image",
      width: 160,
    },
    canRevoke: false,
    delegatedByName: "",
    id: "message-image-1",
    mentionTarget: null,
    role: "other",
    senderAppId: null,
    senderAppProfile: null,
    senderUserId: "user-2",
    time: "10:00",
  }
}

function createConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-17T10:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 1,
    lastMessageSummary: "[图片]",
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
      createdAt: "2026-07-17T10:00:00Z",
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
