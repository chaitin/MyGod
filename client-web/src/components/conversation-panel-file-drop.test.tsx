import { fireEvent, render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ConversationPanel } from "@/components/conversation-panel"
import type { ClientConversation } from "@/lib/client-data-api"

const { compressImageForMessageMock } = vi.hoisted(() => ({
  compressImageForMessageMock: vi.fn(),
}))

vi.mock("@/lib/image-message", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/image-message")>()

  return {
    ...actual,
    compressImageForMessage: compressImageForMessageMock,
  }
})

describe("ConversationPanel file drop", () => {
  beforeEach(() => {
    compressImageForMessageMock
      .mockReset()
      .mockImplementation(async (file: File) => file)
    Object.defineProperties(URL, {
      createObjectURL: {
        configurable: true,
        value: vi.fn(() => "blob:dropped-image"),
      },
      revokeObjectURL: {
        configurable: true,
        value: vi.fn(),
      },
    })
  })

  it("shows a full right-panel image overlay without flickering across children", () => {
    renderConversationPanel(createConversation())
    const panel = screen.getByTestId("chat-detail-shell")
    const header = screen.getByTestId("conversation-panel-header")
    const image = new File(["image"], "photo.png", { type: "image/png" })
    const dataTransfer = createFileDataTransfer([image])

    expect(fireEvent.dragEnter(panel, { dataTransfer })).toBe(false)

    const overlay = screen.getByTestId("conversation-file-drop-overlay")
    expect(panel).toContainElement(overlay)
    expect(overlay).toHaveTextContent("松开发送图片")
    expect(overlay).toHaveTextContent("支持 PNG、JPG 和 WebP")

    fireEvent.dragEnter(header, { dataTransfer })
    fireEvent.dragLeave(header, { dataTransfer })
    expect(
      screen.getByTestId("conversation-file-drop-overlay")
    ).toBeInTheDocument()

    fireEvent.dragLeave(panel, { dataTransfer })
    expect(
      screen.queryByTestId("conversation-file-drop-overlay")
    ).not.toBeInTheDocument()
  })

  it("opens the image dialog for the first supported image only", async () => {
    renderConversationPanel(createConversation())
    const panel = screen.getByTestId("chat-detail-shell")
    const image = new File(["image"], "photo.webp", {
      type: "image/webp",
    })
    const ignoredFile = new File(["document"], "notes.txt", {
      type: "text/plain",
    })
    const dataTransfer = createFileDataTransfer([image, ignoredFile])

    fireEvent.dragEnter(panel, { dataTransfer })
    expect(fireEvent.drop(panel, { dataTransfer })).toBe(false)

    expect(
      await screen.findByRole("dialog", { name: "发送图片" })
    ).toBeInTheDocument()
    expect(compressImageForMessageMock).toHaveBeenCalledTimes(1)
    expect(compressImageForMessageMock).toHaveBeenCalledWith(image)
    expect(
      screen.queryByTestId("conversation-file-drop-overlay")
    ).not.toBeInTheDocument()
  })

  it("opens the attachment dialog for the first non-image file only", async () => {
    renderConversationPanel(createConversation())
    const panel = screen.getByTestId("chat-detail-shell")
    const document = new File(["document"], "requirements.pdf", {
      type: "application/pdf",
    })
    const ignoredImage = new File(["image"], "photo.png", {
      type: "image/png",
    })
    const dataTransfer = createFileDataTransfer([document, ignoredImage])

    expect(fireEvent.drop(panel, { dataTransfer })).toBe(false)

    expect(
      await screen.findByRole("dialog", { name: "发送文件" })
    ).toBeInTheDocument()
    expect(screen.getByText("requirements.pdf")).toBeInTheDocument()
    expect(compressImageForMessageMock).not.toHaveBeenCalled()
  })

  it("does not intercept ordinary text dragging", () => {
    renderConversationPanel(createConversation())
    const panel = screen.getByTestId("chat-detail-shell")
    const dataTransfer = {
      files: [],
      items: [],
      types: ["text/plain"],
    } as unknown as DataTransfer

    expect(fireEvent.dragEnter(panel, { dataTransfer })).toBe(true)
    expect(fireEvent.dragOver(panel, { dataTransfer })).toBe(true)
    expect(
      screen.queryByTestId("conversation-file-drop-overlay")
    ).not.toBeInTheDocument()
  })

  it("rejects file drops when the conversation cannot accept files", () => {
    const file = new File(["document"], "notes.txt", { type: "text/plain" })
    const dataTransfer = createFileDataTransfer([file])

    const unavailableStates = [
      { conversation: null, sending: false },
      { conversation: createConversation(), sending: true },
    ]

    for (const state of unavailableStates) {
      const view = renderConversationPanel(state.conversation, state.sending)
      const panel = screen.getByTestId("chat-detail-shell")

      expect(fireEvent.dragEnter(panel, { dataTransfer })).toBe(false)
      expect(
        screen.queryByTestId("conversation-file-drop-overlay")
      ).not.toBeInTheDocument()
      expect(fireEvent.drop(panel, { dataTransfer })).toBe(false)
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument()

      view.unmount()
    }
  })
})

function renderConversationPanel(
  conversation: ClientConversation | null,
  sending = false
) {
  return render(
    <ConversationPanel
      conversation={conversation}
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
      onSendVoice={async () => null}
      onSendMessage={vi.fn()}
      replyTarget={null}
      richTextMode={false}
      sending={sending}
    />
  )
}

function createConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-10T00:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSender: null,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
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

function createFileDataTransfer(files: File[]) {
  return {
    dropEffect: "none",
    files,
    items: files.map((file) => ({ kind: "file", type: file.type })),
    types: ["Files"],
  } as unknown as DataTransfer
}
