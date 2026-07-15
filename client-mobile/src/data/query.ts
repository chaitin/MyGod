import { QueryClient, queryOptions } from "@tanstack/react-query"

import { fetchAppInfo } from "@/data/app-info-api"
import { fetchContacts } from "@/data/contacts-api"
import { fetchConversations } from "@/data/conversations-api"
import { fetchCurrentUser } from "@/data/current-user-api"

export const CLIENT_DATA_REFRESH_INTERVAL_MS = 15_000

export type ServerTarget = {
  id: string
  url: string
}

export const queryKeys = {
  appInfo: (server: ServerTarget) =>
    ["server", server.id, server.url, "app-info"] as const,
  contacts: (server: ServerTarget) =>
    ["server", server.id, server.url, "contacts"] as const,
  conversations: (server: ServerTarget) =>
    ["server", server.id, server.url, "conversations"] as const,
  conversationMessages: (server: ServerTarget, conversationId: string) =>
    [
      "server",
      server.id,
      server.url,
      "conversation",
      conversationId,
      "messages",
    ] as const,
  currentUser: (server: ServerTarget) =>
    ["server", server.id, server.url, "current-user"] as const,
  temporaryFileUrls: (server: ServerTarget, fileIds: string[]) =>
    ["server", server.id, server.url, "temporary-file-urls", fileIds] as const,
}

export function createClientQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: 1,
        staleTime: 10_000,
      },
    },
  })
}

export function appInfoQueryOptions(server: ServerTarget) {
  return queryOptions({
    queryFn: ({ signal }) => fetchAppInfo(server.url, { signal }),
    queryKey: queryKeys.appInfo(server),
    retry: false,
    staleTime: 0,
  })
}

export function contactsQueryOptions(server: ServerTarget) {
  return queryOptions({
    queryFn: ({ signal }) => fetchContacts(server.url, { signal }),
    queryKey: queryKeys.contacts(server),
    refetchInterval: CLIENT_DATA_REFRESH_INTERVAL_MS,
  })
}

export function currentUserQueryOptions(server: ServerTarget) {
  return queryOptions({
    queryFn: ({ signal }) => fetchCurrentUser(server.url, { signal }),
    queryKey: queryKeys.currentUser(server),
  })
}

export function conversationsQueryOptions(server: ServerTarget) {
  return queryOptions({
    queryFn: ({ signal }) => fetchConversations(server.url, { signal }),
    queryKey: queryKeys.conversations(server),
    refetchInterval: CLIENT_DATA_REFRESH_INTERVAL_MS,
  })
}
