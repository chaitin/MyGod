import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { copyTemporaryImageToClipboard } from "@/lib/image-clipboard"

const { readTemporaryFileURLsMock } = vi.hoisted(() => ({
  readTemporaryFileURLsMock: vi.fn(),
}))

vi.mock("@/lib/client-data-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/client-data-api")>()

  return {
    ...actual,
    readTemporaryFileURLs: readTemporaryFileURLsMock,
  }
})

class TestClipboardItem {
  readonly data: Record<string, Blob | PromiseLike<Blob>>

  constructor(data: Record<string, Blob | PromiseLike<Blob>>) {
    this.data = data
  }
}

describe("copyTemporaryImageToClipboard", () => {
  const originalClipboard = Object.getOwnPropertyDescriptor(
    navigator,
    "clipboard"
  )
  const originalSecureContext = Object.getOwnPropertyDescriptor(
    window,
    "isSecureContext"
  )

  beforeEach(() => {
    readTemporaryFileURLsMock.mockResolvedValue([
      {
        expiresAt: "2026-07-17T12:00:00Z",
        fileId: "file-1",
        url: "https://example.com/image.png",
      },
    ])
    vi.stubGlobal("ClipboardItem", TestClipboardItem)
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: true,
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    restoreProperty(navigator, "clipboard", originalClipboard)
    restoreProperty(window, "isSecureContext", originalSecureContext)
  })

  it("writes a temporary PNG image to the system clipboard", async () => {
    const imageBlob = new Blob(["image"], { type: "image/png" })
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(imageBlob, {
        headers: { "Content-Type": "image/png" },
        status: 200,
      })
    )
    vi.stubGlobal("fetch", fetchMock)

    let copiedBlob: Blob | null = null
    const write = vi.fn(async (items: TestClipboardItem[]) => {
      copiedBlob = await items[0].data["image/png"]
    })
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { write },
    })

    await copyTemporaryImageToClipboard("file-1")

    expect(readTemporaryFileURLsMock).toHaveBeenCalledWith(["file-1"])
    expect(fetchMock).toHaveBeenCalledWith("https://example.com/image.png")
    expect(write).toHaveBeenCalledTimes(1)
    expect(copiedBlob).toBeInstanceOf(Blob)
    expect(copiedBlob?.type).toBe("image/png")
  })
})

function restoreProperty(
  target: object,
  property: PropertyKey,
  descriptor: PropertyDescriptor | undefined
) {
  if (descriptor) {
    Object.defineProperty(target, property, descriptor)
    return
  }

  Reflect.deleteProperty(target, property)
}
