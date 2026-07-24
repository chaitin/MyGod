import { describe, expect, it } from "vitest"

import {
  fileMessageMaxBytes,
  getFileMessageUploadError,
} from "@/lib/file-message"

describe("getFileMessageUploadError", () => {
  it("rejects empty files", () => {
    expect(getFileMessageUploadError({ size: 0 })).toBe("文件不能为空")
  })

  it("accepts files at the upload limit", () => {
    expect(getFileMessageUploadError({ size: fileMessageMaxBytes })).toBeNull()
  })

  it("rejects files above the upload limit", () => {
    expect(getFileMessageUploadError({ size: fileMessageMaxBytes + 1 })).toBe(
      "文件不能超过 200MiB"
    )
  })
})
