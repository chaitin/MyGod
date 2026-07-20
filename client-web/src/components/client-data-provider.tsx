import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router"
import { toast } from "sonner"

import {
  ClientDataRequestError,
  getCurrentClientUser,
  isClientMessageInitiatedByUser,
  listClientContacts,
  listClientConversations,
  listConversationMessages,
  markConversationRead as markConversationReadRequest,
  setConversationPinned as setConversationPinnedRequest,
  type ClientConversation,
  type ClientMessage,
  type ClientMessageTopic,
  type ClientUser,
  type ContactApp,
  type ContactGroup,
  type ContactUser,
  type MarkConversationReadOptions,
} from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientConversationMessageState,
  type ClientDataContextValue,
} from "@/lib/client-data-context"
import {
  createConversationMessageState,
  getClientDataErrorMessage,
  getMessageSummary,
  getNewestMessageSeq,
  mergeConversationMessages,
  mergePageWithAfterResult,
  mergePageWithBeforeResult,
  messagePageLimit,
  orderConversations,
  updatePageWithMessage,
} from "@/lib/client-data-state"
import {
  createClientProject as createClientProjectRequest,
  listClientProjects,
  type ClientProjectDetail,
  type ClientProjectSummary,
} from "@/lib/project-data-api"
import { Button } from "@/components/ui/button"
import { ClientLoadingPage } from "@/components/client-loading-page"
import { useConversationActions } from "@/hooks/use-conversation-actions"
import { useConversationSenders } from "@/hooks/use-conversation-senders"
import { useAppInfo } from "@/lib/app-info-context"

type BootstrapState = "loading" | "ready" | "error"

const minimumBootstrapLoadingMs = 1_000
const refreshIntervalMs = 15_000

