import {
  PROJECT_TASK_REMINDER_TIMEZONE,
  type ProjectTask,
  type ProjectTaskPriority,
  type ProjectTaskReminder,
  type ProjectTaskReminderInput,
  type ProjectTaskReminderState,
  type ProjectTaskStatus,
  type ProjectTaskUser,
} from "@/components/projects/project-types"
import { ClientDataRequestError } from "@/lib/client-data-api"

type ProjectTaskDataFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type ProjectTaskDataSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type ProjectTaskDataErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type ProjectTaskUserResponse = {
  avatar?: string
  id?: string
  name?: string
  nickname?: string
}

type ProjectTaskResponse = {
  assignee?: ProjectTaskUserResponse | null
  canceled_at?: string | null
  completed_at?: string | null
  created_at?: string
  creator?: ProjectTaskUserResponse
  description?: string
  due_date?: string | null
  id?: string
  labels?: string[]
  priority?: number
  reminder?: ProjectTaskReminderResponse | null
  project_id?: string
  start_date?: string | null
  status?: string
  title?: string
  updated_at?: string
}

type ProjectTaskReminderResponse = {
  at?: string | null
  day_of_month?: number | null
  frequency?: string
  last_processed_at?: string | null
  mode?: string
  next_trigger_at?: string | null
  state?: string
  time?: string
  timezone?: string
  weekdays?: number[]
}

type ProjectTaskListResponse = {
  next_cursor?: string | null
  tasks?: ProjectTaskResponse[]
}

export type ListClientProjectTasksOptions = {
  assigneeUserIds?: string[]
  cursor?: string
  keyword?: string
  limit?: number
  priorities?: ProjectTaskPriority[]
  statuses?: ProjectTaskStatus[]
}

export type CreateClientProjectTaskInput = {
  assigneeUserId?: string | null
  description?: string
  dueDate?: string | null
  labels?: string[]
  priority?: ProjectTaskPriority
  reminder?: ProjectTaskReminderInput | null
  startDate?: string | null
  status?: ProjectTaskStatus
  title: string
}

export type UpdateClientProjectTaskInput = {
  assigneeUserId?: string | null
  description?: string
  dueDate?: string | null
  labels?: string[]
  priority?: ProjectTaskPriority
  reminder?: ProjectTaskReminderInput | null
  startDate?: string | null
  status?: ProjectTaskStatus
  title?: string
}

export type ClientProjectTaskPage = {
  nextCursor: string | null
  tasks: ProjectTask[]
}

export async function listClientProjectTasks(
  projectId: string,
  options: ListClientProjectTasksOptions = {},
  fetcher: ProjectTaskDataFetch = fetch
): Promise<ClientProjectTaskPage> {
  const query = new URLSearchParams()
  if (options.assigneeUserIds?.length) {
    query.set("assignee_user_id", options.assigneeUserIds.join(","))
  }
  if (options.keyword) {
    query.set("keyword", options.keyword)
  }
  if (options.priorities?.length) {
    query.set("priority", options.priorities.join(","))
  }
  if (options.statuses?.length) {
    query.set("status", options.statuses.join(","))
  }
  if (options.limit !== undefined) {
    query.set("limit", String(options.limit))
  }
  if (options.cursor) {
    query.set("cursor", options.cursor)
  }
  const suffix = query.size > 0 ? `?${query.toString()}` : ""
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/tasks${suffix}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    | ProjectTaskDataErrorEnvelope
    | ProjectTaskDataSuccessEnvelope<ProjectTaskListResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectTaskRequestError(payload, response, "加载任务列表失败")
  }

  const data = (
    payload as
      ProjectTaskDataSuccessEnvelope<ProjectTaskListResponse> | undefined
  )?.data
  if (!Array.isArray(data?.tasks)) {
    throw new ClientDataRequestError("任务列表响应格式不正确")
  }

  return {
    nextCursor: normalizeNextCursor(data.next_cursor),
    tasks: data.tasks.map(normalizeProjectTask),
  }
}

export async function getClientProjectTask(
  projectId: string,
  taskId: string,
  fetcher: ProjectTaskDataFetch = fetch
): Promise<ProjectTask> {
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    | ProjectTaskDataErrorEnvelope
    | ProjectTaskDataSuccessEnvelope<ProjectTaskResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectTaskRequestError(payload, response, "加载任务详情失败")
  }

  return normalizeProjectTask(
    (payload as ProjectTaskDataSuccessEnvelope<ProjectTaskResponse> | undefined)
      ?.data
  )
}

