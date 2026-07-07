import * as React from "react"
import {
  ImageIcon,
  LoaderCircle,
  MessageCircle,
  Paperclip,
  Send,
  Settings,
  Smile,
} from "lucide-react"

import { cn } from "@/lib/utils"
import type { ClientConversation, ClientMessage } from "@/lib/client-data-api"
import { ConversationInfoDrawer } from "@/components/conversation-info-drawer"
import {
  ExpressionPicker,
  type ExpressionItem,
} from "@/components/expression-picker"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { MessageActionMenu } from "@/components/message-action-menu"
import { UserProfilePopover } from "@/components/user-profile-popover"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Textarea } from "@/components/ui/textarea"

export type ConversationPanelMessage = {
  id: string
  role: "me" | "other"
  author: string
  avatar: string
  body: ClientMessage["body"]
  time: string
  senderUserId: string | null
}

type ConversationPanelProps = {
  conversation: ClientConversation | null
  draft: string
  historyError: string | null
  historyLoading: boolean
  historyLoadingBefore: boolean
  messages: ConversationPanelMessage[]
  onDraftChange: (draft: string) => void
  onLoadBeforeMessages: () => void
  onSendMessage: () => void
  sending: boolean
}

export function ConversationPanel({
  conversation,
  draft,
  historyError,
  historyLoading,
  historyLoadingBefore,
  messages,
  onDraftChange,
  onLoadBeforeMessages,
  onSendMessage,
  sending,
}: ConversationPanelProps) {
  return (
    <main
      className={cn(
        "flex min-w-0 flex-1 flex-col",
        conversation ? "bg-background" : "bg-muted"
      )}
      data-testid="chat-detail-shell"
    >
      {conversation ? (
        <>
          <ConversationPanelHeader conversation={conversation} />
          <ConversationPanelHistory
            conversation={conversation}
            error={historyError}
            loading={historyLoading}
            loadingBefore={historyLoadingBefore}
            messages={messages}
            onLoadBeforeMessages={onLoadBeforeMessages}
          />
          <ConversationPanelComposer
            draft={draft}
            onDraftChange={onDraftChange}
            onSendMessage={onSendMessage}
            sending={sending}
          />
        </>
      ) : (
        <ConversationPanelEmptyState />
      )}
    </main>
  )
}

function ConversationPanelHeader({
  conversation,
}: {
  conversation: ClientConversation
}) {
  return (
    <header
      className="flex h-14 shrink-0 items-center justify-between border-b px-5"
      data-testid="conversation-panel-header"
    >
      <div className="min-w-0">
        <h2 className="truncate text-base font-medium">{conversation.name}</h2>
        <p className="truncate text-xs text-muted-foreground">
          {getConversationHeaderDescription(conversation)}
        </p>
      </div>
      <ConversationInfoDrawer conversationId={conversation.id}>
        <Button
          aria-label="会话设置"
          size="icon-sm"
          title="会话设置"
          type="button"
          variant="ghost"
        >
          <Settings className="size-4" />
        </Button>
      </ConversationInfoDrawer>
    </header>
  )
}

