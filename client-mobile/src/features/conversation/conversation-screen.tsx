import { useLocalSearchParams, useRouter } from "expo-router"
import { useEffect, useMemo, useRef } from "react"
import { Alert } from "react-native"
import { Paragraph, Spinner, YStack } from "tamagui"

import { KeyboardAwareScreen } from "@/components/layout/keyboard-aware-screen"
import { PageHeader } from "@/components/navigation/page-header"
import { ApiRequestError, isUnauthorizedError } from "@/data/api-client"
import {
  useConversationMessages,
  useMarkConversationRead,
  useSendConversationTextMessage,
  useTemporaryFileUrls,
} from "@/data/message-hooks"
import { MessageComposer } from "@/features/conversation/message-composer"
import { MessageList } from "@/features/conversation/message-list"
import {
  buildPresentedMessages,
  collectMessageFileIds,
  createMessageMentionLabelResolver,
  type MessageMentionLabelResolver,
} from "@/features/conversation/conversation-message-presenter"
import { useAuth } from "@/features/auth/auth-context"
import { useServers } from "@/features/servers/server-context"
import { useClientData } from "@/providers/client-data-provider"

const EMPTY_MENTION_RESOLVER: MessageMentionLabelResolver = () => undefined

export function ConversationScreen() {
  const params = useLocalSearchParams<{ conversationId: string }>()
  const conversationId = Array.isArray(params.conversationId)
    ? (params.conversationId[0] ?? "")
    : (params.conversationId ?? "")
  const router = useRouter()
  const { signOut } = useAuth()
  const { selectedServer } = useServers()
  const { contacts, conversations, currentUser, currentUserError, isReady } =
    useClientData()
  const conversation = conversations.find((item) => item.id === conversationId)
  const messagesQuery = useConversationMessages(selectedServer, conversationId)
  const sendMutation = useSendConversationTextMessage(
    selectedServer,
    conversationId
  )
  const { mutate: markRead } = useMarkConversationRead(
    selectedServer,
    conversationId
  )
  const markedReadSeq = useRef(0)
  const fileIds = useMemo(
    () => collectMessageFileIds(messagesQuery.messages),
    [messagesQuery.messages]
  )
  const fileUrlsQuery = useTemporaryFileUrls(selectedServer, fileIds)
  const fileUrls = useMemo(
    () =>
      new Map(
        (fileUrlsQuery.data ?? []).map((item) => [item.fileId, item.url] as const)
      ),
    [fileUrlsQuery.data]
  )
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
    const error = messagesQuery.error ?? currentUserError
    if (isUnauthorizedError(error)) {
      signOut()
      router.replace("/init")
    }
  }, [currentUserError, messagesQuery.error, router, signOut])

  useEffect(() => {
    if (isReady && !conversation) {
      router.replace("/(app)/(tabs)/messages")
    }
  }, [conversation, isReady, router])

  useEffect(() => {
    const newestSeq = messagesQuery.messages[0]?.seq ?? 0
    if (!conversation || newestSeq <= markedReadSeq.current) return

    markedReadSeq.current = newestSeq
    markRead(newestSeq)
  }, [conversation, markRead, messagesQuery.messages])

  async function handleSend(content: string) {
    try {
      await sendMutation.mutateAsync({
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

  function handleRefresh() {
    void messagesQuery.refetch()
  }

  function handleLoadOlder() {
    if (!messagesQuery.hasOlder || messagesQuery.isFetchingOlder) return
    void messagesQuery.fetchOlder()
  }

  return (
    <YStack bg="$background" flex={1}>
      <PageHeader
        onBackPress={() => router.back()}
        title={conversation?.name ?? "对话"}
      />

      <KeyboardAwareScreen edges={["bottom"]} scrollable={false}>
        {!conversation ? (
          <YStack flex={1} items="center" justify="center" p="$6">
            <Paragraph color="$color10">该会话不存在或已被移除</Paragraph>
          </YStack>
        ) : !currentUser ? (
          <YStack flex={1} gap="$2" items="center" justify="center">
            <Spinner />
            <Paragraph color="$color10">正在加载用户信息</Paragraph>
          </YStack>
        ) : (
          <>
            <MessageList
              error={messagesQuery.error}
              fileUrls={fileUrls}
              fileUrlsLoading={fileUrlsQuery.isLoading}
              hasOlder={messagesQuery.hasOlder}
              isFetchingOlder={messagesQuery.isFetchingOlder}
              isLoading={messagesQuery.isLoading}
              isRefreshing={messagesQuery.isRefreshing}
              messages={presentedMessages}
              onLoadOlder={handleLoadOlder}
              onRefresh={handleRefresh}
              resolveMentionLabel={resolveMentionLabel}
              serverUrl={selectedServer.url}
            />
            <MessageComposer
              disabled={sendMutation.isPending}
              onSend={handleSend}
            />
          </>
        )}
      </KeyboardAwareScreen>
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
