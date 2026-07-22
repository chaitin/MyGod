import {
  type Href,
  useIsFocused,
  useLocalSearchParams,
  useRouter,
} from "expo-router"
import { Ellipsis } from "lucide-react-native"
import { useEffect, useMemo, useRef, useState } from "react"
import { Alert, AppState, BackHandler } from "react-native"
import { useSafeAreaInsets } from "react-native-safe-area-context"
import { SizableText, useToastController, YStack } from "tamagui"

import type { AppToastTone } from "@/components/feedback/app-toast"
import { ContentState } from "@/components/feedback/content-state"
import { KeyboardAwareScreen } from "@/components/layout/keyboard-aware-screen"
import {
  PAGE_HEADER_HEIGHT,
  PageHeader,
} from "@/components/navigation/page-header"
import { ApiRequestError, isUnauthorizedError } from "@/data/api-client"
import {
  useConversationMessages,
  useMarkConversationRead,
  useSendConversationFileMessage,
  useSendConversationImageMessage,
  useSendConversationTextMessage,
  useSendConversationVoiceMessage,
  useSetConversationMessageReaction,
} from "@/data/message-hooks"
import type {
  PreparedClientMessageUpload,
  PreparedClientVoiceMessage,
} from "@/data/message-upload"
import {
  openResourceExternally,
  useMessageResources,
} from "@/data/resources"
import {
  useArchiveConversationTopic,
  useConversationTopic,
} from "@/data/topic-hooks"
import {
  type EntityReference,
  getConversationEntityReference,
} from "@/domain/entities/entity-profile"
import {
  buildPresentedMessages,
  collectMessageResources,
  createMessageMentionLabelResolver,
  type MessageMentionLabelResolver,
} from "@/domain/messages/message-presenter"
import {
  MessageComposer,
  type MessageComposerHandle,
} from "@/features/conversation/message-composer"
import { MessageList } from "@/features/conversation/message-list"
import { createMentionCandidates } from "@/features/conversation/mention-model"
import { TopicArchiveDialog } from "@/features/conversation/topic-archive-dialog"
import {
  useAuth,
  useAuthenticatedSession,
} from "@/features/auth/auth-context"
import {
  buildConversationHref,
  buildTopicConversationHref,
} from "@/navigation/conversations"
import { buildEntityDetailHref } from "@/navigation/entity-details"
import { useClientData } from "@/providers/client-data-provider"
import { useRealtime } from "@/realtime/realtime-context"

const EMPTY_MENTION_RESOLVER: MessageMentionLabelResolver = () => undefined

