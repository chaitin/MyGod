import * as React from "react"
import { act, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { ClientConversation } from "@/lib/client-data-api"
import type { ConversationPanelMessage } from "@/lib/conversation-panel-types"
import { ConversationPanelHistory } from "@/components/conversation/conversation-panel-history"
import { formatConversationMessageTime } from "@/lib/conversation-message-presenter"

const testState = vi.hoisted(() => ({
  anchorTop: 100,
  bubbleRenderCount: 0,
  clientHeight: 400,
  resizeObserverCallback: null as ResizeObserverCallback | null,
  scrollHeight: 1_000,
}))

const defaultResizeObserver = window.ResizeObserver

class ControlledResizeObserver implements ResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    testState.resizeObserverCallback = callback
  }

  disconnect() {}
  observe() {}
  unobserve() {}
}

vi.mock("@/components/ui/scroll-area", () => {
  return {
    ScrollArea({
      children,
      viewportProps,
      viewportRef,
    }: {
      children: React.ReactNode
      viewportProps?: React.HTMLAttributes<HTMLDivElement>
      viewportRef?: React.Ref<HTMLDivElement>
    }) {
      function setViewportRef(node: HTMLDivElement | null) {
        if (node) {
          Object.defineProperties(node, {
            clientHeight: {
              configurable: true,
              get: () => testState.clientHeight,
            },
            scrollHeight: {
              configurable: true,
              get: () => testState.scrollHeight,
            },
          })
        }

        if (typeof viewportRef === "function") {
          viewportRef(node)
        } else if (viewportRef) {
          viewportRef.current = node
        }
      }

      return (
        <div>
          <div
            {...viewportProps}
            data-slot="scroll-area-viewport"
            ref={setViewportRef}
          >
            {children}
          </div>
        </div>
      )
    },
  }
})

vi.mock("@/components/conversation/conversation-message", () => ({
  MessageBubble({ message }: { message: ConversationPanelMessage }) {
    testState.bubbleRenderCount += 1
    return (
      <div
        data-conversation-message-id={message.id}
        data-testid={`message-${message.id}`}
        ref={(node) => {
          if (node) {
            node.getBoundingClientRect = () =>
              ({ top: testState.anchorTop }) as DOMRect
          }
        }}
      >
        {message.author}
      </div>
    )
  },
  SystemMessageBadge({ message }: { message: ConversationPanelMessage }) {
    return (
      <div
        data-conversation-message-id={message.id}
        data-testid={`message-${message.id}`}
      >
        {message.author}
      </div>
    )
  },
}))

