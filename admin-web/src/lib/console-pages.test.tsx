import { describe, expect, it } from "vitest"

import {
  consolePages,
  defaultConsolePage,
  getConsolePage,
} from "@/lib/console-pages"

describe("console pages", () => {
  it("includes system settings in the console navigation", () => {
    expect(defaultConsolePage).toBe("/dashboard")
    expect(consolePages.map((page) => page.path)).toContain("/settings")
    expect(consolePages.map((page) => page.label)).toEqual([
      "仪表盘",
      "成员",
      "应用",
      "设置",
    ])
    expect(getConsolePage("/settings").page.title).toBe("系统设置")
  })

  it("does not include the built-in assistant in the console navigation", () => {
    expect(consolePages.map((page) => page.path)).not.toContain("/assistant")
    expect(getConsolePage("/assistant").page.title).toBe("仪表盘")
  })
})
