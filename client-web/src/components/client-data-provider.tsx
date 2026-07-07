import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router"
import { toast } from "sonner"

import {
  addGroupConversationMembers as addGroupConversationMembersRequest,
  ClientDataRequestError,
  createDirectConversation,
  createGroupConversation as createGroupConversationRequest,
  formatClientMessageBodySummary,
  getCurrentClientUser,
  isClientMessageInitiatedByUser,
  listClientContacts,
  listClientConversations,
  listConversationMessages,
  markConversationRead as markConversationReadRequest,
  sendConversationFileMessage,
  sendConversationTextMessage,
  uploadGroupConversationAvatar as uploadGroupConversationAvatarRequest,
  type ClientConversation,
  type ClientMessage,
  type ClientMessagePage,
  type ClientUser,
  type ContactUser,
  type MarkConversationReadOptions,
} from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientConversationMessageState,
  type ClientDataContextValue,
} from "@/lib/client-data-context"
import { createClientMessageId } from "@/lib/message-id"
import { Button } from "@/components/ui/button"
import { ClientLoadingPage } from "@/components/client-loading-page"

type BootstrapState = "loading" | "ready" | "error"

const minimumBootstrapLoadingMs = 2_000
const messagePageLimit = 20
const refreshIntervalMs = 60_000
const emptyConversationMessageState: ClientConversationMessageState = {
  error: null,
  loaded: false,
  loading: false,
  loadingBefore: false,
  messages: [],
  page: null,
  sending: false,
}