export async function createClientProjectTask(
  projectId: string,
  input: CreateClientProjectTaskInput,
  fetcher: ProjectTaskDataFetch = fetch
): Promise<ProjectTask> {
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/tasks`,
    {
      body: JSON.stringify({
        assignee_user_id: input.assigneeUserId ?? null,
        description: input.description ?? "",
        due_date: input.dueDate ?? null,
        labels: input.labels ?? [],
        priority: input.priority ?? 2,
        reminder: serializeProjectTaskReminder(input.reminder ?? null),
        start_date: input.startDate ?? null,
        status: input.status ?? "todo",
        title: input.title,
      }),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "POST",
    }
  )
  const payload = await readJson<
    | ProjectTaskDataErrorEnvelope
    | ProjectTaskDataSuccessEnvelope<ProjectTaskResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectTaskRequestError(payload, response, "创建任务失败")
  }

  return normalizeProjectTask(
    (payload as ProjectTaskDataSuccessEnvelope<ProjectTaskResponse> | undefined)
      ?.data
  )
}

export async function updateClientProjectTask(
  projectId: string,
  taskId: string,
  input: UpdateClientProjectTaskInput,
  fetcher: ProjectTaskDataFetch = fetch
): Promise<ProjectTask> {
  const body: Record<string, unknown> = {}
  if (input.assigneeUserId !== undefined) {
    body.assignee_user_id = input.assigneeUserId
  }
  if (input.description !== undefined) {
    body.description = input.description
  }
  if (input.dueDate !== undefined) {
    body.due_date = input.dueDate
  }
  if (input.labels !== undefined) {
    body.labels = input.labels
  }
  if (input.priority !== undefined) {
    body.priority = input.priority
  }
  if (input.reminder !== undefined) {
    body.reminder = serializeProjectTaskReminder(input.reminder)
  }
  if (input.startDate !== undefined) {
    body.start_date = input.startDate
  }
  if (input.status !== undefined) {
    body.status = input.status
  }
  if (input.title !== undefined) {
    body.title = input.title
  }

  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}`,
    {
      body: JSON.stringify(body),
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      method: "PATCH",
    }
  )
  const payload = await readJson<
    | ProjectTaskDataErrorEnvelope
    | ProjectTaskDataSuccessEnvelope<ProjectTaskResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectTaskRequestError(payload, response, "更新任务失败")
  }

  return normalizeProjectTask(
    (payload as ProjectTaskDataSuccessEnvelope<ProjectTaskResponse> | undefined)
      ?.data
  )
}

function normalizeProjectTask(
  task: ProjectTaskResponse | undefined
): ProjectTask {
  if (
    !task ||
    typeof task.id !== "string" ||
    typeof task.project_id !== "string" ||
    typeof task.title !== "string" ||
    typeof task.description !== "string" ||
    !isProjectTaskStatus(task.status) ||
    !isProjectTaskPriority(task.priority) ||
    !task.creator ||
    !Array.isArray(task.labels) ||
    typeof task.created_at !== "string" ||
    typeof task.updated_at !== "string"
  ) {
    throw new ClientDataRequestError("任务响应格式不正确")
  }

  return {
    assignee: task.assignee ? normalizeProjectTaskUser(task.assignee) : null,
    canceledAt: normalizeNullableString(task.canceled_at),
    completedAt: normalizeNullableString(task.completed_at),
    createdAt: task.created_at,
    creator: normalizeProjectTaskUser(task.creator),
    description: task.description,
    dueDate: normalizeNullableString(task.due_date),
    id: task.id,
    labels: task.labels.filter(
      (label): label is string => typeof label === "string"
    ),
    priority: task.priority,
    reminder: normalizeProjectTaskReminder(task.reminder),
    projectId: task.project_id,
    startDate: normalizeNullableString(task.start_date),
    status: task.status,
    title: task.title,
    updatedAt: task.updated_at,
  }
}

