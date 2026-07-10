import { fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { SendImageMessageDialog } from "@/components/send-image-message-dialog"

describe("SendImageMessageDialog", () => {
  beforeEach(() => {
    Object.defineProperties(URL, {
      createObjectURL: {
        configurable: true,
        value: vi.fn(() => "blob:image-preview"),
      },
      revokeObjectURL: {
        configurable: true,
        value: vi.fn(),
      },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("registers a non-passive wheel listener on the preview area", async () => {
    const addEventListener = vi.spyOn(HTMLElement.prototype, "addEventListener")

    render(
      <SendImageMessageDialog
        conversationName="测试会话"
        image={new File(["image"], "image.png", { type: "image/png" })}
        onConfirm={vi.fn()}
        onOpenChange={vi.fn()}
        open
        sending={false}
      />
    )

    const previewImage = await screen.findByRole("img", {
      name: "待发送图片预览",
    })
    const previewArea = previewImage.parentElement?.parentElement

    expect(previewArea).not.toBeNull()
    const hasNonPassiveWheelListener = addEventListener.mock.calls.some(
      ([eventName, , options], index) =>
        addEventListener.mock.instances[index] === previewArea &&
        eventName === "wheel" &&
        typeof options === "object" &&
        options?.passive === false
    )

    expect(hasNonPassiveWheelListener).toBe(true)
    expect(fireEvent.wheel(previewArea!, { deltaY: -1 })).toBe(false)
  })
})
