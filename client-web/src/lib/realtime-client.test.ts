import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  RealtimeClient,
  buildRealtimeWebSocketURL,
  type RealtimeWebSocketLike,
} from "@/lib/realtime-client"

class FakeWebSocket implements RealtimeWebSocketLike {
  static instances: FakeWebSocket[] = []

  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onopen: ((event: Event) => void) | null = null
  readyState: number = WebSocket.CONNECTING
  sent: string[] = []
  throwOnSend = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.readyState = WebSocket.CLOSED
    this.onclose?.(new CloseEvent("close", { code: 1000 }))
  }

  send(data: string) {
    if (this.throwOnSend) {
      throw new Error("send failed")
    }
    this.sent.push(data)
  }

  open() {
    this.readyState = WebSocket.OPEN
    this.onopen?.(new Event("open"))
  }

  receive(payload: unknown) {
    this.onmessage?.(
      new MessageEvent("message", {
        data: JSON.stringify(payload),
      })
    )
  }

  failClose(code = 1006) {
    this.readyState = WebSocket.CLOSED
    this.onclose?.(new CloseEvent("close", { code }))
  }
}

function createClient() {
  return new RealtimeClient({
    createWebSocket: (url) => new FakeWebSocket(url),
    reconnectDelaysMs: [100],
    url: "ws://example.test/api/client/ws",
  })
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe("RealtimeClient", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("builds a websocket URL from the current location", () => {
    expect(
      buildRealtimeWebSocketURL(new URL("http://localhost:20070/contacts"))
    ).toBe("ws://localhost:20070/api/client/ws")
    expect(buildRealtimeWebSocketURL(new URL("https://example.com/chat"))).toBe(
      "wss://example.com/api/client/ws"
    )
  })

  it("ignores presence events because contacts API owns online status", () => {
    const client = createClient()
    const listener = vi.fn()
    client.subscribe(listener)

    client.connect()
    expect(FakeWebSocket.instances[0]?.url).toBe(
      "ws://example.test/api/client/ws"
    )
    expect(client.getSnapshot().status).toBe("connecting")

    FakeWebSocket.instances[0].open()
    expect(client.getSnapshot().status).toBe("connected")
    listener.mockClear()

    FakeWebSocket.instances[0].receive({
      v: 1,
      kind: "event",
      event: "presence.snapshot",
      payload: {
        users: [
          {
            user_id: "user-1",
            online: true,
            last_online_at: "2026-07-03T01:00:00Z",
          },
        ],
      },
    })

    FakeWebSocket.instances[0].receive({
      v: 1,
      kind: "event",
      event: "presence.changed",
      payload: {
        user_id: "user-1",
        online: false,
        last_online_at: "2026-07-03T01:02:00Z",
      },
    })
    expect(listener).not.toHaveBeenCalled()
    expect(client.getSnapshot()).toEqual({
      ready: false,
      status: "connected",
    })
  })

  it("marks the realtime connection ready only after system.ready", () => {
    const client = createClient()
    client.connect()
    FakeWebSocket.instances[0].open()

    expect(client.getSnapshot().status).toBe("connected")
    expect(client.getSnapshot().ready).toBe(false)

    FakeWebSocket.instances[0].receive({
      v: 1,
      kind: "event",
      event: "system.ready",
      payload: {},
    })
    expect(client.getSnapshot().ready).toBe(true)

    FakeWebSocket.instances[0].failClose()
    expect(client.getSnapshot().ready).toBe(false)
  })

  it("sends request envelopes and resolves matching responses", async () => {
    const client = createClient()
    client.connect()
    FakeWebSocket.instances[0].open()

    const result = client.sendRequest("message.create", { text: "hello" })
    const sent = JSON.parse(FakeWebSocket.instances[0].sent[0])

    expect(sent).toMatchObject({
      kind: "request",
      method: "message.create",
      payload: { text: "hello" },
      v: 1,
    })
    expect(sent.id).toEqual(expect.any(String))

    FakeWebSocket.instances[0].receive({
      v: 1,
      kind: "response",
      reply_to: sent.id,
      ok: true,
      payload: {
        accepted: true,
      },
    })

    await expect(result).resolves.toEqual({ accepted: true })
  })

  it("rejects pending requests when the socket closes before a response", async () => {
    const client = createClient()
    client.connect()
    FakeWebSocket.instances[0].open()

    const result = client.sendRequest("message.create", { text: "hello" })
    let rejectionMessage: string | undefined
    result.catch((error: Error) => {
      rejectionMessage = error.message
    })

    FakeWebSocket.instances[0].failClose()
    await flushPromises()

    expect(rejectionMessage).toBe("实时连接已断开")
  })

  it("rejects a realtime request when socket send fails", async () => {
    const client = createClient()
    client.connect()
    FakeWebSocket.instances[0].open()
    FakeWebSocket.instances[0].throwOnSend = true

    await expect(
      client.sendRequest("message.create", { text: "hello" })
    ).rejects.toThrow("发送实时请求失败")
  })

  it("stops reconnecting and reports unauthorized when auth check fails", async () => {
    const authCheck = vi.fn().mockResolvedValue(false)
    const onUnauthorized = vi.fn()
    const client = new RealtimeClient({
      authCheck,
      createWebSocket: (url: string) => new FakeWebSocket(url),
      onUnauthorized,
      reconnectDelaysMs: [100],
      url: "ws://example.test/api/client/ws",
    } as ConstructorParameters<typeof RealtimeClient>[0] & {
      authCheck: () => Promise<boolean>
      onUnauthorized: () => void
    })

    client.connect()
    FakeWebSocket.instances[0].open()

    FakeWebSocket.instances[0].failClose()
    await flushPromises()

    expect(authCheck).toHaveBeenCalledTimes(1)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(client.getSnapshot().status).toBe("disconnected")

    vi.advanceTimersByTime(100)
    expect(FakeWebSocket.instances).toHaveLength(1)
  })

  it("reconnects after abnormal close and stops reconnecting after disconnect", () => {
    const client = createClient()
    client.connect()
    FakeWebSocket.instances[0].open()

    FakeWebSocket.instances[0].failClose()
    expect(client.getSnapshot().status).toBe("reconnecting")

    vi.advanceTimersByTime(100)
    expect(FakeWebSocket.instances).toHaveLength(2)

    client.disconnect()
    FakeWebSocket.instances[1].failClose()
    vi.advanceTimersByTime(100)

    expect(client.getSnapshot().status).toBe("disconnected")
    expect(FakeWebSocket.instances).toHaveLength(2)
  })
})