export function ClientDataProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const [bootstrapError, setBootstrapError] =
    useState<ClientDataRequestError | null>(null)
  const [bootstrapState, setBootstrapState] =
    useState<BootstrapState>("loading")
  const [conversations, setConversations] = useState<ClientConversation[]>([])
  const [conversationMessageStates, setConversationMessageStates] = useState<
    Record<string, ClientConversationMessageState>
  >({})
  const [contacts, setContacts] = useState<ContactUser[]>([])
  const [contactsError, setContactsError] =
    useState<ClientDataRequestError | null>(null)
  const [contactsLoading, setContactsLoading] = useState(true)
  const [contactsRefreshing, setContactsRefreshing] = useState(false)
  const [me, setMe] = useState<ClientUser | null>(null)
  const [meError, setMeError] = useState<ClientDataRequestError | null>(null)
  const [meLoading, setMeLoading] = useState(true)
  const [meRefreshing, setMeRefreshing] = useState(false)
  const conversationMessageStatesRef = useRef(conversationMessageStates)
  const conversationsRef = useRef(conversations)
  const loadingConversationIdsRef = useRef<Set<string>>(new Set())
  const syncingAfterConversationIdsRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    conversationMessageStatesRef.current = conversationMessageStates
  }, [conversationMessageStates])

  useEffect(() => {
    conversationsRef.current = conversations
  }, [conversations])

  const handleError = useCallback(
    (error: unknown, fallbackMessage: string) => {
      const requestError =
        error instanceof ClientDataRequestError
          ? error
          : new ClientDataRequestError(fallbackMessage)

      if (requestError.status === 401 || requestError.code === "unauthorized") {
        setConversations([])
        setConversationMessageStates({})
        setContacts([])
        setMe(null)
        navigate("/login", { replace: true })
      }

      return requestError
    },
    [navigate]
  )

  const refreshMe = useCallback(async () => {
    const isInitialLoad = me === null
    setMeError(null)
    setMeLoading(isInitialLoad)
    setMeRefreshing(!isInitialLoad)

    try {
      setMe(await getCurrentClientUser())
    } catch (error) {
      const requestError = handleError(error, "加载当前用户失败")
      setMeError(requestError)
      throw requestError
    } finally {
      setMeLoading(false)
      setMeRefreshing(false)
    }
  }, [handleError, me])

  const refreshContacts = useCallback(async () => {
    const isInitialLoad = contacts.length === 0
    setContactsError(null)
    setContactsLoading(isInitialLoad)
    setContactsRefreshing(!isInitialLoad)

    try {
      setContacts(await listClientContacts())
    } catch (error) {
      const requestError = handleError(error, "加载通讯录失败")
      setContactsError(requestError)
      throw requestError
    } finally {
      setContactsLoading(false)
      setContactsRefreshing(false)
    }
  }, [contacts.length, handleError])

  const refreshConversations = useCallback(async () => {
    try {
      setConversations(await listClientConversations())
    } catch (error) {
      throw handleError(error, "加载会话列表失败")
    }
  }, [handleError])

  const updateConversationMessageState = useCallback(
    (
      conversationId: string,
      updater: (
        state: ClientConversationMessageState
      ) => ClientConversationMessageState
    ) => {
      setConversationMessageStates((currentStates) => {
        const previousState =
          currentStates[conversationId] ?? createConversationMessageState()
        const nextState = updater(previousState)

        return {
          ...currentStates,
          [conversationId]: nextState,
        }
      })
    },
    []
  )

  const applyConversationMessageToList = useCallback(
    (message: ClientMessage, options: { countUnread?: boolean } = {}) => {
      const conversationExists = conversationsRef.current.some(
        (conversation) => conversation.id === message.conversationId
      )

      setConversations((currentConversations) => {
        const conversation = currentConversations.find(
          (currentConversation) =>
            currentConversation.id === message.conversationId
        )

        if (!conversation) {
          return currentConversations
        }

        if (message.seq < conversation.lastMessageSeq) {
          return currentConversations
        }

        const shouldIncrementUnread =
          Boolean(options.countUnread) &&
          message.seq > conversation.lastMessageSeq &&
          message.seq > conversation.lastReadSeq
        const updatedConversation: ClientConversation = {
          ...conversation,
          lastMessageAt: message.createdAt,
          lastMessageId: message.id,
          lastMessageSeq: message.seq,
          lastMessageSummary: getMessageSummary(message),
          unreadCount: shouldIncrementUnread
            ? conversation.unreadCount + 1
            : conversation.unreadCount,
        }

        return [
          updatedConversation,
          ...currentConversations.filter(
            (currentConversation) =>
              currentConversation.id !== message.conversationId
          ),
        ]
      })

      if (!conversationExists) {
        void refreshConversations().catch(() => undefined)
      }
    },
    [refreshConversations]
  )

  const updateConversationLastMessage = useCallback(
    (message: ClientMessage) => {
      applyConversationMessageToList(message)
    },
    [applyConversationMessageToList]
  )

  const rememberConversationMessage = useCallback(
    (message: ClientMessage) => {
      applyConversationMessageToList(message)
    },
    [applyConversationMessageToList]
  )

  const mergeIncomingConversationMessage = useCallback(
    (
      message: ClientMessage,
      options: { markLoaded?: boolean; updateList?: boolean } = {}
    ) => {
      updateConversationMessageState(message.conversationId, (state) => {
        const messages = mergeConversationMessages(state.messages, [message])

        return {
          ...state,
          error: null,
          loaded: options.markLoaded ? true : state.loaded,
          messages,
          page: updatePageWithMessage(state.page, messages),
        }
      })
      if (options.updateList !== false) {
        rememberConversationMessage(message)
      }
    },
    [rememberConversationMessage, updateConversationMessageState]
  )

  const currentUserId = me?.id ?? ""
  const handleIncomingConversationMessage = useCallback(
    (
      message: ClientMessage,
      options: { activeConversationId?: string; visible?: boolean } = {}
    ) => {
      const fromCurrentUser =
        currentUserId !== "" &&
        isClientMessageInitiatedByUser(message, currentUserId)
      const visibleInActiveConversation =
        Boolean(options.visible) &&
        options.activeConversationId === message.conversationId

      mergeIncomingConversationMessage(message, { updateList: false })
      applyConversationMessageToList(message, {
        countUnread: !fromCurrentUser && !visibleInActiveConversation,
      })
    },
    [
      applyConversationMessageToList,
      currentUserId,
      mergeIncomingConversationMessage,
    ]
  )

  const markConversationRead = useCallback(
    async (
      conversationId: string,
      options: MarkConversationReadOptions = {}
    ) => {
      if (!conversationId) {
        return
      }

      try {
        const result = await markConversationReadRequest(
          conversationId,
          options
        )
        setConversations((currentConversations) =>
          currentConversations.map((conversation) =>
            conversation.id === result.conversationId
              ? {
                  ...conversation,
                  lastReadSeq: result.lastReadSeq,
                  unreadCount: result.unreadCount,
                }
              : conversation
          )
        )
      } catch (error) {
        throw handleError(error, "标记会话已读失败")
      }
    },
    [handleError]
  )

  const ensureConversationMessages = useCallback(
    (conversationId: string) => {
      if (!conversationId) {
        return
      }

      const state = conversationMessageStatesRef.current[conversationId]
      if (
        state?.loaded ||
        state?.loading ||
        loadingConversationIdsRef.current.has(conversationId)
      ) {
        return
      }

      loadingConversationIdsRef.current.add(conversationId)
      updateConversationMessageState(conversationId, (currentState) => ({
        ...currentState,
        error: null,
        loading: true,
      }))

      void listConversationMessages(conversationId, {
        limit: messagePageLimit,
      })
        .then((result) => {
          updateConversationMessageState(conversationId, (currentState) => ({
            ...currentState,
            error: null,
            loaded: true,
            loading: false,
            messages: mergeConversationMessages(
              currentState.messages,
              result.messages
            ),
            page: result.page,
          }))
        })
        .catch((error: unknown) => {
          const message = getClientDataErrorMessage(error, "加载消息失败")
          updateConversationMessageState(conversationId, (currentState) => ({
            ...currentState,
            error: message,
            loaded: false,
            loading: false,
          }))
          toast.error(message)
        })
        .finally(() => {
          loadingConversationIdsRef.current.delete(conversationId)
        })
    },
    [updateConversationMessageState]
  )

  const loadBeforeConversationMessages = useCallback(
    (conversationId: string) => {
      const state = conversationMessageStatesRef.current[conversationId]
      if (!state?.page?.hasMoreBefore || !state.loaded || state.loadingBefore) {
        return
      }

      const beforeSeq = state.page.oldestSeq
      updateConversationMessageState(conversationId, (currentState) => ({
        ...currentState,
        error: null,
        loadingBefore: true,
      }))

      void listConversationMessages(conversationId, {
        beforeSeq,
        limit: messagePageLimit,
      })
        .then((result) => {
          updateConversationMessageState(conversationId, (currentState) => {
            const messages = mergeConversationMessages(
              currentState.messages,
              result.messages
            )

            return {
              ...currentState,
              error: null,
              loaded: true,
              loadingBefore: false,
              messages,
              page: mergePageWithBeforeResult(
                currentState.page,
                result.page,
                messages
              ),
            }
          })
        })
        .catch((error: unknown) => {
          const message = getClientDataErrorMessage(error, "加载更早消息失败")
          updateConversationMessageState(conversationId, (currentState) => ({
            ...currentState,
            error: message,
            loadingBefore: false,
          }))
          toast.error(message)
        })
    },
    [updateConversationMessageState]
  )

  const syncAfterConversationMessages = useCallback(
    (conversationId: string, afterSeq: number) => {
      if (syncingAfterConversationIdsRef.current.has(conversationId)) {
        return
      }

      syncingAfterConversationIdsRef.current.add(conversationId)

      void listConversationMessages(conversationId, {
        afterSeq,
        limit: messagePageLimit,
      })
        .then((result) => {
          const lastReceivedMessage =
            result.messages[result.messages.length - 1]
          updateConversationMessageState(conversationId, (currentState) => {
            const messages = mergeConversationMessages(
              currentState.messages,
              result.messages
            )

            return {
              ...currentState,
              error: null,
              messages,
              page: mergePageWithAfterResult(
                currentState.page,
                result.page,
                messages
              ),
            }
          })

          if (lastReceivedMessage) {
            rememberConversationMessage(lastReceivedMessage)
          }
        })
        .catch((error: unknown) => {
          toast.error(getClientDataErrorMessage(error, "同步新消息失败"))
        })
        .finally(() => {
          syncingAfterConversationIdsRef.current.delete(conversationId)
        })
    },
    [rememberConversationMessage, updateConversationMessageState]
  )

  const syncLoadedConversationMessages = useCallback(() => {
    for (const [conversationId, state] of Object.entries(
      conversationMessageStatesRef.current
    )) {
      if (!state.loaded) {
        continue
      }

      const newestSeq = getNewestMessageSeq(state)
      if (newestSeq > 0) {
        syncAfterConversationMessages(conversationId, newestSeq)
      }
    }
  }, [syncAfterConversationMessages])

  const sendConversationText = useCallback(
    async (conversationId: string, content: string) => {
      const trimmedContent = content.trim()
      const state = conversationMessageStatesRef.current[conversationId]
      if (!conversationId || !trimmedContent || state?.sending) {
        return null
      }

      const clientMessageId = createClientMessageId()
      updateConversationMessageState(conversationId, (currentState) => ({
        ...currentState,
        sending: true,
      }))

      try {
        const message = await sendConversationTextMessage(conversationId, {
          clientMessageId,
          content: trimmedContent,
        })
        mergeIncomingConversationMessage(message, { markLoaded: true })
        return message
      } catch (error: unknown) {
        toast.error(getClientDataErrorMessage(error, "发送消息失败"))
        return null
      } finally {
        updateConversationMessageState(conversationId, (currentState) => ({
          ...currentState,
          sending: false,
        }))
      }
    },
    [mergeIncomingConversationMessage, updateConversationMessageState]
  )

  const sendConversationFile = useCallback(
    async (conversationId: string, file: File) => {
      const state = conversationMessageStatesRef.current[conversationId]
      if (!conversationId || state?.sending) {
        return null
      }

      const clientMessageId = createClientMessageId()
      updateConversationMessageState(conversationId, (currentState) => ({
        ...currentState,
        sending: true,
      }))

      try {
        const message = await sendConversationFileMessage(conversationId, {
          clientMessageId,
          file,
        })
        mergeIncomingConversationMessage(message, { markLoaded: true })
        return message
      } catch (error: unknown) {
        toast.error(getClientDataErrorMessage(error, "发送文件失败"))
        return null
      } finally {
        updateConversationMessageState(conversationId, (currentState) => ({
          ...currentState,
          sending: false,
        }))
      }
    },
    [mergeIncomingConversationMessage, updateConversationMessageState]
  )

  const getConversationMessageState = useCallback(
    (conversationId: string) => {
      return (
        conversationMessageStates[conversationId] ??
        emptyConversationMessageState
      )
    },
    [conversationMessageStates]
  )

  const getConversation = useCallback(
    (conversationId: string) => {
      return (
        conversations.find(
          (conversation) => conversation.id === conversationId
        ) ?? null
      )
    },
    [conversations]
  )

  const upsertConversation = useCallback((conversation: ClientConversation) => {
    setConversations((currentConversations) => [
      conversation,
      ...currentConversations.filter(
        (currentConversation) => currentConversation.id !== conversation.id
      ),
    ])
  }, [])

  const openDirectConversation = useCallback(
    async (userId: string) => {
      try {
        const conversation = await createDirectConversation(userId)
        upsertConversation(conversation)
        return conversation
      } catch (error) {
        throw handleError(error, "创建一对一会话失败")
      }
    },
    [handleError, upsertConversation]
  )

  const createGroupConversation = useCallback(
    async (name: string, memberIds: string[]) => {
      try {
        const conversation = await createGroupConversationRequest({
          memberIds,
          name,
        })
        upsertConversation(conversation)
        return conversation
      } catch (error) {
        throw handleError(error, "创建群聊失败")
      }
    },
    [handleError, upsertConversation]
  )

  const addGroupConversationMembers = useCallback(
    async (conversationId: string, memberIds: string[]) => {
      try {
        const result = await addGroupConversationMembersRequest(
          conversationId,
          {
            memberIds,
          }
        )
        upsertConversation(result.conversation)
        if (result.message) {
          mergeIncomingConversationMessage(result.message, { markLoaded: true })
        }
        return result.conversation
      } catch (error) {
        throw handleError(error, "添加群聊成员失败")
      }
    },
    [handleError, mergeIncomingConversationMessage, upsertConversation]
  )

  const updateGroupConversationAvatar = useCallback(
    async (conversationId: string, file: File) => {
      try {
        const result = await uploadGroupConversationAvatarRequest(
          conversationId,
          file
        )
        upsertConversation(result.conversation)
        mergeIncomingConversationMessage(result.message, { markLoaded: true })
        return result.conversation
      } catch (error) {
        throw handleError(error, "上传群头像失败")
      }
    },
    [handleError, mergeIncomingConversationMessage, upsertConversation]
  )

  const bootstrap = useCallback(async () => {
    const minimumLoading = wait(minimumBootstrapLoadingMs)

    try {
      const [nextMe, nextContacts, nextConversations] = await Promise.all([
        getCurrentClientUser(),
        listClientContacts(),
        listClientConversations(),
      ])

      await minimumLoading
      setMe(nextMe)
      setContacts(nextContacts)
      setConversations(nextConversations)
      setBootstrapState("ready")
    } catch (error) {
      const requestError = handleError(error, "加载工作区失败")

      if (requestError.status !== 401 && requestError.code !== "unauthorized") {
        await minimumLoading
      }

      setBootstrapError(requestError)
      setBootstrapState("error")
    } finally {
      setMeLoading(false)
      setContactsLoading(false)
    }
  }, [handleError])

  const retryBootstrap = useCallback(async () => {
    setBootstrapError(null)
    setBootstrapState("loading")
    setConversations([])
    setConversationMessageStates({})
    setContactsError(null)
    setContactsLoading(true)
    setContactsRefreshing(false)
    setMeError(null)
    setMeLoading(true)
    setMeRefreshing(false)

    await bootstrap()
  }, [bootstrap])

  useEffect(() => {
    let active = true

    void Promise.resolve().then(() => {
      if (active) {
        return bootstrap()
      }

      return undefined
    })

    return () => {
      active = false
    }
  }, [bootstrap])

  useEffect(() => {
    if (bootstrapState !== "ready") {
      return
    }

    function refresh() {
      void refreshMe().catch(() => undefined)
      void refreshContacts().catch(() => undefined)
    }

    const interval = window.setInterval(refresh, refreshIntervalMs)

    function handleVisibilityChange() {
      if (document.visibilityState === "visible") {
        refresh()
      }
    }

    document.addEventListener("visibilitychange", handleVisibilityChange)

    return () => {
      window.clearInterval(interval)
      document.removeEventListener("visibilitychange", handleVisibilityChange)
    }
  }, [bootstrapState, refreshContacts, refreshMe])

  if (bootstrapState === "loading") {
    return <ClientLoadingPage />
  }

  if (bootstrapState === "error") {
    return (
      <ClientDataErrorPage
        message={bootstrapError?.message ?? "加载工作区失败"}
        onRetry={() => void retryBootstrap()}
      />
    )
  }

  if (!me) {
    return <ClientLoadingPage />
  }

  const value: ClientDataContextValue = {
    addGroupConversationMembers,
    conversations,
    contacts,
    contactsError,
    contactsLoading,
    contactsRefreshing,
    createGroupConversation,
    ensureConversationMessages,
    getConversation,
    getConversationMessageState,
    loadBeforeConversationMessages,
    markConversationRead,
    handleIncomingConversationMessage,
    me,
    meError,
    meLoading,
    meRefreshing,
    mergeIncomingConversationMessage,
    openDirectConversation,
    refreshConversations,
    refreshContacts,
    refreshMe,
    sendConversationFile,
    sendConversationText,
    syncLoadedConversationMessages,
    updateConversationLastMessage,
    updateGroupConversationAvatar,
  }

  return (
    <ClientDataContext.Provider value={value}>
      {children}
    </ClientDataContext.Provider>
  )
}

