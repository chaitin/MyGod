import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ProjectTaskDetailsDialog } from "@/components/projects/project-task-details-dialog"
import type { ProjectTask } from "@/components/projects/project-types"

const mocks = vi.hoisted(() => ({
  getClientProjectTask: vi.fn(),
  listAllClientProjectMembers: vi.fn(),
  listClientProjectTasks: vi.fn(),
  sendConversationCard: vi.fn(),
  updateClientProjectTask: vi.fn(),
}))

vi.mock("@/lib/project-task-data-api", () => ({
  getClientProjectTask: mocks.getClientProjectTask,
  listClientProjectTasks: mocks.listClientProjectTasks,
  updateClientProjectTask: mocks.updateClientProjectTask,
}))

vi.mock("@/lib/project-members", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/lib/project-members")>()
  return {
    ...original,
    listAllClientProjectMembers: mocks.listAllClientProjectMembers,
  }
})

vi.mock("@/lib/client-data-context", () => ({
  useClientData: () => ({
    conversations: [
      {
        avatar: "",
        id: "conversation-1",
        name: "设计群",
        type: "group",
      },
    ],
    sendConversationCard: mocks.sendConversationCard,
  }),
}))

describe("ProjectTaskDetailsDialog card message", () => {
  beforeEach(() => {
    const task = createTask()
    mocks.getClientProjectTask.mockReset()
    mocks.getClientProjectTask.mockResolvedValue(task)
    mocks.listAllClientProjectMembers.mockReset()
    mocks.listAllClientProjectMembers.mockResolvedValue([])
    mocks.listClientProjectTasks.mockReset()
    mocks.listClientProjectTasks.mockResolvedValue({
      nextCursor: null,
      tasks: [],
    })
    mocks.sendConversationCard.mockReset()
    mocks.sendConversationCard.mockResolvedValue({
      id: "message-1",
    })
    mocks.updateClientProjectTask.mockReset()
    mocks.updateClientProjectTask.mockResolvedValue(task)
  })

  it("sends the task card and keeps the task details open", async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(
      <MemoryRouter>
        <ProjectTaskDetailsDialog
          onOpenChange={onOpenChange}
          open
          task={createTask()}
        />
      </MemoryRouter>
    )

    const sendButton = await screen.findByRole("button", {
      name: "发送到对话",
    })
    const descriptionPreview = document.querySelector(
      '[data-slot="task-description-preview"]'
    )
    expect(descriptionPreview).toBeInTheDocument()
    expect(descriptionPreview).toHaveClass(
      "border-input",
      "overflow-hidden",
      "shadow-xs"
    )
    expect(descriptionPreview?.firstElementChild).toHaveClass(
      "h-full",
      "contain-content",
      "overflow-auto"
    )
    expect(descriptionPreview).toHaveTextContent("这是任务说明")
    expect(descriptionPreview?.querySelector("strong")).toHaveTextContent(
      "这是任务说明"
    )
    expect(
      screen.queryByRole("textbox", { name: "详细内容" })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole("radio", { name: "显示渲染结果" })
    ).toHaveAttribute("aria-checked", "true")
    expect(
      screen.getByRole("radio", { name: "显示 Markdown 原文" })
    ).toHaveAttribute("aria-checked", "false")

    await user.click(screen.getByRole("radio", { name: "显示 Markdown 原文" }))
    expect(screen.getByRole("textbox", { name: "详细内容" })).toHaveClass(
      "field-sizing-fixed",
      "h-100",
      "min-h-100",
      "max-h-100",
      "resize-none",
      "font-mono!"
    )
    expect(
      screen.getByRole("radio", { name: "显示渲染结果" })
    ).toHaveAttribute("aria-checked", "false")
    expect(
      screen.getByRole("radio", { name: "显示 Markdown 原文" })
    ).toHaveAttribute("aria-checked", "true")
    await user.click(screen.getByRole("textbox", { name: "标题" }))
    expect(
      screen.getByRole("textbox", { name: "详细内容" })
    ).toBeInTheDocument()
    await user.click(screen.getByRole("radio", { name: "显示渲染结果" }))
    expect(
      document.querySelector('[data-slot="task-description-preview"]')
    ).toBeInTheDocument()
    await user.click(screen.getByRole("radio", { name: "显示 Markdown 原文" }))
    expect(sendButton).toBeEnabled()
    await user.click(sendButton)

    const sendDialog = screen.getByRole("dialog", { name: "发送到对话" })
    expect(sendDialog).not.toHaveTextContent("任务标题")
    expect(sendDialog).not.toHaveTextContent("这是任务说明")
    expect(sendDialog).not.toHaveTextContent("查看详情")

    await user.click(screen.getByRole("radio", { name: "设计群" }))
    await user.click(screen.getByRole("button", { name: "发送" }))

    await waitFor(() => {
      expect(mocks.sendConversationCard).toHaveBeenCalledWith(
        "conversation-1",
        {
          entityId: "task-1",
          entityType: "task",
          type: "entity_card",
        }
      )
    })
    expect(
      screen.queryByRole("dialog", { name: "发送到对话" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("dialog", { name: "任务详情" })).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalled()
  })

  it("configures a recurring reminder in the task form", async () => {
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <ProjectTaskDetailsDialog
          onOpenChange={vi.fn()}
          open
          task={createTask()}
        />
      </MemoryRouter>
    )

    const reminderButton = await screen.findByRole("button", {
      name: "提醒时间",
    })
    expect(reminderButton).toHaveTextContent("不提醒")
    await user.click(reminderButton)
    await user.click(screen.getByRole("button", { name: "重复" }))
    expect(mocks.updateClientProjectTask).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "确定" }))
    await user.click(screen.getByRole("button", { name: "保存" }))

    await waitFor(() => {
      expect(mocks.updateClientProjectTask).toHaveBeenCalledWith(
        "project-1",
        "task-1",
        {
          reminder: expect.objectContaining({
            frequency: "daily",
            mode: "recurring",
            timezone: "Asia/Shanghai",
          }),
        }
      )
    })
  })
})

function createTask(): ProjectTask {
  return {
    assignee: null,
    canceledAt: null,
    completedAt: null,
    createdAt: "2026-07-14T08:00:00Z",
    creator: {
      avatar: "",
      id: "user-1",
      name: "Alice",
      nickname: "",
    },
    description: "**这是任务说明**",
    dueDate: "2026-07-20",
    id: "task-1",
    labels: [],
    priority: 2,
    reminder: null,
    projectId: "project-1",
    startDate: "2026-07-14",
    status: "todo",
    title: "任务标题",
    updatedAt: "2026-07-14T08:00:00Z",
  }
}
