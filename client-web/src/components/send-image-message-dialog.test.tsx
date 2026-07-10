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
    const addEventListener = vi.spyOn(
      HTMLElement.prototype,
      "addEventListener"
    )

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

  it("keeps the dialog fixed at 60vw by 60vh", async () => {
    renderDialog()

    const dialog = await screen.findByRole("dialog")

    expect(dialog).toHaveClass("h-[60vh]", "w-[60vw]", "max-w-[60vw]")
  })

  it("drags a zoomed image with the left mouse button", async () => {
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(300)
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(400)
    renderDialog()

    const previewImage = await screen.findByRole("img", {
      name: "待发送图片预览",
    })
    const previewArea = previewImage.parentElement?.parentElement as HTMLDivElement
    const setPointerCapture = vi.fn()

    Object.defineProperties(previewImage, {
      naturalHeight: { configurable: true, value: 300 },
      naturalWidth: { configurable: true, value: 400 },
    })
    previewArea.setPointerCapture = setPointerCapture

    fireEvent.load(previewImage)
    fireEvent.wheel(previewArea, { deltaY: -1 })
    fireEvent.pointerDown(previewArea, {
      button: 0,
      clientX: 100,
      clientY: 100,
      pointerId: 1,
    })
    fireEvent.pointerMove(previewArea, {
      clientX: 120,
      clientY: 115,
      pointerId: 1,
    })

    expect(setPointerCapture).toHaveBeenCalledWith(1)
    expect(previewImage.style.transform).toContain("translate(20px, 15px)")
  })
})

function renderDialog() {
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
}
