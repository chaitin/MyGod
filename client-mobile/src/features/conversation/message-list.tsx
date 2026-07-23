import { ArrowDown } from "lucide-react-native"
import { useEffect, useMemo, useRef, useState } from "react"
import {
  FlatList,
  RefreshControl,
  StyleSheet,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
} from "react-native"
import {
  Button,
  SizableText,
  Spinner,
  useTheme,
  XStack,
  YStack,
} from "tamagui"

import { ContentState } from "@/components/feedback/content-state"
import { AppButton } from "@/components/forms/app-button"
import { ThemedIcon } from "@/components/icons/themed-icon"
import { MessageBubble } from "@/features/conversation/message-bubble"
import type { EntityReference } from "@/domain/entities/entity-profile"
import type {
  MessageMentionLabelResolver,
  PresentedMessage,
} from "@/domain/messages/message-presenter"
import {
  formatMessageTimeMarker,
  shouldShowMessageTimeMarker,
} from "@/domain/messages/message-presenter"
import type { ServerTarget } from "@/data/query"
import type { ResourceLoadState } from "@/data/resources"

export function MessageList({
  canAddReaction,
  conversationId,
  currentUserId,
  error,
  hasOlder,
  isFetchingOlder,
  isLoading,
  isPullRefreshing,
  messages,
  onAvatarPress,
  onAvatarLongPress,
  onContentTouch,
  onImagePress,
  onLoadOlder,
  onRefresh,
  onResourceError,
  onResourcePress,
  onSetReaction,
  onVoiceResourcePress,
  onMentionPress,
  onOpenTopic,
  resolveMentionLabel,
  resourceStates,
  server,
}: {
  canAddReaction: boolean
  conversationId: string
  currentUserId: string
  error: Error | null
  hasOlder: boolean
  isFetchingOlder: boolean
  isLoading: boolean
  isPullRefreshing: boolean
  messages: PresentedMessage[]
  onAvatarLongPress?: (sender: EntityReference) => void
  onAvatarPress: (sender: EntityReference) => void
  onContentTouch: () => void
  onImagePress: (fileId: string) => void
  onLoadOlder: () => void
  onRefresh: () => void
  onResourceError: (fileId: string) => void
  onResourcePress: (fileId: string) => void
  onSetReaction?: (
    messageId: string,
    text: string,
    reacted: boolean
  ) => Promise<void>
  onVoiceResourcePress: (fileId: string) => void
  onMentionPress: (target: EntityReference) => void
  onOpenTopic: (conversationId: string) => void
  resolveMentionLabel: MessageMentionLabelResolver
  resourceStates: ReadonlyMap<string, ResourceLoadState>
  server: ServerTarget
}) {
  const theme = useTheme()
  const listItems = useMemo(() => buildMessageListItems(messages), [messages])
  const listRef = useRef<FlatList<MessageListItem>>(null)
  const nearBottomRef = useRef(true)
  const initializedMessagesRef = useRef(false)
  const previousConversationIdRef = useRef("")
  const previousNewestMessageIdRef = useRef<string | null>(null)
  const previousMessagesLengthRef = useRef(0)
  const pendingScrollRef = useRef<PendingScroll>(null)
  const [pendingNewMessageCount, setPendingNewMessageCount] = useState(0)

  useEffect(() => {
    if (previousConversationIdRef.current !== conversationId) {
      previousConversationIdRef.current = conversationId
      previousNewestMessageIdRef.current = null
      previousMessagesLengthRef.current = 0
      initializedMessagesRef.current = false
      nearBottomRef.current = true
      pendingScrollRef.current = null
      setPendingNewMessageCount(0)
    }

    if (!initializedMessagesRef.current) {
      if (!isLoading) {
        initializedMessagesRef.current = true
        previousNewestMessageIdRef.current = messages[0]?.id ?? null
        previousMessagesLengthRef.current = messages.length
        if (messages.length > 0) {
          scheduleScrollToLatest(listRef, pendingScrollRef, false)
        }
      }
      return
    }

    const newestMessageId = messages[0]?.id ?? null
    const previousNewestMessageId = previousNewestMessageIdRef.current
    if (newestMessageId && newestMessageId !== previousNewestMessageId) {
      const newMessages = getNewMessages(
        messages,
        previousNewestMessageId,
        previousMessagesLengthRef.current
      )

      if (newMessages.length > 0) {
        if (nearBottomRef.current) {
          scheduleScrollToLatest(listRef, pendingScrollRef, true)
          setPendingNewMessageCount(0)
        } else {
          setPendingNewMessageCount(
            (currentCount) => currentCount + newMessages.length
          )
        }
      }
    }

    previousNewestMessageIdRef.current = newestMessageId
    previousMessagesLengthRef.current = messages.length
  }, [conversationId, isLoading, messages])

  function handleScroll(event: NativeSyntheticEvent<NativeScrollEvent>) {
    const nearBottom = event.nativeEvent.contentOffset.y <= 80
    nearBottomRef.current = nearBottom

    if (nearBottom) {
      setPendingNewMessageCount((currentCount) =>
        currentCount === 0 ? currentCount : 0
      )
    }
  }

  function handleContentSizeChange() {
    performPendingScroll(listRef, pendingScrollRef)
  }

  function handleJumpToLatest() {
    nearBottomRef.current = true
    setPendingNewMessageCount(0)
    scheduleScrollToLatest(listRef, pendingScrollRef, true)
  }

  if (isLoading) {
    return <ContentState loading message="正在加载消息" />
  }

  if (error && messages.length === 0) {
    return (
      <ContentState message={error.message} tone="error">
        <YStack maxW={240} width="100%">
          <AppButton
            accessibilityLabel="重新加载消息"
            onPress={onRefresh}
            theme="gray"
            variant="outlined"
            width="100%"
          >
            重试
          </AppButton>
        </YStack>
      </ContentState>
    )
  }

  if (messages.length === 0) {
    return <ContentState message="暂无消息，发送第一条消息开始对话" />
  }

  return (
    <YStack flex={1} position="relative">
      <FlatList
        ref={listRef}
        contentContainerStyle={styles.content}
        data={listItems}
        inverted
        ItemSeparatorComponent={() => <YStack height="$1" />}
        keyboardDismissMode="on-drag"
        keyboardShouldPersistTaps="handled"
        keyExtractor={(item) => item.key}
        ListFooterComponent={
          hasOlder || isFetchingOlder ? (
            <YStack items="center" pb="$3">
              <Button
                disabled={isFetchingOlder}
                icon={isFetchingOlder ? <Spinner /> : undefined}
                onPress={onLoadOlder}
                size="$3"
                variant="outlined"
              >
                {isFetchingOlder ? "正在加载" : "加载更早消息"}
              </Button>
            </YStack>
          ) : null
        }
        maintainVisibleContentPosition={{
          autoscrollToTopThreshold: 80,
          minIndexForVisible: 0,
        }}
        onContentSizeChange={handleContentSizeChange}
        onEndReached={hasOlder && !isFetchingOlder ? onLoadOlder : undefined}
        onEndReachedThreshold={0.2}
        onScroll={handleScroll}
        onTouchStart={onContentTouch}
        refreshControl={
          <RefreshControl
            colors={[String(theme.color10.val)]}
            onRefresh={onRefresh}
            refreshing={isPullRefreshing}
            tintColor={String(theme.color10.val)}
          />
        }
        renderItem={({ item }) =>
          item.type === "time" ? (
            <MessageTimeMarker createdAt={item.createdAt} />
          ) : (
            <MessageBubble
              canAddReaction={canAddReaction}
              currentUserId={currentUserId}
              message={item.message}
              onAvatarLongPress={onAvatarLongPress}
              onAvatarPress={onAvatarPress}
              onImagePress={onImagePress}
              onMentionPress={onMentionPress}
              onOpenTopic={onOpenTopic}
              onResourceError={onResourceError}
              onResourcePress={onResourcePress}
              onSetReaction={onSetReaction}
              onVoiceResourcePress={onVoiceResourcePress}
              resolveMentionLabel={resolveMentionLabel}
              resourceStates={resourceStates}
              server={server}
            />
          )
        }
        scrollEventThrottle={16}
        showsVerticalScrollIndicator={false}
        style={styles.list}
      />

      {pendingNewMessageCount > 0 ? (
        <XStack b="$4" justify="center" l={0} position="absolute" r={0}>
          <Button
            icon={<ThemedIcon icon={ArrowDown} size={18} />}
            onPress={handleJumpToLatest}
            rounded="$10"
            size="$3"
          >
            {pendingNewMessageCount} 条新消息
          </Button>
        </XStack>
      ) : null}
    </YStack>
  )
}

