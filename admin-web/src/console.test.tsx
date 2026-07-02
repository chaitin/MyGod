import { describe, expect, it } from "vitest"

import consoleSourceText from "./console.tsx?raw"

describe("console header", () => {
  it("centers the sidebar separator with the breadcrumb text", () => {
    expect(consoleSourceText).toContain("data-vertical:self-center")
  })
})
