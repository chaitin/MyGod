import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ProjectTaskReminderField } from "@/components/projects/project-task-reminder-field"

describe("ProjectTaskReminderField", () => {
  it("keeps reminder edits in a draft until confirmed", async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(
      <ProjectTaskReminderField
        onValueChange={onValueChange}
        status="todo"
        value={null}
      />
    )

    await user.click(screen.getByRole("button", { name: "提醒时间" }))
    await user.click(screen.getByRole("button", { name: "重复" }))
    expect(onValueChange).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "取消" }))
    expect(onValueChange).not.toHaveBeenCalled()
    expect(screen.getByRole("button", { name: "提醒时间" })).toHaveTextContent(
      "不提醒"
    )
  })

  it("edits one-time reminders as Asia/Shanghai wall time", async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(
      <ProjectTaskReminderField
        onValueChange={onValueChange}
        status="todo"
        value={{
          at: "2026-07-15T01:30:00.000Z",
          mode: "once",
          timezone: "Asia/Shanghai",
        }}
      />
    )

    await user.click(screen.getByRole("button", { name: "提醒时间" }))
    const input = screen.getByLabelText("日期和时间")
    expect(input).toHaveValue("2026-07-15T09:30")

    fireEvent.change(input, { target: { value: "2026-07-16T10:45" } })
    expect(onValueChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "确定" }))

    expect(onValueChange).toHaveBeenCalledWith({
      at: "2026-07-16T02:45:00.000Z",
      mode: "once",
      timezone: "Asia/Shanghai",
    })
  })
})
