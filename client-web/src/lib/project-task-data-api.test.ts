import { describe, expect, it, vi } from "vitest"

import {
  getClientProjectTask,
  updateClientProjectTask,
} from "@/lib/project-task-data-api"

describe("project task reminder data API", () => {
  it("normalizes a weekly reminder", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      jsonResponse({
        success: true,
        data: taskResponse({
          mode: "recurring",
          frequency: "weekly",
          timezone: "Asia/Singapore",
          time: "09:30",
          weekdays: [1, 3, 5],
          next_trigger_at: "2026-07-17T01:30:00Z",
          last_processed_at: null,
          state: "scheduled",
        }),
      })
    )

    const task = await getClientProjectTask("project-1", "task-1", fetcher)
    expect(task.reminder).toEqual({
      frequency: "weekly",
      lastProcessedAt: null,
      mode: "recurring",
      nextTriggerAt: "2026-07-17T01:30:00Z",
      state: "scheduled",
      time: "09:30",
      timezone: "Asia/Shanghai",
      weekdays: [1, 3, 5],
    })
  })

  it("serializes a monthly reminder update", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      jsonResponse({
        success: true,
        data: taskResponse({
          mode: "recurring",
          frequency: "monthly",
          timezone: "Asia/Singapore",
          time: "18:20",
          day_of_month: 31,
          next_trigger_at: "2026-07-31T10:20:00Z",
          last_processed_at: null,
          state: "scheduled",
        }),
      })
    )

    await updateClientProjectTask(
      "project-1",
      "task-1",
      {
        reminder: {
          dayOfMonth: 31,
          frequency: "monthly",
          mode: "recurring",
          time: "18:20",
          timezone: "Asia/Singapore",
        },
      },
      fetcher
    )

    const request = fetcher.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(request.body))).toEqual({
      reminder: {
        day_of_month: 31,
        frequency: "monthly",
        mode: "recurring",
        time: "18:20",
        timezone: "Asia/Shanghai",
      },
    })
  })
})

function taskResponse(reminder: Record<string, unknown>) {
  return {
    assignee: null,
    canceled_at: null,
    completed_at: null,
    created_at: "2026-07-15T00:00:00Z",
    creator: { avatar: "", id: "user-1", name: "Alice", nickname: "" },
    description: "",
    due_date: null,
    id: "task-1",
    labels: [],
    priority: 2,
    project_id: "project-1",
    reminder,
    start_date: null,
    status: "todo",
    title: "Task",
    updated_at: "2026-07-15T00:00:00Z",
  }
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status: 200,
  })
}
