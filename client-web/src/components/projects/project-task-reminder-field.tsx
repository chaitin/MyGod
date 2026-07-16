import * as React from "react"
import { Bell, CalendarClock, Repeat2, X } from "lucide-react"

import {
  PROJECT_TASK_REMINDER_TIMEZONE,
  type ProjectTaskReminderInput,
  type ProjectTaskReminderState,
  type ProjectTaskStatus,
} from "@/components/projects/project-types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

const weekdays = [
  { label: "一", value: 1 },
  { label: "二", value: 2 },
  { label: "三", value: 3 },
  { label: "四", value: 4 },
  { label: "五", value: 5 },
  { label: "六", value: 6 },
  { label: "日", value: 7 },
] as const

export function ProjectTaskReminderField({
  disabled = false,
  onValueChange,
  state,
  status,
  value,
}: {
  disabled?: boolean
  onValueChange: (value: ProjectTaskReminderInput | null) => void
  state?: ProjectTaskReminderState
  status: ProjectTaskStatus
  value: ProjectTaskReminderInput | null
}) {
  const [open, setOpen] = React.useState(false)
  const [draft, setDraft] = React.useState<ProjectTaskReminderInput | null>(null)
  const paused = status === "done" || status === "canceled"

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraft(cloneReminderInput(value))
    }
    setOpen(nextOpen)
  }

  return (
    <Popover onOpenChange={handleOpenChange} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-label="提醒时间"
          className={cn(
            "w-full min-w-0 justify-start px-2.5 font-normal",
            !value && "text-muted-foreground"
          )}
          disabled={disabled}
          type="button"
          variant="outline"
        >
          <Bell />
          <span className="min-w-0 truncate">
            {formatReminderSummary(value, paused, state)}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-sm font-medium">提醒时间</p>
            <p className="text-xs text-muted-foreground">
              到时向当前负责人发送任务卡片
            </p>
          </div>
          {draft && (
            <Button
              aria-label="清除提醒"
              onClick={() => setDraft(null)}
              size="icon-xs"
              title="清除提醒"
              type="button"
              variant="ghost"
            >
              <X />
            </Button>
          )}
        </div>

        {!draft ? (
          <div className="grid grid-cols-2 gap-2">
            <Button
              className="h-auto gap-1.5 whitespace-nowrap py-3"
              onClick={() => setDraft(createDefaultOnceReminder())}
              type="button"
              variant="outline"
            >
              <CalendarClock />
              一次性
            </Button>
            <Button
              className="h-auto gap-1.5 whitespace-nowrap py-3"
              onClick={() => setDraft(createDefaultRecurringReminder())}
              type="button"
              variant="outline"
            >
              <Repeat2 />
              重复
            </Button>
          </div>
        ) : (
          <>
            <ToggleGroup
              className="w-full"
              onValueChange={(mode) => {
                if (mode === "once") {
                  setDraft(createDefaultOnceReminder())
                } else if (mode === "recurring") {
                  setDraft(createDefaultRecurringReminder())
                }
              }}
              type="single"
              value={draft.mode}
              variant="outline"
            >
              <ToggleGroupItem className="flex-1" value="once">
                一次性
              </ToggleGroupItem>
              <ToggleGroupItem className="flex-1" value="recurring">
                重复
              </ToggleGroupItem>
            </ToggleGroup>

            {draft.mode === "once" ? (
              <div className="grid gap-2">
                <Label htmlFor="task-reminder-once-at">日期和时间</Label>
                <Input
                  id="task-reminder-once-at"
                  min={minimumLocalDateTime()}
                  onChange={(event) => {
                    const at = shanghaiDateTimeToISO(event.target.value)
                    if (at) {
                      setDraft({ ...draft, at })
                    }
                  }}
                  step={60}
                  type="datetime-local"
                  value={isoToShanghaiDateTime(draft.at)}
                />
              </div>
            ) : (
              <RecurringReminderFields
                onValueChange={setDraft}
                value={draft}
              />
            )}

            <div className="text-xs text-muted-foreground">
              时区：{PROJECT_TASK_REMINDER_TIMEZONE}
              {paused && " · 任务完成或取消期间暂停提醒"}
            </div>
          </>
        )}

        <div className="flex justify-end gap-2 border-t pt-3">
          <Button onClick={() => setOpen(false)} type="button" variant="ghost">
            取消
          </Button>
          <Button
            onClick={() => {
              onValueChange(cloneReminderInput(draft))
              setOpen(false)
            }}
            type="button"
          >
            确定
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function RecurringReminderFields({
  onValueChange,
  value,
}: {
  onValueChange: (value: ProjectTaskReminderInput) => void
  value: Extract<ProjectTaskReminderInput, { mode: "recurring" }>
}) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label>重复周期</Label>
        <Select
          onValueChange={(frequency) =>
            onValueChange(changeFrequency(value, frequency))
          }
          value={value.frequency}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="daily">每天</SelectItem>
            <SelectItem value="weekly">每周</SelectItem>
            <SelectItem value="monthly">每月</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {value.frequency === "weekly" && (
        <div className="grid gap-2">
          <Label>星期</Label>
          <ToggleGroup
            className="w-full"
            onValueChange={(selected) => {
              if (selected.length > 0) {
                onValueChange({
                  ...value,
                  weekdays: selected.map(Number).sort((a, b) => a - b),
                })
              }
            }}
            spacing={1}
            type="multiple"
            value={(value.weekdays ?? []).map(String)}
            variant="outline"
          >
            {weekdays.map((weekday) => (
              <ToggleGroupItem
                aria-label={`星期${weekday.label}`}
                className="min-w-0 flex-1 px-0"
                key={weekday.value}
                value={String(weekday.value)}
              >
                {weekday.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>
      )}

      {value.frequency === "monthly" && (
        <div className="grid gap-2">
          <Label>每月日期</Label>
          <Select
            onValueChange={(day) =>
              onValueChange({ ...value, dayOfMonth: Number(day) })
            }
            value={String(value.dayOfMonth ?? 1)}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="max-h-64">
              {Array.from({ length: 31 }, (_, index) => index + 1).map(
                (day) => (
                  <SelectItem key={day} value={String(day)}>
                    {day} 日
                  </SelectItem>
                )
              )}
            </SelectContent>
          </Select>
        </div>
      )}

      <div className="grid gap-2">
        <Label htmlFor="task-reminder-time">提醒时间</Label>
        <Input
          id="task-reminder-time"
          onChange={(event) => {
            if (event.target.value) {
              onValueChange({ ...value, time: event.target.value })
            }
          }}
          step={60}
          type="time"
          value={value.time}
        />
      </div>
    </div>
  )
}

function changeFrequency(
  value: Extract<ProjectTaskReminderInput, { mode: "recurring" }>,
  frequency: string
): Extract<ProjectTaskReminderInput, { mode: "recurring" }> {
  const base = {
    mode: "recurring" as const,
    time: value.time,
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
  }
  if (frequency === "weekly") {
    return { ...base, frequency, weekdays: [currentISOWeekday()] }
  }
  if (frequency === "monthly") {
    return {
      ...base,
      dayOfMonth: shanghaiDate(new Date()).getUTCDate(),
      frequency,
    }
  }
  return { ...base, frequency: "daily" }
}

function createDefaultOnceReminder(): ProjectTaskReminderInput {
  const at = new Date(Date.now() + 60 * 60 * 1000)
  at.setSeconds(0, 0)
  return {
    at: at.toISOString(),
    mode: "once",
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
  }
}

function createDefaultRecurringReminder(): ProjectTaskReminderInput {
  const at = shanghaiDate(new Date(Date.now() + 60 * 60 * 1000))
  return {
    frequency: "daily",
    mode: "recurring",
    time: `${String(at.getUTCHours()).padStart(2, "0")}:${String(at.getUTCMinutes()).padStart(2, "0")}`,
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
  }
}

function currentISOWeekday() {
  const weekday = shanghaiDate(new Date()).getUTCDay()
  return weekday === 0 ? 7 : weekday
}

function formatReminderSummary(
  reminder: ProjectTaskReminderInput | null,
  paused: boolean,
  state?: ProjectTaskReminderState
) {
  if (!reminder) {
    return "不提醒"
  }
  let summary: string
  if (reminder.mode === "once") {
    const at = new Date(reminder.at)
    summary = Number.isNaN(at.getTime())
      ? "一次性提醒"
      : new Intl.DateTimeFormat("zh-CN", {
          day: "numeric",
          hour: "2-digit",
          hour12: false,
          minute: "2-digit",
          month: "numeric",
          timeZone: PROJECT_TASK_REMINDER_TIMEZONE,
          year: "numeric",
        }).format(at)
  } else if (reminder.frequency === "daily") {
    summary = `每天 ${reminder.time}`
  } else if (reminder.frequency === "weekly") {
    const labels = (reminder.weekdays ?? [])
      .map(
        (value) => weekdays.find((weekday) => weekday.value === value)?.label
      )
      .filter(Boolean)
      .join("、")
    summary = `每周${labels} ${reminder.time}`
  } else {
    summary = `每月 ${reminder.dayOfMonth} 日 ${reminder.time}`
  }
  if (paused || state === "paused") {
    return `已暂停 · ${summary}`
  }
  if (state === "fired") {
    return `已提醒 · ${summary}`
  }
  if (state === "expired") {
    return `已过期 · ${summary}`
  }
  return summary
}

function isoToShanghaiDateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ""
  }
  return shanghaiDate(date).toISOString().slice(0, 16)
}

function shanghaiDateTimeToISO(value: string) {
  if (!value) {
    return null
  }
  const date = new Date(`${value}:00+08:00`)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

function minimumLocalDateTime() {
  const date = new Date(Date.now() + 60_000)
  date.setSeconds(0, 0)
  return shanghaiDate(date).toISOString().slice(0, 16)
}

function shanghaiDate(date: Date) {
  return new Date(date.getTime() + 8 * 60 * 60 * 1000)
}

function cloneReminderInput(
  value: ProjectTaskReminderInput | null
): ProjectTaskReminderInput | null {
  if (!value) {
    return null
  }
  if (value.mode === "once") {
    return {
      at: value.at,
      mode: "once",
      timezone: PROJECT_TASK_REMINDER_TIMEZONE,
    }
  }
  return {
    ...value,
    timezone: PROJECT_TASK_REMINDER_TIMEZONE,
    weekdays: value.weekdays ? [...value.weekdays] : undefined,
  }
}
