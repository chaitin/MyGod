import { createContext, useContext } from "react"

import {
  type ClientConversation,
  type ClientDataRequestError,
  type ClientUser,
  type ContactUser,
} from "@/lib/client-data-api"

export type ClientDataContextValue = {
  conversations: ClientConversation[]
  contacts: ContactUser[]
  contactsError: ClientDataRequestError | null
  contactsLoading: boolean
  contactsRefreshing: boolean
  me: ClientUser
  meError: ClientDataRequestError | null
  meLoading: boolean
  meRefreshing: boolean
  openDirectConversation: (userId: string) => Promise<ClientConversation>
  refreshConversations: () => Promise<void>
  refreshContacts: () => Promise<void>
  refreshMe: () => Promise<void>
}

export const ClientDataContext = createContext<ClientDataContextValue | null>(
  null
)

export function useClientData() {
  const context = useContext(ClientDataContext)

  if (!context) {
    throw new Error("useClientData must be used within ClientDataProvider")
  }

  return context
}
