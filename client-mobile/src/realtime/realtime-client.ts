import {
  parseRealtimeEnvelope,
  realtimeEvents,
} from "@/realtime/realtime-protocol"

export type RealtimeConnectionStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected"

export type RealtimeSnapshot = {
  ready: boolean
  status: RealtimeConnectionStatus
}

type RealtimeSocket = Pick<
  WebSocket,
  | "close"
  | "onclose"
  | "onerror"
  | "onmessage"
  | "onopen"
  | "readyState"
>

type RealtimeClientOptions = {
  authCheck?: () => boolean | Promise<boolean>
  createSocket?: (url: string) => RealtimeSocket
  onUnauthorized?: () => void
  reconnectDelaysMs?: number[]
  url: string
}

type RealtimeEventHandler = (event: string, payload: unknown) => void

const DEFAULT_RECONNECT_DELAYS_MS = [
  500,
  1_000,
  2_000,
  5_000,
  10_000,
  30_000,
]

export class RealtimeClient {
  private readonly authCheck?: () => boolean | Promise<boolean>
  private readonly createSocket: (url: string) => RealtimeSocket
  private readonly eventListeners = new Set<RealtimeEventHandler>()
  private readonly listeners = new Set<() => void>()
  private readonly onUnauthorized?: () => void
  private readonly reconnectDelaysMs: number[]
  private readonly url: string

  private ready = false
  private reconnectAttempt = 0
  private reconnectSequence = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private shouldReconnect = false
  private socket: RealtimeSocket | null = null
  private status: RealtimeConnectionStatus = "disconnected"

  constructor(options: RealtimeClientOptions) {
    this.authCheck = options.authCheck
    this.createSocket = options.createSocket ?? ((url) => new WebSocket(url))
    this.onUnauthorized = options.onUnauthorized
    this.reconnectDelaysMs =
      options.reconnectDelaysMs?.length
        ? options.reconnectDelaysMs
        : DEFAULT_RECONNECT_DELAYS_MS
    this.url = options.url
  }

  connect() {
    this.shouldReconnect = true

    if (this.socket || this.reconnectTimer) {
      return
    }

    this.openSocket("connecting")
  }

  disconnect() {
    this.shouldReconnect = false
    this.reconnectAttempt = 0
    this.reconnectSequence += 1
    this.clearReconnectTimer()

    const socket = this.socket
    this.socket = null
    this.ready = false
    this.status = "disconnected"
    socket?.close()
    this.notify()
  }

  getSnapshot(): RealtimeSnapshot {
    return { ready: this.ready, status: this.status }
  }

  subscribe(listener: () => void) {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  subscribeEvent(handler: RealtimeEventHandler) {
    this.eventListeners.add(handler)
    return () => this.eventListeners.delete(handler)
  }

  private openSocket(status: RealtimeConnectionStatus) {
    this.clearReconnectTimer()
    this.ready = false
    this.status = status
    this.notify()

    const socket = this.createSocket(this.url)
    this.socket = socket

    socket.onopen = () => {
      if (this.socket !== socket) {
        return
      }

      this.status = "connected"
      this.notify()
    }
    socket.onmessage = (event) => {
      if (this.socket === socket) {
        this.handleMessage(event.data)
      }
    }
    socket.onerror = () => undefined
    socket.onclose = () => {
      void this.handleSocketClose(socket)
    }
  }

  private handleMessage(data: unknown) {
    const envelope = parseRealtimeEnvelope(data)
    if (!envelope || envelope.kind !== "event" || !envelope.event) {
      return
    }

    if (envelope.event === realtimeEvents.systemReady) {
      this.ready = true
      this.reconnectAttempt = 0
      this.notify()
    }

    for (const listener of this.eventListeners) {
      listener(envelope.event, envelope.payload)
    }
  }

  private async handleSocketClose(socket: RealtimeSocket) {
    if (this.socket !== socket) {
      return
    }

    this.socket = null
    this.ready = false

    if (!this.shouldReconnect) {
      this.status = "disconnected"
      this.notify()
      return
    }

    const reconnectSequence = this.reconnectSequence + 1
    this.reconnectSequence = reconnectSequence
    this.status = "reconnecting"
    this.notify()

    const authorized = await this.checkReconnectAuthorization()
    if (
      reconnectSequence !== this.reconnectSequence ||
      !this.shouldReconnect ||
      this.socket
    ) {
      return
    }

    if (!authorized) {
      this.shouldReconnect = false
      this.status = "disconnected"
      this.notify()
      this.onUnauthorized?.()
      return
    }

    this.scheduleReconnect()
  }

  private async checkReconnectAuthorization() {
    if (!this.authCheck) {
      return true
    }

    try {
      return await this.authCheck()
    } catch {
      // A failed auth check can be caused by the same network outage. Keep
      // reconnecting unless the server explicitly reports an invalid session.
      return true
    }
  }

  private scheduleReconnect() {
    const delay =
      this.reconnectDelaysMs[
        Math.min(this.reconnectAttempt, this.reconnectDelaysMs.length - 1)
      ] ?? DEFAULT_RECONNECT_DELAYS_MS.at(-1)!

    this.reconnectAttempt += 1
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (this.shouldReconnect) {
        this.openSocket("connecting")
      }
    }, delay)
  }

  private clearReconnectTimer() {
    if (!this.reconnectTimer) {
      return
    }

    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
  }

  private notify() {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export function buildRealtimeWebSocketUrl(serverUrl: string) {
  const url = new URL("api/client/ws", ensureTrailingSlash(serverUrl))
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:"
  return url.toString()
}

function ensureTrailingSlash(value: string) {
  return `${value.replace(/\/+$/, "")}/`
}
