import * as React from "react"
import { Bot, MessagesSquare } from "lucide-react"
import { toast } from "sonner"
import { getAvatarInitial } from "@/lib/avatar"
import { cn } from "@/lib/utils"
import {
  formatClientMessageBodySummary,
  type ClientConversation,
} from "@/lib/client-data-api"
import {
  formatMentionTemplateText,
  parseMentionTemplate,
  type MentionLabelResolver,
} from "@/lib/message-mentions"
import { AppProfilePopover } from "@/components/app-profile-popover"
import { MessageAttachment } from "@/components/message-attachment"
import { MessageImage } from "@/components/message-image"
import { MessageTextWithLinks } from "@/components/message-inline-link"
import { MessageLink } from "@/components/message-link"
import { MessageMarkdown } from "@/components/message-markdown"
import { MessageCard } from "@/components/message-card"
import { MessageRenderErrorBoundary } from "@/components/message-render-error-boundary"
import { MessageVoice } from "@/components/message-voice"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { MessageActionMenu } from "@/components/message-action-menu"
import { UserProfilePopover } from "@/components/user-profile-popover"
import type {
  ConversationPanelMentionTarget,
  ConversationPanelMessage,
  ConversationPanelReplyTarget,
} from "@/lib/conversation-panel-types"

const MessageChart = React.lazy(async () => {
  const module = await import("@/components/message-chart")
  return { default: module.MessageChart }
})

export const SystemMessageBadge = React.memo(function SystemMessageBadge({
  currentUserId,
  mentionLabelResolver,
  message,
}: {
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
  message: ConversationPanelMessage
}) {
  return (
    <div
      className="flex justify-center"
      data-conversation-message-id={message.id}
    >
      <Badge
        className="h-auto max-w-[min(80%,36rem)] text-center leading-relaxed whitespace-normal"
        variant="secondary"
      >
        <MessageBodyRenderer
          body={message.body}
          currentUserId={currentUserId}
          mentionLabelResolver={mentionLabelResolver}
        />
      </Badge>
    </div>
  )
}, areSystemMessageBadgePropsEqual)

type MessageBubbleProps = {
  message: ConversationPanelMessage
  conversation: ClientConversation
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
  onInsertMention: (target: ConversationPanelMentionTarget) => void
  onForward?: (message: ConversationPanelMessage) => void
  onMultiSelect?: (message: ConversationPanelMessage) => void
  onReply: (message: ConversationPanelMessage) => void
  onRevoke: (message: ConversationPanelMessage) => void
  onToggleSelected?: (message: ConversationPanelMessage) => void
  selectable?: boolean
  selected?: boolean
  selectionMode?: boolean
}

