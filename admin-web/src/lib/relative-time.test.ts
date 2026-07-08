import { describe, expect, it } from "vitest"

import { formatRelativeTimeDistance } from "@/lib/relative-time"

describe("relative time", () => {
  const now = new Date("2026-07-09T12:00:00Z")

  it("formats recent relative time with a space between number and unit", () => {
    expect(formatRelativeTimeDistance("2026-07-09T11:55:00Z", now)).toBe(
      "5 分钟"
    )
    expect(formatRelativeTimeDistance("2026-07-09T00:00:00Z", now)).toBe(
      "12 小时"
    )
  })

  it("keeps formatting days after 30 days", () => {
    expect(formatRelativeTimeDistance("2026-06-01T12:00:00Z", now)).toBe(
      "38 天"
    )
  })

  it("formats years after 365 days", () => {
    expect(formatRelativeTimeDistance("2025-07-09T12:00:00Z", now)).toBe(
      "1 年"
    )
    expect(formatRelativeTimeDistance("2024-07-09T12:00:00Z", now)).toBe(
      "2 年"
    )
  })
})
