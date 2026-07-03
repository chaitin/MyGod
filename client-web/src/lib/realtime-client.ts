import { createClientMessageId } from "@/lib/message-id"

export type RealtimeConnectionStatus =
  "connecting" | "connected" | "reconnecting" | "disconnected"

export type RealtimeSnapshot = {
  ready: boolean
  status: RealtimeConnectionStatus
}

export type RealtimeWebSocketLike = {
  close: () => void
  onclose: ((event: CloseEvent) => void) | null
  onerror: ((event: Event) => void) | null
  onmessage: ((event: MessageEvent) => void) | null
  onopen: ((event: Event) => void) | null
  readyState: number
  send: (data: string) => void
}

type RealtimeEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  event?: string
  id?: string
  kind?: string
  ok?: boolean
  payload?: unknown
  reply_to?: string
  v?: number
}

type PendingRequest = {
  reject: (error: Error) => void
  resolve: (value: unknown) => void
}

type RealtimeClientOptions = {
  authCheck?: () => boolean | Promise<boolean>
  createWebSocket?: (url: string) => RealtimeWebSocketLike
  onUnauthorized?: () => void
  reconnectDelaysMs?: number[]
  url?: string
}

const protocolVersion = 1
const defaultReconnectDelaysMs = [500, 1_000, 2_000, 5_000, 10_000, 30_000]

export class RealtimeClient {
  private authCheck?: () => boolean | Promise<boolean>
  private createWebSocket: (url: string) => RealtimeWebSocketLike
  private listeners = new Set<() => void>()
  private onUnauthorized?: () => void
  private pendingRequests = new Map<string, PendingRequest>()
  private ready = false
  private reconnectAttempt = 0
  private reconnectDelaysMs: number[]
  private reconnectSequence = 0
  private reconnectTimer: number | null = null
  private shouldReconnect = false
  private socket: RealtimeWebSocketLike | null = null
  private status: RealtimeConnectionStatus = "disconnected"
  private url: string

  constructor(options: RealtimeClientOptions = {}) {
    this.authCheck = options.authCheck
    this.url = options.url ?? buildRealtimeWebSocketURL(window.location)
    this.createWebSocket =
      options.createWebSocket ?? ((url) => new WebSocket(url))
    this.onUnauthorized = options.onUnauthorized
    this.reconnectDelaysMs =
      options.reconnectDelaysMs ?? defaultReconnectDelaysMs
  }

  connect() {
    if (this.socket && this.socket.readyState !== WebSocket.CLOSED) {
      return
    }

    this.shouldReconnect = true
    this.openSocket("connecting")
  }

  disconnect() {
    this.shouldReconnect = false
    this.reconnectSequence += 1
    this.clearReconnectTimer()
    this.rejectPendingRequests(new Error("实时连接已断开"))
    this.ready = false
    this.status = "disconnected"
    this.socket?.close()
    this.socket = null
    this.notify()
  }

  getSnapshot(): RealtimeSnapshot {
    return {
      ready: this.ready,
      status: this.status,
    }
  }

  sendRequest(method: string, payload: unknown) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("实时连接未建立"))
    }

    const id = createClientMessageId()
    const request = {
      v: protocolVersion,
      kind: "request",
      id,
      method,
      payload,
    }

    const socket = this.socket
    const message = JSON.stringify(request)

    return new Promise<unknown>((resolve, reject) => {
      this.pendingRequests.set(id, { reject, resolve })
      try {
        socket.send(message)
      } catch {
        this.pendingRequests.delete(id)
        reject(new Error("发送实时请求失败"))
      }
    })
  }

  subscribe(listener: () => void) {
    this.listeners.add(listener)

    return () => {
      this.listeners.delete(listener)
    }
  }

  private openSocket(status: RealtimeConnectionStatus) {
    this.clearReconnectTimer()
    this.ready = false
    this.status = status
    this.notify()

    const socket = this.createWebSocket(this.url)
    this.socket = socket
    socket.onopen = () => {
      this.reconnectAttempt = 0
      this.status = "connected"
      this.notify()
    }
    socket.onmessage = (event) => {
      this.handleMessage(event.data)
    }
    socket.onerror = () => undefined
    socket.onclose = () => {
      void this.handleSocketClose(socket)
    }
  }

  private async handleSocketClose(socket: RealtimeWebSocketLike) {
    if (this.socket === socket) {
      this.socket = null
    }
    this.rejectPendingRequests(new Error("实时连接已断开"))
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

    if (!this.authCheck) {
      this.scheduleReconnect()
      return
    }

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
      this.clearReconnectTimer()
      this.status = "disconnected"
      this.notify()
      this.onUnauthorized?.()
      return
    }

    this.scheduleReconnect()
  }

  private async checkReconnectAuthorization() {
    try {
      return await this.authCheck!()
    } catch {
      return true
    }
  }

  private scheduleReconnect() {
    const delay =
      this.reconnectDelaysMs[
        Math.min(this.reconnectAttempt, this.reconnectDelaysMs.length - 1)
      ] ?? defaultReconnectDelaysMs[defaultReconnectDelaysMs.length - 1]
    this.reconnectAttempt += 1
    this.reconnectTimer = window.setTimeout(() => {
      if (!this.shouldReconnect) {
        return
      }
      this.openSocket("connecting")
    }, delay)
  }

  private handleMessage(data: unknown) {
    if (typeof data !== "string") {
      return
    }

    let envelope: RealtimeEnvelope
    try {
      envelope = JSON.parse(data) as RealtimeEnvelope
    } catch {
      return
    }
    if (envelope.v !== protocolVersion) {
      return
    }

    if (envelope.kind === "event") {
      this.handleEvent(envelope)
      return
    }
    if (envelope.kind === "response") {
      this.handleResponse(envelope)
    }
  }

  private handleEvent(envelope: RealtimeEnvelope) {
    if (envelope.event === "system.ready") {
      this.ready = true
      this.notify()
      return
    }
  }

  private handleResponse(envelope: RealtimeEnvelope) {
    const replyTo = envelope.reply_to
    if (!replyTo) {
      return
    }

    const pending = this.pendingRequests.get(replyTo)
    if (!pending) {
      return
    }
    this.pendingRequests.delete(replyTo)

    if (envelope.ok) {
      pending.resolve(envelope.payload)
      return
    }

    pending.reject(new Error(envelope.error?.message ?? "实时请求失败"))
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer === null) {
      return
    }
    window.clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
  }

  private rejectPendingRequests(error: Error) {
    for (const pending of this.pendingRequests.values()) {
      pending.reject(error)
    }
    this.pendingRequests.clear()
  }

  private notify() {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export function buildRealtimeWebSocketURL(
  location: Pick<Location | URL, "host" | "protocol">
) {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"

  return `${protocol}//${location.host}/api/client/ws`
}
