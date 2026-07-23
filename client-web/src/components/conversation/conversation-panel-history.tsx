import * as React from "react"
import { ArrowDown, LoaderCircle, MessageCircle } from "lucide-react"
import { type ClientConversation } from "@/lib/client-data-api"
import { type MentionLabelResolver } from "@/lib/message-mentions"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Button } from "@/components/ui/button"
import {
  MessageBubble,
  SystemMessageBadge,
} from "@/components/conversation/conversation-message"
import { formatConversationMessageTime } from "@/lib/conversation-message-presenter"
import { conversationMessageRetentionLimit } from "@/lib/client-data-state"
import type {
  ConversationPanelMentionTarget,
  ConversationPanelMessage,
  ConversationPanelMessageSelection,
} from "@/lib/conversation-panel-types"

export const ConversationPanelHistory = React.memo(
  function ConversationPanelHistory({
    canReply = true,
    conversation,
    currentUserId,
    error,
    loading,
    loadingBefore,
    header,
    mentionLabelResolver,
    messages,
    messageSelection,
    onCompactMessages = noop,
    onRegisterMessageView = noopRegistration,
    onForwardMessage,
    onCreateTopic,
    onLoadBeforeMessages,
    onStartMessageSelection,
    onInsertMention,
    onOpenTopic,
    onReplyToMessage,
    onRevokeMessage,
    onSetMessageReaction,
    onRespondToChoice,
    onToggleMessageSelection,
  }: {
    canReply?: boolean
    conversation: ClientConversation
    currentUserId: string
    error: string | null
    loading: boolean
    loadingBefore: boolean
    header?: React.ReactNode
    mentionLabelResolver: MentionLabelResolver
    messages: ConversationPanelMessage[]
    messageSelection?: ConversationPanelMessageSelection
    onCompactMessages?: () => void
    onRegisterMessageView?: (conversationId: string) => () => void
    onForwardMessage?: (message: ConversationPanelMessage) => void
    onCreateTopic?: (message: ConversationPanelMessage) => void
    onLoadBeforeMessages: () => void
    onStartMessageSelection?: (message: ConversationPanelMessage) => void
    onInsertMention: (target: ConversationPanelMentionTarget) => void
    onOpenTopic?: (conversationId: string) => void
    onReplyToMessage: (message: ConversationPanelMessage) => void
    onRevokeMessage?: (message: ConversationPanelMessage) => void
    onSetMessageReaction?: (
      message: ConversationPanelMessage,
      text: string,
      reacted: boolean
    ) => Promise<void>
    onRespondToChoice?: (
      message: ConversationPanelMessage,
      optionIds: string[]
    ) => Promise<void>
    onToggleMessageSelection?: (message: ConversationPanelMessage) => void
  }) {
    const viewportRef = React.useRef<HTMLDivElement | null>(null)
    const contentResizeObserverRef = React.useRef<ResizeObserver | null>(null)
    const nearBottomRef = React.useRef(true)
    const previousConversationIdRef = React.useRef<string | null>(null)
    const previousFirstMessageIdRef = React.useRef<string | null>(null)
    const previousLastMessageIdRef = React.useRef<string | null>(null)
    const previousMessagesLengthRef = React.useRef(0)
    const beforeLoadSnapshotRef = React.useRef<ScrollSnapshot | null>(null)
    const previousLoadingBeforeRef = React.useRef(loadingBefore)
    const historyLoadStartFirstMessageIdRef = React.useRef<string | null>(
      loadingBefore ? (messages[0]?.id ?? null) : null
    )
    const lastHistoryLoadedAtRef = React.useRef(0)
    const onCompactMessagesRef = React.useRef(onCompactMessages)
    const [viewportNearBottom, setViewportNearBottom] = React.useState(true)
    const [pendingNewMessageCount, setPendingNewMessageCount] =
      React.useState(0)

    React.useEffect(() => {
      onCompactMessagesRef.current = onCompactMessages
    }, [onCompactMessages])

    React.useEffect(
      () => onRegisterMessageView(conversation.id),
      [conversation.id, onRegisterMessageView]
    )

    React.useEffect(
      () => () => {
        onCompactMessagesRef.current()
      },
      []
    )

    React.useEffect(() => {
      const wasLoadingBefore = previousLoadingBeforeRef.current
      if (!wasLoadingBefore && loadingBefore) {
        historyLoadStartFirstMessageIdRef.current = messages[0]?.id ?? null
      } else if (wasLoadingBefore && !loadingBefore) {
        const firstMessageId = messages[0]?.id ?? null
        if (firstMessageId !== historyLoadStartFirstMessageIdRef.current) {
          lastHistoryLoadedAtRef.current = Date.now()
        }
        historyLoadStartFirstMessageIdRef.current = null
      }
      previousLoadingBeforeRef.current = loadingBefore
    }, [loadingBefore, messages])

    React.useEffect(() => {
      if (
        messages.length <= conversationMessageRetentionLimit ||
        !viewportNearBottom ||
        loadingBefore ||
        messageSelection?.active
      ) {
        return
      }

      const remainingProtectionMs = Math.max(
        lastHistoryLoadedAtRef.current + historyRetentionMs - Date.now(),
        0
      )
      if (remainingProtectionMs === 0) {
        onCompactMessages()
        return
      }

      const timeout = window.setTimeout(() => {
        if (nearBottomRef.current) {
          onCompactMessagesRef.current()
        }
      }, remainingProtectionMs)
      return () => window.clearTimeout(timeout)
    }, [
      loadingBefore,
      messageSelection?.active,
      messages.length,
      onCompactMessages,
      viewportNearBottom,
    ])

    const setHistoryContentRef = React.useCallback(
      (content: HTMLDivElement | null) => {
        contentResizeObserverRef.current?.disconnect()
        contentResizeObserverRef.current = null

        if (!content) {
          return
        }

        const observer = new ResizeObserver(() => {
          const viewport = viewportRef.current
          if (!viewport || !nearBottomRef.current) {
            return
          }

          scrollToBottom(viewport)
        })
        observer.observe(content)
        contentResizeObserverRef.current = observer
      },
      []
    )

    React.useLayoutEffect(() => {
      const viewport = viewportRef.current
      if (!viewport) {
        return
      }

      const firstMessageId = messages[0]?.id ?? null
      const lastMessageId = messages[messages.length - 1]?.id ?? null
      const previousConversationId = previousConversationIdRef.current
      const previousFirstMessageId = previousFirstMessageIdRef.current
      const previousLastMessageId = previousLastMessageIdRef.current
      const previousMessagesLength = previousMessagesLengthRef.current
      const changedConversation = previousConversationId !== conversation.id

      if (changedConversation) {
        scrollToBottom(viewport)
        nearBottomRef.current = true
        setViewportNearBottom(true)
        beforeLoadSnapshotRef.current = null
        setPendingNewMessageCount(0)
      } else {
        if (
          firstMessageId &&
          previousFirstMessageId &&
          firstMessageId !== previousFirstMessageId &&
          beforeLoadSnapshotRef.current
        ) {
          restoreScrollPositionAfterPrepend(
            viewport,
            beforeLoadSnapshotRef.current
          )
          const nearBottom = isNearBottom(viewport)
          nearBottomRef.current = nearBottom
          setViewportNearBottom(nearBottom)
          beforeLoadSnapshotRef.current = null
        }

        if (
          lastMessageId &&
          previousLastMessageId !== lastMessageId &&
          messages.length >= previousMessagesLength
        ) {
          const appendedMessages = getAppendedMessages(
            messages,
            previousLastMessageId,
            previousMessagesLength
          )
          const shouldFollowLatest =
            nearBottomRef.current ||
            appendedMessages.some((message) => message.role === "me")

          if (shouldFollowLatest) {
            scrollToBottom(viewport)
            nearBottomRef.current = true
            setViewportNearBottom(true)
            setPendingNewMessageCount(0)
          } else {
            const incomingMessageCount = appendedMessages.filter(
              (message) => message.role !== "me"
            ).length
            if (incomingMessageCount > 0) {
              setPendingNewMessageCount(
                (currentCount) => currentCount + incomingMessageCount
              )
            }
          }
        }
      }

      previousConversationIdRef.current = conversation.id
      previousFirstMessageIdRef.current = firstMessageId
      previousLastMessageIdRef.current = lastMessageId
      previousMessagesLengthRef.current = messages.length
    }, [conversation.id, messages])

    function handleViewportScroll(event: React.UIEvent<HTMLDivElement>) {
      const viewport = event.currentTarget
      const nearBottom = isNearBottom(viewport)

      nearBottomRef.current = nearBottom
      setViewportNearBottom((current) =>
        current === nearBottom ? current : nearBottom
      )
      if (nearBottom) {
        setPendingNewMessageCount((currentCount) =>
          currentCount === 0 ? currentCount : 0
        )
      }

      if (loadingBefore) {
        const snapshot = beforeLoadSnapshotRef.current
        if (snapshot) {
          beforeLoadSnapshotRef.current = createScrollSnapshot(
            viewport,
            snapshot.anchorMessageId
          )
        }
        return
      }

      if (viewport.scrollTop > 80) {
        return
      }

      beforeLoadSnapshotRef.current = createScrollSnapshot(
        viewport,
        messages[0]?.id ?? null
      )
      onLoadBeforeMessages()
    }

    function handleJumpToLatest() {
      const viewport = viewportRef.current
      if (!viewport) {
        return
      }

      scrollToBottom(viewport)
      nearBottomRef.current = true
      setViewportNearBottom(true)
      setPendingNewMessageCount(0)
    }

    function handleHistoryContextMenu(event: React.MouseEvent<HTMLDivElement>) {
      if (
        event.target instanceof Element &&
        event.target.closest("[data-message-action-trigger]")
      ) {
        return
      }

      event.preventDefault()
    }

    if (loading && !header) {
      return (
        <div
          className="flex min-h-0 flex-1 items-center justify-center gap-2 bg-muted/10 text-sm text-muted-foreground"
          data-testid="conversation-history-loading"
        >
          <LoaderCircle className="size-4 animate-spin" />
          <span>正在加载消息</span>
        </div>
      )
    }

    if (error && messages.length === 0 && !header) {
      return (
        <div
          className="flex min-h-0 flex-1 items-center justify-center bg-muted/10 px-6 text-center text-sm text-muted-foreground"
          data-testid="conversation-history-error"
        >
          {error}
        </div>
      )
    }

    if (messages.length === 0 && header) {
      return (
        <div className="relative min-h-0 flex-1">
          <ScrollArea
            className="size-full bg-muted/10"
            data-testid="conversation-panel-history"
            viewportProps={{
              className: "[&>div]:block! [&>div]:w-full! [&>div]:min-w-0!",
              onContextMenu: handleHistoryContextMenu,
            }}
            viewportRef={viewportRef}
          >
            <div
              ref={setHistoryContentRef}
              className="flex w-full flex-col gap-5 px-5 py-6"
              data-testid="conversation-history-content"
            >
              {header}
              {loading && (
                <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
                  <LoaderCircle className="size-3.5 animate-spin" />
                  <span>正在加载话题回复</span>
                </div>
              )}
              {error && (
                <div className="text-center text-xs text-muted-foreground">
                  {error}
                </div>
              )}
            </div>
          </ScrollArea>
        </div>
      )
    }

    if (messages.length === 0) {
      return (
        <div className="flex min-h-0 flex-1 flex-col bg-muted/10">
          <Empty
            className="h-full min-h-0 flex-1 rounded-none"
            data-testid="conversation-history-empty"
          >
            <EmptyMedia>
              <MessageCircle className="size-14 text-muted-foreground/25" />
            </EmptyMedia>
            <EmptyHeader>
              <EmptyTitle>暂无消息</EmptyTitle>
              <EmptyDescription>发送第一条消息开始对话</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </div>
      )
    }

    return (
      <div className="relative min-h-0 flex-1">
        <ScrollArea
          className="size-full bg-muted/10"
          data-testid="conversation-panel-history"
          viewportProps={{
            className: "[&>div]:block! [&>div]:w-full! [&>div]:min-w-0!",
            onContextMenu: handleHistoryContextMenu,
            onScroll: handleViewportScroll,
          }}
          viewportRef={viewportRef}
        >
          <div
            ref={setHistoryContentRef}
            className="flex w-full flex-col gap-5 px-5 py-6"
            data-testid="conversation-history-content"
          >
            {header}
            {loadingBefore && (
              <div
                className="flex items-center justify-center gap-2 text-xs text-muted-foreground"
                data-testid="conversation-history-loading-before"
              >
                <LoaderCircle className="size-3.5 animate-spin" />
                <span>正在加载更早消息</span>
              </div>
            )}
            {messages.map((message, index) => (
              <React.Fragment key={message.id}>
                {shouldShowMessageTimeMarker(messages[index - 1], message) && (
                  <div
                    className="text-center text-xs text-muted-foreground"
                    data-message-time-marker
                  >
                    {formatConversationMessageTime(message.createdAt)}
                  </div>
                )}
                {message.role === "system" ? (
                  <SystemMessageBadge
                    currentUserId={currentUserId}
                    mentionLabelResolver={mentionLabelResolver}
                    message={message}
                  />
                ) : (
                  <MessageBubble
                    canReply={canReply}
                    message={message}
                    conversation={conversation}
                    currentUserId={currentUserId}
                    mentionLabelResolver={mentionLabelResolver}
                    onForward={
                      isMessageForwardable(message)
                        ? onForwardMessage
                        : undefined
                    }
                    onCreateTopic={onCreateTopic}
                    onInsertMention={onInsertMention}
                    onOpenTopic={onOpenTopic}
                    onMultiSelect={
                      isMessageForwardable(message)
                        ? onStartMessageSelection
                        : undefined
                    }
                    onReply={onReplyToMessage}
                    onRevoke={onRevokeMessage}
                    onSetReaction={onSetMessageReaction}
                    onRespondToChoice={onRespondToChoice}
                    onToggleSelected={onToggleMessageSelection}
                    selectable={isMessageForwardable(message)}
                    selected={messageSelection?.selectedMessageIds.has(
                      message.id
                    )}
                    selectionMode={messageSelection?.active}
                  />
                )}
              </React.Fragment>
            ))}
          </div>
        </ScrollArea>
        {pendingNewMessageCount > 0 && (
          <Button
            className="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full shadow-md"
            onClick={handleJumpToLatest}
            size="sm"
            type="button"
            variant="secondary"
          >
            <ArrowDown className="size-4" />
            {pendingNewMessageCount} 条新消息
          </Button>
        )}
      </div>
    )
  }
)

