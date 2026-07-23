import type { ClientTopicSourceMessage } from "@/lib/client-data-api"

export function isTopicSourceMessageSelectable(
  message: ClientTopicSourceMessage
) {
  return (
    message.body.type !== "revoked" &&
    message.body.type !== "unsupported" &&
    message.body.type !== "system_event"
  )
}
