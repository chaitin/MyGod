import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TopicSourceBanner } from "@/components/conversation/topic-source-banner"
import type { ClientTopicSourceMessage } from "@/lib/client-data-api"

vi.mock("@/components/user-profile-popover", () => ({
  UserProfilePopover: ({ children }: { children: ReactNode }) => children,
}))

describe("TopicSourceBanner", () => {
  it("renders the preserved source body instead of reducing it to its summary", () => {
    const sourceMessage: ClientTopicSourceMessage = {
      body: { content: "完整来源消息", type: "text" },
      createdAt: "2026-07-20T04:00:00Z",
      id: "message-1",
      revokedAt: null,
      sender: {
        avatar: "/avatars/alice.webp",
        id: "user-1",
        name: "Alice",
        type: "user",
      },
      seq: 8,
      summary: "不同的摘要",
    }

    render(
      <TopicSourceBanner currentUserId="user-2" sourceMessage={sourceMessage} />
    )

    expect(screen.getByText("完整来源消息")).toBeInTheDocument()
    expect(screen.queryByText("不同的摘要")).not.toBeInTheDocument()
  })

  it("renders and updates reactions on the source message", async () => {
    const onSetReaction = vi.fn().mockResolvedValue(undefined)
    render(
      <TopicSourceBanner
        currentUserId="user-2"
        onSetReaction={onSetReaction}
        reactions={[
          {
            count: 2,
            reactedByMe: true,
            text: "👍",
            users: [
              { id: "user-1", name: "Alice" },
              { id: "user-2", name: "Bob" },
            ],
          },
        ]}
        sourceMessage={createSourceMessage()}
      />
    )

    const reactionChip = screen.getByRole("button", {
      name: "移除表情 👍",
    })
    const addButton = screen.getByRole("button", { name: "添加表情" })
    const bubbleLine = addButton.closest('[data-slot="message-bubble-line"]')
    expect(bubbleLine).toContainElement(
      screen.getByTestId("topic-source-message-bubble")
    )
    expect(screen.getByTestId("topic-source-message-bubble")).toContainElement(
      reactionChip
    )

    fireEvent.click(reactionChip)
    await waitFor(() => expect(onSetReaction).toHaveBeenCalledWith("👍", false))
    expect(screen.getByRole("button", { name: "添加表情" })).toBeInTheDocument()
  })

  it("supports forwarding and selecting the source message", async () => {
    const user = userEvent.setup()
    const onForward = vi.fn()
    const onMultiSelect = vi.fn()
    const onToggleSelected = vi.fn()
    const sourceMessage = createSourceMessage()
    const view = render(
      <TopicSourceBanner
        currentUserId="user-2"
        onForward={onForward}
        onMultiSelect={onMultiSelect}
        onToggleSelected={onToggleSelected}
        sourceMessage={sourceMessage}
      />
    )

    fireEvent.contextMenu(screen.getByTestId("topic-source-message-bubble"))
    fireEvent.click(screen.getByRole("menuitem", { name: "多选" }))
    expect(onMultiSelect).toHaveBeenCalledWith(sourceMessage)

    await user.click(screen.getByRole("button", { name: "更多操作" }))
    await user.click(screen.getByRole("menuitem", { name: "转发" }))
    expect(onForward).toHaveBeenCalledWith(sourceMessage)

    view.rerender(
      <TopicSourceBanner
        currentUserId="user-2"
        onForward={onForward}
        onMultiSelect={onMultiSelect}
        onToggleSelected={onToggleSelected}
        selected
        selectionMode
        sourceMessage={sourceMessage}
      />
    )

    const checkbox = screen.getByRole("checkbox", {
      name: "取消选择Alice的消息",
    })
    expect(checkbox).toBeChecked()
    fireEvent.click(checkbox)
    expect(onToggleSelected).toHaveBeenCalledWith(sourceMessage)
  })

  it("renders and answers a choice source message", async () => {
    const onRespondToChoice = vi.fn().mockResolvedValue(undefined)
    const sourceMessage: ClientTopicSourceMessage = {
      ...createSourceMessage(),
      body: {
        content: "请选择项目",
        contentType: "text",
        options: [
          { id: "project-a", label: "项目 A" },
          { id: "project-b", label: "项目 B" },
        ],
        selection: "single",
        type: "choice",
      },
    }
    render(
      <TopicSourceBanner
        currentUserId="user-2"
        onRespondToChoice={onRespondToChoice}
        sourceChoice={{
          myOptionIds: [],
          options: [
            { id: "project-a", responseCount: 1 },
            { id: "project-b", responseCount: 2 },
          ],
          responseCount: 3,
        }}
        sourceChoiceStatus="active"
        sourceMessage={sourceMessage}
      />
    )

    expect(screen.queryByText("3 人已回答")).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole("radio", { name: "项目 B" }))
    fireEvent.click(screen.getByRole("button", { name: "提交" }))
    await waitFor(() =>
      expect(onRespondToChoice).toHaveBeenCalledWith(["project-b"])
    )
  })
})

function createSourceMessage(): ClientTopicSourceMessage {
  return {
    body: { content: "完整来源消息", type: "text" },
    createdAt: "2026-07-20T04:00:00Z",
    id: "message-1",
    revokedAt: null,
    sender: {
      avatar: "/avatars/alice.webp",
      id: "user-1",
      name: "Alice",
      type: "user",
    },
    seq: 8,
    summary: "不同的摘要",
  }
}
