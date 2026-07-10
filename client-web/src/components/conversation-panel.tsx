import * as React from "react"
import {
  FolderClosed,
  ImageIcon,
  LoaderCircle,
  MessageCircle,
  Paperclip,
  Send,
  Settings,
  Smile,
  Upload,
  UsersRound,
  X,
} from "lucide-react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { getConversationMemberMentionLabel } from "@/lib/conversation-mention-labels"
import {
  formatClientMessageBodySummary,
  type ClientConversation,
  type ClientConversationMember,
  type ClientMessage,
} from "@/lib/client-data-api"
import {
  compressImageForMessage,
  imageMessageMaxBytes,
  isAcceptedImageMessageFile,
  isAcceptedImageMessageMimeType,
} from "@/lib/image-message"
import {
  createMentionToken,
  formatMentionTemplateText,
  parseMentionTemplate,
  type MentionLabelResolver,
  type MentionTargetType,
} from "@/lib/message-mentions"
import type {
  ConversationDraftMention,
  ConversationDraftReplyTarget,
} from "@/lib/conversation-drafts"
import {
  createPinyinSearchText,
  normalizePinyinSearchQuery,
} from "@/lib/pinyin-search"
import { AddGroupMembersDialog } from "@/components/add-group-members-dialog"
import { AppProfilePopover } from "@/components/app-profile-popover"
import { ConversationInfoDrawer } from "@/components/conversation-info-drawer"
import {
  ExpressionPicker,
  type ExpressionItem,
} from "@/components/expression-picker"
import { GroupAvatar } from "@/components/group-avatar"
import { GroupProfilePopover } from "@/components/group-profile-popover"
import { MarkdownIcon } from "@/components/icons/markdown-icon"
import { MessageAttachment } from "@/components/message-attachment"
import { MessageImage } from "@/components/message-image"
import { MessageTextWithLinks } from "@/components/message-inline-link"
import { MessageLink } from "@/components/message-link"
import { MessageMarkdown } from "@/components/message-markdown"
import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarImage,
} from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { MessageActionMenu } from "@/components/message-action-menu"
import { SendFileMessageDialog } from "@/components/send-file-message-dialog"
import { SendImageMessageDialog } from "@/components/send-image-message-dialog"
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
import { Toggle } from "@/components/ui/toggle"

export type ConversationPanelMessage = {
  id: string
  role: "me" | "other" | "system"
  author: string
  avatar: string
  body: ClientMessage["body"]
  canRevoke: boolean
  delegatedByName: string
  mentionTarget: ConversationPanelMentionTarget | null
  replyTo?: ConversationPanelReplyTarget
  time: string
  senderAppId: string | null
  senderAppProfile: ConversationPanelAppProfile | null
  senderUserId: string | null
}

export type ConversationPanelAppProfile = {
  avatar: string
  description: string
  id: string
  name: string
  online: boolean
}

export type ConversationPanelMentionTarget = {
  id: string
  label: string
  targetType: MentionTargetType
}

export type ConversationPanelReplyTarget = ConversationDraftReplyTarget

const maxFileMessageUploadBytes = 20 * 1024 * 1024
const maxMentionCandidateResults = 50
const fallbackMentionLabelResolver: MentionLabelResolver = () => undefined
const emptyDraftMentions: ConversationDraftMention[] = []

type MentionCandidate = {
  avatar: string
  description: string
  searchText: string
} & ConversationPanelMentionTarget

type ConversationPanelComposerHandle = {
  focus: () => void
  insertMention: (target: ConversationPanelMentionTarget) => void
  openDroppedFile: (file: File) => void
}

type DraggedFileKind = "file" | "image"

type MentionTrigger = {
  query: string
  start: number
}

type ConversationPanelProps = {
  conversation: ClientConversation | null
  conversationOnline?: boolean
  currentUserId: string
  draft: string
  draftMentions?: ConversationDraftMention[]
  historyError: string | null
  historyLoading: boolean
  historyLoadingBefore: boolean
  mentionLabelResolver?: MentionLabelResolver
  messages: ConversationPanelMessage[]
  onDraftBlur?: () => void
  onDraftChange: (draft: string, mentions: ConversationDraftMention[]) => void
  onCancelReply: () => void
  onReplyToMessage: (message: ConversationPanelMessage) => void
  onRevokeMessage: (message: ConversationPanelMessage) => void
  onSendFile: (file: File) => Promise<ClientMessage | null>
  onSendImage: (image: File) => Promise<ClientMessage | null>
  onLoadBeforeMessages: () => void
  onRichTextModeChange: (richTextMode: boolean) => void
  onSendMessage: (content?: string) => void
  replyTarget: ConversationPanelReplyTarget | null
  richTextMode: boolean
  sending: boolean
}

