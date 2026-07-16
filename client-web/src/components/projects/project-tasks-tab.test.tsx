import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ProjectTasksTab } from "@/components/projects/project-tasks-tab"
import type { ProjectTask } from "@/components/projects/project-types"

const projectTaskApiMocks = vi.hoisted(() => ({
  getClientProjectTask: vi.fn(),
  listClientProjectTasks: vi.fn(),
}))

vi.mock("@/lib/project-task-data-api", async (importOriginal) => {
  const original = await importOriginal<
    typeof import("@/lib/project-task-data-api")
  >()
  return {
    ...original,
    getClientProjectTask: projectTaskApiMocks.getClientProjectTask,
    listClientProjectTasks: projectTaskApiMocks.listClientProjectTasks,
  }
})

vi.mock("@/lib/project-members", () => ({
  listAllClientProjectMembers: vi.fn().mockResolvedValue([]),
}))

vi.mock("@/components/projects/project-task-details-dialog", () => ({
  ProjectTaskDetailsDialog: ({
    onOpenChange,
    task,
  }: {
    onOpenChange: (open: boolean) => void
    task: ProjectTask
  }) => (
    <div aria-label="任务详情" role="dialog">
      <span>{task.title}</span>
      <button onClick={() => onOpenChange(false)} type="button">
        关闭详情
      </button>
    </div>
  ),
}))

describe("ProjectTasksTab task details route state", () => {
  beforeEach(() => {
    window.localStorage.clear()
    projectTaskApiMocks.getClientProjectTask.mockReset()
    projectTaskApiMocks.listClientProjectTasks.mockReset()
  })

  it("opens a linked task after refresh and removes only taskId when closed", async () => {
    const user = userEvent.setup()
    const task = createProjectTask()
    projectTaskApiMocks.listClientProjectTasks.mockResolvedValue({
      nextCursor: null,
      tasks: [],
    })
    projectTaskApiMocks.getClientProjectTask.mockResolvedValue(task)

    renderProjectTasksTab("/projects/project-1?source=link&taskId=task-1")

    expect(await screen.findByRole("dialog", { name: "任务详情" })).toHaveTextContent(
      task.title
    )
    expect(projectTaskApiMocks.getClientProjectTask).toHaveBeenCalledWith(
      "project-1",
      "task-1"
    )

    await user.click(screen.getByRole("button", { name: "关闭详情" }))

    await waitFor(() => {
      expect(screen.getByTestId("location-search")).toHaveTextContent(
        "?source=link"
      )
    })
    expect(screen.queryByRole("dialog", { name: "任务详情" })).not.toBeInTheDocument()
  })

  it("adds taskId to the URL when a task is opened", async () => {
    const user = userEvent.setup()
    const task = createProjectTask()
    projectTaskApiMocks.listClientProjectTasks.mockResolvedValue({
      nextCursor: null,
      tasks: [task],
    })

    renderProjectTasksTab("/projects/project-1?source=list")

    await user.click(
      await screen.findByRole("button", {
        name: `查看任务详情：${task.title}`,
      })
    )

    expect(await screen.findByRole("dialog", { name: "任务详情" })).toBeInTheDocument()
    expect(screen.getByTestId("location-search")).toHaveTextContent(
      "source=list&taskId=task-1"
    )
  })
})

function renderProjectTasksTab(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route
          path="/projects/:projectId"
          element={
            <>
              <ProjectTasksTab
                onTasksChanged={vi.fn().mockResolvedValue(undefined)}
                projectId="project-1"
              />
              <LocationSearch />
            </>
          }
        />
      </Routes>
    </MemoryRouter>
  )
}

function LocationSearch() {
  const location = useLocation()
  return <output data-testid="location-search">{location.search}</output>
}

function createProjectTask(): ProjectTask {
  const creator = {
    avatar: "",
    id: "user-1",
    name: "Creator",
    nickname: "创建人",
  }
  return {
    assignee: null,
    canceledAt: null,
    completedAt: null,
    createdAt: "2026-07-14T01:00:00Z",
    creator,
    description: "任务描述",
    dueDate: "2026-07-20",
    id: "task-1",
    labels: [],
    priority: 2,
    reminder: null,
    projectId: "project-1",
    startDate: "2026-07-14",
    status: "todo",
    title: "路由任务",
    updatedAt: "2026-07-14T01:00:00Z",
  }
}
