import { fireEvent, render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { MessageImage } from "@/components/message-image"

const { readTemporaryFileURLsMock } = vi.hoisted(() => ({
  readTemporaryFileURLsMock: vi.fn(),
}))

vi.mock("@/lib/client-data-api", async (importOriginal) => {
  const actual = await importOriginal<
    typeof import("@/lib/client-data-api")
  >()

  return {
    ...actual,
    readTemporaryFileURLs: readTemporaryFileURLsMock,
  }
})

describe("MessageImage", () => {
  beforeEach(() => {
    readTemporaryFileURLsMock.mockResolvedValue([
      {
        expiresAt: "2026-07-10T21:00:00Z",
        fileId: "file-1",
        url: "https://example.com/image.png",
      },
    ])
  })

  it("allows the native preview context menu without bubbling to message actions", async () => {
    const onContextMenu = vi.fn()

    render(
      <div onContextMenu={onContextMenu}>
        <MessageImage image={{ fileId: "file-1", type: "image" }} />
      </div>
    )

    fireEvent.click(await screen.findByRole("button", { name: "预览图片" }))
    const previewImage = await screen.findByRole("img", {
      name: "图片消息预览",
    })

    const contextMenuAllowed = fireEvent.contextMenu(previewImage)

    expect(contextMenuAllowed).toBe(true)
    expect(onContextMenu).not.toHaveBeenCalled()
  })

  it("consumes the preview wheel gesture while zooming", async () => {
    render(<MessageImage image={{ fileId: "file-1", type: "image" }} />)

    fireEvent.click(await screen.findByRole("button", { name: "预览图片" }))
    const previewImage = await screen.findByRole("img", {
      name: "图片消息预览",
    })
    const previewArea = previewImage.parentElement as HTMLDivElement

    const wheelNotCanceled = fireEvent.wheel(previewArea, { deltaY: -1 })

    expect(wheelNotCanceled).toBe(false)
    expect(previewArea).toHaveClass("cursor-grab")
  })
})
