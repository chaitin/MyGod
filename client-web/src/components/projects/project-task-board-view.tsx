import * as React from "react"
import {
  closestCorners,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import { createPortal } from "react-dom"
import { toast } from "sonner"

import type {
  ProjectTask,
  ProjectTaskStatus,
} from "@/components/projects/project-types"
import {
  ProjectTaskAssigneeAvatar,
  ProjectTaskPriorityIcon,
  ProjectTaskStatusIcon,
} from "@/components/projects/project-task-view-elements"
import {
  projectTaskPriorityLabels,
  projectTaskStatusDetails,
} from "@/components/projects/project-task-view-utils"
import { UpdateProjectTaskDateDialog } from "@/components/projects/update-project-task-date-dialog"
import { UpdateProjectTaskPriorityDialog } from "@/components/projects/update-project-task-priority-dialog"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { updateClientProjectTask } from "@/lib/project-task-data-api"
import { cn } from "@/lib/utils"

const boardColumns: ProjectTaskStatus[] = [
  "todo",
  "in_progress",
  "done",
  "canceled",
]

export function ProjectTaskBoardView({
  emptyMessage = "暂无任务",
  onOpenTask,
  onTaskStatusChange,
  onTaskUpdated,
  tasks,
}: {
  emptyMessage?: string
  onOpenTask: (task: ProjectTask) => void
  onTaskStatusChange: (taskId: string, status: ProjectTaskStatus) => void
  onTaskUpdated: () => Promise<void>
  tasks: ProjectTask[]
}) {
  const [draggedTaskId, setDraggedTaskId] = React.useState<string | null>(null)
  const [updatingTaskIds, setUpdatingTaskIds] = React.useState<Set<string>>(
    () => new Set()
  )
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 6 },
    }),
    useSensor(KeyboardSensor)
  )

  const draggedTask = draggedTaskId
    ? (tasks.find((task) => task.id === draggedTaskId) ?? null)
    : null

  async function handleDragEnd(event: DragEndEvent) {
    setDraggedTaskId(null)

    const taskId = String(event.active.id)
    const nextStatus = parseProjectTaskStatus(event.over?.id)
    const task = tasks.find((candidate) => candidate.id === taskId)
    if (!task || !nextStatus || task.status === nextStatus) {
      return
    }

    const previousStatus = task.status
    onTaskStatusChange(taskId, nextStatus)
    setTaskUpdating(taskId, true)

    try {
      await updateClientProjectTask(task.projectId, task.id, {
        status: nextStatus,
      })
      toast.success(`任务已移至${projectTaskStatusDetails[nextStatus].label}`)
      await onTaskUpdated()
    } catch (error) {
      onTaskStatusChange(taskId, previousStatus)
      toast.error(error instanceof Error ? error.message : "更新任务状态失败")
    } finally {
      setTaskUpdating(taskId, false)
    }
  }

  function setTaskUpdating(taskId: string, updating: boolean) {
    setUpdatingTaskIds((current) => {
      const next = new Set(current)
      if (updating) {
        next.add(taskId)
      } else {
        next.delete(taskId)
      }
      return next
    })
  }

  return (
    <>
      <DndContext
        collisionDetection={closestCorners}
        onDragCancel={() => setDraggedTaskId(null)}
        onDragEnd={(event) => void handleDragEnd(event)}
        onDragStart={(event) => setDraggedTaskId(String(event.active.id))}
        sensors={sensors}
      >
        <div className="min-h-0 flex-1 overflow-x-auto">
          <div className="grid h-full min-h-0 min-w-[64rem] grid-cols-4 gap-3">
            {boardColumns.map((status) => (
              <BoardColumn
                dragging={draggedTaskId !== null}
                emptyMessage={emptyMessage}
                key={status}
                onOpenTask={onOpenTask}
                onTaskUpdated={onTaskUpdated}
                status={status}
                tasks={tasks.filter((task) => task.status === status)}
                updatingTaskIds={updatingTaskIds}
              />
            ))}
          </div>
        </div>
        {typeof document !== "undefined" &&
          createPortal(
            <DragOverlay dropAnimation={null}>
              {draggedTask && <BoardTaskOverlay task={draggedTask} />}
            </DragOverlay>,
            document.body
          )}
      </DndContext>
    </>
  )
}

