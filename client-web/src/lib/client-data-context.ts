import { createContext, useContext } from "react"

import {
  type ClientConversation,
  type ClientDataRequestError,
  type ClientMessage,
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
  createGroupConversation: (
    name: string,
    memberIds: string[]
  ) => Promise<ClientConversation>
  openDirectConversation: (userId: string) => Promise<ClientConversation>
  refreshConversations: () => Promise<void>
  refreshContacts: () => Promise<void>
  refreshMe: () => Promise<void>
  updateConversationLastMessage: (message: ClientMessage) => void
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
