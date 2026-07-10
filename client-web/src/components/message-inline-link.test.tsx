import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { MessageTextWithLinks } from "@/components/message-inline-link"

describe("MessageTextWithLinks", () => {
  it("renders detected URLs as safe external links", () => {
    render(
      <MessageTextWithLinks text="打开 https://example.com/path?q=1#detail 查看" />
    )

    const link = screen.getByRole("link", {
      name: "https://example.com/path?q=1#detail",
    })

    expect(link).toHaveAttribute("href", "https://example.com/path?q=1#detail")
    expect(link).toHaveAttribute("target", "_blank")
    expect(link).toHaveAttribute("rel", "noreferrer")
  })
})
