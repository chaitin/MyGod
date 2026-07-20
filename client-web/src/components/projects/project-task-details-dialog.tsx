import * as React from "react"
import {
  ChevronsDown,
  ChevronsUp,
  Circle,
  CircleCheckBig,
  CircleDot,
  CircleX,
  Equal,
  Eye,
  Pencil,
  Send,
} from "lucide-react"
import { toast } from "sonner"

import { ProjectMemberCombobox } from "@/components/projects/project-member-combobox"
import { ProjectTaskDatePicker } from "@/components/projects/project-task-date-picker"
import { ProjectTaskLabelsCombobox } from "@/components/projects/project-task-labels-combobox"
import { ProjectTaskReminderField } from "@/components/projects/project-task-reminder-field"
import { SendCardDialog } from "@/components/conversation/send-card-dialog"
import { MessageMarkdown } from "@/components/message-markdown"
import type {
  ProjectTask,
  ProjectTaskPriority,
  ProjectTaskReminderInput,
  ProjectTaskStatus,
} from "@/components/projects/project-types"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { ClientProjectMember } from "@/lib/project-data-api"
import { listAllClientProjectMembers } from "@/lib/project-members"
import {
  getClientProjectTask,
  listClientProjectTasks,
  type UpdateClientProjectTaskInput,
  updateClientProjectTask,
} from "@/lib/project-task-data-api"

type TaskEditForm = {
  assigneeUserId: string
  description: string
  dueDate: string
  labels: string[]
  priority: ProjectTaskPriority
  reminder: ProjectTaskReminderInput | null
  startDate: string
  status: ProjectTaskStatus
  title: string
}

type NormalizedTaskEditForm = {
  assigneeUserId: string | null
  description: string
  dueDate: string | null
  labels: string[]
  priority: ProjectTaskPriority
  reminder: ProjectTaskReminderInput | null
  startDate: string | null
  status: ProjectTaskStatus
  title: string
}