describe("ConversationPanelHistory", () => {
  beforeEach(() => {
    testState.anchorTop = 100
    testState.bubbleRenderCount = 0
    testState.clientHeight = 400
    testState.resizeObserverCallback = null
    testState.scrollHeight = 1_000
    Object.defineProperty(window, "ResizeObserver", {
      configurable: true,
      value: ControlledResizeObserver,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    Object.defineProperty(window, "ResizeObserver", {
      configurable: true,
      value: defaultResizeObserver,
    })
  })

  it("follows an incoming message when already near the bottom", () => {
    const props = createProps([createMessage("message-1", "other")])
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()

    testState.scrollHeight = 1_100
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[...props.messages, createMessage("message-2", "other")]}
      />
    )

    expect(viewport.scrollTop).toBe(1_100)
    expect(screen.queryByText(/条新消息/)).not.toBeInTheDocument()
  })

  it("stays at the bottom when existing message content grows", () => {
    const props = createProps([createMessage("message-1", "other")])
    render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()

    testState.scrollHeight = 1_100
    testState.resizeObserverCallback?.([], {} as ResizeObserver)

    expect(viewport.scrollTop).toBe(1_100)
  })

  it("does not move when existing message content grows away from the bottom", () => {
    const props = createProps([createMessage("message-1", "other")])
    render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 100
    fireEvent.scroll(viewport)

    testState.scrollHeight = 1_100
    testState.resizeObserverCallback?.([], {} as ResizeObserver)

    expect(viewport.scrollTop).toBe(100)
  })

  it("keeps the reading position and reports incoming messages away from the bottom", () => {
    const props = createProps([createMessage("message-1", "other")])
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 100
    fireEvent.scroll(viewport)

    testState.scrollHeight = 1_100
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[...props.messages, createMessage("message-2", "other")]}
      />
    )

    expect(viewport.scrollTop).toBe(100)
    expect(screen.getByRole("button", { name: "1 条新消息" })).toBeVisible()

    fireEvent.click(screen.getByRole("button", { name: "1 条新消息" }))

    expect(viewport.scrollTop).toBe(1_100)
    expect(screen.queryByText(/条新消息/)).not.toBeInTheDocument()
  })

  it("follows a message sent by the current user away from the bottom", () => {
    const props = createProps([createMessage("message-1", "other")])
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 100
    fireEvent.scroll(viewport)

    testState.scrollHeight = 1_100
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[...props.messages, createMessage("message-2", "me")]}
      />
    )

    expect(viewport.scrollTop).toBe(1_100)
    expect(screen.queryByText(/条新消息/)).not.toBeInTheDocument()
  })

  it("preserves the visible position when older messages are prepended", () => {
    const onLoadBeforeMessages = vi.fn()
    const props = createProps(
      [createMessage("message-2", "other")],
      onLoadBeforeMessages
    )
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 20
    fireEvent.scroll(viewport)

    expect(onLoadBeforeMessages).toHaveBeenCalledOnce()

    testState.anchorTop = 300
    testState.scrollHeight = 1_200
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[createMessage("message-1", "other"), ...props.messages]}
      />
    )

    expect(viewport.scrollTop).toBe(220)
  })

  it("handles older and newer messages arriving in the same render", () => {
    const props = createProps([createMessage("message-2", "other")])
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 20
    fireEvent.scroll(viewport)

    testState.anchorTop = 300
    testState.scrollHeight = 1_300
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[
          createMessage("message-1", "other"),
          ...props.messages,
          createMessage("message-3", "other"),
        ]}
      />
    )

    expect(viewport.scrollTop).toBe(220)
    expect(screen.getByRole("button", { name: "1 条新消息" })).toBeVisible()
  })

  it("does not rerender message rows when all history props are unchanged", () => {
    const props = createProps([createMessage("message-1", "other")])
    const { rerender } = render(<ConversationPanelHistory {...props} />)

    expect(testState.bubbleRenderCount).toBe(1)

    rerender(<ConversationPanelHistory {...props} />)

    expect(testState.bubbleRenderCount).toBe(1)
  })

  it("compacts an oversized history when it is at the bottom", () => {
    const onCompactMessages = vi.fn()
    const messages = Array.from({ length: 301 }, (_, index) =>
      createMessage(`message-${index + 1}`, "other")
    )

    render(
      <ConversationPanelHistory
        {...createProps(messages)}
        onCompactMessages={onCompactMessages}
      />
    )

    expect(onCompactMessages).toHaveBeenCalledOnce()
  })

  it("does not compact while reading away from the bottom", () => {
    const onCompactMessages = vi.fn()
    const initialMessages = Array.from({ length: 300 }, (_, index) =>
      createMessage(`message-${index + 1}`, "other")
    )
    const props = {
      ...createProps(initialMessages),
      onCompactMessages,
    }
    const { rerender } = render(<ConversationPanelHistory {...props} />)
    const viewport = getViewport()
    viewport.scrollTop = 100
    fireEvent.scroll(viewport)

    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[...initialMessages, createMessage("message-301", "other")]}
      />
    )

    expect(onCompactMessages).not.toHaveBeenCalled()
  })

  it("compacts the conversation cache when leaving the history", () => {
    const onCompactMessages = vi.fn()
    const { unmount } = render(
      <ConversationPanelHistory
        {...createProps([createMessage("message-1", "other")])}
        onCompactMessages={onCompactMessages}
      />
    )

    unmount()

    expect(onCompactMessages).toHaveBeenCalledOnce()
  })

  it("protects recently loaded history for three minutes", () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-07-23T00:00:00Z"))
    const onCompactMessages = vi.fn()
    const latestMessages = Array.from({ length: 300 }, (_, index) =>
      createMessage(`message-${index + 2}`, "other")
    )
    const props = {
      ...createProps(latestMessages),
      onCompactMessages,
    }
    const { rerender } = render(<ConversationPanelHistory {...props} />)

    rerender(<ConversationPanelHistory {...props} loadingBefore />)
    rerender(
      <ConversationPanelHistory
        {...props}
        messages={[createMessage("message-1", "other"), ...latestMessages]}
      />
    )

    expect(onCompactMessages).not.toHaveBeenCalled()

    act(() => vi.advanceTimersByTime(3 * 60 * 1000))

    expect(onCompactMessages).toHaveBeenCalledOnce()
  })

  it("marks the newer message when adjacent messages are more than one hour apart", () => {
    const firstMessage = createMessage(
      "message-1",
      "other",
      "2026-07-21T10:00:00Z"
    )
    const exactlyOneHourLater = createMessage(
      "message-2",
      "other",
      "2026-07-21T11:00:00Z"
    )
    const moreThanOneHourLater = createMessage(
      "message-3",
      "other",
      "2026-07-21T12:00:01Z"
    )

    render(
      <ConversationPanelHistory
        {...createProps([
          firstMessage,
          exactlyOneHourLater,
          moreThanOneHourLater,
        ])}
      />
    )

    const markers = document.querySelectorAll("[data-message-time-marker]")
    expect(markers).toHaveLength(1)
    expect(markers[0]).toHaveTextContent(
      formatConversationMessageTime(moreThanOneHourLater.createdAt)
    )
  })
})

function getViewport() {
  const viewport = document.querySelector<HTMLDivElement>(
    '[data-slot="scroll-area-viewport"]'
  )
  if (!viewport) {
    throw new Error("消息历史滚动容器不存在")
  }
  return viewport
}

function createProps(
  messages: ConversationPanelMessage[],
  onLoadBeforeMessages = vi.fn()
) {
  return {
    conversation: createConversation(),
    currentUserId: "user-me",
    error: null,
    loading: false,
    loadingBefore: false,
    mentionLabelResolver: () => undefined,
    messages,
    onInsertMention: vi.fn(),
    onLoadBeforeMessages,
    onReplyToMessage: vi.fn(),
    onRevokeMessage: vi.fn(),
  }
}

function createConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-14T10:00:00Z",
    id: "conversation-1",
    lastMentionedSeq: 0,
    lastMessageAt: "2026-07-14T10:00:00Z",
    lastMessageId: "message-1",
    lastMessageSeq: 1,
    lastMessageSender: null,
    lastMessageSummary: "测试消息",
    lastChoiceSeq: 0,
    lastReadSeq: 1,
    memberCount: 2,
    name: "测试会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
  }
}

function createMessage(
  id: string,
  role: ConversationPanelMessage["role"],
  createdAt = "2026-07-14T10:00:00Z"
): ConversationPanelMessage {
  return {
    author: role === "me" ? "我" : "Alice",
    avatar: "",
    body: { content: id, type: "text" },
    canRevoke: role === "me",
    createdAt,
    delegatedByName: "",
    id,
    mentionTarget: null,
    reactionVersion: 0,
    reactions: [],
    role,
    senderAppId: "",
    senderAppProfile: null,
    senderUserId: role === "me" ? "user-me" : "user-other",
    time: "10:00",
  }
}