function normalizeProjectTaskReminder(
  value: ProjectTaskReminderResponse | null | undefined
): ProjectTaskReminder | null {
  if (value === null || value === undefined) {
    return null
  }
  if (
    (value.mode !== "once" && value.mode !== "recurring") ||
    typeof value.timezone !== "string" ||
    !isProjectTaskReminderState(value.state)
  ) {
    throw new ClientDataRequestError("任务提醒响应格式不正确")
  }
  const runtime = {
    lastProcessedAt: normalizeNullableString(value.last_processed_at),
    nextTriggerAt: normalizeNullableString(value.next_trigger_at),
    state: value.state,
  }
  if (value.mode === "once") {
    if (typeof value.at !== "string") {
      throw new ClientDataRequestError("任务提醒响应格式不正确")
    }
    return {
      ...runtime,
      at: value.at,
      mode: "once",
      timezone: PROJECT_TASK_REMINDER_TIMEZONE,
    }
  }
  if (
    (value.frequency !== "daily" &&
      value.frequency !== "weekly" &&
      value.frequency !== "monthly") ||
    typeof value.time !== "string"
  ) {
    throw new ClientDataRequestError("任务提醒响应格式不正确")
  }
  const reminder: ProjectTaskReminder = {
    ...runtime,
    frequency: value.frequency,
    mode: "recurring",
    time: value.time,
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
  }
  if (value.frequency === "weekly") {
    if (!Array.isArray(value.weekdays)) {
      throw new ClientDataRequestError("任务提醒响应格式不正确")
    }
    reminder.weekdays = value.weekdays.filter(
      (weekday): weekday is number =>
        Number.isInteger(weekday) && weekday >= 1 && weekday <= 7
    )
  }
  if (value.frequency === "monthly") {
    if (
      typeof value.day_of_month !== "number" ||
      !Number.isInteger(value.day_of_month)
    ) {
      throw new ClientDataRequestError("任务提醒响应格式不正确")
    }
    reminder.dayOfMonth = value.day_of_month
  }
  return reminder
}

function serializeProjectTaskReminder(
  value: ProjectTaskReminderInput | null
): Record<string, unknown> | null {
  if (value === null) {
    return null
  }
  if (value.mode === "once") {
    return {
      at: value.at,
      mode: value.mode,
      timezone: PROJECT_TASK_REMINDER_TIMEZONE,
    }
  }
  return {
    day_of_month: value.frequency === "monthly" ? value.dayOfMonth : undefined,
    frequency: value.frequency,
    mode: value.mode,
    time: value.time,
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
    weekdays: value.frequency === "weekly" ? value.weekdays : undefined,
  }
}

function isProjectTaskReminderState(
  value: unknown
): value is ProjectTaskReminderState {
  return (
    value === "scheduled" ||
    value === "paused" ||
    value === "fired" ||
    value === "expired"
  )
}

function normalizeProjectTaskUser(
  user: ProjectTaskUserResponse
): ProjectTaskUser {
  if (typeof user.id !== "string" || typeof user.name !== "string") {
    throw new ClientDataRequestError("任务用户响应格式不正确")
  }
  return {
    avatar: user.avatar ?? "",
    id: user.id,
    name: user.name,
    nickname: user.nickname ?? "",
  }
}

function isProjectTaskStatus(value: unknown): value is ProjectTaskStatus {
  return (
    value === "todo" ||
    value === "in_progress" ||
    value === "done" ||
    value === "canceled"
  )
}

function isProjectTaskPriority(value: unknown): value is ProjectTaskPriority {
  return value === 1 || value === 2 || value === 3
}

function normalizeNullableString(value: unknown) {
  if (value === null || value === undefined) {
    return null
  }
  if (typeof value !== "string") {
    throw new ClientDataRequestError("任务响应格式不正确")
  }
  return value
}

function normalizeNextCursor(value: unknown) {
  if (value === null || value === undefined) {
    return null
  }
  if (typeof value !== "string") {
    throw new ClientDataRequestError("任务分页游标响应格式不正确")
  }
  return value
}

function createProjectTaskRequestError(
  payload:
    | ProjectTaskDataErrorEnvelope
    | ProjectTaskDataSuccessEnvelope<unknown>
    | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as ProjectTaskDataErrorEnvelope | undefined)?.error
  return new ClientDataRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    { code: error?.code, status: response.status }
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  if (!response.headers.get("content-type")?.includes("application/json")) {
    return undefined
  }
  return response.json() as Promise<T>
}