function isMessageAvailable(message: ConversationPanelMessage) {
  return message.body.type !== "revoked" && message.body.type !== "unsupported"
}

function isMessageForwardable(message: ConversationPanelMessage) {
  return isMessageAvailable(message) && message.body.type !== "choice"
}

const messageTimeMarkerThresholdMs = 60 * 60 * 1000
const historyRetentionMs = 3 * 60 * 1000

function noop() {}

function noopRegistration() {
  return noop
}

function shouldShowMessageTimeMarker(
  previousMessage: ConversationPanelMessage | undefined,
  message: ConversationPanelMessage
) {
  if (!previousMessage) {
    return false
  }

  const previousTime = new Date(previousMessage.createdAt).getTime()
  const messageTime = new Date(message.createdAt).getTime()
  if (Number.isNaN(previousTime) || Number.isNaN(messageTime)) {
    return false
  }

  return messageTime - previousTime > messageTimeMarkerThresholdMs
}

function scrollToBottom(viewport: HTMLDivElement) {
  viewport.scrollTop = viewport.scrollHeight
}

function isNearBottom(viewport: HTMLDivElement) {
  return (
    viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= 80
  )
}

type ScrollSnapshot = {
  anchorMessageId: string | null
  anchorTop: number | null
  scrollHeight: number
  scrollTop: number
}

