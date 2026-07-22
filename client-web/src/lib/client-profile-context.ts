import * as React from "react"

import type { ClientDataContextValue } from "@/lib/client-data-context"
import type { ClientProfileStore } from "@/lib/client-profile-store"

export type ClientProfileData = Pick<
  ClientDataContextValue,
  | "contactApps"
  | "contacts"
  | "me"
  | "openAppConversation"
  | "openDirectConversation"
>

export type ClientProfileContextValue = Pick<
  ClientProfileData,
  "openAppConversation" | "openDirectConversation"
> & { store: ClientProfileStore }

export const ClientProfileContext =
  React.createContext<ClientProfileContextValue | null>(null)

export function useOptionalClientProfileContext() {
  return React.useContext(ClientProfileContext)
}

export function useClientAppProfile(appId: string | null | undefined) {
  const store = useClientProfileStore()
  const subscribe = React.useCallback(
    (listener: () => void) => store.subscribeApp(appId, listener),
    [appId, store]
  )
  const getSnapshot = React.useCallback(
    () => store.getApp(appId),
    [appId, store]
  )

  return React.useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function useClientCurrentUserId() {
  const store = useClientProfileStore()

  return React.useSyncExternalStore(
    React.useCallback(
      (listener: () => void) => store.subscribeCurrentUserId(listener),
      [store]
    ),
    React.useCallback(() => store.getCurrentUserId(), [store]),
    React.useCallback(() => store.getCurrentUserId(), [store])
  )
}

export function useClientUserProfile(userId: string | null | undefined) {
  const store = useClientProfileStore()
  const subscribe = React.useCallback(
    (listener: () => void) => store.subscribeUser(userId, listener),
    [store, userId]
  )
  const getSnapshot = React.useCallback(
    () => store.getUser(userId),
    [store, userId]
  )

  return React.useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

function useClientProfileStore() {
  const profileData = React.useContext(ClientProfileContext)
  if (!profileData) {
    throw new Error("ClientProfileProvider is required")
  }
  return profileData.store
}
