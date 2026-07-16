export type ProjectTaskStatus = "todo" | "in_progress" | "done" | "canceled"

export type ProjectTaskPriority = 1 | 2 | 3

export const PROJECT_TASK_REMINDER_TIMEZONE = "Asia/Shanghai"

export type ProjectTaskReminderState =
  "scheduled" | "paused" | "fired" | "expired"

export type ProjectTaskOnceReminderInput = {
  at: string
  mode: "once"
  timezone: string
}

export type ProjectTaskRecurringReminderInput = {
  dayOfMonth?: number
  frequency: "daily" | "weekly" | "monthly"
  mode: "recurring"
  time: string
  timezone: string
  weekdays?: number[]
}

export type ProjectTaskReminderInput =
  ProjectTaskOnceReminderInput | ProjectTaskRecurringReminderInput

export type ProjectTaskReminder = ProjectTaskReminderInput & {
  lastProcessedAt: string | null
  nextTriggerAt: string | null
  state: ProjectTaskReminderState
}

export type ProjectTaskUser = {
  avatar: string
  id: string
  name: string
  nickname: string
}

export type ProjectTask = {
  assignee: ProjectTaskUser | null
  canceledAt: string | null
  completedAt: string | null
  createdAt: string
  creator: ProjectTaskUser
  description: string
  dueDate: string | null
  id: string
  labels: string[]
  priority: ProjectTaskPriority
  reminder: ProjectTaskReminder | null
  projectId: string
  startDate: string | null
  status: ProjectTaskStatus
  title: string
  updatedAt: string
}