function wait(ms: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

function getMessageSummary(message: ClientMessage) {
  return formatClientMessageBodySummary(message.body)
}

function createConversationMessageState(): ClientConversationMessageState {
  return {
    error: null,
    loaded: false,
    loading: false,
    loadingBefore: false,
    messages: [],
    page: null,
    sending: false,
  }
}

function mergeConversationMessages(
  currentMessages: ClientMessage[],
  nextMessages: ClientMessage[]
) {
  const messagesById = new Map<string, ClientMessage>()

  for (const message of currentMessages) {
    messagesById.set(message.id, message)
  }
  for (const message of nextMessages) {
    messagesById.set(message.id, message)
  }

  return Array.from(messagesById.values()).sort((messageA, messageB) => {
    if (messageA.seq !== messageB.seq) {
      return messageA.seq - messageB.seq
    }

    return messageA.createdAt.localeCompare(messageB.createdAt)
  })
}

function updatePageWithMessage(
  page: ClientMessagePage | null,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: false,
    hasMoreBefore: page?.hasMoreBefore ?? false,
    limit: page?.limit ?? messagePageLimit,
    newestSeq: lastMessage?.seq ?? 0,
    oldestSeq: firstMessage?.seq ?? 0,
  }
}

function mergePageWithBeforeResult(
  currentPage: ClientMessagePage | null,
  resultPage: ClientMessagePage,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: currentPage?.hasMoreAfter ?? resultPage.hasMoreAfter,
    hasMoreBefore: resultPage.hasMoreBefore,
    limit: resultPage.limit,
    newestSeq: lastMessage?.seq ?? currentPage?.newestSeq ?? 0,
    oldestSeq: firstMessage?.seq ?? resultPage.oldestSeq,
  }
}

