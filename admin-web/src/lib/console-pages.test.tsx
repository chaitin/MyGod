import { describe, expect, it } from "vitest"

import { consolePages, getConsolePage } from "@/lib/console-pages"

describe("console pages", () => {
  it("includes system settings in the console navigation", () => {
    expect(consolePages.map((page) => page.path)).toContain("/settings")
    expect(getConsolePage("/settings").page.title).toBe("系统设置")
  })

  it("does not include the built-in assistant in the console navigation", () => {
    expect(consolePages.map((page) => page.path)).not.toContain("/assistant")
    expect(getConsolePage("/assistant").page.title).toBe("成员管理")
  })
})
