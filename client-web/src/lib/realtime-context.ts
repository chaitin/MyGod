import { createContext, useContext } from "react"

import type { RealtimeConnectionStatus } from "@/lib/realtime-client"

export type RealtimeContextValue = {
  sendRealtimeRequest: (method: string, payload: unknown) => Promise<unknown>
  status: RealtimeConnectionStatus
}

export const RealtimeContext = createContext<RealtimeContextValue | null>(null)

export function useRealtime() {
  const context = useContext(RealtimeContext)

  if (!context) {
    throw new Error("useRealtime must be used within ClientRealtimeProvider")
  }

  return context
}
