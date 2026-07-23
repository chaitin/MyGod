import { createContext, useContext } from "react"

import {
  type ClientConversation,
  type ClientDataRequestError,
  type MarkConversationReadOptions,
  type ClientMessage,
  type MessageReactionsUpdatedEvent,
  type MessageChoiceUpdatedEvent,
  type MessageReactionSnapshot,
  type ClientMessageTopic,
  type ClientCardSendInput,
  type ClientMessagePage,
  type ClientUser,
  type ContactApp,
  type ContactGroup,
  type ContactUser,
} from "@/lib/client-data-api"
import type {
  ClientProjectDetail,
  ClientProjectSummary,
} from "@/lib/project-data-api"
import type { VoiceMessageRecording } from "@/lib/voice-message"

export type ClientConversationMessageState = {
  error: string | null
  loaded: boolean
  loading: boolean
  loadingBefore: boolean
  messages: ClientMessage[]
  page: ClientMessagePage | null
  sending: boolean
}

export type SendConversationMessageOptions = {
  replyToMessageId?: string
}

export type ClientDataContextValue = {
  contactApps: ContactApp[]
  contactGroups: ContactGroup[]
  conversations: ClientConversation[]
  contacts: ContactUser[]
  contactsError: ClientDataRequestError | null
  contactsLoading: boolean
  contactsRefreshing: boolean
  foregroundConversationId?: string
  me: ClientUser
  meError: ClientDataRequestError | null
  meLoading: boolean
  meRefreshing: boolean
  personalProject: ClientProjectSummary
  projects: ClientProjectSummary[]
  projectsError: ClientDataRequestError | null
  projectsLoading: boolean
  projectsLoadingMore: boolean
  projectsNextCursor: string | null
  projectsRefreshing: boolean
  addGroupConversationMembers: (
    conversationId: string,
    memberIds: string[],
    appIds?: string[]
  ) => Promise<ClientConversation>
  createGroupConversation: (
    name: string,
    memberIds: string[],
    appIds?: string[]
  ) => Promise<ClientConversation>
  createProject: (
    name: string,
    groupIds?: string[]
  ) => Promise<ClientProjectDetail>
  dissolveGroupConversation: (conversationId: string) => Promise<void>
  dismissConversation: (conversationId: string) => Promise<void>
  compactConversationMessages: (conversationId: string) => void
  registerConversationMessageView: (conversationId: string) => () => void
  ensureConversationMessages: (conversationId: string) => void
  getConversation: (conversationId: string) => ClientConversation | null
  getConversationMessageState: (
    conversationId: string
  ) => ClientConversationMessageState
  loadBeforeConversationMessages: (conversationId: string) => void
  markConversationRead: (
    conversationId: string,
    options?: MarkConversationReadOptions
  ) => Promise<void>
  setConversationPinned: (
    conversationId: string,
    pinned: boolean
  ) => Promise<void>
  setConversationMuted: (
    conversationId: string,
    muted: boolean
  ) => Promise<void>
  handleIncomingConversationMessage: (
    message: ClientMessage,
    options?: { activeConversationId?: string; visible?: boolean }
  ) => void
  handleIncomingConversationMessageUpdate: (message: ClientMessage) => void
  handleIncomingMessageReactionsUpdate: (
    event: MessageReactionsUpdatedEvent
  ) => void
  handleIncomingMessageChoiceUpdate: (event: MessageChoiceUpdatedEvent) => void
  respondToChoice: (
    conversationId: string,
    messageId: string,
    optionIds: string[]
  ) => Promise<void>
  updateConversationLastChoiceSeq: (
    conversationId: string,
    lastChoiceSeq: number
  ) => void
  updateConversationLastMentionedSeq: (
    conversationId: string,
    lastMentionedSeq: number
  ) => void
  updateMessageTopic?: (
    parentConversationId: string,
    sourceMessageId: string,
    topic: Pick<ClientMessageTopic, "archived" | "conversationId">
  ) => void
  mergeIncomingConversationMessage: (
    message: ClientMessage,
    options?: { markLoaded?: boolean; updateList?: boolean }
  ) => void
  openDirectConversation: (userId: string) => Promise<ClientConversation>
  openAppConversation: (appId: string) => Promise<ClientConversation>
  restoreConversation: (conversationId: string) => Promise<ClientConversation>
  joinGroupConversation: (conversationId: string) => Promise<ClientConversation>
  leaveGroupConversation: (conversationId: string) => Promise<void>
  removeConversation: (conversationId: string) => void
  removeGroupConversationMember: (
    conversationId: string,
    memberId: string,
    memberType?: "user" | "app"
  ) => Promise<ClientConversation>
  revokeConversationMessage: (
    conversationId: string,
    messageId: string
  ) => Promise<void>
  setMessageReaction: (
    conversationId: string,
    messageId: string,
    text: string,
    reacted: boolean
  ) => Promise<MessageReactionSnapshot>
  setGroupConversationPublic: (
    conversationId: string
  ) => Promise<ClientConversation>
  setGroupConversationPrivate: (
    conversationId: string
  ) => Promise<ClientConversation>
  updateGroupConversationName: (
    conversationId: string,
    name: string
  ) => Promise<ClientConversation>
  refreshConversations: () => Promise<void>
  refreshContacts: () => Promise<void>
  refreshMe: () => Promise<void>
  refreshProjects: () => Promise<void>
  loadMoreProjects: () => Promise<void>
  sendConversationText: (
    conversationId: string,
    content: string,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationMarkdown: (
    conversationId: string,
    content: string,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationLink: (
    conversationId: string,
    url: string,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationCard: (
    conversationId: string,
    card: ClientCardSendInput,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationFile: (
    conversationId: string,
    file: File,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationImage: (
    conversationId: string,
    image: File,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  sendConversationVoice: (
    conversationId: string,
    voice: VoiceMessageRecording,
    options?: SendConversationMessageOptions
  ) => Promise<ClientMessage | null>
  setForegroundConversationId?: (conversationId: string) => void
  syncLoadedConversationMessages: () => void
  updateConversationLastMessage: (message: ClientMessage) => void
  updateConversationPinned: (conversationId: string, pinned: boolean) => void
  updateConversationMuted: (conversationId: string, muted: boolean) => void
  updateGroupConversationAvatar: (
    conversationId: string,
    file: File
  ) => Promise<ClientConversation>
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