function ConversationPanelHistory({
  conversation,
  error,
  loading,
  loadingBefore,
  messages,
  onLoadBeforeMessages,
}: {
  conversation: ClientConversation
  error: string | null
  loading: boolean
  loadingBefore: boolean
  messages: ConversationPanelMessage[]
  onLoadBeforeMessages: () => void
}) {
  const viewportRef = React.useRef<HTMLDivElement | null>(null)
  const previousConversationIdRef = React.useRef<string | null>(null)
  const previousFirstMessageIdRef = React.useRef<string | null>(null)
  const previousLastMessageIdRef = React.useRef<string | null>(null)
  const previousMessagesLengthRef = React.useRef(0)
  const beforeLoadSnapshotRef = React.useRef<{
    scrollHeight: number
    scrollTop: number
  } | null>(null)

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
      beforeLoadSnapshotRef.current = null
    } else if (
      firstMessageId &&
      previousFirstMessageId &&
      firstMessageId !== previousFirstMessageId &&
      beforeLoadSnapshotRef.current
    ) {
      const snapshot = beforeLoadSnapshotRef.current
      viewport.scrollTop =
        snapshot.scrollTop + (viewport.scrollHeight - snapshot.scrollHeight)
      beforeLoadSnapshotRef.current = null
    } else if (
      lastMessageId &&
      previousLastMessageId !== lastMessageId &&
      messages.length >= previousMessagesLength
    ) {
      scrollToBottom(viewport)
    }

    previousConversationIdRef.current = conversation.id
    previousFirstMessageIdRef.current = firstMessageId
    previousLastMessageIdRef.current = lastMessageId
    previousMessagesLengthRef.current = messages.length
  }, [conversation.id, messages])

  function handleViewportScroll(event: React.UIEvent<HTMLDivElement>) {
    const viewport = event.currentTarget

    if (loadingBefore || viewport.scrollTop > 80) {
      return
    }

    beforeLoadSnapshotRef.current = {
      scrollHeight: viewport.scrollHeight,
      scrollTop: viewport.scrollTop,
    }
    onLoadBeforeMessages()
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

  if (loading) {
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

  if (error && messages.length === 0) {
    return (
      <div
        className="flex min-h-0 flex-1 items-center justify-center bg-muted/10 px-6 text-center text-sm text-muted-foreground"
        data-testid="conversation-history-error"
      >
        {error}
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <Empty
        className="h-full min-h-0 flex-1 rounded-none bg-muted/10"
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
    )
  }

  return (
    <ScrollArea
      className="min-h-0 flex-1 bg-muted/10"
      data-testid="conversation-panel-history"
      viewportProps={{
        onContextMenu: handleHistoryContextMenu,
        onScroll: handleViewportScroll,
      }}
      viewportRef={viewportRef}
    >
      <div
        className="flex w-full flex-col gap-5 px-5 py-6"
        data-testid="conversation-history-content"
      >
        {loadingBefore && (
          <div
            className="flex items-center justify-center gap-2 text-xs text-muted-foreground"
            data-testid="conversation-history-loading-before"
          >
            <LoaderCircle className="size-3.5 animate-spin" />
            <span>正在加载更早消息</span>
          </div>
        )}
        {messages.map((message) => (
          <MessageBubble
            key={message.id}
            message={message}
            conversation={conversation}
          />
        ))}
      </div>
    </ScrollArea>
  )
}

function ConversationPanelComposer({
  draft,
  onDraftChange,
  onSendMessage,
  sending,
}: {
  draft: string
  onDraftChange: (draft: string) => void
  onSendMessage: () => void
  sending: boolean
}) {
  const textareaRef = React.useRef<HTMLTextAreaElement | null>(null)
  const [expressionPickerOpen, setExpressionPickerOpen] = React.useState(false)

  function handleComposerKeyDown(
    event: React.KeyboardEvent<HTMLTextAreaElement>
  ) {
    if (event.key !== "Enter") {
      return
    }

    if (event.shiftKey || event.ctrlKey) {
      event.preventDefault()
      insertTextareaText(event.currentTarget, "\n", onDraftChange)
      return
    }

    event.preventDefault()
    if (!sending) {
      onSendMessage()
    }
  }

  function handleExpressionSelect(item: ExpressionItem) {
    const textarea = textareaRef.current

    if (!textarea) {
      onDraftChange(draft + item.value)
      setExpressionPickerOpen(false)
      return
    }

    insertTextareaText(textarea, item.value, onDraftChange)
    setExpressionPickerOpen(false)
    window.requestAnimationFrame(() => {
      textarea.focus()
    })
  }

  return (
    <footer
      className="shrink-0 border-t p-4"
      data-testid="conversation-panel-composer"
    >
      <div
        className="flex w-full flex-col gap-2"
        data-testid="conversation-panel-composer-content"
      >
        <div data-testid="conversation-panel-editor-row">
          <Textarea
            ref={textareaRef}
            value={draft}
            onChange={(event) => onDraftChange(event.target.value)}
            onKeyDown={handleComposerKeyDown}
            placeholder="输入消息"
            className="max-h-48 min-h-24 resize-none"
          />
        </div>
        <div
          className="flex items-center justify-between gap-2"
          data-testid="conversation-panel-toolbar-row"
        >
          <div className="flex items-center gap-1">
            <Popover
              open={expressionPickerOpen}
              onOpenChange={setExpressionPickerOpen}
            >
              <PopoverTrigger asChild>
                <Button
                  aria-label="选择表情"
                  size="icon-sm"
                  title="选择表情"
                  type="button"
                  variant="ghost"
                >
                  <Smile className="size-4" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-auto p-3" side="top">
                <ExpressionPicker onSelect={handleExpressionSelect} />
              </PopoverContent>
            </Popover>
            <Button
              aria-label="上传文件"
              disabled
              size="icon-sm"
              title="上传文件"
              type="button"
              variant="ghost"
            >
              <Paperclip className="size-4" />
            </Button>
            <Button
              aria-label="插入图片"
              disabled
              size="icon-sm"
              title="插入图片"
              type="button"
              variant="ghost"
            >
              <ImageIcon className="size-4" />
            </Button>
          </div>
          <Button
            type="button"
            aria-label="发送消息"
            className="shrink-0"
            disabled={sending}
            onClick={onSendMessage}
          >
            {sending ? (
              <LoaderCircle className="size-4 animate-spin" />
            ) : (
              <Send className="size-4" />
            )}
            <span aria-hidden="true">发送</span>
          </Button>
        </div>
      </div>
    </footer>
  )
}

function insertTextareaText(
  textarea: HTMLTextAreaElement,
  text: string,
  onChange: (value: string) => void
) {
  const selectionStart = textarea.selectionStart
  const selectionEnd = textarea.selectionEnd
  const nextValue =
    textarea.value.slice(0, selectionStart) +
    text +
    textarea.value.slice(selectionEnd)
  const nextCursor = selectionStart + text.length

  textarea.value = nextValue
  textarea.setSelectionRange(nextCursor, nextCursor)
  onChange(nextValue)
}

function ConversationPanelEmptyState() {
  return (
    <div
      className="flex flex-1 items-center justify-center self-stretch text-sm text-muted-foreground"
      data-testid="chat-empty-state"
    >
      选择一个会话开始聊天
    </div>
  )
}

function MessageBubble({
  message,
  conversation,
}: {
  message: ConversationPanelMessage
  conversation: ClientConversation
}) {
  const fromMe = message.role === "me"
  const fallback = fromMe ? "我" : getConversationInitial(conversation.name)

  return (
    <div className={cn("flex gap-3", fromMe ? "justify-end" : "justify-start")}>
      {!fromMe && <MessageAvatar fallback={fallback} message={message} />}
      <div
        className={cn(
          "flex max-w-[min(70%,42rem)] flex-col gap-1",
          fromMe && "items-end"
        )}
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>{message.author}</span>
          <span>{message.time}</span>
        </div>
        <MessageActionMenu>
          <div
            className={cn(
              "rounded-md px-4 py-3 text-sm leading-relaxed shadow-xs",
              fromMe
                ? "bg-teal-100 text-foreground hover:bg-teal-200 data-[state=open]:bg-teal-200 dark:bg-teal-950 hover:dark:bg-teal-900 dark:data-[state=open]:bg-teal-900"
                : "bg-neutral-200 text-foreground hover:bg-neutral-300 data-[state=open]:bg-neutral-300 dark:bg-neutral-800 hover:dark:bg-neutral-700 dark:data-[state=open]:bg-neutral-700"
            )}
            data-message-action-trigger
          >
            <MessageBodyRenderer body={message.body} />
          </div>
        </MessageActionMenu>
      </div>
      {fromMe && (
        <MessageAvatar
          fallback="我"
          fallbackClassName="bg-primary text-primary-foreground"
          message={message}
        />
      )}
    </div>
  )
}

function MessageAvatar({
  fallback,
  fallbackClassName,
  message,
}: {
  fallback: string
  fallbackClassName?: string
  message: ConversationPanelMessage
}) {
  const avatar = (
    <Avatar className="mt-1 size-8 rounded-sm bg-muted after:rounded-sm">
      {message.avatar && (
        <AvatarImage
          alt={message.author}
          className="rounded-sm"
          src={message.avatar}
        />
      )}
      <AvatarFallback className={cn("rounded-sm", fallbackClassName)}>
        {fallback}
      </AvatarFallback>
    </Avatar>
  )

  return (
    <UserProfilePopover userId={message.senderUserId}>
      {avatar}
    </UserProfilePopover>
  )
}

function MessageBodyRenderer({
  body,
}: {
  body: ConversationPanelMessage["body"]
}) {
  switch (body.type) {
    case "text":
      return <TextMessageBody content={body.content} />
  }
}

function TextMessageBody({ content }: { content: string }) {
  return <span className="break-words whitespace-pre-wrap">{content}</span>
}

function getConversationHeaderDescription(conversation: ClientConversation) {
  if (conversation.type === "direct") {
    return "私聊"
  }
  if (conversation.type === "app") {
    return "应用会话"
  }

  return `${conversation.memberCount} 人群聊`
}

function getConversationInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}

function scrollToBottom(viewport: HTMLDivElement) {
  viewport.scrollTop = viewport.scrollHeight
}
