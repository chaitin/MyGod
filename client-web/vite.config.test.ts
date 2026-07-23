import { describe, expect, it } from "vitest"

import config, { createClientVersionManifest } from "./vite.config"

describe("vite dev proxy", () => {
  it("allows the maosite.cc development host", () => {
    expect(config.server?.allowedHosts).toContain("maosite.cc")
  })

  it("rewrites websocket origin while preserving the client API host", () => {
    const proxy = config.server?.proxy
    if (!proxy || typeof proxy === "string" || Array.isArray(proxy)) {
      throw new Error("expected object proxy config")
    }

    const clientWebSocketProxy = proxy["/api/client/ws"]
    if (!clientWebSocketProxy || typeof clientWebSocketProxy === "string") {
      throw new Error("expected /api/client/ws proxy options")
    }

    expect(clientWebSocketProxy.ws).toBe(true)
    expect(clientWebSocketProxy.changeOrigin).toBe(true)
    expect(clientWebSocketProxy.rewriteWsOrigin).toBe(true)

    const clientProxy = proxy["/api/client"]
    if (!clientProxy || typeof clientProxy === "string") {
      throw new Error("expected /api/client proxy options")
    }

    expect(clientProxy.changeOrigin).toBe(false)
  })

  it("serializes the client commit into the version manifest", () => {
    expect(createClientVersionManifest("abc123")).toBe('{"commit":"abc123"}\n')
  })
})