export const MessageBubble = React.memo(function MessageBubble({
  message,
  conversation,
  currentUserId,
  mentionLabelResolver,
  onInsertMention,
  onForward,
  onMultiSelect,
  onReply,
  onRevoke,
  onToggleSelected,
  selectable = true,
  selected = false,
  selectionMode = false,
}: MessageBubbleProps) {
  const fromMe = message.role === "me"
  const fallback = message.senderAppId ? (
    <Bot className="size-4" />
  ) : fromMe ? (
    "我"
  ) : (
    getAvatarInitial(conversation.name)
  )
  const canInsertAuthorMention =
    conversation.type === "group" && message.mentionTarget !== null
  const unavailable =
    message.body.type === "revoked" || message.body.type === "unsupported"
  const copyText = getMessageCopyText(message, mentionLabelResolver)
  const bubbleRef = React.useRef<HTMLDivElement | null>(null)
  const selectedCopyTextRef = React.useRef("")

  function handleMessageContextMenu() {
    selectedCopyTextRef.current = getSelectedTextWithinElement(
      bubbleRef.current
    )
  }

  function handleCopyMessage() {
    const selectedText = selectedCopyTextRef.current
    selectedCopyTextRef.current = ""

    void copyMessageToClipboard(
      message,
      selectedText,
      bubbleRef.current,
      mentionLabelResolver
    )
  }

  function handleAuthorMentionClick() {
    if (!message.mentionTarget) {
      return
    }

    onInsertMention(message.mentionTarget)
  }

  function handleSelectionClick(event: React.MouseEvent<HTMLDivElement>) {
    if (!selectionMode || !selectable || !onToggleSelected) {
      return
    }
    if (
      event.target instanceof Element &&
      event.target.closest("[data-slot=checkbox]")
    ) {
      return
    }
    event.preventDefault()
    event.stopPropagation()
    onToggleSelected(message)
  }

  const messageBody = (
    <div
      className={cn(
        "max-w-full rounded-md p-3 text-sm leading-relaxed shadow-sm",
        fromMe
          ? "bg-teal-100/60 text-foreground dark:bg-teal-950/80"
          : "bg-zinc-100 text-foreground dark:bg-zinc-800",
        !selectionMode &&
          (fromMe
            ? "hover:bg-teal-100/80 data-[state=open]:bg-teal-100/80 hover:dark:bg-teal-950 dark:data-[state=open]:bg-teal-950"
            : "hover:bg-zinc-200/60 data-[state=open]:bg-zinc-200 hover:dark:bg-zinc-700/60 dark:data-[state=open]:bg-zinc-700")
      )}
      data-message-action-trigger={
        !selectionMode && !unavailable ? "" : undefined
      }
      onContextMenu={
        !selectionMode && !unavailable ? handleMessageContextMenu : undefined
      }
      ref={bubbleRef}
    >
      {message.replyTo && <MessageReplyReference replyTo={message.replyTo} />}
      <MessageBodyRenderer
        body={message.body}
        currentUserId={currentUserId}
        mentionLabelResolver={mentionLabelResolver}
      />
    </div>
  )

  return (
    <div
      className={cn(
        "relative rounded-md transition-colors",
        selectionMode && "px-3 py-2 pl-11",
        selected && "bg-primary/5"
      )}
      data-conversation-message-id={message.id}
      data-message-selection-row
      onClickCapture={handleSelectionClick}
    >
      {selectionMode && (
        <Checkbox
          aria-label={`${selected ? "取消选择" : "选择"}${message.author}的消息`}
          checked={selected}
          className="absolute top-4 left-3"
          disabled={!selectable}
          onCheckedChange={() => onToggleSelected?.(message)}
        />
      )}
      <div
        className={cn("flex gap-3", fromMe ? "justify-end" : "justify-start")}
      >
        {!fromMe && <MessageAvatar fallback={fallback} message={message} />}
        <div
          className={cn(
            "flex max-w-[min(70%,64rem)] flex-col gap-1",
            fromMe ? "items-end" : "items-start"
          )}
        >
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {canInsertAuthorMention && !selectionMode ? (
              <button
                className="cursor-pointer p-0 text-muted-foreground transition-colors hover:text-sky-500"
                onClick={handleAuthorMentionClick}
                onMouseDown={(event) => event.preventDefault()}
                type="button"
              >
                {message.author}
              </button>
            ) : (
              <span>{message.author}</span>
            )}
            <span>{message.time}</span>
          </div>
          {selectionMode || unavailable ? (
            messageBody
          ) : (
            <MessageActionMenu
              canRevoke={message.canRevoke}
              copyDisabled={!copyText}
              onCopy={handleCopyMessage}
              onForward={onForward ? () => onForward(message) : undefined}
              onMultiSelect={
                onMultiSelect ? () => onMultiSelect(message) : undefined
              }
              onReply={() => onReply(message)}
              onRevoke={() => onRevoke(message)}
            >
              {messageBody}
            </MessageActionMenu>
          )}
          {message.delegatedByName && (
            <div className="text-xs text-muted-foreground">
              由 {message.delegatedByName} 代发
            </div>
          )}
        </div>
        {fromMe && (
          <MessageAvatar
            fallback="我"
            fallbackClassName="bg-primary text-primary-foreground"
            message={message}
          />
        )}
      </div>
    </div>
  )
}, areMessageBubblePropsEqual)

function areSystemMessageBadgePropsEqual(
  previous: {
    currentUserId: string
    mentionLabelResolver: MentionLabelResolver
    message: ConversationPanelMessage
  },
  next: {
    currentUserId: string
    mentionLabelResolver: MentionLabelResolver
    message: ConversationPanelMessage
  }
) {
  return (
    previous.currentUserId === next.currentUserId &&
    previous.message.body === next.message.body &&
    (previous.mentionLabelResolver === next.mentionLabelResolver ||
      !messageBodyUsesMentionLabels(next.message.body))
  )
}