function createScrollSnapshot(
  viewport: HTMLDivElement,
  anchorMessageId: string | null
): ScrollSnapshot {
  return {
    anchorMessageId,
    anchorTop: getMessageTop(viewport, anchorMessageId),
    scrollHeight: viewport.scrollHeight,
    scrollTop: viewport.scrollTop,
  }
}

function restoreScrollPositionAfterPrepend(
  viewport: HTMLDivElement,
  snapshot: ScrollSnapshot
) {
  const nextAnchorTop = getMessageTop(viewport, snapshot.anchorMessageId)
  if (snapshot.anchorTop !== null && nextAnchorTop !== null) {
    viewport.scrollTop += nextAnchorTop - snapshot.anchorTop
    return
  }

  viewport.scrollTop =
    snapshot.scrollTop + (viewport.scrollHeight - snapshot.scrollHeight)
}

function getMessageTop(
  viewport: HTMLDivElement,
  messageId: string | null
): number | null {
  if (!messageId) {
    return null
  }

  const messageElement = Array.from(
    viewport.querySelectorAll<HTMLElement>("[data-conversation-message-id]")
  ).find((element) => element.dataset.conversationMessageId === messageId)

  return messageElement?.getBoundingClientRect().top ?? null
}

function getAppendedMessages(
  messages: ConversationPanelMessage[],
  previousLastMessageId: string | null,
  previousMessagesLength: number
) {
  const previousLastMessageIndex = previousLastMessageId
    ? messages.findIndex((message) => message.id === previousLastMessageId)
    : -1

  if (previousLastMessageIndex >= 0) {
    return messages.slice(previousLastMessageIndex + 1)
  }

  return messages.slice(previousMessagesLength)
}
