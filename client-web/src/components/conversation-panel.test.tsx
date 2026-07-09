import { render, screen, waitFor } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { ConversationPanel } from "@/components/conversation-panel"
import type { ClientConversation } from "@/lib/client-data-api"

describe("ConversationPanel", () => {
  it("focuses the composer textarea when a conversation is opened", async () => {
    render(
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

    await waitFor(() => expect(composer).toHaveFocus())
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
