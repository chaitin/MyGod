import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState } from "react"
import { AppState, Platform, type AppStateStatus } from "react-native"

import { isUnauthorizedError } from "@/data/api-client"
import { fetchCurrentUser } from "@/data/current-user-api"
import type { ServerTarget } from "@/data/query"
import { useAuth } from "@/features/auth/auth-context"
import { useServers } from "@/features/servers/server-context"
import {
  prepareMessageNotifications,
  showBackgroundMessageNotification,
} from "@/notifications/message-notifications"
import {
  applyRealtimeEvent,
  refreshClientDataOnForeground,
  synchronizeRealtimeData,
} from "@/realtime/realtime-cache"
import {
  buildRealtimeWebSocketUrl,
  RealtimeClient,
  type RealtimeSnapshot,
} from "@/realtime/realtime-client"
import {
  DISCONNECTED_REALTIME_SNAPSHOT,
  RealtimeContext,
} from "@/realtime/realtime-context"
import { realtimeEvents } from "@/realtime/realtime-protocol"

export function RealtimeProvider({ children }: React.PropsWithChildren) {
  const queryClient = useQueryClient()
  const { isAuthenticated, signOut } = useAuth()
  const { isHydrated, selectedServer } = useServers()
  const [snapshot, setSnapshot] = useState<RealtimeSnapshot>(
    DISCONNECTED_REALTIME_SNAPSHOT
  )
  const server = useMemo<ServerTarget>(
    () => ({ id: selectedServer.id, url: selectedServer.url }),
    [selectedServer.id, selectedServer.url]
  )
  const realtimeEnabled =
    isAuthenticated &&
    isHydrated &&
    canConnectFromCurrentPlatform(server.url)

  useEffect(() => {
    if (!realtimeEnabled) {
      return
    }

    let isActive = true
    let synchronization = Promise.resolve()
    let currentAppState = AppState.currentState
    const client = new RealtimeClient({
      authCheck: async () => {
        try {
          await fetchCurrentUser(server.url)
          return true
        } catch (error: unknown) {
          if (isUnauthorizedError(error)) {
            return false
          }
          throw error
        }
      },
      onUnauthorized: signOut,
      url: buildRealtimeWebSocketUrl(server.url),
    })

    const unsubscribeSnapshot = client.subscribe(() => {
      if (isActive) {
        setSnapshot(client.getSnapshot())
      }
    })
    const unsubscribeEvents = client.subscribeEvent((event, payload) => {
      if (event === realtimeEvents.systemReady) {
        enqueueSynchronization(() =>
          synchronizeRealtimeData(queryClient, server)
        )
        return
      }

      void applyRealtimeEvent(queryClient, server, event, payload)
        .then(({ message }) => {
          if (
            event === realtimeEvents.messageCreated &&
            message &&
            currentAppState !== "active"
          ) {
            void showBackgroundMessageNotification(
              queryClient,
              server,
              message
            ).catch(() => undefined)
          }
        })
        .catch(handleRealtimeDataError)
    })

    function enqueueSynchronization(task: () => Promise<void>) {
      synchronization = synchronization
        .catch(() => undefined)
        .then(task)
        .catch(handleRealtimeDataError)
    }

    function handleRealtimeDataError(error: unknown) {
      if (isActive && isUnauthorizedError(error)) {
        signOut()
      }
    }

    function handleAppStateChange(status: AppStateStatus) {
      const wasActive = currentAppState === "active"
      currentAppState = status

      if (status === "active" && !wasActive) {
        client.connect()
        void prepareMessageNotifications().catch(() => undefined)
        enqueueSynchronization(() =>
          refreshClientDataOnForeground(queryClient, server)
        )
      }
    }

    const appStateSubscription = AppState.addEventListener(
      "change",
      handleAppStateChange
    )
    client.connect()
    if (currentAppState === "active") {
      void prepareMessageNotifications().catch(() => undefined)
    }

    return () => {
      isActive = false
      appStateSubscription.remove()
      unsubscribeEvents()
      unsubscribeSnapshot()
      client.disconnect()
    }
  }, [queryClient, realtimeEnabled, server, signOut])

  const value = useMemo(() => {
    if (!realtimeEnabled) {
      return DISCONNECTED_REALTIME_SNAPSHOT
    }

    return { ready: snapshot.ready, status: snapshot.status }
  }, [realtimeEnabled, snapshot.ready, snapshot.status])

  return (
    <RealtimeContext.Provider value={value}>
      {children}
    </RealtimeContext.Provider>
  )
}

function canConnectFromCurrentPlatform(serverUrl: string) {
  if (Platform.OS !== "web" || typeof window === "undefined") {
    return true
  }

  // Browsers control the Origin header and the current server only permits
  // same-origin websocket upgrades. Native Android/iOS connections are not
  // subject to this browser restriction.
  try {
    return new URL(serverUrl).origin === window.location.origin
  } catch {
    return false
  }
}
