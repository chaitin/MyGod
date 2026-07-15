import { useQuery } from "@tanstack/react-query"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
} from "react"

import { isUnauthorizedError } from "@/data/api-client"
import type {
  ClientContacts,
  ClientConversation,
  ClientUser,
} from "@/data/models"
import {
  contactsQueryOptions,
  conversationsQueryOptions,
  currentUserQueryOptions,
} from "@/data/query"
import { useAuth } from "@/features/auth/auth-context"
import { useServers } from "@/features/servers/server-context"

const EMPTY_CONTACTS: ClientContacts = {
  apps: [],
  groups: [],
  users: [],
}

type ClientDataContextValue = {
  contacts: ClientContacts
  contactsError: Error | null
  conversations: ClientConversation[]
  conversationsError: Error | null
  currentUser: ClientUser | null
  currentUserError: Error | null
  error: Error | null
  isContactsRefreshing: boolean
  isConversationsRefreshing: boolean
  isReady: boolean
  isRefreshing: boolean
  refresh: () => Promise<void>
  refreshContacts: () => Promise<void>
  refreshConversations: () => Promise<void>
}

const ClientDataContext = createContext<ClientDataContextValue | null>(null)

export function ClientDataProvider({ children }: React.PropsWithChildren) {
  const { isAuthenticated, signOut } = useAuth()
  const { isHydrated, selectedServer } = useServers()
  const enabled = isAuthenticated && isHydrated
  const contactsQuery = useQuery({
    ...contactsQueryOptions(selectedServer),
    enabled,
  })
  const conversationsQuery = useQuery({
    ...conversationsQueryOptions(selectedServer),
    enabled,
  })
  const currentUserQuery = useQuery({
    ...currentUserQueryOptions(selectedServer),
    enabled,
  })
  const error =
    currentUserQuery.error ?? contactsQuery.error ?? conversationsQuery.error

  useEffect(() => {
    if (isUnauthorizedError(error)) {
      signOut()
    }
  }, [error, signOut])

  const refreshContacts = useCallback(async () => {
    const result = await contactsQuery.refetch()

    if (result.error) {
      throw result.error
    }
  }, [contactsQuery])

  const refreshConversations = useCallback(async () => {
    const result = await conversationsQuery.refetch()

    if (result.error) {
      throw result.error
    }
  }, [conversationsQuery])

  const refresh = useCallback(async () => {
    await Promise.all([refreshContacts(), refreshConversations()])
  }, [refreshContacts, refreshConversations])

  const value = useMemo(
    () => ({
      contacts: enabled ? (contactsQuery.data ?? EMPTY_CONTACTS) : EMPTY_CONTACTS,
      contactsError: enabled ? contactsQuery.error : null,
      conversations: enabled ? (conversationsQuery.data ?? []) : [],
      conversationsError: enabled ? conversationsQuery.error : null,
      currentUser: enabled ? (currentUserQuery.data ?? null) : null,
      currentUserError: enabled ? currentUserQuery.error : null,
      error: enabled ? error : null,
      isContactsRefreshing: enabled && contactsQuery.isFetching,
      isConversationsRefreshing: enabled && conversationsQuery.isFetching,
      isReady:
        enabled &&
        currentUserQuery.data !== undefined &&
        contactsQuery.data !== undefined &&
        conversationsQuery.data !== undefined,
      isRefreshing:
        enabled &&
        (contactsQuery.isFetching || conversationsQuery.isFetching),
      refresh,
      refreshContacts,
      refreshConversations,
    }),
    [
      contactsQuery.data,
      contactsQuery.error,
      contactsQuery.isFetching,
      conversationsQuery.data,
      conversationsQuery.error,
      conversationsQuery.isFetching,
      currentUserQuery.data,
      currentUserQuery.error,
      enabled,
      error,
      refresh,
      refreshContacts,
      refreshConversations,
    ]
  )

  return (
    <ClientDataContext.Provider value={value}>
      {children}
    </ClientDataContext.Provider>
  )
}

export function useClientData() {
  const value = useContext(ClientDataContext)

  if (!value) {
    throw new Error("useClientData 必须在 ClientDataProvider 内使用")
  }

  return value
}
