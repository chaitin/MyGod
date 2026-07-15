import { createContext, useContext } from "react"

import type {
  RealtimeConnectionStatus,
  RealtimeSnapshot,
} from "@/realtime/realtime-client"

export type RealtimeContextValue = RealtimeSnapshot

export const RealtimeContext = createContext<RealtimeContextValue | null>(null)

export function useRealtime(): RealtimeContextValue {
  const value = useContext(RealtimeContext)

  if (!value) {
    throw new Error("useRealtime 必须在 RealtimeProvider 内使用")
  }

  return value
}

export const DISCONNECTED_REALTIME_SNAPSHOT: RealtimeContextValue = {
  ready: false,
  status: "disconnected" satisfies RealtimeConnectionStatus,
}