function areMessageBubblePropsEqual(
  previous: MessageBubbleProps,
  next: MessageBubbleProps
) {
  return (
    previous.conversation.name === next.conversation.name &&
    previous.conversation.type === next.conversation.type &&
    previous.currentUserId === next.currentUserId &&
    previous.onForward === next.onForward &&
    previous.onInsertMention === next.onInsertMention &&
    previous.onMultiSelect === next.onMultiSelect &&
    previous.onReply === next.onReply &&
    previous.onRevoke === next.onRevoke &&
    previous.onToggleSelected === next.onToggleSelected &&
    previous.selectable === next.selectable &&
    previous.selected === next.selected &&
    previous.selectionMode === next.selectionMode &&
    arePanelMessagesEqual(previous.message, next.message) &&
    (previous.mentionLabelResolver === next.mentionLabelResolver ||
      !messageBodyUsesMentionLabels(next.message.body))
  )
}

function arePanelMessagesEqual(
  previous: ConversationPanelMessage,
  next: ConversationPanelMessage
) {
  return (
    previous.id === next.id &&
    previous.author === next.author &&
    previous.avatar === next.avatar &&
    previous.body === next.body &&
    previous.canRevoke === next.canRevoke &&
    previous.delegatedByName === next.delegatedByName &&
    previous.role === next.role &&
    previous.senderAppId === next.senderAppId &&
    previous.senderUserId === next.senderUserId &&
    previous.time === next.time &&
    areMentionTargetsEqual(previous.mentionTarget, next.mentionTarget) &&
    areReplyTargetsEqual(previous.replyTo, next.replyTo) &&
    areAppProfilesEqual(previous.senderAppProfile, next.senderAppProfile)
  )
}

function areMentionTargetsEqual(
  previous: ConversationPanelMessage["mentionTarget"],
  next: ConversationPanelMessage["mentionTarget"]
) {
  return (
    previous === next ||
    (previous !== null &&
      next !== null &&
      previous.id === next.id &&
      previous.label === next.label &&
      previous.targetType === next.targetType)
  )
}

function areReplyTargetsEqual(
  previous: ConversationPanelMessage["replyTo"],
  next: ConversationPanelMessage["replyTo"]
) {
  return (
    previous === next ||
    (previous !== undefined &&
      next !== undefined &&
      previous.id === next.id &&
      previous.author === next.author &&
      previous.summary === next.summary)
  )
}

function areAppProfilesEqual(
  previous: ConversationPanelMessage["senderAppProfile"],
  next: ConversationPanelMessage["senderAppProfile"]
) {
  return (
    previous === next ||
    (previous !== null &&
      next !== null &&
      previous.id === next.id &&
      previous.avatar === next.avatar &&
      previous.description === next.description &&
      previous.name === next.name &&
      previous.online === next.online)
  )
}

async function copyMessageToClipboard(
  message: ConversationPanelMessage,
  selectedText: string,
  messageElement: HTMLElement | null,
  mentionLabelResolver: MentionLabelResolver
) {
  const text =
    (selectedText.trim()
      ? selectedText
      : getSelectedTextWithinElement(messageElement)) ||
    getMessageCopyText(message, mentionLabelResolver)
  if (!text) {
    toast.error("没有可复制内容")
    return
  }

  try {
    await writeClipboardText(text)
    toast.success("已复制")
  } catch {
    toast.error("复制失败")
  }
}

function getSelectedTextWithinElement(element: HTMLElement | null) {
  if (!element) {
    return ""
  }

  const selection = window.getSelection()
  const selectedText = selection?.toString() ?? ""
  if (!selection || selection.isCollapsed || !selectedText.trim()) {
    return ""
  }

  for (let index = 0; index < selection.rangeCount; index += 1) {
    if (rangeIntersectsElement(selection.getRangeAt(index), element)) {
      return selectedText
    }
  }

  return ""
}

function rangeIntersectsElement(range: Range, element: HTMLElement) {
  try {
    return range.intersectsNode(element)
  } catch {
    return false
  }
}

function getMessageCopyText(
  message: ConversationPanelMessage,
  mentionLabelResolver: MentionLabelResolver
) {
  switch (message.body.type) {
    case "file":
      return message.body.name
    case "image":
      return ""
    case "voice":
      return ""
    case "revoked":
      return ""
    case "unsupported":
      return ""
    case "link":
      return message.body.url
    case "card":
      return `${message.body.title}\n${message.body.description}\n${message.body.url}`
    case "chart":
      return `${message.body.title}\n${message.body.description}`
    case "markdown":
    case "text":
      return formatMentionTemplateText(
        message.body.content,
        mentionLabelResolver
      )
    case "forward_bundle":
      return formatClientMessageBodySummary(message.body)
    case "system_event":
      return formatClientMessageBodySummary(message.body)
  }
}

