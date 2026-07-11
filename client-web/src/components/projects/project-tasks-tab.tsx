import * as React from "react"
import {
  CalendarDays,
  ChartNoAxesGantt,
  ChevronDown,
  Columns3,
  ListTodo,
  Search,
} from "lucide-react"

import { ProjectTaskBoardView } from "@/components/projects/project-task-board-view"
import { ProjectTaskCalendarView } from "@/components/projects/project-task-calendar-view"
import { ProjectTaskGanttView } from "@/components/projects/project-task-gantt-view"
import { ProjectTaskListView } from "@/components/projects/project-task-list-view"
import type { ProjectTask } from "@/components/projects/project-types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

const taskViews = [
  { value: "list", label: "任务列表", icon: ListTodo },
  { value: "board", label: "看板", icon: Columns3 },
  { value: "calendar", label: "日历", icon: CalendarDays },
  { value: "gantt", label: "甘特图", icon: ChartNoAxesGantt },
] as const

type TaskView = (typeof taskViews)[number]["value"]

export function ProjectTasksTab({ tasks }: { tasks: ProjectTask[] }) {
  const [activeView, setActiveView] = React.useState<TaskView>("list")

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-muted/10">
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex min-w-0 flex-col gap-4 p-4">
          <TaskToolbar activeView={activeView} onViewChange={setActiveView} />
          <TaskViewContent activeView={activeView} tasks={tasks} />
        </div>
      </ScrollArea>
    </div>
  )
}

function TaskToolbar({
  activeView,
  onViewChange,
}: {
  activeView: TaskView
  onViewChange: (view: TaskView) => void
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-3">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <FilterButton label="状态" />
        <FilterButton label="优先级" />
        <FilterButton label="负责人" />
        <div className="relative min-w-52 sm:min-w-64">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="搜索任务内容"
            className="pl-8"
            placeholder="搜索任务内容"
            type="search"
          />
        </div>
        <Button type="button" variant="outline">
          搜索
        </Button>
      </div>
      <TaskViewSwitcher value={activeView} onValueChange={onViewChange} />
    </div>
  )
}

function FilterButton({ label }: { label: string }) {
  return (
    <Button type="button" variant="outline">
      {label}
      <ChevronDown data-icon="inline-end" />
    </Button>
  )
}

function TaskViewSwitcher({
  onValueChange,
  value,
}: {
  onValueChange: (view: TaskView) => void
  value: TaskView
}) {
  return (
    <div
      aria-label="任务视图"
      className="flex shrink-0 items-center rounded-md border bg-background p-0.5"
      role="group"
    >
      {taskViews.map((view) => {
        const Icon = view.icon
        const active = value === view.value

        return (
          <Button
            aria-label={view.label}
            aria-pressed={active}
            className={cn(
              "size-7 rounded-sm text-muted-foreground",
              active && "bg-muted text-foreground shadow-xs"
            )}
            key={view.value}
            onClick={() => onValueChange(view.value)}
            size="icon-xs"
            title={view.label}
            type="button"
            variant="ghost"
          >
            <Icon />
          </Button>
        )
      })}
    </div>
  )
}

function TaskViewContent({
  activeView,
  tasks,
}: {
  activeView: TaskView
  tasks: ProjectTask[]
}) {
  switch (activeView) {
    case "board":
      return <ProjectTaskBoardView />
    case "calendar":
      return <ProjectTaskCalendarView />
    case "gantt":
      return <ProjectTaskGanttView />
    default:
      return <ProjectTaskListView tasks={tasks} />
  }
}