function mergePageWithAfterResult(
  currentPage: ClientMessagePage | null,
  resultPage: ClientMessagePage,
  messages: ClientMessage[]
): ClientMessagePage {
  const firstMessage = messages[0]
  const lastMessage = messages[messages.length - 1]

  return {
    hasMoreAfter: resultPage.hasMoreAfter,
    hasMoreBefore: currentPage?.hasMoreBefore ?? resultPage.hasMoreBefore,
    limit: resultPage.limit,
    newestSeq: lastMessage?.seq ?? resultPage.newestSeq,
    oldestSeq: firstMessage?.seq ?? currentPage?.oldestSeq ?? 0,
  }
}

function getNewestMessageSeq(state: ClientConversationMessageState) {
  const lastMessage = state.messages[state.messages.length - 1]

  return Math.max(state.page?.newestSeq ?? 0, lastMessage?.seq ?? 0)
}

function getClientDataErrorMessage(error: unknown, fallbackMessage: string) {
  if (error instanceof ClientDataRequestError) {
    return error.message
  }

  return fallbackMessage
}

function ClientDataErrorPage({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="flex h-svh items-center justify-center bg-background px-4 text-foreground">
      <div className="flex max-w-sm flex-col items-center gap-3 text-center">
        <h1 className="text-base font-medium">工作区加载失败</h1>
        <p className="text-sm text-muted-foreground">{message}</p>
        <Button onClick={onRetry} type="button" variant="outline">
          重试
        </Button>
      </div>
    </div>
  )
}