async function writeClipboardText(text: string) {
  if (!window.isSecureContext || !navigator.clipboard?.writeText) {
    throw new Error("clipboard is unavailable")
  }

  await navigator.clipboard.writeText(text)
}

function MessageReplyReference({
  replyTo,
}: {
  replyTo: ConversationPanelReplyTarget
}) {
  return (
    <div className="mb-2 border-l-2 border-foreground/20 pl-2 text-xs">
      <div className="truncate font-medium text-foreground/80">
        {replyTo.author}
      </div>
      <div className="line-clamp-2 text-muted-foreground">
        {replyTo.summary}
      </div>
    </div>
  )
}

function MessageAvatar({
  fallback,
  fallbackClassName,
  message,
}: {
  fallback: React.ReactNode
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

  return <MessageAvatarProfile message={message}>{avatar}</MessageAvatarProfile>
}

function MessageAvatarProfile({
  children,
  message,
}: {
  children: React.ReactNode
  message: ConversationPanelMessage
}) {
  if (message.senderAppId) {
    return (
      <AppProfilePopover
        appId={message.senderAppId}
        fallbackProfile={message.senderAppProfile}
        triggerAriaLabel={`${message.author}资料`}
      >
        {children}
      </AppProfilePopover>
    )
  }

  return (
    <UserProfilePopover userId={message.senderUserId}>
      {children}
    </UserProfilePopover>
  )
}

type MessageBodyRendererProps = {
  body: ConversationPanelMessage["body"]
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
}

const MessageBodyRenderer = React.memo(function MessageBodyRenderer({
  body,
  currentUserId,
  mentionLabelResolver,
}: MessageBodyRendererProps) {
  switch (body.type) {
    case "file":
      return <MessageAttachment file={body} />
    case "image":
      return <MessageImage image={body} />
    case "voice":
      return <MessageVoice voice={body} />
    case "link":
      return <MessageLink link={body} />
    case "card":
      return <MessageCard card={body} />
    case "chart":
      return (
        <MessageRenderErrorBoundary
          fallback={
            <span className="text-muted-foreground">暂不支持查看该消息</span>
          }
          resetKey={body}
        >
          <React.Suspense
            fallback={
              <div className="h-64 w-160 max-w-full animate-pulse rounded-sm bg-foreground/5" />
            }
          >
            <MessageChart chart={body} />
          </React.Suspense>
        </MessageRenderErrorBoundary>
      )
    case "markdown":
      return (
        <MessageMarkdown
          content={body.content}
          currentUserId={currentUserId}
          mentionLabelResolver={mentionLabelResolver}
        />
      )
    case "text":
      return (
        <TextMessageBody
          content={body.content}
          currentUserId={currentUserId}
          mentionLabelResolver={mentionLabelResolver}
        />
      )
    case "forward_bundle":
      return (
        <ForwardBundleMessage
          body={body}
          currentUserId={currentUserId}
          mentionLabelResolver={mentionLabelResolver}
        />
      )
    case "revoked":
      return <span className="text-muted-foreground">该消息已被撤回</span>
    case "unsupported":
      return <span className="text-muted-foreground">暂不支持查看该消息</span>
    case "system_event":
      return <span>{formatClientMessageBodySummary(body)}</span>
  }
}, areMessageBodyRendererPropsEqual)

function areMessageBodyRendererPropsEqual(
  previous: MessageBodyRendererProps,
  next: MessageBodyRendererProps
) {
  return (
    previous.body === next.body &&
    previous.currentUserId === next.currentUserId &&
    (previous.mentionLabelResolver === next.mentionLabelResolver ||
      !messageBodyUsesMentionLabels(next.body))
  )
}

function messageBodyUsesMentionLabels(
  body: ConversationPanelMessage["body"]
): boolean {
  if (body.type === "text" || body.type === "markdown") {
    return body.content.includes("{(@")
  }

  if (body.type === "forward_bundle") {
    return body.items.some((item) => messageBodyUsesMentionLabels(item.body))
  }

  return false
}

function ForwardBundleMessage({
  body,
  currentUserId,
  mentionLabelResolver,
}: {
  body: Extract<ConversationPanelMessage["body"], { type: "forward_bundle" }>
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
}) {
  const summary = formatClientMessageBodySummary(body)

  return (
    <Dialog>
      <DialogTrigger asChild>
        <button
          className="flex w-72 max-w-full cursor-pointer items-center gap-3 rounded-sm text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
          type="button"
        >
          <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-background/70 text-muted-foreground">
            <MessagesSquare aria-hidden="true" className="size-5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block font-medium">聊天记录</span>
            <span className="block max-w-80 truncate text-xs text-muted-foreground">
              {summary}
            </span>
          </span>
        </button>
      </DialogTrigger>
      <DialogContent
        aria-describedby={undefined}
        className="max-h-[80vh] grid-rows-[auto_minmax(0,1fr)] gap-4 overflow-hidden sm:max-w-2xl"
      >
        <DialogHeader>
          <DialogTitle>聊天记录</DialogTitle>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto overscroll-contain rounded-md border px-4">
          {body.items.map((item, index) => (
            <div
              className="border-b py-4 last:border-b-0"
              key={`${item.sentAt}-${index}`}
            >
              <div className="mb-2 flex items-center justify-between gap-3 text-xs text-muted-foreground">
                <span className="truncate font-medium text-foreground/80">
                  {item.senderName}
                </span>
                <span className="shrink-0">
                  {formatForwardBundleItemTime(item.sentAt)}
                </span>
              </div>
              <ForwardBundleItemBody
                body={item.body}
                currentUserId={currentUserId}
                mentionLabelResolver={mentionLabelResolver}
              />
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ForwardBundleItemBody({
  body,
  currentUserId,
  mentionLabelResolver,
}: {
  body: ConversationPanelMessage["body"]
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
}) {
  return (
    <div
      className="w-full rounded-md bg-zinc-100 p-3 dark:bg-zinc-800"
      data-forward-bundle-item-body
    >
      <MessageBodyRenderer
        body={body}
        currentUserId={currentUserId}
        mentionLabelResolver={mentionLabelResolver}
      />
    </div>
  )
}

function formatForwardBundleItemTime(sentAt: string) {
  const date = new Date(sentAt)
  if (Number.isNaN(date.getTime())) {
    return ""
  }
  const dateText = new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
  }).format(date)
  const timeText = new Intl.DateTimeFormat("zh-CN", {
    timeStyle: "short",
  }).format(date)

  return `${dateText} ${timeText}`
}

function TextMessageBody({
  content,
  currentUserId,
  mentionLabelResolver,
}: {
  content: string
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
}) {
  const parts = parseMentionTemplate(content, mentionLabelResolver)

  return (
    <span className="break-all whitespace-pre-wrap">
      {parts.map((part, index) =>
        part.type === "text" ? (
          <MessageTextWithLinks key={`text-${index}`} text={part.text} />
        ) : (
          <MentionTextPart
            key={`${part.targetType}-${part.id}-${index}`}
            currentUserId={currentUserId}
            part={part}
          />
        )
      )}
    </span>
  )
}

function MentionTextPart({
  currentUserId,
  part,
}: {
  currentUserId: string
  part: Extract<
    ReturnType<typeof parseMentionTemplate>[number],
    { type: "mention" }
  >
}) {
  const isCurrentUserMention =
    part.targetType === "all" ||
    (part.targetType === "user" && isSameUserId(part.id, currentUserId))
  const content = (
    <span className={getMentionTextClassName(isCurrentUserMention)}>
      {part.label}
    </span>
  )

  if (part.targetType !== "user") {
    if (part.targetType === "app") {
      return (
        <AppProfilePopover
          appId={part.id}
          fallbackProfile={{
            avatar: "",
            description: "",
            id: part.id,
            name: part.label.replace(/^@/, ""),
            online: false,
          }}
        >
          {content}
        </AppProfilePopover>
      )
    }

    return content
  }

  return <UserProfilePopover userId={part.id}>{content}</UserProfilePopover>
}

function getMentionTextClassName(isCurrentUserMention: boolean) {
  return isCurrentUserMention
    ? "mx-0.5 font-medium text-amber-600 hover:text-amber-700"
    : "mx-0.5 font-medium text-sky-500 hover:text-sky-600"
}

function isSameUserId(userId: string | undefined, currentUserId: string) {
  return userId?.toLowerCase() === currentUserId.toLowerCase()
}
