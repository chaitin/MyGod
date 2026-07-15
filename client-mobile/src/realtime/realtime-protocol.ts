export const REALTIME_PROTOCOL_VERSION = 1

export const realtimeEvents = {
  conversationMemberMentioned: "conversation.member_mentioned",
  conversationRemoved: "conversation.removed",
  messageCreated: "message.created",
  messageUpdated: "message.updated",
  systemReady: "system.ready",
} as const

export type RealtimeEnvelope = {
  event?: string
  id?: string
  kind: "event" | "response"
  payload?: unknown
  replyTo?: string
  version: number
}

export function parseRealtimeEnvelope(value: unknown): RealtimeEnvelope | null {
  if (typeof value !== "string") {
    return null
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return null
  }

  const envelope = asRecord(parsed)
  if (
    !envelope ||
    envelope.v !== REALTIME_PROTOCOL_VERSION ||
    (envelope.kind !== "event" && envelope.kind !== "response")
  ) {
    return null
  }

  return {
    event: asString(envelope.event),
    id: asString(envelope.id),
    kind: envelope.kind,
    payload: envelope.payload,
    replyTo: asString(envelope.reply_to),
    version: REALTIME_PROTOCOL_VERSION,
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function asString(value: unknown) {
  return typeof value === "string" ? value : undefined
}
