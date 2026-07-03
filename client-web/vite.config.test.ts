import { describe, expect, it } from "vitest"

import config from "./vite.config"

describe("vite dev proxy", () => {
  it("preserves the original host when proxying client websocket requests", () => {
    const proxy = config.server?.proxy
    if (!proxy || typeof proxy === "string" || Array.isArray(proxy)) {
      throw new Error("expected object proxy config")
    }

    const clientProxy = proxy["/api/client"]
    if (!clientProxy || typeof clientProxy === "string") {
      throw new Error("expected /api/client proxy options")
    }

    expect(clientProxy.ws).toBe(true)
    expect(clientProxy.changeOrigin).toBe(false)
  })
})