export function ProjectTaskDetailsDialog({
  onOpenChange,
  onUpdated,
  open,
  task,
}: {
  onOpenChange: (open: boolean) => void
  onUpdated?: () => Promise<void>
  open: boolean
  task: ProjectTask
}) {
  const initialForm = createTaskEditForm(task)
  const [baseline, setBaseline] = React.useState<NormalizedTaskEditForm>(() =>
    normalizeTaskEditForm(initialForm)
  )
  const [details, setDetails] = React.useState(task)
  const [descriptionEditing, setDescriptionEditing] = React.useState(false)
  const [error, setError] = React.useState("")
  const [form, setForm] = React.useState<TaskEditForm>(initialForm)
  const [loading, setLoading] = React.useState(true)
  const [labelOptions, setLabelOptions] = React.useState<string[]>([])
  const [labelsError, setLabelsError] = React.useState("")
  const [labelsLoading, setLabelsLoading] = React.useState(true)
  const [members, setMembers] = React.useState<ClientProjectMember[]>([])
  const [membersError, setMembersError] = React.useState("")
  const [membersLoading, setMembersLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)
  const [sendDialogOpen, setSendDialogOpen] = React.useState(false)
  const assigneeComboboxPortal = React.useRef<HTMLDivElement | null>(null)

  React.useEffect(() => {
    if (!open) {
      return
    }

    let active = true
    void getClientProjectTask(task.projectId, task.id)
      .then((nextDetails) => {
        if (!active) {
          return
        }
        const loadedForm = createTaskEditForm(nextDetails)
        setBaseline(normalizeTaskEditForm(loadedForm))
        setDetails(nextDetails)
        setDescriptionEditing(false)
        setForm(loadedForm)
      })
      .catch((loadError: unknown) => {
        if (active) {
          setError(
            loadError instanceof Error ? loadError.message : "加载任务详情失败"
          )
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false)
        }
      })

    void listAllClientProjectMembers(task.projectId)
      .then((nextMembers) => {
        if (active) {
          setMembers(nextMembers.filter((member) => member.status === "active"))
        }
      })
      .catch((loadError: unknown) => {
        if (active) {
          setMembersError(
            loadError instanceof Error ? loadError.message : "加载项目成员失败"
          )
        }
      })
      .finally(() => {
        if (active) {
          setMembersLoading(false)
        }
      })

    void listAllProjectTaskLabels(task.projectId, task.id)
      .then((nextLabels) => {
        if (active) {
          setLabelOptions(nextLabels)
        }
      })
      .catch((loadError: unknown) => {
        if (active) {
          setLabelsError(
            loadError instanceof Error ? loadError.message : "加载候选标签失败"
          )
        }
      })
      .finally(() => {
        if (active) {
          setLabelsLoading(false)
        }
      })

    return () => {
      active = false
    }
  }, [open, task])

  const normalizedForm = normalizeTaskEditForm(form)
  const validationError = getTaskEditValidationError(normalizedForm)
  const dirty = !taskEditFormsEqual(normalizedForm, baseline)
  const canSave = dirty && !loading && !saving && !validationError
  const fallbackAssignee = createFallbackProjectMember(details)
  const memberOptions =
    fallbackAssignee &&
    !members.some((member) => member.id === fallbackAssignee.id)
      ? [fallbackAssignee, ...members]
      : members
  const selectedAssignee = memberOptions.find(
    (member) => member.id === form.assigneeUserId
  )
  const card = {
    entityId: details.id,
    entityType: "task",
    type: "entity_card",
  } as const

  function updateForm<K extends keyof TaskEditForm>(
    field: K,
    value: TaskEditForm[K]
  ) {
    setForm((current) => ({ ...current, [field]: value }))
  }

  function handleOpenChange(nextOpen: boolean) {
    if (saving) {
      return
    }
    if (!nextOpen) {
      const resetForm = createTaskEditForm(details)
      setBaseline(normalizeTaskEditForm(resetForm))
      setDescriptionEditing(false)
      setError("")
      setForm(resetForm)
      setLoading(true)
      setLabelOptions([])
      setLabelsError("")
      setLabelsLoading(true)
      setMembers([])
      setMembersError("")
      setMembersLoading(true)
      setSendDialogOpen(false)
    }
    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSave) {
      return
    }

    setSaving(true)
    try {
      const updatedTask = await updateClientProjectTask(
        task.projectId,
        task.id,
        createTaskEditPatch(normalizedForm, baseline)
      )
      const updatedForm = createTaskEditForm(updatedTask)
      setBaseline(normalizeTaskEditForm(updatedForm))
      setDetails(updatedTask)
      setDescriptionEditing(false)
      setError("")
      setForm(updatedForm)
      toast.success("任务已保存")
      onOpenChange(false)
      await onUpdated?.()
    } catch (saveError) {
      toast.error(
        saveError instanceof Error ? saveError.message : "保存任务失败"
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent
        className="max-h-[85vh] gap-5 overflow-y-auto sm:max-w-5xl"
        onPointerDownOutside={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            任务详情
            {loading && <Spinner />}
          </DialogTitle>
          <DialogDescription className="sr-only">
            查看并修改任务详情。
          </DialogDescription>
        </DialogHeader>

        <form className="grid gap-5" onSubmit={handleSubmit}>
          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(18rem,1fr)] lg:items-start">
            <div className="grid min-w-0 content-start gap-5">
              <TaskField htmlFor="task-details-title" label="标题">
                <Input
                  autoFocus
                  disabled={loading || saving}
                  id="task-details-title"
                  maxLength={240}
                  onChange={(event) => updateForm("title", event.target.value)}
                  value={form.title}
                />
              </TaskField>

              <TaskField label="标签">
                <ProjectTaskLabelsCombobox
                  disabled={loading || saving}
                  loading={labelsLoading}
                  onValueChange={(labels) => updateForm("labels", labels)}
                  options={labelOptions}
                  portalContainer={assigneeComboboxPortal}
                  value={form.labels}
                />
                {labelsError && (
                  <p className="text-xs text-destructive">{labelsError}</p>
                )}
              </TaskField>

              <TaskField
                action={
                  <ToggleGroup
                    aria-label="详细内容显示模式"
                    className="shrink-0"
                    disabled={loading || saving}
                    onValueChange={(value) => {
                      if (value) {
                        setDescriptionEditing(value === "source")
                      }
                    }}
                    spacing={0}
                    type="single"
                    value={descriptionEditing ? "source" : "preview"}
                    variant="outline"
                  >
                    <ToggleGroupItem
                      aria-label="显示渲染结果"
                      className="h-6 min-w-0 px-2 data-[state=off]:text-muted-foreground"
                      title="预览"
                      value="preview"
                    >
                      <Eye className="size-3.5" />
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      aria-label="显示 Markdown 原文"
                      className="h-6 min-w-0 px-2 data-[state=off]:text-muted-foreground"
                      title="编辑原文"
                      value="source"
                    >
                      <Pencil className="size-3.5" />
                    </ToggleGroupItem>
                  </ToggleGroup>
                }
                htmlFor={
                  descriptionEditing ? "task-details-description" : undefined
                }
                label="详细内容"
              >
                {descriptionEditing ? (
                  <Textarea
                    autoFocus
                    className="field-sizing-fixed h-100 max-h-100 min-h-100 resize-none font-mono!"
                    disabled={loading || saving}
                    id="task-details-description"
                    onChange={(event) =>
                      updateForm("description", event.target.value)
                    }
                    placeholder="支持 Markdown"
                    value={form.description}
                  />
                ) : (
                  <div
                    className="h-100 overflow-hidden rounded-md border border-input bg-transparent text-sm shadow-xs dark:bg-input/30"
                    data-slot="task-description-preview"
                  >
                    <div className="h-full overflow-auto px-2.5 py-2 contain-content">
                      {form.description.trim() ? (
                        <MessageMarkdown content={form.description} />
                      ) : (
                        <span className="text-muted-foreground">
                          暂无详细内容
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </TaskField>
            </div>

            <div className="grid min-w-0 content-start gap-5">
              <div className="grid gap-4">
                <TaskField label="状态">
                  <Select
                    disabled={loading || saving}
                    onValueChange={(value) =>
                      updateForm("status", value as ProjectTaskStatus)
                    }
                    value={form.status}
                  >
                    <SelectTrigger aria-label="任务状态" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="todo">
                        <Circle className="text-amber-600" />
                        待办
                      </SelectItem>
                      <SelectItem value="in_progress">
                        <CircleDot className="text-sky-600" />
                        进行中
                      </SelectItem>
                      <SelectItem value="done">
                        <CircleCheckBig className="text-emerald-600" />
                        已完成
                      </SelectItem>
                      <SelectItem value="canceled">
                        <CircleX className="text-stone-500" />
                        已取消
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </TaskField>

                <TaskField label="创建人">
                  <DisabledUserInput user={details.creator} />
                </TaskField>

                <TaskField label="优先级">
                  <Select
                    disabled={loading || saving}
                    onValueChange={(value) =>
                      updateForm(
                        "priority",
                        Number(value) as ProjectTaskPriority
                      )
                    }
                    value={String(form.priority)}
                  >
                    <SelectTrigger aria-label="任务优先级" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="3">
                        <ChevronsUp className="text-rose-600" />高
                      </SelectItem>
                      <SelectItem value="2">
                        <Equal className="text-amber-600" />中
                      </SelectItem>
                      <SelectItem value="1">
                        <ChevronsDown className="text-muted-foreground" />低
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </TaskField>
              </div>

              <div className="grid gap-4">
                <TaskField label="负责人">
                  <ProjectMemberCombobox
                    disabled={loading || saving || membersLoading}
                    loading={membersLoading}
                    members={memberOptions}
                    onValueChange={(member: ClientProjectMember | null) =>
                      updateForm("assigneeUserId", member?.id ?? "")
                    }
                    portalContainer={assigneeComboboxPortal}
                    showEmptyEmail={false}
                    value={selectedAssignee ?? null}
                  />
                  {membersError && (
                    <p className="text-xs text-destructive">{membersError}</p>
                  )}
                </TaskField>
              </div>

              <div className="grid gap-4">
                <TaskField label="开始日期">
                  <ProjectTaskDatePicker
                    disabled={loading || saving}
                    label="开始日期"
                    maximum={form.dueDate || undefined}
                    onValueChange={(value) => updateForm("startDate", value)}
                    value={form.startDate}
                  />
                </TaskField>
                <TaskField label="截止日期">
                  <ProjectTaskDatePicker
                    disabled={loading || saving}
                    label="截止日期"
                    minimum={form.startDate || undefined}
                    onValueChange={(value) => updateForm("dueDate", value)}
                    value={form.dueDate}
                  />
                </TaskField>
                <TaskField label="提醒时间">
                  <ProjectTaskReminderField
                    disabled={loading || saving}
                    onValueChange={(value) => updateForm("reminder", value)}
                    state={
                      details.status === form.status &&
                      reminderInputsEqual(
                        form.reminder,
                        toReminderInput(details.reminder)
                      )
                        ? details.reminder?.state
                        : undefined
                    }
                    status={form.status}
                    value={form.reminder}
                  />
                </TaskField>
              </div>

              {(validationError || error) && (
                <p className="text-xs text-destructive">
                  {validationError || error}
                </p>
              )}
            </div>
          </div>

          <DialogFooter className="sm:justify-between">
            <Button
              disabled={loading || saving || dirty || Boolean(error)}
              onClick={() => setSendDialogOpen(true)}
              title={
                dirty
                  ? "请先保存修改后再发送"
                  : error
                    ? "任务详情加载失败，暂不能发送"
                    : undefined
              }
              type="button"
              variant="outline"
            >
              <Send />
              发送到对话
            </Button>
            <div className="flex justify-end gap-2">
              <Button
                disabled={saving}
                onClick={() => handleOpenChange(false)}
                type="button"
                variant="outline"
              >
                关闭
              </Button>
              <Button disabled={!canSave} type="submit">
                {saving && <Spinner />}
                保存
              </Button>
            </div>
          </DialogFooter>
        </form>
        <div
          className="absolute top-0 left-0 size-0"
          ref={assigneeComboboxPortal}
        />
      </DialogContent>
      <SendCardDialog
        card={card}
        onOpenChange={setSendDialogOpen}
        open={sendDialogOpen}
      />
    </Dialog>
  )
}

function TaskField({
  action,
  children,
  htmlFor,
  label,
}: {
  action?: React.ReactNode
  children: React.ReactNode
  htmlFor?: string
  label: string
}) {
  return (
    <div className="grid content-start gap-2">
      {action ? (
        <div className="flex items-center justify-between gap-2">
          <Label htmlFor={htmlFor}>{label}</Label>
          {action}
        </div>
      ) : (
        <Label htmlFor={htmlFor}>{label}</Label>
      )}
      {children}
    </div>
  )
}

function DisabledUserInput({ user }: { user: ProjectTask["creator"] }) {
  const displayName = user.nickname || user.name
  const initial = Array.from(displayName.trim())[0]?.toUpperCase() ?? "?"

  return (
    <InputGroup>
      <InputGroupAddon align="inline-start">
        <Avatar className="size-5 rounded-sm after:rounded-sm">
          {user.avatar && (
            <AvatarImage
              alt={displayName}
              className="rounded-sm"
              src={user.avatar}
            />
          )}
          <AvatarFallback className="rounded-sm text-[10px]">
            {initial}
          </AvatarFallback>
        </Avatar>
      </InputGroupAddon>
      <InputGroupInput aria-label="创建人" disabled value={displayName} />
    </InputGroup>
  )
}

function createTaskEditForm(task: ProjectTask): TaskEditForm {
  return {
    assigneeUserId: task.assignee?.id ?? "",
    description: task.description,
    dueDate: task.dueDate ?? "",
    labels: [...task.labels],
    priority: task.priority,
    reminder: toReminderInput(task.reminder),
    startDate: task.startDate ?? "",
    status: task.status,
    title: task.title,
  }
}

function createFallbackProjectMember(
  task: ProjectTask
): ClientProjectMember | null {
  if (!task.assignee) {
    return null
  }
  return {
    avatar: task.assignee.avatar,
    displayName: task.assignee.nickname || task.assignee.name,
    email: "",
    id: task.assignee.id,
    name: task.assignee.name,
    nickname: task.assignee.nickname,
    role: "member",
    sourceGroupIds: [],
    status: "active",
  }
}

function normalizeTaskEditForm(form: TaskEditForm): NormalizedTaskEditForm {
  return {
    assigneeUserId: form.assigneeUserId || null,
    description: form.description,
    dueDate: form.dueDate || null,
    labels: normalizeLabels(form.labels),
    priority: form.priority,
    reminder: normalizeReminderInput(form.reminder),
    startDate: form.startDate || null,
    status: form.status,
    title: form.title.trim(),
  }
}

function normalizeLabels(values: string[]) {
  const labels: string[] = []
  const seen = new Set<string>()
  for (const value of values) {
    const label = value.trim()
    const key = label.toLocaleLowerCase()
    if (label && !seen.has(key)) {
      seen.add(key)
      labels.push(label)
    }
  }
  return labels
}

function getTaskEditValidationError(form: NormalizedTaskEditForm) {
  const titleLength = Array.from(form.title).length
  if (titleLength < 1 || titleLength > 240) {
    return "标题长度必须为 1 到 240 个字符"
  }
  if (form.startDate && form.dueDate && form.startDate > form.dueDate) {
    return "开始日期不能晚于截止日期"
  }
  if (form.labels.length > 20) {
    return "标签不能超过 20 个"
  }
  if (form.labels.some((label) => Array.from(label).length > 32)) {
    return "每个标签不能超过 32 个字符"
  }
  return ""
}

function taskEditFormsEqual(
  left: NormalizedTaskEditForm,
  right: NormalizedTaskEditForm
) {
  return (
    left.assigneeUserId === right.assigneeUserId &&
    left.description === right.description &&
    left.dueDate === right.dueDate &&
    left.priority === right.priority &&
    reminderInputsEqual(left.reminder, right.reminder) &&
    left.startDate === right.startDate &&
    left.status === right.status &&
    left.title === right.title &&
    left.labels.length === right.labels.length &&
    left.labels.every((label, index) => label === right.labels[index])
  )
}

function createTaskEditPatch(
  form: NormalizedTaskEditForm,
  baseline: NormalizedTaskEditForm
): UpdateClientProjectTaskInput {
  const patch: UpdateClientProjectTaskInput = {}
  if (form.assigneeUserId !== baseline.assigneeUserId) {
    patch.assigneeUserId = form.assigneeUserId
  }
  if (form.description !== baseline.description) {
    patch.description = form.description
  }
  if (form.dueDate !== baseline.dueDate) {
    patch.dueDate = form.dueDate
  }
  if (
    form.labels.length !== baseline.labels.length ||
    form.labels.some((label, index) => label !== baseline.labels[index])
  ) {
    patch.labels = form.labels
  }
  if (form.priority !== baseline.priority) {
    patch.priority = form.priority
  }
  if (!reminderInputsEqual(form.reminder, baseline.reminder)) {
    patch.reminder = form.reminder
  }
  if (form.startDate !== baseline.startDate) {
    patch.startDate = form.startDate
  }
  if (form.status !== baseline.status) {
    patch.status = form.status
  }
  if (form.title !== baseline.title) {
    patch.title = form.title
  }
  return patch
}

function toReminderInput(
  reminder: ProjectTask["reminder"] | undefined
): ProjectTaskReminderInput | null {
  if (!reminder) {
    return null
  }
  if (reminder.mode === "once") {
    return {
      at: reminder.at,
      mode: "once",
      timezone: reminder.timezone,
    }
  }
  return normalizeReminderInput(reminder)
}

function normalizeReminderInput(
  reminder: ProjectTaskReminderInput | null
): ProjectTaskReminderInput | null {
  if (!reminder) {
    return null
  }
  if (reminder.mode === "once") {
    return { at: reminder.at, mode: "once", timezone: reminder.timezone }
  }
  if (reminder.frequency === "weekly") {
    return {
      frequency: "weekly",
      mode: "recurring",
      time: reminder.time,
      timezone: reminder.timezone,
      weekdays: [...(reminder.weekdays ?? [])].sort((a, b) => a - b),
    }
  }
  if (reminder.frequency === "monthly") {
    return {
      dayOfMonth: reminder.dayOfMonth,
      frequency: "monthly",
      mode: "recurring",
      time: reminder.time,
      timezone: reminder.timezone,
    }
  }
  return {
    frequency: "daily",
    mode: "recurring",
    time: reminder.time,
    timezone: reminder.timezone,
  }
}

function reminderInputsEqual(
  left: ProjectTaskReminderInput | null,
  right: ProjectTaskReminderInput | null
) {
  return (
    JSON.stringify(normalizeReminderInput(left)) ===
    JSON.stringify(normalizeReminderInput(right))
  )
}

async function listAllProjectTaskLabels(
  projectId: string,
  excludedTaskId: string
) {
  const labels = new Map<string, string>()
  const seenCursors = new Set<string>()
  let cursor: string | undefined

  do {
    const page = await listClientProjectTasks(projectId, {
      cursor,
      limit: 100,
    })
    for (const projectTask of page.tasks) {
      if (projectTask.id === excludedTaskId) {
        continue
      }
      for (const label of projectTask.labels) {
        const key = label.toLocaleLowerCase()
        if (!labels.has(key)) {
          labels.set(key, label)
        }
      }
    }
    if (!page.nextCursor || seenCursors.has(page.nextCursor)) {
      break
    }
    seenCursors.add(page.nextCursor)
    cursor = page.nextCursor
  } while (cursor)

  return Array.from(labels.values()).sort((left, right) =>
    left.localeCompare(right, "zh-CN")
  )
}
