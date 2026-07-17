import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { compressImageForMessage } from "@/lib/image-message"

describe("compressImageForMessage", () => {
  const originalImage = globalThis.Image
  const originalCreateElement = document.createElement.bind(document)
  const canvas = {
    getContext: vi.fn(() => ({ drawImage: vi.fn() })),
    height: 0,
    toBlob: vi.fn(),
    toDataURL: vi.fn(),
    width: 0,
  }

  beforeEach(() => {
    canvas.getContext.mockClear()
    canvas.toBlob.mockReset()
    canvas.toDataURL.mockReset()
    Object.defineProperties(URL, {
      createObjectURL: {
        configurable: true,
        value: vi.fn(() => "blob:source-image"),
      },
      revokeObjectURL: {
        configurable: true,
        value: vi.fn(),
      },
    })
    vi.stubGlobal(
      "Image",
      class {
        naturalHeight = 600
        naturalWidth = 800
        onerror: (() => void) | null = null
        onload: (() => void) | null = null

        set src(_value: string) {
          queueMicrotask(() => this.onload?.())
        }
      }
    )
    vi.spyOn(document, "createElement").mockImplementation((tagName) =>
      tagName === "canvas"
        ? (canvas as unknown as HTMLCanvasElement)
        : originalCreateElement(tagName)
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    globalThis.Image = originalImage
  })

  it("keeps a valid WebP canvas result", async () => {
    const webP = new Blob(["RIFF", "0000", "WEBP", "content"], {
      type: "image/webp",
    })
    canvas.toBlob.mockImplementationOnce((callback: BlobCallback) =>
      callback(webP)
    )

    const result = await compressImageForMessage(
      new File(["source"], "photo.jpg", { type: "image/jpeg" })
    )

    expect(result.name).toBe("photo.webp")
    expect(result.type).toBe("image/webp")
  })

  it("uses PNG when Safari falls back from WebP encoding", async () => {
    const png = new Blob(["png"], { type: "image/png" })
    canvas.toBlob.mockImplementationOnce((callback: BlobCallback) =>
      callback(png)
    )

    const result = await compressImageForMessage(
      new File(["source"], "photo.jpg", { type: "image/jpeg" })
    )

    expect(result.name).toBe("photo.png")
    expect(result.type).toBe("image/png")
  })
})