function BoardColumn({
  dragging,
  emptyMessage,
  onOpenTask,
  onTaskUpdated,
  status,
  tasks,
  updatingTaskIds,
}: {
  dragging: boolean
  emptyMessage: string
  onOpenTask: (task: ProjectTask) => void
  onTaskUpdated: () => Promise<void>
  status: ProjectTaskStatus
  tasks: ProjectTask[]
  updatingTaskIds: Set<string>
}) {
  const { isOver, setNodeRef } = useDroppable({ id: status })
  const details = projectTaskStatusDetails[status]

  return (
    <section
      ref={setNodeRef}
      aria-labelledby={`project-task-board-${status}`}
      className={cn(
        "flex min-h-0 min-w-0 flex-col rounded-md border-2 border-transparent bg-muted/50 p-2 transition-colors",
        dragging &&
          `border-dashed ${getProjectTaskDraggingColumnColor(status)} ${getProjectTaskDraggingColumnBorderColor(status)}`,
        isOver &&
          `${getProjectTaskDropTargetColor(status)} ${getProjectTaskDropTargetBorderColor(status)}`
      )}
    >
      <header className="flex h-9 shrink-0 items-center gap-2 px-1.5">
        <ProjectTaskStatusIcon
          className={getProjectTaskStatusColor(status)}
          status={status}
        />
        <h2 className="text-sm font-medium" id={`project-task-board-${status}`}>
          {details.label}
        </h2>
        <Badge
          className="ml-auto min-w-5 bg-background px-1.5 tabular-nums"
          variant="secondary"
        >
          {tasks.length}
        </Badge>
      </header>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex min-h-full flex-col gap-2 pr-2" role="list">
          {tasks.map((task) => (
            <BoardTask
              draggingDisabled={updatingTaskIds.size > 0}
              key={task.id}
              onOpen={() => onOpenTask(task)}
              onUpdated={onTaskUpdated}
              task={task}
            />
          ))}
          {tasks.length === 0 && (
            <div className="flex min-h-24 items-center justify-center rounded-md border border-dashed text-xs text-muted-foreground">
              {emptyMessage}
            </div>
          )}
        </div>
      </ScrollArea>
    </section>
  )
}

function BoardTask({
  draggingDisabled,
  onOpen,
  onUpdated,
  task,
}: {
  draggingDisabled: boolean
  onOpen: () => void
  onUpdated: () => Promise<void>
  task: ProjectTask
}) {
  const { attributes, isDragging, listeners, setNodeRef } = useDraggable({
    disabled: draggingDisabled,
    id: task.id,
  })
  const [dueDateDialogOpen, setDueDateDialogOpen] = React.useState(false)
  const [priorityDialogOpen, setPriorityDialogOpen] = React.useState(false)
  const overdue = isOverdue(task)

  return (
    <>
      <div
        ref={setNodeRef}
        className={cn(
          "group/task w-full cursor-pointer rounded-md border bg-background p-3 text-left shadow-xs transition-colors hover:bg-muted focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none",
          isDragging && "opacity-40"
        )}
        onClick={onOpen}
        role="listitem"
      >
        <div
          {...attributes}
          {...listeners}
          aria-label={`任务：${task.title}。按 Enter 查看详情，按空格拖动`}
          className={cn(
            "cursor-grab touch-none select-none active:cursor-grabbing",
            draggingDisabled && "cursor-default"
          )}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault()
              onOpen()
              return
            }
            listeners?.onKeyDown?.(event)
          }}
        >
          <div className="line-clamp-2 text-sm leading-snug font-medium transition-colors group-hover/task:text-sky-600">
            {task.title}
          </div>
          {task.description && (
            <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
              {task.description}
            </p>
          )}
        </div>
        <div className="mt-3 flex min-w-0 items-center gap-2">
          <Badge asChild variant="outline">
            <button
              aria-label={`修改任务优先级，当前为${projectTaskPriorityLabels[task.priority]}`}
              className="cursor-pointer font-normal hover:ring-1 hover:ring-ring/50"
              onClick={(event) => {
                event.stopPropagation()
                setPriorityDialogOpen(true)
              }}
              type="button"
            >
              <ProjectTaskPriorityIcon priority={task.priority} />
              {projectTaskPriorityLabels[task.priority]}
            </button>
          </Badge>
          {task.dueDate && (
            <Badge asChild variant={overdue ? "warning" : "outline"}>
              <button
                aria-label={`修改任务截止日期，当前为${task.dueDate}`}
                className={cn(
                  "cursor-pointer hover:ring-1 hover:ring-ring/50",
                  overdue &&
                    "bg-amber-500/10 text-amber-600 dark:bg-amber-500/15 dark:text-amber-400",
                  !overdue && "text-muted-foreground"
                )}
                onClick={(event) => {
                  event.stopPropagation()
                  setDueDateDialogOpen(true)
                }}
                type="button"
              >
                截止 <time dateTime={task.dueDate}>{task.dueDate}</time>
              </button>
            </Badge>
          )}
          {task.assignee && (
            <ProjectTaskAssigneeAvatar
              assignee={task.assignee}
              className="ml-auto"
            />
          )}
        </div>
      </div>
      {dueDateDialogOpen && (
        <UpdateProjectTaskDateDialog
          currentValue={task.dueDate}
          dateType="due"
          onOpenChange={setDueDateDialogOpen}
          onUpdated={onUpdated}
          open
          otherValue={task.startDate}
          projectId={task.projectId}
          taskId={task.id}
        />
      )}
      {priorityDialogOpen && (
        <UpdateProjectTaskPriorityDialog
          currentPriority={task.priority}
          onOpenChange={setPriorityDialogOpen}
          onUpdated={onUpdated}
          open
          projectId={task.projectId}
          taskId={task.id}
        />
      )}
    </>
  )
}

