import { render, screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ExpressionPicker } from "@/components/expression-picker"

describe("ExpressionPicker", () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it("renders eight frequent expressions and an eight-by-eight expression grid", () => {
    render(<ExpressionPicker onSelect={vi.fn()} />)

    const frequentSection = screen.getByRole("region", { name: "常用" })
    const allSection = screen.getByRole("region", { name: "所有表情" })
    const allGrid = within(allSection).getByRole("button", {
      name: "笑脸",
    }).parentElement

    const frequentButtons = within(frequentSection).getAllByRole("button")

    expect(frequentButtons).toHaveLength(8)
    expect(
      frequentButtons.map((button) => button.getAttribute("aria-label"))
    ).toEqual([
      "笑哭",
      "微笑",
      "大哭",
      "赞",
      "爱心",
      "鼓掌",
      "拜托",
      "庆祝礼花",
    ])
    expect(within(allSection).getAllByRole("button")).toHaveLength(64)
    expect(
      within(allSection).getByRole("button", { name: "捂脸" })
    ).toBeVisible()
    expect(
      within(allSection).getByRole("button", { name: "握手" })
    ).toBeVisible()
    expect(
      within(allSection).getByRole("button", { name: "错误" })
    ).toBeVisible()
    expect(
      within(allSection).queryByRole("button", { name: "口罩" })
    ).not.toBeInTheDocument()
    expect(allGrid).toHaveStyle({
      gridTemplateColumns: "repeat(8, minmax(0, 1fr))",
    })
  })
})
