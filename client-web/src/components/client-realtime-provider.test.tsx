import { act, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { MemoryRouter } from "react-router"
import { toast } from "sonner"

import { ClientRealtimeProvider } from "@/components/client-realtime-provider"
import {
  RealtimeClient,
  type RealtimeWebSocketLike,
} from "@/lib/realtime-client"
import { useRealtime } from "@/lib/realtime-context"

vi.mock("sonner", async (importOriginal) => {
  const actual = await importOriginal<typeof import("sonner")>()

  return {
    ...actual,
    toast: {
      ...actual.toast,
      success: vi.fn(),
      warning: vi.fn(),
    },
  }
})

class FakeWebSocket implements RealtimeWebSocketLike {
  static instances: FakeWebSocket[] = []

  closeCount = 0
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onopen: ((event: Event) => void) | null = null
  readyState: number = WebSocket.CONNECTING
  sent: string[] = []
  url: string

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.closeCount += 1
    this.readyState = WebSocket.CLOSED
    this.onclose?.(new CloseEvent("close", { code: 1000 }))
  }

  failClose(code = 1006) {
    this.readyState = WebSocket.CLOSED
    this.onclose?.(new CloseEvent("close", { code }))
  }

  send(data: string) {
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
}

function createClient() {
  return new RealtimeClient({
    createWebSocket: (url) => new FakeWebSocket(url),
    reconnectDelaysMs: [100],
    url: "ws://example.test/api/client/ws",
  })
}

function RealtimeProbe() {
  const { status } = useRealtime()

  return (
    <div>
      <span data-testid="realtime-status">{status}</span>
    </div>
  )
}

describe("ClientRealtimeProvider", () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.mocked(toast.success).mockClear()
    vi.mocked(toast.warning).mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("connects on mount, exposes connection status, and disconnects on unmount", async () => {
    const { unmount } = render(
      <MemoryRouter>
        <ClientRealtimeProvider client={createClient()}>
          <RealtimeProbe />
        </ClientRealtimeProvider>
      </MemoryRouter>
    )

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(screen.queryByTestId("realtime-status")).not.toBeInTheDocument()

    FakeWebSocket.instances[0].open()
    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(screen.queryByTestId("realtime-status")).not.toBeInTheDocument()

    FakeWebSocket.instances[0].receive({
      v: 1,
      kind: "event",
      event: "system.ready",
      payload: {},
    })
    expect(await screen.findByText("connected")).toBeInTheDocument()

    unmount()

    expect(FakeWebSocket.instances[0].closeCount).toBe(1)
  })

  it("shows reconnecting and restored toasts after an established connection drops", async () => {
    vi.useFakeTimers()

    render(
      <MemoryRouter>
        <ClientRealtimeProvider client={createClient()}>
          <RealtimeProbe />
        </ClientRealtimeProvider>
      </MemoryRouter>
    )

    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    await act(async () => {
      FakeWebSocket.instances[0].failClose()
    })
    expect(toast.warning).not.toHaveBeenCalled()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100)
    })
    await act(async () => {
      FakeWebSocket.instances[1].open()
    })
    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    await act(async () => {
      FakeWebSocket.instances[1].receive({
        v: 1,
        kind: "event",
        event: "system.ready",
        payload: {},
      })
    })
    expect(screen.getByText("connected")).toBeInTheDocument()
    expect(toast.success).not.toHaveBeenCalled()

    await act(async () => {
      FakeWebSocket.instances[1].failClose()
    })
    expect(toast.warning).toHaveBeenCalledWith("网络断开，正在尝试重新连接")

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100)
    })
    await act(async () => {
      FakeWebSocket.instances[2].open()
    })
    expect(toast.success).not.toHaveBeenCalled()
    await act(async () => {
      FakeWebSocket.instances[2].receive({
        v: 1,
        kind: "event",
        event: "system.ready",
        payload: {},
      })
    })

    expect(screen.getByText("connected")).toBeInTheDocument()
    expect(toast.success).toHaveBeenCalledWith("网络已恢复连接")
  })
})
