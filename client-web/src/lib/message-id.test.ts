import { describe, expect, it, vi } from "vitest"

import { createClientMessageId } from "@/lib/message-id"

const uuidV4Pattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

describe("createClientMessageId", () => {
  it("falls back to getRandomValues when randomUUID is unavailable", () => {
    const getRandomValues = vi.fn((bytes: Uint8Array) => {
      bytes.set([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x06, 0x77, 0x08, 0x99, 0xaa, 0xbb,
        0xcc, 0xdd, 0xee, 0xff,
      ])

      return bytes
    })

    expect(createClientMessageId({ getRandomValues })).toBe(
      "00112233-4455-4677-8899-aabbccddeeff"
    )
    expect(getRandomValues).toHaveBeenCalledOnce()
  })

  it("returns a UUID-shaped id without Web Crypto support", () => {
    expect(createClientMessageId({})).toMatch(uuidV4Pattern)
  })
})
