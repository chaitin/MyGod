import { render } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { GlobalBeforeUnloadGuard } from "@/components/global-before-unload-guard"

describe("GlobalBeforeUnloadGuard", () => {
  it("asks the browser to confirm before refreshing or closing the page", () => {
    const { unmount } = render(<GlobalBeforeUnloadGuard />)
    const event = new Event("beforeunload", {
      cancelable: true,
    }) as BeforeUnloadEvent

    Object.defineProperty(event, "returnValue", {
      configurable: true,
      value: undefined,
      writable: true,
    })

    window.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(event.returnValue).toBe("")

    unmount()
  })

  it("removes the browser leave guard after unmount", () => {
    const { unmount } = render(<GlobalBeforeUnloadGuard />)
    unmount()

    const event = new Event("beforeunload", {
      cancelable: true,
    }) as BeforeUnloadEvent

    window.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(false)
  })
})
