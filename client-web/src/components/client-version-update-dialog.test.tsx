import { act, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ClientVersionUpdateDialog } from "@/components/client-version-update-dialog"
import { clientVersionCheckIntervalMs } from "@/lib/client-version"

describe("ClientVersionUpdateDialog", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-07-23T00:00:00Z"))
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("checks every 300 seconds and opens when the commit changes", async () => {
    const fetchLatestCommit = vi.fn().mockResolvedValue("new-commit")

    render(
      <ClientVersionUpdateDialog
        currentCommit="old-commit"
        fetchLatestCommit={fetchLatestCommit}
      />
    )

    expect(fetchLatestCommit).not.toHaveBeenCalled()
    await advanceToNextCheck()

    expect(fetchLatestCommit).toHaveBeenCalledOnce()
    expect(screen.getByRole("alertdialog")).toBeVisible()
    expect(screen.getByText("发现新版本")).toBeVisible()
  })

  it("does not open when the commit is unchanged", async () => {
    const fetchLatestCommit = vi.fn().mockResolvedValue("same-commit")

    render(
      <ClientVersionUpdateDialog
        currentCommit="same-commit"
        fetchLatestCommit={fetchLatestCommit}
      />
    )
    await advanceToNextCheck()

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument()
  })

  it("reloads the page after confirming the update", async () => {
    const reload = vi.fn()
    render(
      <ClientVersionUpdateDialog
        currentCommit="old-commit"
        fetchLatestCommit={vi.fn().mockResolvedValue("new-commit")}
        reload={reload}
      />
    )
    await advanceToNextCheck()

    fireEvent.click(screen.getByRole("button", { name: "确认更新" }))

    expect(reload).toHaveBeenCalledOnce()
  })

  it("suppresses the selected version for one hour", async () => {
    const fetchLatestCommit = vi.fn().mockResolvedValue("new-commit")
    render(
      <ClientVersionUpdateDialog
        currentCommit="old-commit"
        fetchLatestCommit={fetchLatestCommit}
      />
    )
    await advanceToNextCheck()

    fireEvent.click(screen.getByRole("button", { name: "一小时内不再提醒" }))
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(55 * 60 * 1000)
    })
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument()

    await advanceToNextCheck()
    expect(screen.getByRole("alertdialog")).toBeVisible()
  })
})

async function advanceToNextCheck() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(clientVersionCheckIntervalMs)
  })
}
