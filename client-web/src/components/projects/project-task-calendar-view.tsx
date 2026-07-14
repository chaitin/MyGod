import * as React from "react"
import { ChevronDown, ChevronLeft, ChevronRight } from "lucide-react"

import type { ProjectTask } from "@/components/projects/project-types"
import {
  ProjectTaskAssigneeAvatar,
  ProjectTaskStatusIcon,
} from "@/components/projects/project-task-view-elements"
import {
  addCalendarDays,
  formatDateKey,
  getProjectTaskBlockClassName,
  getProjectTaskBlockHoverClassName,
  getProjectTaskDateRange,
} from "@/components/projects/project-task-view-utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"

const weekdayLabels = ["一", "二", "三", "四", "五", "六", "日"]
const calendarTaskStatusOrder = {
  todo: 0,
  in_progress: 1,
  done: 2,
  canceled: 3,
} satisfies Record<ProjectTask["status"], number>

export function ProjectTaskCalendarView({
  emptyMessage = "暂无任务",
  onOpenTask,
  tasks,
}: {
  emptyMessage?: string
  onOpenTask: (task: ProjectTask) => void
  tasks: ProjectTask[]
}) {
  const [monthPickerOpen, setMonthPickerOpen] = React.useState(false)
  const [monthPickerYear, setMonthPickerYear] = React.useState(() =>
    new Date().getFullYear()
  )
  const [visibleMonth, setVisibleMonth] = React.useState(
    () => new Date(new Date().getFullYear(), new Date().getMonth(), 1)
  )

  const calendarDays = getCalendarDays(visibleMonth)
  const tasksByDate = getTasksByDate(tasks, calendarDays)
  const unscheduledTasks = tasks.filter(
    (task) => !getProjectTaskDateRange(task)
  )
  const today = new Date()
  const todayKey = formatDateKey(today)
  const currentMonthStart = new Date(today.getFullYear(), today.getMonth(), 1)

  return (
    <>
      {tasks.length === 0 ? (
        <div className="flex min-h-80 items-center justify-center text-sm text-muted-foreground">
          {emptyMessage}
        </div>
      ) : (
        <div className="grid gap-4">
          {unscheduledTasks.length > 0 && (
            <UnscheduledCalendarTasks
              onOpenTask={onOpenTask}
              tasks={unscheduledTasks}
            />
          )}
          <section className="overflow-hidden rounded-md border bg-background shadow-xs">
            <header className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
              <Popover
                onOpenChange={(open) => {
                  setMonthPickerOpen(open)
                  if (open) {
                    setMonthPickerYear(visibleMonth.getFullYear())
                  }
                }}
                open={monthPickerOpen}
              >
                <PopoverTrigger asChild>
                  <Button type="button" variant="ghost">
                    {visibleMonth.getFullYear()} 年{" "}
                    {visibleMonth.getMonth() + 1} 月
                    <ChevronDown />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="start" className="w-72 p-3">
                  <div className="mb-3 flex items-center justify-between">
                    <Button
                      aria-label="上一年"
                      onClick={() => setMonthPickerYear((year) => year - 1)}
                      size="icon-sm"
                      title="上一年"
                      type="button"
                      variant="ghost"
                    >
                      <ChevronLeft />
                    </Button>
                    <span className="text-sm font-medium tabular-nums">
                      {monthPickerYear} 年
                    </span>
                    <Button
                      aria-label="下一年"
                      onClick={() => setMonthPickerYear((year) => year + 1)}
                      size="icon-sm"
                      title="下一年"
                      type="button"
                      variant="ghost"
                    >
                      <ChevronRight />
                    </Button>
                  </div>
                  <div className="grid grid-cols-3 gap-1">
                    {Array.from({ length: 12 }, (_, month) => {
                      const monthDate = new Date(monthPickerYear, month, 1)
                      const current =
                        monthPickerYear === today.getFullYear() &&
                        month === today.getMonth()
                      const past = monthDate < currentMonthStart
                      const selected =
                        visibleMonth.getFullYear() === monthPickerYear &&
                        visibleMonth.getMonth() === month

                      return (
                        <Button
                          aria-current={selected ? "date" : undefined}
                          className={cn(
                            past && "text-muted-foreground",
                            current &&
                              "bg-foreground text-background hover:bg-foreground/90 hover:text-background"
                          )}
                          key={month}
                          onClick={() => {
                            setVisibleMonth(new Date(monthPickerYear, month, 1))
                            setMonthPickerOpen(false)
                          }}
                          size="sm"
                          type="button"
                          variant={selected ? "secondary" : "ghost"}
                        >
                          {month + 1} 月
                        </Button>
                      )
                    })}
                  </div>
                </PopoverContent>
              </Popover>
              <Button
                onClick={() => {
                  const currentDate = new Date()
                  setVisibleMonth(
                    new Date(
                      currentDate.getFullYear(),
                      currentDate.getMonth(),
                      1
                    )
                  )
                }}
                size="sm"
                type="button"
                variant="outline"
              >
                今天
              </Button>
            </header>
            <div className="overflow-x-auto">
              <div className="min-w-[52rem]">
                <div className="grid grid-cols-7 border-b bg-muted/30">
                  {weekdayLabels.map((label, index) => (
                    <div
                      className={cn(
                        "border-r py-2 text-center text-xs font-medium last:border-r-0",
                        index > 4 && "bg-muted/40"
                      )}
                      key={label}
                    >
                      周{label}
                    </div>
                  ))}
                </div>
                <div className="grid grid-cols-7">
                  {calendarDays.map((date, index) => {
                    const dateKey = formatDateKey(date)
                    const dateTasks = tasksByDate.get(dateKey) ?? []
                    const isCurrentMonth =
                      date.getMonth() === visibleMonth.getMonth()
                    const isPast = dateKey < todayKey
                    const isToday = dateKey === todayKey

                    return (
                      <div
                        className={cn(
                          "min-h-16 border-r border-b p-1.5 last:border-r-0",
                          index % 7 === 6 && "border-r-0",
                          index >= 35 && "border-b-0",
                          !isToday &&
                            (!isCurrentMonth || isPast) &&
                            "text-muted-foreground",
                          isPast && "bg-muted",
                          isToday && "bg-foreground/10"
                        )}
                        key={dateKey}
                      >
                        <div className="mb-2 flex h-6 items-center">
                          <time
                            className={cn(
                              "flex h-6 items-center justify-center rounded-md px-1.5 text-xs tabular-nums",
                              !isCurrentMonth &&
                                !isToday &&
                                "text-muted-foreground"
                            )}
                            dateTime={dateKey}
                          >
                            {date.getMonth() + 1} 月 {date.getDate()} 日
                            {isToday && " - 今天"}
                          </time>
                        </div>
                        <div className="grid gap-1">
                          {dateTasks.map((task) => (
                            <CalendarTask
                              key={task.id}
                              onOpen={() => onOpenTask(task)}
                              task={task}
                            />
                          ))}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          </section>
        </div>
      )}
    </>
  )
}

function UnscheduledCalendarTasks({
  onOpenTask,
  tasks,
}: {
  onOpenTask: (task: ProjectTask) => void
  tasks: ProjectTask[]
}) {
  return (
    <Collapsible
      className="overflow-hidden rounded-md border bg-background shadow-xs"
      defaultOpen
    >
      <CollapsibleTrigger asChild>
        <Button
          className="group/collapsible-trigger h-10 w-full justify-start rounded-none px-3 text-muted-foreground"
          type="button"
          variant="ghost"
        >
          <ChevronRight className="transition-transform group-data-[state=open]/collapsible-trigger:rotate-90" />
          未设置日期
          <Badge
            className="ml-auto min-w-5 bg-background px-1.5 tabular-nums"
            variant="secondary"
          >
            {tasks.length}
          </Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t p-4">
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {tasks.map((task) => (
            <button
              className="group/task flex min-w-0 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset"
              key={task.id}
              onClick={() => onOpenTask(task)}
              type="button"
            >
              <ProjectTaskStatusIcon
                className={cn(
                  "shrink-0",
                  getProjectTaskStatusColor(task.status)
                )}
                status={task.status}
              />
              <span className="min-w-0 flex-1 truncate transition-colors group-hover/task:text-sky-600">
                {task.title}
              </span>
              {task.assignee && (
                <ProjectTaskAssigneeAvatar
                  assignee={task.assignee}
                  className="size-4 bg-muted"
                />
              )}
            </button>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function CalendarTask({
  onOpen,
  task,
}: {
  onOpen: () => void
  task: ProjectTask
}) {
  return (
    <button
      aria-label={`查看任务详情：${task.title}`}
      className={cn(
        "flex h-8 w-full cursor-pointer items-center gap-1.5 overflow-hidden rounded-sm px-1.5 text-left text-xs transition-colors focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none",
        getProjectTaskBlockClassName(task.status),
        getProjectTaskBlockHoverClassName(task.status)
      )}
      onClick={onOpen}
      title={task.title}
      type="button"
    >
      {task.assignee && (
        <ProjectTaskAssigneeAvatar
          assignee={task.assignee}
          className="size-4 bg-muted"
        />
      )}
      <span className="min-w-0 truncate">{task.title}</span>
    </button>
  )
}

function getProjectTaskStatusColor(status: ProjectTask["status"]) {
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

function getCalendarDays(month: Date) {
  const firstDay = new Date(month.getFullYear(), month.getMonth(), 1)
  const mondayBasedOffset = (firstDay.getDay() + 6) % 7
  const calendarStart = addCalendarDays(firstDay, -mondayBasedOffset)
  return Array.from({ length: 42 }, (_, index) =>
    addCalendarDays(calendarStart, index)
  )
}

function getTasksByDate(tasks: ProjectTask[], calendarDays: Date[]) {
  const result = new Map<string, ProjectTask[]>()
  const calendarStart = calendarDays[0]
  const calendarEnd = calendarDays[calendarDays.length - 1]

  for (const task of tasks) {
    const range = getProjectTaskDateRange(task)
    if (!range || range.end < calendarStart || range.start > calendarEnd) {
      continue
    }
    let date = range.start < calendarStart ? calendarStart : range.start
    const end = range.end > calendarEnd ? calendarEnd : range.end
    while (date <= end) {
      const dateKey = formatDateKey(date)
      result.set(dateKey, [...(result.get(dateKey) ?? []), task])
      date = addCalendarDays(date, 1)
    }
  }

  for (const dateTasks of result.values()) {
    dateTasks.sort((left, right) => {
      if (left.status !== right.status) {
        return (
          calendarTaskStatusOrder[left.status] -
          calendarTaskStatusOrder[right.status]
        )
      }
      return right.priority - left.priority
    })
  }
  return result
}
