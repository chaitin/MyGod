import { describe, expect, it } from "vitest"

import { formatActivityTime } from "@/lib/activity-time"

describe("formatActivityTime", () => {
  const now = new Date("2026-07-03T20:00:00")

  it("formats activity times according to validity and local day", () => {
    expect(formatActivityTime("2026-07-03T16:05:00", now)).toBe("16:05")
    expect(formatActivityTime("2026-07-02T16:05:00", now)).toBe("07-02")
    expect(formatActivityTime(null, now)).toBe("")
    expect(formatActivityTime("not-a-date", now)).toBe("")
  })
})