function BoardTaskOverlay({ task }: { task: ProjectTask }) {
  return (
    <div className="w-full rounded-md border bg-background p-3 text-left shadow-lg">
      <div className="line-clamp-2 text-sm leading-snug font-medium">
        {task.title}
      </div>
      {task.description && (
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
          {task.description}
        </p>
      )}
      <div className="mt-3 flex min-w-0 items-center gap-2">
        <Badge className="font-normal" variant="outline">
          <ProjectTaskPriorityIcon priority={task.priority} />
          {projectTaskPriorityLabels[task.priority]}
        </Badge>
        {task.dueDate && (
          <Badge variant={isOverdue(task) ? "warning" : "outline"}>
            截止 {task.dueDate}
          </Badge>
        )}
        {task.assignee && (
          <ProjectTaskAssigneeAvatar
            assignee={task.assignee}
            className="ml-auto"
          />
        )}
      </div>
    </div>
  )
}

function getProjectTaskStatusColor(status: ProjectTaskStatus) {
  switch (status) {
    case "in_progress":
      return "text-sky-600"
    case "done":
      return "text-emerald-600"
    case "canceled":
      return "text-stone-500"
    default:
      return "text-amber-600"
  }
}

function getProjectTaskDraggingColumnColor(status: ProjectTaskStatus) {
  switch (status) {
    case "in_progress":
      return "bg-sky-50 dark:bg-sky-950"
    case "done":
      return "bg-emerald-50 dark:bg-emerald-950"
    case "canceled":
      return "bg-stone-50 dark:bg-stone-950"
    default:
      return "bg-amber-50 dark:bg-amber-950"
  }
}

function getProjectTaskDropTargetColor(status: ProjectTaskStatus) {
  switch (status) {
    case "in_progress":
      return "bg-sky-100 dark:bg-sky-900"
    case "done":
      return "bg-emerald-100 dark:bg-emerald-900"
    case "canceled":
      return "bg-stone-100 dark:bg-stone-900"
    default:
      return "bg-amber-100 dark:bg-amber-900"
  }
}

function getProjectTaskDraggingColumnBorderColor(status: ProjectTaskStatus) {
  switch (status) {
    case "in_progress":
      return "border-sky-200 dark:border-sky-800"
    case "done":
      return "border-emerald-200 dark:border-emerald-800"
    case "canceled":
      return "border-stone-200 dark:border-stone-800"
    default:
      return "border-amber-200 dark:border-amber-800"
  }
}

function getProjectTaskDropTargetBorderColor(status: ProjectTaskStatus) {
  switch (status) {
    case "in_progress":
      return "border-sky-300 dark:border-sky-700"
    case "done":
      return "border-emerald-300 dark:border-emerald-700"
    case "canceled":
      return "border-stone-300 dark:border-stone-700"
    default:
      return "border-amber-300 dark:border-amber-700"
  }
}

function parseProjectTaskStatus(value: unknown): ProjectTaskStatus | null {
  const status = String(value ?? "")
  return boardColumns.includes(status as ProjectTaskStatus)
    ? (status as ProjectTaskStatus)
    : null
}

function isOverdue(task: ProjectTask) {
  return Boolean(
    task.dueDate &&
    task.status !== "done" &&
    task.status !== "canceled" &&
    task.dueDate < getTodayDateKey()
  )
}

function getTodayDateKey() {
  const today = new Date()
  return [
    today.getFullYear(),
    String(today.getMonth() + 1).padStart(2, "0"),
    String(today.getDate()).padStart(2, "0"),
  ].join("-")
}