export function ConversationPanel({
  conversation,
  conversationOnline,
  currentUserId,
  draft,
  draftMentions = emptyDraftMentions,
  historyError,
  historyLoading,
  historyLoadingBefore,
  mentionLabelResolver = fallbackMentionLabelResolver,
  messages,
  onDraftBlur,
  onDraftChange,
  onCancelReply,
  onReplyToMessage,
  onRevokeMessage,
  onSendFile,
  onSendImage,
  onLoadBeforeMessages,
  onRichTextModeChange,
  onSendMessage,
  replyTarget,
  richTextMode,
  sending,
}: ConversationPanelProps) {
  const composerRef = React.useRef<ConversationPanelComposerHandle | null>(null)
  const fileDragDepthRef = React.useRef(0)
  const [draggedFileKind, setDraggedFileKind] =
    React.useState<DraggedFileKind | null>(null)

  const insertComposerMention = React.useCallback(
    (target: ConversationPanelMentionTarget) => {
      if (conversation?.type !== "group") {
        composerRef.current?.focus()
        return
      }

      composerRef.current?.insertMention(target)
    },
    [conversation?.type]
  )

  const handleReplyToMessage = React.useCallback(
    (message: ConversationPanelMessage) => {
      onReplyToMessage(message)

      if (conversation?.type === "group" && message.mentionTarget) {
        composerRef.current?.insertMention(message.mentionTarget)
        return
      }

      composerRef.current?.focus()
    },
    [conversation?.type, onReplyToMessage]
  )

  function resetFileDrag() {
    fileDragDepthRef.current = 0
    setDraggedFileKind(null)
  }

  function handlePanelDragEnter(event: React.DragEvent<HTMLElement>) {
    if (!hasDraggedFiles(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    event.dataTransfer.dropEffect = conversation && !sending ? "copy" : "none"

    if (!conversation || sending) {
      return
    }

    fileDragDepthRef.current += 1
    setDraggedFileKind(getDraggedFileKind(event.dataTransfer))
  }

  function handlePanelDragOver(event: React.DragEvent<HTMLElement>) {
    if (!hasDraggedFiles(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    event.dataTransfer.dropEffect = conversation && !sending ? "copy" : "none"
  }

  function handlePanelDragLeave(event: React.DragEvent<HTMLElement>) {
    if (fileDragDepthRef.current === 0) {
      return
    }

    event.preventDefault()
    fileDragDepthRef.current = Math.max(0, fileDragDepthRef.current - 1)

    if (fileDragDepthRef.current === 0) {
      setDraggedFileKind(null)
    }
  }

  function handlePanelDrop(event: React.DragEvent<HTMLElement>) {
    if (!hasDraggedFiles(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    const file = event.dataTransfer.files[0]

    resetFileDrag()

    if (!conversation || sending || !file) {
      return
    }

    composerRef.current?.openDroppedFile(file)
  }

  return (
    <main
      className={cn(
        "relative flex min-w-0 flex-1 flex-col",
        conversation ? "bg-background" : "bg-muted"
      )}
      data-testid="chat-detail-shell"
      onDragEnter={handlePanelDragEnter}
      onDragLeave={handlePanelDragLeave}
      onDragOver={handlePanelDragOver}
      onDrop={handlePanelDrop}
    >
      {conversation ? (
        <>
          <ConversationPanelHeader
            conversation={conversation}
            currentUserId={currentUserId}
            online={conversationOnline}
          />
          <ConversationPanelHistory
            conversation={conversation}
            error={historyError}
            loading={historyLoading}
            loadingBefore={historyLoadingBefore}
            currentUserId={currentUserId}
            mentionLabelResolver={mentionLabelResolver}
            messages={messages}
            onLoadBeforeMessages={onLoadBeforeMessages}
            onInsertMention={insertComposerMention}
            onReplyToMessage={handleReplyToMessage}
            onRevokeMessage={onRevokeMessage}
          />
          <ConversationPanelComposer
            ref={composerRef}
            conversation={conversation}
            draft={draft}
            draftMentions={draftMentions}
            replyTarget={replyTarget}
            onCancelReply={onCancelReply}
            onDraftBlur={onDraftBlur}
            onDraftChange={onDraftChange}
            onSendFile={onSendFile}
            onSendImage={onSendImage}
            onSendMessage={onSendMessage}
            onRichTextModeChange={onRichTextModeChange}
            richTextMode={richTextMode}
            sending={sending}
          />
        </>
      ) : (
        <ConversationPanelEmptyState />
      )}
      {conversation && draggedFileKind && (
        <ConversationFileDropOverlay kind={draggedFileKind} />
      )}
    </main>
  )
}

function ConversationFileDropOverlay({ kind }: { kind: DraggedFileKind }) {
  const isImage = kind === "image"

  return (
    <div
      aria-live="polite"
      className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center bg-teal-700/10 p-3 backdrop-blur-[2px]"
      data-testid="conversation-file-drop-overlay"
      role="status"
    >
      <div className="flex size-full max-h-60 max-w-100 flex-col items-center justify-center gap-2 rounded-md border-2 border-dashed border-teal-500 bg-background/60 text-teal-700 dark:text-teal-300">
        <span className="flex size-11 items-center justify-center rounded-full bg-teal-500/15">
          <Upload aria-hidden="true" className="size-5" />
        </span>
        <span className="text-sm font-medium">
          {isImage ? "松开发送图片" : "松开发送文件"}
        </span>
        <span className="text-xs text-muted-foreground">
          {isImage ? "支持 PNG、JPG 和 WebP" : "将作为附件发送"}
        </span>
      </div>
    </div>
  )
}

function ConversationPanelHeader({
  conversation,
  currentUserId,
  online,
}: {
  conversation: ClientConversation
  currentUserId: string
  online?: boolean
}) {
  return (
    <header
      className="flex h-14 shrink-0 items-center justify-between border-b px-5"
      data-testid="conversation-panel-header"
    >
      <div className="flex min-w-0 items-center gap-3 pr-3">
        <ConversationPanelHeaderProfileAvatar
          conversation={conversation}
          currentUserId={currentUserId}
          online={online}
        />
        <div className="flex min-w-0 items-baseline gap-2">
          <h2 className="min-w-0 truncate text-sm font-medium">
            {conversation.name}
          </h2>
          {conversation.type === "group" && (
            <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <UsersRound className="size-3" />
              {getGroupMemberCount(conversation)} 人
            </span>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {conversation.type === "group" && (
          <AddGroupMembersDialog conversation={conversation} />
        )}
        <Button
          aria-label="历史附件"
          disabled
          size="icon-sm"
          title="历史附件"
          type="button"
          variant="ghost"
        >
          <FolderClosed className="size-4" />
        </Button>
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
      </div>
    </header>
  )
}

function getGroupMemberCount(conversation: ClientConversation) {
  return conversation.memberCount || conversation.members?.length || 0
}

function ConversationPanelHeaderProfileAvatar({
  conversation,
  currentUserId,
  online,
}: {
  conversation: ClientConversation
  currentUserId: string
  online?: boolean
}) {
  const avatar = (
    <ConversationPanelHeaderAvatar
      conversation={conversation}
      online={online}
    />
  )

  if (conversation.type === "group") {
    return (
      <GroupProfilePopover conversation={conversation}>
        {avatar}
      </GroupProfilePopover>
    )
  }

  if (conversation.type === "direct") {
    const otherMember = conversation.members?.find(
      (member) => member.type === "user" && member.id !== currentUserId
    )

    if (!otherMember) {
      return avatar
    }

    return (
      <UserProfilePopover
        fallbackProfile={otherMember}
        triggerAriaLabel={`${conversation.name}资料`}
        userId={otherMember.id}
      >
        {avatar}
      </UserProfilePopover>
    )
  }

  const appMember = conversation.members?.find(
    (member) => member.type === "app"
  )
  const appId = appMember?.id ?? conversation.id

  return (
    <AppProfilePopover
      appId={appId}
      fallbackProfile={{
        avatar: appMember?.avatar || conversation.avatar,
        description: "",
        id: appId,
        name: appMember?.name || conversation.name,
        online: online ?? false,
      }}
      triggerAriaLabel={`${conversation.name}资料`}
    >
      {avatar}
    </AppProfilePopover>
  )
}

function ConversationPanelHeaderAvatar({
  conversation,
  online,
}: {
  conversation: ClientConversation
  online?: boolean
}) {
  if (conversation.type === "group") {
    return (
      <GroupAvatar
        avatar={conversation.avatar}
        className="size-8"
        members={conversation.members}
        name={conversation.name}
      />
    )
  }

  return (
    <Avatar className="size-8 rounded-sm bg-muted after:rounded-sm">
      {conversation.avatar && (
        <AvatarImage
          alt={conversation.name}
          className="rounded-sm"
          src={conversation.avatar}
        />
      )}
      <AvatarFallback className="rounded-sm">
        {getConversationInitial(conversation.name)}
      </AvatarFallback>
      {online !== undefined && <ConversationAvatarBadge online={online} />}
    </Avatar>
  )
}

function ConversationAvatarBadge({ online }: { online: boolean }) {
  return (
    <AvatarBadge
      aria-label={online ? "在线" : "离线"}
      className={
        online ? "bg-emerald-500" : "bg-neutral-400 dark:bg-neutral-500"
      }
    />
  )
}

function ConversationPanelHistory({
  conversation,
  currentUserId,
  error,
  loading,
  loadingBefore,
  mentionLabelResolver,
  messages,
  onLoadBeforeMessages,
  onInsertMention,
  onReplyToMessage,
  onRevokeMessage,
}: {
  conversation: ClientConversation
  currentUserId: string
  error: string | null
  loading: boolean
  loadingBefore: boolean
  mentionLabelResolver: MentionLabelResolver
  messages: ConversationPanelMessage[]
  onLoadBeforeMessages: () => void
  onInsertMention: (target: ConversationPanelMentionTarget) => void
  onReplyToMessage: (message: ConversationPanelMessage) => void
  onRevokeMessage: (message: ConversationPanelMessage) => void
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
        {messages.map((message) =>
          message.role === "system" ? (
            <SystemMessageBadge
              key={message.id}
              currentUserId={currentUserId}
              mentionLabelResolver={mentionLabelResolver}
              message={message}
            />
          ) : (
            <MessageBubble
              key={message.id}
              message={message}
              conversation={conversation}
              currentUserId={currentUserId}
              mentionLabelResolver={mentionLabelResolver}
              onInsertMention={onInsertMention}
              onReply={onReplyToMessage}
              onRevoke={onRevokeMessage}
            />
          )
        )}
      </div>
    </ScrollArea>
  )
}

const ConversationPanelComposer = React.forwardRef<
  ConversationPanelComposerHandle,
  {
  conversation: ClientConversation
  draft: string
  draftMentions: ConversationDraftMention[]
  replyTarget: ConversationPanelReplyTarget | null
  onCancelReply: () => void
  onDraftBlur?: () => void
  onDraftChange: (draft: string, mentions: ConversationDraftMention[]) => void
  onSendFile: (file: File) => Promise<ClientMessage | null>
  onSendImage: (image: File) => Promise<ClientMessage | null>
  onRichTextModeChange: (richTextMode: boolean) => void
  onSendMessage: (content?: string) => void
  richTextMode: boolean
  sending: boolean
  }
>(function ConversationPanelComposer(
  {
    conversation,
    draft,
    draftMentions,
    replyTarget,
    onCancelReply,
    onDraftBlur,
    onDraftChange,
    onSendFile,
    onSendImage,
    onRichTextModeChange,
    onSendMessage,
    richTextMode,
    sending,
  },
  ref
) {
  const fileInputRef = React.useRef<HTMLInputElement | null>(null)
  const imageInputRef = React.useRef<HTMLInputElement | null>(null)
  const mentionOptionRefs = React.useRef<Array<HTMLButtonElement | null>>([])
  const textareaRef = React.useRef<HTMLTextAreaElement | null>(null)
  const previousSendingRef = React.useRef(sending)
  const shouldFocusAfterSendingRef = React.useRef(false)
  const [expressionPickerOpen, setExpressionPickerOpen] = React.useState(false)
  const [fileDialogOpen, setFileDialogOpen] = React.useState(false)
  const [imageDialogOpen, setImageDialogOpen] = React.useState(false)
  const [imagePreparing, setImagePreparing] = React.useState(false)
  const [mentionTrigger, setMentionTrigger] =
    React.useState<MentionTrigger | null>(null)
  const [selectedMentionIndex, setSelectedMentionIndex] = React.useState(0)
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null)
  const [selectedImage, setSelectedImage] = React.useState<File | null>(null)
  const mentionCandidates = React.useMemo(
    () =>
      conversation.type === "group"
        ? createMentionCandidates(conversation.members ?? [])
        : [],
    [conversation.members, conversation.type]
  )
  const filteredMentionCandidates = React.useMemo(
    () =>
      filterMentionCandidates(mentionCandidates, mentionTrigger?.query ?? ""),
    [mentionCandidates, mentionTrigger?.query]
  )

  React.useImperativeHandle(ref, () => ({
    focus() {
      window.requestAnimationFrame(() => {
        textareaRef.current?.focus()
      })
    },
    insertMention(target) {
      insertMentionTarget(target)
    },
    openDroppedFile(file) {
      if (sending || imagePreparing) {
        return
      }

      if (isAcceptedImageMessageFile(file)) {
        void prepareSelectedImage(file)
        return
      }

      prepareSelectedFile(file)
    },
  }))

  React.useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  React.useEffect(() => {
    if (!mentionTrigger) {
      return
    }

    const visibleSelectedIndex = getVisibleMentionIndex(
      selectedMentionIndex,
      filteredMentionCandidates.length
    )

    mentionOptionRefs.current[visibleSelectedIndex]?.scrollIntoView({
      block: "nearest",
    })
  }, [filteredMentionCandidates.length, mentionTrigger, selectedMentionIndex])

  React.useEffect(() => {
    if (!replyTarget) {
      return
    }

    const frame = window.requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })

    return () => {
      window.cancelAnimationFrame(frame)
    }
  }, [replyTarget])

  React.useEffect(() => {
    const wasSending = previousSendingRef.current
    previousSendingRef.current = sending

    if (sending || !wasSending || !shouldFocusAfterSendingRef.current) {
      return
    }

    shouldFocusAfterSendingRef.current = false
    const textarea = textareaRef.current
    if (!textarea) {
      return
    }

    textarea.focus()
  }, [sending])

  function handleDraftChange(event: React.ChangeEvent<HTMLTextAreaElement>) {
    const nextDraft = event.target.value
    const cursor = event.target.selectionStart
    const nextMentions = syncDraftMentions(draftMentions, draft, nextDraft)

    onDraftChange(nextDraft, nextMentions)
    updateMentionTrigger(nextDraft, cursor)
  }

  function updateMentionTrigger(value: string, cursor: number) {
    if (conversation.type !== "group" || mentionCandidates.length === 0) {
      setMentionTrigger(null)
      setSelectedMentionIndex(0)
      return
    }

    setMentionTrigger(getMentionTrigger(value, cursor))
    setSelectedMentionIndex(0)
  }

  function handleSendMessage() {
    if (sending || !draft.trim()) {
      return
    }

    shouldFocusAfterSendingRef.current = true
    onSendMessage(createDraftMentionTemplate(draft, draftMentions))
    setMentionTrigger(null)
    setSelectedMentionIndex(0)
  }

  function handleComposerKeyDown(
    event: React.KeyboardEvent<HTMLTextAreaElement>
  ) {
    if (isImeCompositionKeyEvent(event)) {
      return
    }

    if (mentionTrigger && filteredMentionCandidates.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault()
        setSelectedMentionIndex(
          (currentIndex) =>
            (currentIndex + 1) % filteredMentionCandidates.length
        )
        return
      }
      if (event.key === "ArrowUp") {
        event.preventDefault()
        setSelectedMentionIndex(
          (currentIndex) =>
            (currentIndex - 1 + filteredMentionCandidates.length) %
            filteredMentionCandidates.length
        )
        return
      }
      if (event.key === "Escape") {
        event.preventDefault()
        setMentionTrigger(null)
        return
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault()
        insertMentionCandidate(
          filteredMentionCandidates[
            getVisibleMentionIndex(
              selectedMentionIndex,
              filteredMentionCandidates.length
            )
          ] ??
            filteredMentionCandidates[0]
        )
        return
      }
    }

    if (event.key !== "Enter") {
      return
    }

    if (sending) {
      event.preventDefault()
      return
    }

    if (event.shiftKey || event.ctrlKey) {
      event.preventDefault()
      insertTextareaText(event.currentTarget, "\n", handleTextareaValueChange)
      return
    }

    event.preventDefault()
    handleSendMessage()
  }

  function handleTextareaValueChange(value: string, cursor?: number) {
    onDraftChange(value, syncDraftMentions(draftMentions, draft, value))
    updateMentionTrigger(value, cursor ?? value.length)
  }

  function insertMentionCandidate(candidate: MentionCandidate | undefined) {
    if (!candidate) {
      return
    }

    const textarea = textareaRef.current
    const cursor = textarea?.selectionStart ?? draft.length
    const trigger = getMentionTrigger(draft, cursor)

    insertMentionTarget(candidate, {
      end: cursor,
      start: trigger?.start ?? cursor,
    })
  }

  function insertMentionTarget(
    target: ConversationPanelMentionTarget,
    range?: {
      end: number
      start: number
    }
  ) {
    const textarea = textareaRef.current
    const selectionStart = range?.start ?? textarea?.selectionStart ?? draft.length
    const selectionEnd = range?.end ?? textarea?.selectionEnd ?? selectionStart

    const mentionText = `@${target.label}`
    const insertedText = `${mentionText} `
    const nextDraft =
      draft.slice(0, selectionStart) + insertedText + draft.slice(selectionEnd)
    const nextMention: ConversationDraftMention = {
      end: selectionStart + mentionText.length,
      id: target.id,
      label: target.label,
      start: selectionStart,
      targetType: target.targetType,
    }

    const nextMentions = [
      ...syncDraftMentions(
        draftMentions.filter(
          (mention) =>
            mention.end <= selectionStart || mention.start >= selectionEnd
        ),
        draft,
        nextDraft
      ),
      nextMention,
    ].sort((mentionA, mentionB) => mentionA.start - mentionB.start)

    onDraftChange(nextDraft, nextMentions)
    setMentionTrigger(null)
    setSelectedMentionIndex(0)

    window.requestAnimationFrame(() => {
      if (!textareaRef.current) {
        return
      }

      const nextCursor = selectionStart + insertedText.length
      textareaRef.current.focus()
      textareaRef.current.setSelectionRange(nextCursor, nextCursor)
    })
  }

  function handleExpressionSelect(item: ExpressionItem) {
    if (sending) {
      return
    }

    const textarea = textareaRef.current

    if (!textarea) {
      handleTextareaValueChange(draft + item.value)
      setExpressionPickerOpen(false)
      return
    }

    insertTextareaText(textarea, item.value, handleTextareaValueChange)
    setExpressionPickerOpen(false)
    window.requestAnimationFrame(() => {
      textarea.focus()
    })
  }

  function handleFileButtonClick() {
    fileInputRef.current?.click()
  }

  function handleImageButtonClick() {
    imageInputRef.current?.click()
  }

  function handleFileInputChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0] ?? null

    event.target.value = ""

    if (!file) {
      return
    }

    prepareSelectedFile(file)
  }

  function prepareSelectedFile(file: File) {
    if (file.size > maxFileMessageUploadBytes) {
      setSelectedFile(null)
      setFileDialogOpen(false)
      toast.error("文件大于 20MB，无法上传")
      return
    }

    setSelectedFile(file)
    setFileDialogOpen(true)
  }

  function handleImageInputChange(event: React.ChangeEvent<HTMLInputElement>) {
    const image = event.target.files?.[0] ?? null

    event.target.value = ""

    if (!image) {
      return
    }

    void prepareSelectedImage(image)
  }

  function handleComposerPaste(
    event: React.ClipboardEvent<HTMLTextAreaElement>
  ) {
    const image = getClipboardImageFile(event.clipboardData)

    if (!image) {
      return
    }

    event.preventDefault()
    void prepareSelectedImage(image)
  }

  async function prepareSelectedImage(image: File) {
    if (sending || imagePreparing) {
      return
    }

    setImagePreparing(true)
    setSelectedImage(null)
    setImageDialogOpen(false)

    try {
      const compressedImage = await compressImageForMessage(image)

      if (compressedImage.size > imageMessageMaxBytes) {
        toast.error("图片大于 2MB，无法上传")
        return
      }

      setSelectedImage(compressedImage)
      setImageDialogOpen(true)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取图片失败")
    } finally {
      setImagePreparing(false)
    }
  }

  function handleFileDialogOpenChange(open: boolean) {
    if (sending) {
      return
    }

    setFileDialogOpen(open)

    if (!open) {
      setSelectedFile(null)
    }
  }

  async function handleFileSendConfirm() {
    if (!selectedFile || sending) {
      return
    }

    const message = await onSendFile(selectedFile)

    if (message) {
      setFileDialogOpen(false)
      setSelectedFile(null)
    }
  }

  function handleImageDialogOpenChange(open: boolean) {
    if (sending) {
      return
    }

    setImageDialogOpen(open)

    if (!open) {
      setSelectedImage(null)
    }
  }

  async function handleImageSendConfirm() {
    if (!selectedImage || sending) {
      return
    }

    const message = await onSendImage(selectedImage)

    if (message) {
      setImageDialogOpen(false)
      setSelectedImage(null)
    }
  }

  return (
    <footer
      className="shrink-0 border-t p-4"
      data-testid="conversation-panel-composer"
    >
      <input
        ref={fileInputRef}
        className="hidden"
        onChange={handleFileInputChange}
        type="file"
      />
      <input
        ref={imageInputRef}
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={handleImageInputChange}
        type="file"
      />
      <div
        className="flex w-full flex-col gap-2"
        data-testid="conversation-panel-composer-content"
      >
        {replyTarget && (
          <div
            className="flex min-h-11 items-center justify-between gap-3 rounded-md border bg-muted/40 px-3 py-2"
            data-testid="conversation-reply-preview"
          >
            <div className="min-w-0">
              <div className="truncate text-xs font-medium">
                回复 {replyTarget.author}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {replyTarget.summary}
              </div>
            </div>
            <Button
              aria-label="取消回复"
              disabled={sending}
              onClick={onCancelReply}
              size="icon-sm"
              title="取消回复"
              type="button"
              variant="ghost"
            >
              <X className="size-4" />
            </Button>
          </div>
        )}
        <div className="relative" data-testid="conversation-panel-editor-row">
          <Textarea
            ref={textareaRef}
            value={draft}
            aria-disabled={sending}
            onBlur={onDraftBlur}
            onChange={handleDraftChange}
            onKeyDown={handleComposerKeyDown}
            onSelect={(event) =>
              updateMentionTrigger(
                event.currentTarget.value,
                event.currentTarget.selectionStart
              )
            }
            onPaste={handleComposerPaste}
            placeholder={richTextMode ? "输入 Markdown 消息" : "输入消息"}
            readOnly={sending}
            className="max-h-48 min-h-24 resize-none"
          />
          {mentionTrigger && filteredMentionCandidates.length > 0 && (
            <div className="absolute bottom-full left-0 z-20 mb-2 max-h-72 w-72 overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md">
              {filteredMentionCandidates.map((candidate, index) => (
                <Button
                  key={`${candidate.targetType}-${candidate.id}`}
                  ref={(element) => {
                    mentionOptionRefs.current[index] = element
                  }}
                  className={cn(
                    "h-auto w-full justify-start gap-2 px-2 py-1.5 text-left",
                    index ===
                      getVisibleMentionIndex(
                        selectedMentionIndex,
                        filteredMentionCandidates.length
                      ) && "bg-accent"
                  )}
                  onMouseDown={(event) => {
                    event.preventDefault()
                    insertMentionCandidate(candidate)
                  }}
                  type="button"
                  variant="ghost"
                >
                  <Avatar
                    className={cn(
                      "size-6 rounded-sm after:rounded-sm",
                      candidate.targetType === "all" ? "bg-teal-500" : "bg-muted"
                    )}
                    data-size="sm"
                  >
                    {candidate.targetType === "all" ? (
                      <AvatarFallback className="rounded-sm bg-transparent text-background">
                        <UsersRound className="size-3.5" />
                      </AvatarFallback>
                    ) : candidate.avatar ? (
                      <AvatarImage
                        alt={candidate.label}
                        className="rounded-sm"
                        src={candidate.avatar}
                      />
                    ) : (
                      <AvatarFallback className="rounded-sm text-xs">
                        {getConversationInitial(candidate.label)}
                      </AvatarFallback>
                    )}
                  </Avatar>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">
                      {candidate.label}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {candidate.description}
                    </span>
                  </span>
                </Button>
              ))}
            </div>
          )}
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
                  disabled={sending}
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
              disabled={sending}
              onClick={handleFileButtonClick}
              size="icon-sm"
              title="上传文件"
              type="button"
              variant="ghost"
            >
              <Paperclip className="size-4" />
            </Button>
            <Button
              aria-label="插入图片"
              disabled={sending || imagePreparing}
              onClick={handleImageButtonClick}
              size="icon-sm"
              title="插入图片"
              type="button"
              variant="ghost"
            >
              {imagePreparing ? (
                <LoaderCircle className="size-4 animate-spin" />
              ) : (
                <ImageIcon className="size-4" />
              )}
            </Button>
            <Toggle
              aria-label="支持 markdown"
              className="size-8 p-0"
              disabled={sending}
              onPressedChange={onRichTextModeChange}
              pressed={richTextMode}
              size="sm"
              title="支持 markdown"
              type="button"
            >
              <MarkdownIcon className="size-4" />
            </Toggle>
          </div>
          <Button
            type="button"
            aria-label="发送消息"
            className="shrink-0"
            disabled={sending}
            onClick={handleSendMessage}
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
      <SendFileMessageDialog
        conversationName={conversation.name}
        file={selectedFile}
        onConfirm={() => void handleFileSendConfirm()}
        onOpenChange={handleFileDialogOpenChange}
        open={fileDialogOpen}
        sending={sending}
      />
      <SendImageMessageDialog
        conversationName={conversation.name}
        image={selectedImage}
        onConfirm={() => void handleImageSendConfirm()}
        onOpenChange={handleImageDialogOpenChange}
        open={imageDialogOpen}
        sending={sending}
      />
    </footer>
  )
})

function getClipboardImageFile(clipboardData: DataTransfer) {
  for (const item of Array.from(clipboardData.items)) {
    if (!item.type.startsWith("image/")) {
      continue
    }

    const file = item.getAsFile()

    if (file) {
      return file
    }
  }

  return null
}

function hasDraggedFiles(dataTransfer: DataTransfer) {
  return Array.from(dataTransfer.types).includes("Files")
}

function getDraggedFileKind(dataTransfer: DataTransfer): DraggedFileKind {
  const firstFileItem = Array.from(dataTransfer.items).find(
    (item) => item.kind === "file"
  )

  return firstFileItem && isAcceptedImageMessageMimeType(firstFileItem.type)
    ? "image"
    : "file"
}

function insertTextareaText(
  textarea: HTMLTextAreaElement,
  text: string,
  onChange: (value: string, cursor: number) => void
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
  onChange(nextValue, nextCursor)
}

function isImeCompositionKeyEvent(
  event: React.KeyboardEvent<HTMLTextAreaElement>
) {
  return event.nativeEvent.isComposing || event.keyCode === 229
}

function createMentionCandidates(
  members: ClientConversationMember[]
): MentionCandidate[] {
  const memberCandidates = members
    .map((member): MentionCandidate | null => {
      const label = getConversationMemberMentionLabel(member)
      if (!label) {
        return null
      }

      const description =
        member.type === "app" ? "应用" : member.email || member.phone || "成员"
      const searchText = createPinyinSearchText([
        label,
        member.name,
        member.nickname,
        member.email,
        member.phone,
        member.type,
      ])

      return {
        avatar: member.avatar,
        description,
        id: member.id,
        label,
        searchText,
        targetType: member.type,
      }
    })
    .filter((candidate): candidate is MentionCandidate => candidate !== null)

  return [
    {
      avatar: "",
      description: "所有成员",
      id: "all",
      label: "所有人",
      searchText: createPinyinSearchText([
        "所有人",
        "全体",
        "all",
        "everyone",
      ]),
      targetType: "all",
    },
    ...memberCandidates,
  ]
}

function filterMentionCandidates(
  candidates: MentionCandidate[],
  query: string
) {
  const normalizedQuery = normalizePinyinSearchQuery(query)
  const filteredCandidates = normalizedQuery
    ? candidates.filter((candidate) =>
        candidate.searchText.includes(normalizedQuery)
      )
    : candidates

  return filteredCandidates.slice(0, maxMentionCandidateResults)
}

function getVisibleMentionIndex(index: number, length: number) {
  if (length <= 0) {
    return 0
  }

  return Math.min(index, length - 1)
}

function getMentionTrigger(
  value: string,
  cursor: number
): MentionTrigger | null {
  const beforeCursor = value.slice(0, cursor)
  const start = beforeCursor.lastIndexOf("@")
  if (start < 0) {
    return null
  }

  const query = value.slice(start + 1, cursor)
  if (/[\s@]/.test(query)) {
    return null
  }

  return {
    query,
    start,
  }
}

function syncDraftMentions(
  mentions: ConversationDraftMention[],
  previousValue: string,
  value: string
): ConversationDraftMention[] {
  if (!value) {
    return []
  }

  const textChange = getTextChange(previousValue, value)
  const nextMentions: ConversationDraftMention[] = []

  for (const mention of mentions) {
    const text = getDraftMentionText(mention)
    if (previousValue.slice(mention.start, mention.end) !== text) {
      continue
    }

    const nextMention = shiftDraftMention(mention, textChange)

    if (!nextMention) {
      continue
    }

    if (value.slice(nextMention.start, nextMention.end) === text) {
      nextMentions.push(nextMention)
    }
  }

  return nextMentions
}

type TextChange = {
  delta: number
  newEnd: number
  oldEnd: number
  start: number
}

function getTextChange(previousValue: string, value: string): TextChange {
  let start = 0
  while (
    start < previousValue.length &&
    start < value.length &&
    previousValue[start] === value[start]
  ) {
    start += 1
  }

  let unchangedSuffixLength = 0
  while (
    unchangedSuffixLength < previousValue.length - start &&
    unchangedSuffixLength < value.length - start &&
    previousValue[previousValue.length - 1 - unchangedSuffixLength] ===
      value[value.length - 1 - unchangedSuffixLength]
  ) {
    unchangedSuffixLength += 1
  }

  const oldEnd = previousValue.length - unchangedSuffixLength
  const newEnd = value.length - unchangedSuffixLength

  return {
    delta: newEnd - oldEnd,
    newEnd,
    oldEnd,
    start,
  }
}

function shiftDraftMention(
  mention: ConversationDraftMention,
  textChange: TextChange
) {
  if (mention.end <= textChange.start) {
    return mention
  }

  if (mention.start >= textChange.oldEnd) {
    return {
      ...mention,
      end: mention.end + textChange.delta,
      start: mention.start + textChange.delta,
    }
  }

  return null
}

function createDraftMentionTemplate(
  value: string,
  mentions: ConversationDraftMention[]
) {
  let content = value
  const validMentions = mentions
    .filter(
      (mention) =>
        value.slice(mention.start, mention.end) === getDraftMentionText(mention)
    )
    .sort((mentionA, mentionB) => mentionB.start - mentionA.start)

  for (const mention of validMentions) {
    content =
      content.slice(0, mention.start) +
      createMentionToken({
        id: mention.id,
        type: mention.targetType,
      }) +
      content.slice(mention.end)
  }

  return content
}

function getDraftMentionText(mention: Pick<ConversationDraftMention, "label">) {
  return `@${mention.label}`
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

function SystemMessageBadge({
  currentUserId,
  mentionLabelResolver,
  message,
}: {
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
  message: ConversationPanelMessage
}) {
  return (
    <div className="flex justify-center">
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
}

function MessageBubble({
  message,
  conversation,
  currentUserId,
  mentionLabelResolver,
  onInsertMention,
  onReply,
  onRevoke,
}: {
  message: ConversationPanelMessage
  conversation: ClientConversation
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
  onInsertMention: (target: ConversationPanelMentionTarget) => void
  onReply: (message: ConversationPanelMessage) => void
  onRevoke: (message: ConversationPanelMessage) => void
}) {
  const fromMe = message.role === "me"
  const fallback = fromMe ? "我" : getConversationInitial(conversation.name)
  const canInsertAuthorMention =
    conversation.type === "group" && message.mentionTarget !== null
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

  return (
    <div className={cn("flex gap-3", fromMe ? "justify-end" : "justify-start")}>
      {!fromMe && <MessageAvatar fallback={fallback} message={message} />}
      <div
        className={cn(
          "flex max-w-[min(70%,64rem)] flex-col gap-1",
          fromMe ? "items-end" : "items-start"
        )}
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {canInsertAuthorMention ? (
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
        <MessageActionMenu
          canRevoke={message.canRevoke}
          copyDisabled={!copyText}
          onCopy={handleCopyMessage}
          onReply={() => onReply(message)}
          onRevoke={() => onRevoke(message)}
        >
          <div
            className={cn(
              "max-w-full rounded-md p-3 text-sm leading-relaxed shadow-xs",
              fromMe
                ? "bg-teal-100 text-foreground hover:bg-teal-200/70 data-[state=open]:bg-teal-200/70 dark:bg-teal-950 hover:dark:bg-teal-900/70 dark:data-[state=open]:bg-teal-900/70"
                : "bg-neutral-200/80 text-foreground hover:bg-neutral-200 data-[state=open]:bg-neutral-200 dark:bg-neutral-800/80 hover:dark:bg-neutral-800 dark:data-[state=open]:bg-neutral-800"
            )}
            data-message-action-trigger
            onContextMenu={handleMessageContextMenu}
            ref={bubbleRef}
          >
            {message.replyTo && (
              <MessageReplyReference replyTo={message.replyTo} />
            )}
            <MessageBodyRenderer
              body={message.body}
              currentUserId={currentUserId}
              mentionLabelResolver={mentionLabelResolver}
            />
          </div>
        </MessageActionMenu>
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
    case "revoked":
      return ""
    case "link":
      return message.body.url
    case "markdown":
    case "text":
      return formatMentionTemplateText(
        message.body.content,
        mentionLabelResolver
      )
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
    <MessageAvatarProfile message={message}>{avatar}</MessageAvatarProfile>
  )
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

function MessageBodyRenderer({
  body,
  currentUserId,
  mentionLabelResolver,
}: {
  body: ConversationPanelMessage["body"]
  currentUserId: string
  mentionLabelResolver: MentionLabelResolver
}) {
  switch (body.type) {
    case "file":
      return <MessageAttachment file={body} />
    case "image":
      return <MessageImage image={body} />
    case "link":
      return <MessageLink link={body} />
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
    case "revoked":
      return <span className="text-muted-foreground">该消息已被撤回</span>
    case "system_event":
      return <span>{formatClientMessageBodySummary(body)}</span>
  }
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

function getConversationInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}

function scrollToBottom(viewport: HTMLDivElement) {
  viewport.scrollTop = viewport.scrollHeight
}