export function ConversationScreen() {
  const params = useLocalSearchParams<{
    conversationId: string
    parentConversationId?: string
    topic?: string
  }>()
  const conversationId = Array.isArray(params.conversationId)
    ? (params.conversationId[0] ?? "")
    : (params.conversationId ?? "")
  const parentConversationId = Array.isArray(params.parentConversationId)
    ? (params.parentConversationId[0] ?? "")
    : (params.parentConversationId ?? "")
  const router = useRouter()
  const toast = useToastController()
  const isFocused = useIsFocused()
  const insets = useSafeAreaInsets()
  const { invalidateSession } = useAuth()
  const { activateConversation } = useRealtime()
  const session = useAuthenticatedSession()
  const [appIsActive, setAppIsActive] = useState(
    () => AppState.currentState === "active"
  )
  const appIsActiveRef = useRef(appIsActive)
  const composerRef = useRef<MessageComposerHandle>(null)
  const [topicArchiveDialogOpen, setTopicArchiveDialogOpen] = useState(false)
  const { contacts, conversations, currentUser, currentUserError, isReady } =
    useClientData()
  const listedConversation = conversations.find(
    (item) => item.id === conversationId
  )
  const expectsTopic =
    Boolean(parentConversationId) ||
    params.topic === "1" ||
    listedConversation?.type === "topic"
  const topicQuery = useConversationTopic(
    session,
    conversationId,
    expectsTopic
  )
  const conversation = topicQuery.data?.conversation ?? listedConversation
  const isTopicConversation = expectsTopic || conversation?.type === "topic"
  const topicArchived = Boolean(conversation?.topic?.archived)
  const archiveTopicMutation = useArchiveConversationTopic(
    session,
    conversationId
  )
  const conversationEntity =
    conversation && currentUser && conversation.type !== "topic"
      ? getConversationEntityReference(conversation, currentUser.id)
      : null
  const mentionCandidates = useMemo(
    () =>
      conversation?.type === "group" ||
      conversation?.topic?.parentConversationType === "group"
        ? createMentionCandidates(conversation.members ?? [])
        : [],
    [conversation]
  )
  const messagesQuery = useConversationMessages(session, conversationId)
  const sendTextMutation = useSendConversationTextMessage(
    session,
    conversationId
  )
  const sendFileMutation = useSendConversationFileMessage(
    session,
    conversationId
  )
  const sendImageMutation = useSendConversationImageMessage(
    session,
    conversationId
  )
  const sendVoiceMutation = useSendConversationVoiceMessage(
    session,
    conversationId
  )
  const setReactionMutation = useSetConversationMessageReaction(
    session,
    conversationId
  )
  const isSending =
    sendTextMutation.isPending ||
    sendFileMutation.isPending ||
    sendImageMutation.isPending ||
    sendVoiceMutation.isPending
  const { mutateAsync: markRead } = useMarkConversationRead(
    session,
    conversationId
  )
  const readStateConversationId = useRef("")
  const confirmedReadSeq = useRef(0)
  const requestedReadSeq = useRef(0)
  const messageResources = useMemo(
    () => collectMessageResources(messagesQuery.messages),
    [messagesQuery.messages]
  )
  const resources = useMessageResources(session, messageResources)
  const resolveMentionLabel = useMemo(
    () =>
      conversation && currentUser
        ? createMessageMentionLabelResolver({
            contacts,
            conversation,
            currentUser,
          })
        : EMPTY_MENTION_RESOLVER,
    [contacts, conversation, currentUser]
  )
  const presentedMessages = useMemo(
    () =>
      conversation && currentUser
        ? buildPresentedMessages({
            contacts,
            conversation,
            currentUser,
            messages: messagesQuery.messages,
            resolveMentionLabel,
          })
        : [],
    [
      contacts,
      conversation,
      currentUser,
      messagesQuery.messages,
      resolveMentionLabel,
    ]
  )

  useEffect(() => {
    const subscription = AppState.addEventListener("change", (status) => {
      const active = status === "active"
      appIsActiveRef.current = active
      setAppIsActive(active)
    })

    return () => subscription.remove()
  }, [])

  useEffect(() => {
    if (!isFocused || !conversationId) return
    return activateConversation(conversationId)
  }, [activateConversation, conversationId, isFocused])

  useEffect(() => {
    if (!isFocused || !parentConversationId) return

    const subscription = BackHandler.addEventListener(
      "hardwareBackPress",
      () => {
        if (topicArchiveDialogOpen) {
          if (!archiveTopicMutation.isPending) {
            setTopicArchiveDialogOpen(false)
          }
          return true
        }
        router.replace(buildConversationHref(parentConversationId))
        return true
      }
    )
    return () => subscription.remove()
  }, [
    archiveTopicMutation.isPending,
    isFocused,
    parentConversationId,
    router,
    topicArchiveDialogOpen,
  ])

  useEffect(() => {
    const error = messagesQuery.error ?? topicQuery.error ?? currentUserError
    if (isUnauthorizedError(error)) {
      void invalidateSession()
      router.replace("/init")
    }
  }, [
    currentUserError,
    invalidateSession,
    messagesQuery.error,
    router,
    topicQuery.error,
  ])

  useEffect(() => {
    if (isReady && !conversation && !expectsTopic) {
      router.replace("/messages")
    }
  }, [conversation, expectsTopic, isReady, router])

  useEffect(() => {
    if (!conversation) return

    if (readStateConversationId.current !== conversationId) {
      readStateConversationId.current = conversationId
      confirmedReadSeq.current = conversation.lastReadSeq
      requestedReadSeq.current = conversation.lastReadSeq
    }

    if (!isFocused || !appIsActiveRef.current) return

    const newestSeq = Math.max(
      conversation.lastMessageSeq,
      messagesQuery.messages[0]?.seq ?? 0
    )
    const hasUnread = conversation.unreadCount > 0

    function markLatestRead() {
      if (!appIsActiveRef.current) return
      const hasUnreadProgress =
        hasUnread || newestSeq > requestedReadSeq.current
      if (!hasUnreadProgress) return

      requestedReadSeq.current = Math.max(
        requestedReadSeq.current,
        newestSeq
      )
      void markRead(newestSeq)
        .then((result) => {
          confirmedReadSeq.current = Math.max(
            confirmedReadSeq.current,
            result.lastReadSeq
          )
        })
        .catch(() => {
          if (requestedReadSeq.current === newestSeq) {
            requestedReadSeq.current = confirmedReadSeq.current
          }
        })
    }

    markLatestRead()
    const interval = setInterval(markLatestRead, 20_000)
    return () => clearInterval(interval)
  }, [
    appIsActive,
    conversation,
    conversationId,
    isFocused,
    markRead,
    messagesQuery.messages,
  ])

  async function handleSend(content: string) {
    try {
      await sendTextMutation.mutateAsync({
        clientMessageId: createClientMessageId(),
        content,
      })
      return true
    } catch (error: unknown) {
      Alert.alert(
        "发送失败",
        error instanceof ApiRequestError ? error.message : "消息发送失败，请重试。"
      )
      return false
    }
  }

  async function handleSendUpload(selection: PreparedClientMessageUpload) {
    try {
      if (selection.kind === "image") {
        await sendImageMutation.mutateAsync({
          clientMessageId: createClientMessageId(),
          image: selection.upload,
        })
      } else {
        await sendFileMutation.mutateAsync({
          clientMessageId: createClientMessageId(),
          file: selection.upload,
        })
      }
      return true
    } catch (error: unknown) {
      Alert.alert(
        selection.kind === "image" ? "图片发送失败" : "文件发送失败",
        error instanceof ApiRequestError
          ? error.message
          : "消息发送失败，请重试。"
      )
      return false
    }
  }

  async function handleSendVoice(recording: PreparedClientVoiceMessage) {
    try {
      await sendVoiceMutation.mutateAsync({
        clientMessageId: createClientMessageId(),
        durationMS: recording.durationMS,
        voice: recording.upload,
      })
      return true
    } catch (error: unknown) {
      Alert.alert(
        "语音发送失败",
        error instanceof Error
          ? error.message
          : "消息发送失败，请重试。"
      )
      return false
    }
  }

  async function handleSetReaction(
    messageId: string,
    text: string,
    reacted: boolean
  ) {
    try {
      await setReactionMutation.mutateAsync({ messageId, reacted, text })
    } catch (error: unknown) {
      if (isUnauthorizedError(error)) {
        void invalidateSession()
        router.replace("/init")
      } else {
        toast.show(
          error instanceof ApiRequestError
            ? error.message
            : "更新消息表情失败，请重试。",
          { customData: { tone: "error" satisfies AppToastTone } }
        )
      }
      throw error
    }
  }

  function handleRefresh() {
    void messagesQuery.refetch()
  }

  function handleLoadOlder() {
    if (!messagesQuery.hasOlder || messagesQuery.isFetchingOlder) return
    void messagesQuery.fetchOlder()
  }

  function handleAvatarPress(sender: EntityReference) {
    router.push(buildEntityDetailHref(sender))
  }

  function handleConversationDetails() {
    if (!conversationEntity) return
    router.push(buildEntityDetailHref(conversationEntity))
  }

  function handleOpenTopic(topicConversationId: string) {
    router.push(
      buildTopicConversationHref(conversationId, topicConversationId)
    )
  }

  function handleBack() {
    if (parentConversationId) {
      router.replace(buildConversationHref(parentConversationId))
      return
    }
    router.back()
  }

  async function handleArchiveTopic() {
    if (archiveTopicMutation.isPending) return

    try {
      await archiveTopicMutation.mutateAsync()
      setTopicArchiveDialogOpen(false)
      toast.show("话题已关闭", {
        customData: { tone: "success" satisfies AppToastTone },
      })
    } catch (error: unknown) {
      Alert.alert(
        "关闭话题失败",
        error instanceof ApiRequestError ? error.message : "请稍后重试。"
      )
    }
  }

  function handleAvatarLongPress(sender: EntityReference) {
    if (
      (conversation?.type !== "group" &&
        conversation?.topic?.parentConversationType !== "group") ||
      sender.type === "group"
    ) {
      return
    }

    const label = resolveMentionLabel({
      id: sender.id,
      type: sender.type,
    })?.trim()
    if (!label) return

    composerRef.current?.insertMention({
      id: sender.id,
      label,
      targetType: sender.type,
    })
  }

  async function handleResourcePress(fileId: string) {
    try {
      const resource = await resources.ensure(fileId)
      await openResourceExternally(resource)
    } catch (error: unknown) {
      Alert.alert(
        "无法打开文件",
        error instanceof Error ? error.message : "文件下载失败，请重试。"
      )
    }
  }

  function handleImagePress(fileId: string) {
    router.push({
      pathname: "/image-preview",
      params: { fileId },
    } as unknown as Href)
  }

  async function handleVoiceResourcePress(fileId: string) {
    try {
      await resources.ensure(fileId)
    } catch (error: unknown) {
      Alert.alert(
        "无法播放语音",
        error instanceof Error ? error.message : "语音下载失败，请重试。"
      )
    }
  }

  return (
    <YStack bg="$background" flex={1}>
      <PageHeader
        actionLabel={isTopicConversation ? "关闭话题" : "查看对话详情"}
        compactActionIcon={Ellipsis}
        compactIconButtons
        onActionPress={
          isTopicConversation
            ? topicQuery.data?.canArchive && !topicArchived
              ? () => setTopicArchiveDialogOpen(true)
              : undefined
            : conversationEntity
              ? handleConversationDetails
              : undefined
        }
        onBackPress={handleBack}
        title={conversation?.name ?? "对话"}
      />

      <KeyboardAwareScreen
        contentBackground="$backgroundLight"
        edges={["bottom"]}
        keyboardVerticalOffset={insets.top + PAGE_HEADER_HEIGHT}
        scrollable={false}
      >
        {!conversation ? (
          <ContentState
            loading={expectsTopic && topicQuery.isLoading}
            message={
              expectsTopic
                ? topicQuery.error?.message ?? "正在加载话题"
                : "该会话不存在或已被移除"
            }
            tone={topicQuery.error ? "error" : undefined}
          />
        ) : !currentUser ? (
          <ContentState loading message="正在加载用户信息" />
        ) : (
          <>
            <MessageList
              canAddReaction={!topicArchived}
              conversationId={conversation.id}
              currentUserId={currentUser.id}
              error={messagesQuery.error}
              hasOlder={messagesQuery.hasOlder}
              isFetchingOlder={messagesQuery.isFetchingOlder}
              isLoading={messagesQuery.isLoading}
              isRefreshing={messagesQuery.isRefreshing}
              messages={presentedMessages}
              onAvatarLongPress={
                conversation.type === "group"
                  ? handleAvatarLongPress
                  : undefined
              }
              onAvatarPress={handleAvatarPress}
              onContentTouch={() =>
                composerRef.current?.dismissAccessory()
              }
              onImagePress={handleImagePress}
              onLoadOlder={handleLoadOlder}
              onRefresh={handleRefresh}
              onResourceError={(fileId) =>
                void resources.reload(fileId).catch(() => undefined)
              }
              onResourcePress={(fileId) => void handleResourcePress(fileId)}
              onSetReaction={handleSetReaction}
              onVoiceResourcePress={(fileId) =>
                void handleVoiceResourcePress(fileId)
              }
              onMentionPress={handleAvatarPress}
              onOpenTopic={handleOpenTopic}
              resolveMentionLabel={resolveMentionLabel}
              resourceStates={resources.states}
              server={session}
            />
            {topicArchived ? (
              <YStack bg="$background" items="center" p="$4">
                <SizableText color="$color10" size="$3">
                  话题已关闭，无法继续发言
                </SizableText>
              </YStack>
            ) : (
              <MessageComposer
                disabled={isSending}
                mentionCandidates={mentionCandidates}
                onSend={handleSend}
                onSendUpload={handleSendUpload}
                onSendVoice={handleSendVoice}
                ref={composerRef}
                server={session}
              />
            )}
          </>
        )}
      </KeyboardAwareScreen>

      <TopicArchiveDialog
        onConfirm={() => void handleArchiveTopic()}
        onOpenChange={setTopicArchiveDialogOpen}
        open={topicArchiveDialogOpen}
        saving={archiveTopicMutation.isPending}
      />
    </YStack>
  )
}

function createClientMessageId() {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID()
  }

  let seed = Date.now()
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (value) => {
    const random = (seed + Math.random() * 16) % 16 | 0
    seed = Math.floor(seed / 16)
    return (value === "x" ? random : (random & 0x3) | 0x8).toString(16)
  })
}
