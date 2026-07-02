import { describe, expect, it } from "vitest"

import avatarSourceText from "./avatar.tsx?raw"

describe("avatar styling", () => {
  it("uses the muted color as the transparent image backing", () => {
    expect(avatarSourceText).toMatch(/group\/avatar[^"]*bg-muted/)
  })
})
