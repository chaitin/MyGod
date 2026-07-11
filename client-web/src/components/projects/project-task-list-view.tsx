import { Circle, CircleCheckBig, CircleX, Flag } from "lucide-react"

import type { ProjectTask } from "@/components/projects/project-types"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"

const priorityLabels = {
  high: "高",
  medium: "中",
  low: "低",
} satisfies Record<ProjectTask["priority"], string>

const statusLabels = {
  todo: "待办",
  in_progress: "进行中",
  done: "已完成",
  canceled: "已取消",
} satisfies Record<ProjectTask["status"], string>

export function ProjectTaskListView({ tasks }: { tasks: ProjectTask[] }) {
  return (
    <section className="overflow-hidden rounded-md border bg-background shadow-xs">
      <Table className="min-w-216 table-fixed">
        <TableHeader className="bg-muted/40">
          <TableRow className="hover:bg-muted/40">
            <TableHead className="h-8 w-[38%] px-4 text-xs text-muted-foreground">
              任务
            </TableHead>
            <TableHead className="h-8 w-28 text-xs text-muted-foreground">
              状态
            </TableHead>
            <TableHead className="h-8 w-24 text-xs text-muted-foreground">
              优先级
            </TableHead>
            <TableHead className="h-8 w-32 text-xs text-muted-foreground">
              负责人
            </TableHead>
            <TableHead className="h-8 w-24 text-xs text-muted-foreground">
              开始日期
            </TableHead>
            <TableHead className="h-8 w-24 pr-4 text-xs text-muted-foreground">
              截止日期
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </TableBody>
      </Table>
    </section>
  )
}

function TaskRow({ task }: { task: ProjectTask }) {
  const completed = task.status === "done"
  const canceled = task.status === "canceled"
  const closed = completed || canceled

  return (
    <TableRow className="h-15 hover:bg-muted/35">
      <TableCell className="px-4 py-3">
        <div className="flex min-w-0 items-center gap-3">
          {completed ? (
            <CircleCheckBig
              aria-hidden="true"
              className="size-4 shrink-0 text-emerald-600"
            />
          ) : canceled ? (
            <CircleX
              aria-hidden="true"
              className="size-4 shrink-0 text-muted-foreground"
            />
          ) : (
            <Circle
              aria-hidden="true"
              className={cn(
                "size-4 shrink-0 text-muted-foreground",
                task.status === "in_progress" && "fill-sky-500/20 text-sky-600"
              )}
            />
          )}
          <span
            className={cn(
              "truncate text-sm",
              closed && "text-muted-foreground line-through"
            )}
          >
            {task.title}
          </span>
        </div>
      </TableCell>
      <TableCell>
        <TaskStatus status={task.status} />
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-1.5 text-xs">
          <Flag
            aria-hidden="true"
            className={cn(
              "size-3.5",
              task.priority === "high"
                ? "text-rose-600"
                : task.priority === "medium"
                  ? "text-amber-600"
                  : "text-muted-foreground"
            )}
          />
          {priorityLabels[task.priority]}
        </div>
      </TableCell>
      <TableCell>
        {task.assignee ? (
          <div className="flex items-center gap-2">
            <Avatar className="size-6">
              <AvatarFallback className={task.assignee.tone}>
                {task.assignee.initials}
              </AvatarFallback>
            </Avatar>
            <span className="truncate text-xs">{task.assignee.name}</span>
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">未指派</span>
        )}
      </TableCell>
      <TableCell>
        <TaskDate value={task.startDate} />
      </TableCell>
      <TableCell className="pr-4">
        <TaskDate value={task.dueDate} />
      </TableCell>
    </TableRow>
  )
}

function TaskStatus({ status }: { status: ProjectTask["status"] }) {
  return (
    <span
      className={cn(
        "text-xs whitespace-nowrap",
        status === "in_progress"
          ? "text-sky-700 dark:text-sky-300"
          : status === "done"
            ? "text-emerald-700 dark:text-emerald-300"
            : status === "canceled"
              ? "text-rose-700 dark:text-rose-300"
              : "text-muted-foreground"
      )}
    >
      {statusLabels[status]}
    </span>
  )
}

function TaskDate({ value }: { value: string | null }) {
  if (!value) {
    return <span className="text-xs text-muted-foreground">未设置</span>
  }

  return (
    <time
      className="text-xs whitespace-nowrap text-muted-foreground"
      dateTime={value}
    >
      {value}
    </time>
  )
}