export function ClientDataProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const { setAuthenticated } = useAppInfo()
  const [bootstrapError, setBootstrapError] =
    useState<ClientDataRequestError | null>(null)
  const [bootstrapState, setBootstrapState] =
    useState<BootstrapState>("loading")
  const [conversations, setConversations] = useState<ClientConversation[]>([])
  const [conversationMessageStates, setConversationMessageStates] = useState<
    Record<string, ClientConversationMessageState>
  >({})
  const [foregroundConversationId, setForegroundConversationId] = useState("")
  const [contactApps, setContactApps] = useState<ContactApp[]>([])
  const [contactGroups, setContactGroups] = useState<ContactGroup[]>([])
  const [contacts, setContacts] = useState<ContactUser[]>([])
  const [contactsError, setContactsError] =
    useState<ClientDataRequestError | null>(null)
  const [contactsLoading, setContactsLoading] = useState(true)
  const [contactsRefreshing, setContactsRefreshing] = useState(false)
  const [me, setMe] = useState<ClientUser | null>(null)
  const [meError, setMeError] = useState<ClientDataRequestError | null>(null)
  const [meLoading, setMeLoading] = useState(true)
  const [meRefreshing, setMeRefreshing] = useState(false)
  const [personalProject, setPersonalProject] =
    useState<ClientProjectSummary | null>(null)
  const [projects, setProjects] = useState<ClientProjectSummary[]>([])
  const [projectsError, setProjectsError] =
    useState<ClientDataRequestError | null>(null)
  const [projectsLoading, setProjectsLoading] = useState(true)
  const [projectsLoadingMore, setProjectsLoadingMore] = useState(false)
  const [projectsNextCursor, setProjectsNextCursor] = useState<string | null>(
    null
  )
  const [projectsRefreshing, setProjectsRefreshing] = useState(false)
  const conversationMessageStatesRef = useRef(conversationMessageStates)
  const conversationsRef = useRef(conversations)
  const mountedRef = useRef(true)
  const loadingConversationIdsRef = useRef<Set<string>>(new Set())
  const syncingAfterConversationIdsRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    conversationMessageStatesRef.current = conversationMessageStates
  }, [conversationMessageStates])

  useEffect(() => {
    conversationsRef.current = conversations
  }, [conversations])

  useEffect(() => {
    mountedRef.current = true

    return () => {
      mountedRef.current = false
    }
  }, [])

  const handleError = useCallback(
    (error: unknown, fallbackMessage: string) => {
      const requestError =
        error instanceof ClientDataRequestError
          ? error
          : new ClientDataRequestError(fallbackMessage)

      if (requestError.status === 401 || requestError.code === "unauthorized") {
        if (!mountedRef.current) {
          return requestError
        }

        setAuthenticated(false)
        setConversations([])
        setConversationMessageStates({})
        setContactApps([])
        setContactGroups([])
        setContacts([])
        setPersonalProject(null)
        setProjects([])
        setMe(null)
        navigate("/login", { replace: true })
      }

      return requestError
    },
    [navigate, setAuthenticated]
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
    const isInitialLoad =
      contacts.length === 0 &&
      contactApps.length === 0 &&
      contactGroups.length === 0
    setContactsError(null)
    setContactsLoading(isInitialLoad)
    setContactsRefreshing(!isInitialLoad)

    try {
      const nextContacts = await listClientContacts()
      setContactApps(nextContacts.apps)
      setContactGroups(nextContacts.groups)
      setContacts(nextContacts.users)
    } catch (error) {
      const requestError = handleError(error, "加载通讯录失败")
      setContactsError(requestError)
      throw requestError
    } finally {
      setContactsLoading(false)
      setContactsRefreshing(false)
    }
  }, [contactApps.length, contactGroups.length, contacts.length, handleError])

  const refreshConversations = useCallback(async () => {
    try {
      setConversations(orderConversations(await listClientConversations()))
    } catch (error) {
      throw handleError(error, "加载会话列表失败")
    }
  }, [handleError])

  const refreshProjects = useCallback(async () => {
    const isInitialLoad = personalProject === null && projects.length === 0
    setProjectsError(null)
    setProjectsLoading(isInitialLoad)
    setProjectsRefreshing(!isInitialLoad)

    try {
      const page = await listClientProjects({ limit: 100 })
      setPersonalProject(page.personalProject)
      setProjects(page.projects)
      setProjectsNextCursor(page.nextCursor)
    } catch (error) {
      const requestError = handleError(error, "加载项目列表失败")
      setProjectsError(requestError)
      throw requestError
    } finally {
      setProjectsLoading(false)
      setProjectsRefreshing(false)
    }
  }, [handleError, personalProject, projects.length])

  const loadMoreProjects = useCallback(async () => {
    if (!projectsNextCursor || projectsLoadingMore) {
      return
    }

    setProjectsLoadingMore(true)
    try {
      const page = await listClientProjects({
        cursor: projectsNextCursor,
        limit: 100,
      })
      setPersonalProject(page.personalProject)
      setProjects((currentProjects) => {
        const projectById = new Map(
          currentProjects.map((project) => [project.id, project])
        )

        for (const project of page.projects) {
          projectById.set(project.id, project)
        }

        return Array.from(projectById.values())
      })
      setProjectsNextCursor(page.nextCursor)
    } catch (error) {
      throw handleError(error, "加载更多项目失败")
    } finally {
      setProjectsLoadingMore(false)
    }
  }, [handleError, projectsLoadingMore, projectsNextCursor])

  const createProject = useCallback(
    async (name: string, groupIds: string[] = []) => {
      let project: ClientProjectDetail

      try {
        project = await createClientProjectRequest({ groupIds, name })
      } catch (error) {
        throw handleError(error, "创建项目失败")
      }

      try {
        await refreshProjects()
      } catch {
        throw new ClientDataRequestError("项目已创建，但刷新项目列表失败")
      }

      return project
    },
    [handleError, refreshProjects]
  )

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

        return orderConversations([
          updatedConversation,
          ...currentConversations.filter(
            (currentConversation) =>
              currentConversation.id !== message.conversationId
          ),
        ])
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

  const updateTopicSourcePreview = useCallback((message: ClientMessage) => {
    const topicConversation = conversationsRef.current.find(
      (conversation) =>
        conversation.id === message.conversationId &&
        conversation.type === "topic"
    )
    const topic = topicConversation?.topic
    if (!topic || message.sender.type === "system") {
      return
    }

    setConversationMessageStates((currentStates) => {
      const parentState = currentStates[topic.parentConversationId]
      if (!parentState) {
        return currentStates
      }
      let changed = false
      const messages = parentState.messages.map((sourceMessage) => {
        if (
          sourceMessage.id !== topic.sourceMessageId ||
          !sourceMessage.topic
        ) {
          return sourceMessage
        }
        const existingReplies = (
          sourceMessage.topic.recentReplies ?? []
        ).filter((reply) => reply.id !== message.id)
        const recentReplies =
          message.body.type === "revoked"
            ? existingReplies
            : [
                ...existingReplies,
                {
                  createdAt: message.createdAt,
                  id: message.id,
                  sender: message.sender,
                  summary: getMessageSummary(message),
                },
              ].slice(-3)
        changed = true
        return {
          ...sourceMessage,
          topic: { ...sourceMessage.topic, recentReplies },
        }
      })
      return changed
        ? {
            ...currentStates,
            [topic.parentConversationId]: { ...parentState, messages },
          }
        : currentStates
    })
  }, [])

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
      updateTopicSourcePreview(message)
      if (options.updateList !== false) {
        rememberConversationMessage(message)
      }
    },
    [
      rememberConversationMessage,
      updateConversationMessageState,
      updateTopicSourcePreview,
    ]
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

  const handleIncomingConversationMessageUpdate = useCallback(
    (message: ClientMessage) => {
      setConversationMessageStates((currentStates) => {
        const state = currentStates[message.conversationId]
        if (!state?.messages.some((existing) => existing.id === message.id)) {
          return currentStates
        }

        const messages = mergeConversationMessages(state.messages, [message])

        return {
          ...currentStates,
          [message.conversationId]: {
            ...state,
            error: null,
            messages,
            page: updatePageWithMessage(state.page, messages),
          },
        }
      })
      updateTopicSourcePreview(message)
    },
    [updateTopicSourcePreview]
  )

  const updateConversationLastMentionedSeq = useCallback(
    (conversationId: string, lastMentionedSeq: number) => {
      if (!conversationId || lastMentionedSeq <= 0) {
        return
      }

      setConversations((currentConversations) =>
        currentConversations.map((conversation) =>
          conversation.id === conversationId
            ? {
                ...conversation,
                lastMentionedSeq: Math.max(
                  conversation.lastMentionedSeq,
                  lastMentionedSeq
                ),
              }
            : conversation
        )
      )
    },
    []
  )

  const updateConversationPinned = useCallback(
    (conversationId: string, pinned: boolean) => {
      if (!conversationId) {
        return
      }
      setConversations((currentConversations) =>
        orderConversations(
          currentConversations.map((conversation) =>
            conversation.id === conversationId
              ? { ...conversation, pinned }
              : conversation
          )
        )
      )
    },
    []
  )

  const setConversationPinned = useCallback(
    async (conversationId: string, pinned: boolean) => {
      try {
        const result = await setConversationPinnedRequest(
          conversationId,
          pinned
        )
        updateConversationPinned(result.conversationId, result.pinned)
      } catch (error) {
        throw handleError(error, pinned ? "置顶会话失败" : "取消置顶失败")
      }
    },
    [handleError, updateConversationPinned]
  )

  const updateMessageTopic = useCallback(
    (
      parentConversationId: string,
      sourceMessageId: string,
      topic: Pick<ClientMessageTopic, "archived" | "conversationId">
    ) => {
      setConversations((currentConversations) =>
        topic.archived
          ? currentConversations.filter(
              (conversation) => conversation.id !== topic.conversationId
            )
          : currentConversations.map((conversation) =>
              conversation.id === topic.conversationId && conversation.topic
                ? {
                    ...conversation,
                    topic: { ...conversation.topic, archived: false },
                  }
                : conversation
            )
      )
      setConversationMessageStates((currentStates) => {
        const state = currentStates[parentConversationId]
        if (!state) {
          return currentStates
        }
        let changed = false
        const messages = state.messages.map((message) => {
          if (message.id !== sourceMessageId) {
            return message
          }
          changed = true
          return {
            ...message,
            topic: {
              ...message.topic,
              ...topic,
              recentReplies: message.topic?.recentReplies ?? [],
            },
          }
        })
        return changed
          ? {
              ...currentStates,
              [parentConversationId]: { ...state, messages },
            }
          : currentStates
      })
    },
    []
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

  const {
    sendConversationFile,
    sendConversationImage,
    sendConversationLink,
    sendConversationMarkdown,
    sendConversationCard,
    sendConversationText,
    sendConversationVoice,
  } = useConversationSenders({
    conversationMessageStatesRef,
    mergeIncomingConversationMessage,
    updateConversationMessageState,
  })

  const {
    addGroupConversationMembers,
    createGroupConversation,
    dissolveGroupConversation,
    getConversation,
    getConversationMessageState,
    joinGroupConversation,
    leaveGroupConversation,
    openAppConversation,
    openDirectConversation,
    removeConversation,
    removeGroupConversationMember,
    revokeConversationMessage,
    setGroupConversationPrivate,
    setGroupConversationPublic,
    updateGroupConversationAvatar,
    updateGroupConversationName,
  } = useConversationActions({
    conversations,
    conversationMessageStates,
    handleError,
    mergeIncomingConversationMessage,
    navigate,
    refreshContacts,
    setConversationMessageStates,
    setConversations,
  })

  const bootstrap = useCallback(async () => {
    const minimumLoading = wait(minimumBootstrapLoadingMs)

    try {
      const [nextMe, nextContacts, nextConversations, nextProjects] =
        await Promise.all([
          getCurrentClientUser(),
          listClientContacts(),
          listClientConversations(),
          listClientProjects({ limit: 100 }),
        ])

      await minimumLoading
      setMe(nextMe)
      setContactApps(nextContacts.apps)
      setContactGroups(nextContacts.groups)
      setContacts(nextContacts.users)
      setConversations(orderConversations(nextConversations))
      setPersonalProject(nextProjects.personalProject)
      setProjects(nextProjects.projects)
      setProjectsNextCursor(nextProjects.nextCursor)
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
      setProjectsLoading(false)
    }
  }, [handleError])

  const retryBootstrap = useCallback(async () => {
    setBootstrapError(null)
    setBootstrapState("loading")
    setConversations([])
    setConversationMessageStates({})
    setContactApps([])
    setContactGroups([])
    setContactsError(null)
    setContactsLoading(true)
    setContactsRefreshing(false)
    setPersonalProject(null)
    setProjects([])
    setProjectsError(null)
    setProjectsLoading(true)
    setProjectsLoadingMore(false)
    setProjectsNextCursor(null)
    setProjectsRefreshing(false)
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
      void refreshConversations().catch(() => undefined)
      void refreshProjects().catch(() => undefined)
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
  }, [
    bootstrapState,
    refreshContacts,
    refreshConversations,
    refreshMe,
    refreshProjects,
  ])

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

  if (!me || !personalProject) {
    return <ClientLoadingPage />
  }

  const value: ClientDataContextValue = {
    addGroupConversationMembers,
    contactApps,
    contactGroups,
    conversations,
    contacts,
    contactsError,
    contactsLoading,
    contactsRefreshing,
    createGroupConversation,
    createProject,
    dissolveGroupConversation,
    ensureConversationMessages,
    foregroundConversationId,
    getConversation,
    getConversationMessageState,
    joinGroupConversation,
    leaveGroupConversation,
    loadBeforeConversationMessages,
    markConversationRead,
    setConversationPinned,
    handleIncomingConversationMessage,
    handleIncomingConversationMessageUpdate,
    me,
    meError,
    meLoading,
    meRefreshing,
    mergeIncomingConversationMessage,
    openAppConversation,
    openDirectConversation,
    personalProject,
    projects,
    projectsError,
    projectsLoading,
    projectsLoadingMore,
    projectsNextCursor,
    projectsRefreshing,
    refreshConversations,
    refreshContacts,
    refreshMe,
    refreshProjects,
    loadMoreProjects,
    removeConversation,
    removeGroupConversationMember,
    revokeConversationMessage,
    sendConversationFile,
    sendConversationImage,
    sendConversationLink,
    sendConversationMarkdown,
    sendConversationCard,
    sendConversationText,
    sendConversationVoice,
    setForegroundConversationId,
    setGroupConversationPrivate,
    setGroupConversationPublic,
    syncLoadedConversationMessages,
    updateConversationLastMessage,
    updateConversationLastMentionedSeq,
    updateConversationPinned,
    updateMessageTopic,
    updateGroupConversationAvatar,
    updateGroupConversationName,
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