type PendingScroll = {
  animated: boolean
}

type MessageListItem =
  | {
      key: string
      message: PresentedMessage
      type: "message"
    }
  | {
      createdAt: string
      key: string
      type: "time"
    }

function buildMessageListItems(messages: PresentedMessage[]): MessageListItem[] {
  const items: MessageListItem[] = []

  for (let index = 0; index < messages.length; index += 1) {
    const message = messages[index]
    if (!message) continue

    items.push({ key: `message:${message.id}`, message, type: "message" })

    const olderMessage = messages[index + 1]
    if (
      olderMessage &&
      shouldShowMessageTimeMarker(olderMessage.createdAt, message.createdAt)
    ) {
      items.push({
        createdAt: message.createdAt,
        key: `time:${olderMessage.id}:${message.id}`,
        type: "time",
      })
    }
  }

  return items
}

function MessageTimeMarker({ createdAt }: { createdAt: string }) {
  const label = formatMessageTimeMarker(createdAt)
  if (!label) return null

  return (
    <XStack justify="center">
      <SizableText color="$color10" size="$2">
        {label}
      </SizableText>
    </XStack>
  )
}

function scheduleScrollToLatest(
  listRef: React.RefObject<FlatList<MessageListItem> | null>,
  pendingScrollRef: React.MutableRefObject<PendingScroll | null>,
  animated: boolean
) {
  pendingScrollRef.current = { animated }
  requestAnimationFrame(() => performPendingScroll(listRef, pendingScrollRef))
}

function performPendingScroll(
  listRef: React.RefObject<FlatList<MessageListItem> | null>,
  pendingScrollRef: React.MutableRefObject<PendingScroll | null>
) {
  const list = listRef.current
  const pendingScroll = pendingScrollRef.current
  if (!list || !pendingScroll) return

  pendingScrollRef.current = null
  list.scrollToOffset({ animated: pendingScroll.animated, offset: 0 })
}

function getNewMessages(
  messages: PresentedMessage[],
  previousNewestMessageId: string | null,
  previousMessagesLength: number
) {
  const previousNewestIndex = previousNewestMessageId
    ? messages.findIndex((message) => message.id === previousNewestMessageId)
    : -1

  if (previousNewestIndex > 0) {
    return messages.slice(0, previousNewestIndex)
  }
  if (previousNewestIndex === 0) {
    return []
  }

  const addedCount = Math.max(messages.length - previousMessagesLength, 1)
  return messages.slice(0, addedCount)
}

const styles = StyleSheet.create({
  content: {
    paddingBottom: 16,
    paddingTop: 16,
  },
  list: {
    flex: 1,
  },
})
