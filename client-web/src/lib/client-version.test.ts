import { afterEach, describe, expect, it, vi } from "vitest"

import {
  clientVersionReminderSnoozeMs,
  clientVersionRequestTimeoutMs,
  fetchLatestClientBuildCommit,
  isClientVersionReminderSnoozed,
  snoozeClientVersionReminder,
} from "@/lib/client-version"

describe("client version", () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it("loads the latest commit without using an HTTP cache", async () => {
    const fetcher = async () =>
      new Response(JSON.stringify({ commit: "new-commit" }), { status: 200 })

    await expect(fetchLatestClientBuildCommit(fetcher)).resolves.toBe(
      "new-commit"
    )
  })

  it("rejects an invalid version manifest", async () => {
    const fetcher = async () =>
      new Response(JSON.stringify({ commit: "" }), { status: 200 })

    await expect(fetchLatestClientBuildCommit(fetcher)).rejects.toThrow(
      "客户端版本信息格式不正确"
    )
  })

  it("aborts a version request that exceeds the timeout", async () => {
    vi.useFakeTimers()
    const fetcher = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"))
          })
        })
    )

    const result = fetchLatestClientBuildCommit(fetcher).catch(
      (error: unknown) => error
    )
    await vi.advanceTimersByTimeAsync(clientVersionRequestTimeoutMs)

    await expect(result).resolves.toMatchObject({ name: "AbortError" })
    expect(fetcher).toHaveBeenCalledOnce()
  })

  it("snoozes only the selected commit for one hour", () => {
    const storage = createStorage()
    const now = new Date("2026-07-23T00:00:00Z").getTime()

    snoozeClientVersionReminder("new-commit", now, storage)

    expect(isClientVersionReminderSnoozed("new-commit", now, storage)).toBe(
      true
    )
    expect(isClientVersionReminderSnoozed("other-commit", now, storage)).toBe(
      false
    )
    expect(
      isClientVersionReminderSnoozed(
        "new-commit",
        now + clientVersionReminderSnoozeMs,
        storage
      )
    ).toBe(false)
  })
})

function createStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  }
}
