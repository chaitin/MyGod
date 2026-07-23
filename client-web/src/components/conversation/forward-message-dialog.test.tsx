import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ForwardMessageDialog } from "@/components/conversation/forward-message-dialog"
import type {
  ClientConversation,
  ForwardConversationMessagesResult,
} from "@/lib/client-data-api"

describe("ForwardMessageDialog", () => {
  it("selects multiple conversations including the current one", async () => {
    const user = userEvent.setup()
    const onComplete = vi.fn()
    const onForward = vi.fn(async () => ({
      failedCount: 0,
      results: [
        {
          conversationId: "conversation-1",
          messages: [],
          status: "sent" as const,
        },
        {
          conversationId: "conversation-2",
          messages: [],
          status: "sent" as const,
        },
      ],
      sentCount: 2,
    }))
    const onOpenChange = vi.fn()

    render(
      <ForwardMessageDialog
        conversations={[
          createConversation("conversation-1", "当前群聊", "group"),
          createConversation("conversation-2", "智能助手", "app"),
        ]}
        messageCount={2}
        onComplete={onComplete}
        onForward={onForward}
        onOpenChange={onOpenChange}
        open
      />
    )

    expect(screen.queryByText("当前会话")).not.toBeInTheDocument()
    await user.click(screen.getByRole("checkbox", { name: "当前群聊" }))
    await user.click(screen.getByRole("checkbox", { name: "智能助手" }))
    await user.click(screen.getByRole("button", { name: "转发（2）" }))

    expect(onForward).toHaveBeenCalledWith(["conversation-1", "conversation-2"])
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("retries only failed conversations after partial success", async () => {
    const user = userEvent.setup()
    const onComplete = vi.fn()
    const onForward = vi
      .fn<
        (
          targetConversationIds: string[]
        ) => Promise<ForwardConversationMessagesResult>
      >()
      .mockResolvedValueOnce({
        failedCount: 1,
        results: [
          {
            conversationId: "conversation-1",
            messages: [],
            status: "sent",
          },
          {
            conversationId: "conversation-2",
            error: { code: "forbidden", message: "无权发送" },
            messages: [],
            status: "failed",
          },
        ],
        sentCount: 1,
      })
      .mockResolvedValueOnce({
        failedCount: 0,
        results: [
          {
            conversationId: "conversation-2",
            messages: [],
            status: "sent",
          },
        ],
        sentCount: 1,
      })
    const onOpenChange = vi.fn()

    render(
      <ForwardMessageDialog
        conversations={[
          createConversation("conversation-1", "会话一", "group"),
          createConversation("conversation-2", "会话二", "direct"),
        ]}
        messageCount={2}
        onComplete={onComplete}
        onForward={onForward}
        onOpenChange={onOpenChange}
        open
      />
    )

    await user.click(screen.getByRole("checkbox", { name: "会话一" }))
    await user.click(screen.getByRole("checkbox", { name: "会话二" }))
    await user.click(screen.getByRole("button", { name: "转发（2）" }))

    await waitFor(() => expect(onForward).toHaveBeenCalledTimes(1))
    expect(onForward).toHaveBeenNthCalledWith(1, [
      "conversation-1",
      "conversation-2",
    ])
    expect(screen.getByRole("checkbox", { name: "会话一" })).toBeDisabled()

    await user.click(screen.getByRole("button", { name: "转发（1）" }))

    await waitFor(() => expect(onForward).toHaveBeenCalledTimes(2))
    expect(onForward).toHaveBeenNthCalledWith(2, ["conversation-2"])
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("cannot close while forwarding is in progress", async () => {
    const user = userEvent.setup()
    const deferred = createDeferred<ForwardConversationMessagesResult>()
    const onForward = vi.fn(() => deferred.promise)
    const onOpenChange = vi.fn()

    render(
      <ForwardMessageDialog
        conversations={[
          createConversation("conversation-1", "当前群聊", "group"),
        ]}
        messageCount={1}
        onComplete={vi.fn()}
        onForward={onForward}
        onOpenChange={onOpenChange}
        open
      />
    )

    await user.click(screen.getByRole("checkbox", { name: "当前群聊" }))
    await user.click(screen.getByRole("button", { name: "转发（1）" }))

    expect(
      screen.queryByRole("button", { name: "Close" })
    ).not.toBeInTheDocument()
    await user.keyboard("{Escape}")
    expect(onOpenChange).not.toHaveBeenCalled()

    await act(async () => {
      deferred.resolve({
        failedCount: 1,
        results: [
          {
            conversationId: "conversation-1",
            error: { code: "forbidden", message: "无权发送" },
            messages: [],
            status: "failed",
          },
        ],
        sentCount: 0,
      })
      await deferred.promise
    })

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument()
    )
  })
})

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function createConversation(
  id: string,
  name: string,
  type: ClientConversation["type"]
): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-13T10:00:00Z",
    id,
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSender: null,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    name,
    type,
    unreadCount: 0,
    visibility: "private",
  }
}
