import { describe, expect, it } from "vitest"

import { formatConversationMessageTime } from "@/lib/conversation-message-presenter"

describe("formatConversationMessageTime", () => {
  const now = new Date("2026-07-16T20:00:00")

  it("shows only the time for messages from today", () => {
    expect(formatConversationMessageTime("2026-07-16T09:05:00", now)).toBe(
      "09:05"
    )
  })

  it("adds month and day for historical messages from this year", () => {
    expect(formatConversationMessageTime("2026-01-02T09:05:00", now)).toBe(
      "01/02 09:05"
    )
  })

  it("adds the year for historical messages from another year", () => {
    expect(formatConversationMessageTime("2025-12-31T23:59:00", now)).toBe(
      "2025/12/31 23:59"
    )
  })

  it("returns an empty string for invalid timestamps", () => {
    expect(formatConversationMessageTime("not-a-date", now)).toBe("")
  })
})
