import * as React from "react"
import { Bot, Ellipsis, LoaderCircle, MessageSquareOff, X } from "lucide-react"
import { toast } from "sonner"

import {
  archiveConversationTopic,
  getConversationTopic,
  normalizeConversationRemovedEventPayload,
  participateConversationTopic,
  type ClientMessage,
  type ClientTopicDetail,
  type ClientTopicSourceMessage,
} from "@/lib/client-data-api"
import { getAvatarInitial } from "@/lib/avatar"
import { getClientDataErrorMessage } from "@/lib/client-data-state"
import { createConversationMentionLabelResolver } from "@/lib/conversation-mention-labels"
import { useClientData } from "@/lib/client-data-context"
import { useRealtime } from "@/lib/realtime-context"
import { type MentionLabelResolver } from "@/lib/message-mentions"
import type {
  ConversationDraftMention,
  ConversationDraftReplyTarget,
} from "@/lib/conversation-drafts"
import {
  formatConversationMessageSummary,
  toConversationPanelMessage,
} from "@/lib/conversation-message-presenter"
import type { VoiceMessageRecording } from "@/lib/voice-message"
import { cn } from "@/lib/utils"
import {
  ConversationPanel,
  type ConversationPanelMessage,
} from "@/components/conversation-panel"
import { MessageBodyRenderer } from "@/components/conversation/conversation-message"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"

const emptyMessages: ClientMessage[] = []
const emptyMentionLabelResolver: MentionLabelResolver = () => undefined

type TopicDrawerProps = {
  conversationId: string
  onOpenChange: (open: boolean) => void
  open: boolean
}

export function TopicDrawer(props: TopicDrawerProps) {
  return (
    <TopicDrawerContent
      key={`${props.conversationId}:${props.open ? "open" : "closed"}`}
      {...props}
    />
  )
}

function TopicDrawerContent({
  conversationId,
  onOpenChange,
  open,
}: TopicDrawerProps) {
  const {
    contactApps,
    contacts,
    ensureConversationMessages,
    getConversation,
    getConversationMessageState,
    loadBeforeConversationMessages,
    markConversationRead,
    me,
    refreshConversations,
    revokeConversationMessage,
    sendConversationFile,
    sendConversationImage,
    sendConversationLink,
    sendConversationMarkdown,
    sendConversationText,
    sendConversationVoice,
    updateMessageTopic,
  } = useClientData()
  const [detail, setDetail] = React.useState<ClientTopicDetail | null>(null)
  const [error, setError] = React.useState("")
  const [loading, setLoading] = React.useState(Boolean(open && conversationId))
  const [mutating, setMutating] = React.useState(false)
  const [archiveConfirmOpen, setArchiveConfirmOpen] = React.useState(false)
  const [draft, setDraft] = React.useState("")
  const [draftMentions, setDraftMentions] = React.useState<
    ConversationDraftMention[]
  >([])
  const [replyTarget, setReplyTarget] =
    React.useState<ConversationDraftReplyTarget | null>(null)
  const [richTextMode, setRichTextMode] = React.useState(false)
  React.useEffect(() => {
    if (!open || !conversationId) {
      return
    }
    let active = true
    void getConversationTopic(conversationId)
      .then((value) => {
        if (!active) return
        setDetail(value)
        ensureConversationMessages(value.conversation.id)
      })
      .catch((requestError) => {
        if (active) {
          setError(getClientDataErrorMessage(requestError, "加载话题失败"))
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [conversationId, ensureConversationMessages, open])

  const detailConversation = detail?.conversation ?? null
  const listedConversation = detailConversation
    ? getConversation(detailConversation.id)
    : null
  const parentMessageState = detail
    ? getConversationMessageState(detail.parentConversation.id)
    : undefined
  const parentSourceTopic = parentMessageState?.messages.find(
    (message) => message.id === detail?.sourceMessage.id
  )?.topic
  const baseConversation = listedConversation ?? detailConversation
  const synchronizedArchived =
    listedConversation?.topic?.archived ??
    parentSourceTopic?.archived ??
    detailConversation?.topic?.archived ??
    false
  const conversation = React.useMemo(() => {
    if (
      baseConversation?.topic &&
      baseConversation.topic.archived !== synchronizedArchived
    ) {
      return {
        ...baseConversation,
        topic: { ...baseConversation.topic, archived: synchronizedArchived },
      }
    }
    return baseConversation
  }, [baseConversation, synchronizedArchived])
  const messageState = conversation
    ? getConversationMessageState(conversation.id)
    : undefined
  const clientMessages = messageState?.messages ?? emptyMessages
  const messagesById = React.useMemo(
    () => new Map(clientMessages.map((message) => [message.id, message])),
    [clientMessages]
  )
  const contactsById = React.useMemo(
    () => new Map(contacts.map((contact) => [contact.id, contact])),
    [contacts]
  )
  const appsById = React.useMemo(() => {
    const result = new Map<string, (typeof contactApps)[number]>()
    for (const app of contactApps) {
      result.set(app.id, app)
      result.set(app.name, app)
    }
    return result
  }, [contactApps])
  const mentionLabelResolver = React.useMemo(
    () =>
      createConversationMentionLabelResolver({
        appsById,
        contactsById,
        conversationMembers: conversation?.members,
        currentUser: me,
      }),
    [appsById, contactsById, conversation?.members, me]
  )
  const messages = React.useMemo(
    () =>
      conversation
        ? clientMessages.map((message) =>
            toConversationPanelMessage(
              message,
              conversation,
              me,
              contactsById,
              appsById,
              messagesById,
              mentionLabelResolver
            )
          )
        : [],
    [
      appsById,
      clientMessages,
      contactsById,
      conversation,
      me,
      mentionLabelResolver,
      messagesById,
    ]
  )

  React.useEffect(() => {
    if (
      !open ||
      !conversation ||
      !conversation.topic?.participating ||
      !messageState?.loaded ||
      conversation.lastReadSeq >= conversation.lastMessageSeq
    ) {
      return
    }
    void markConversationRead(conversation.id).catch(() => undefined)
  }, [conversation, markConversationRead, messageState?.loaded, open])

  function updateDraft(value: string, mentions: ConversationDraftMention[]) {
    setDraft(value)
    setDraftMentions(mentions)
  }

  function replyToMessage(message: ConversationPanelMessage) {
    setReplyTarget({
      author: message.author,
      id: message.id,
      summary: formatConversationMessageSummary(
        message.body,
        mentionLabelResolver
      ),
    })
  }

  function sendMessage(contentOverride?: string) {
    if (!conversation || messageState?.sending) return
    const content = (contentOverride ?? draft).trim()
    if (!content) return
    const link = normalizeSingleLinkMessageURL(draft.trim())
    const send = link
      ? sendConversationLink
      : richTextMode
        ? sendConversationMarkdown
        : sendConversationText
    void send(conversation.id, link ?? content, {
      replyToMessageId: replyTarget?.id,
    }).then((message) => {
      if (message) {
        setDraft("")
        setDraftMentions([])
        setReplyTarget(null)
      }
    })
  }

  async function sendFile(file: File) {
    if (!conversation) return null
    const message = await sendConversationFile(conversation.id, file, {
      replyToMessageId: replyTarget?.id,
    })
    if (message) setReplyTarget(null)
    return message
  }

  async function sendImage(image: File) {
    if (!conversation) return null
    const message = await sendConversationImage(conversation.id, image, {
      replyToMessageId: replyTarget?.id,
    })
    if (message) setReplyTarget(null)
    return message
  }

  async function sendVoice(voice: VoiceMessageRecording) {
    if (!conversation) return null
    const message = await sendConversationVoice(conversation.id, voice, {
      replyToMessageId: replyTarget?.id,
    })
    if (message) setReplyTarget(null)
    return message
  }

  async function participate() {
    if (!detail || mutating) return
    const targetConversationId = detail.conversation.id
    setMutating(true)
    try {
      const nextConversation =
        await participateConversationTopic(targetConversationId)
      setDetail({
        ...detail,
        canParticipate: false,
        conversation: nextConversation,
      })
      toast.success("已参与话题")
      void refreshConversations().catch(() => undefined)
    } catch (requestError) {
      toast.error(getClientDataErrorMessage(requestError, "参与话题失败"))
    } finally {
      setMutating(false)
    }
  }

  async function archive() {
    if (!detail || mutating) return
    const targetConversationId = detail.conversation.id
    setMutating(true)
    try {
      const nextConversation =
        await archiveConversationTopic(targetConversationId)
      setDetail({
        ...detail,
        canArchive: false,
        canParticipate: false,
        conversation: nextConversation,
      })
      updateMessageTopic?.(
        detail.parentConversation.id,
        detail.sourceMessage.id,
        { archived: true, conversationId: targetConversationId }
      )
      setArchiveConfirmOpen(false)
      toast.success("话题已关闭")
      void refreshConversations().catch(() => undefined)
    } catch (requestError) {
      toast.error(getClientDataErrorMessage(requestError, "关闭话题失败"))
    } finally {
      setMutating(false)
    }
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      {open && (
        <TopicRemovalSync
          conversationId={conversationId}
          onRemoved={() => onOpenChange(false)}
          parentConversationId={detail?.parentConversation.id}
        />
      )}
      <SheetContent
        className="min-h-0 gap-0 overflow-hidden p-0 data-[side=right]:w-[80vw] data-[side=right]:sm:max-w-400"
        showCloseButton={false}
      >
        <SheetTitle className="sr-only">话题</SheetTitle>
        <SheetDescription className="sr-only">
          查看并参与当前消息创建的话题
        </SheetDescription>
        {loading ? (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" />
            正在加载话题
          </div>
        ) : error || !conversation ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center text-sm text-muted-foreground">
            <span>{error || "话题不存在"}</span>
            <Button onClick={() => onOpenChange(false)} variant="secondary">
              关闭
            </Button>
          </div>
        ) : (
          <ConversationPanel
            conversation={conversation}
            currentUserId={me.id}
            draft={draft}
            draftMentions={draftMentions}
            headerActions={
              <>
                {detail?.canArchive && !conversation.topic?.archived && (
                  <TopicArchiveMenu
                    disabled={mutating}
                    onSelect={() => setArchiveConfirmOpen(true)}
                  />
                )}
                <SheetClose asChild>
                  <Button aria-label="关闭话题" size="icon-sm" variant="ghost">
                    <X className="size-4" />
                  </Button>
                </SheetClose>
              </>
            }
            historyError={messageState?.error ?? null}
            historyLoading={Boolean(
              messageState && !messageState.loaded && !messageState.error
            )}
            historyLoadingBefore={Boolean(messageState?.loadingBefore)}
            historyHeader={
              <TopicSourceBanner
                currentUserId={me.id}
                mentionLabelResolver={mentionLabelResolver}
                sourceMessage={detail?.sourceMessage}
              />
            }
            mentionLabelResolver={mentionLabelResolver}
            messages={messages}
            onCancelReply={() => setReplyTarget(null)}
            onDraftChange={updateDraft}
            onLoadBeforeMessages={() =>
              loadBeforeConversationMessages(conversation.id)
            }
            onReplyToMessage={replyToMessage}
            onRevokeMessage={(message) =>
              void revokeConversationMessage(conversation.id, message.id).catch(
                (requestError) =>
                  toast.error(
                    getClientDataErrorMessage(requestError, "撤回消息失败")
                  )
              )
            }
            onRichTextModeChange={setRichTextMode}
            onSendFile={sendFile}
            onSendImage={sendImage}
            onSendMessage={sendMessage}
            onSendVoice={sendVoice}
            readOnlyFooter={
              detail?.canParticipate &&
              !conversation.topic?.participating &&
              !conversation.topic?.archived ? (
                <div className="flex items-center justify-between gap-3 border-t bg-muted/30 px-5 py-3">
                  <span className="text-sm text-muted-foreground">
                    参与后可发言，并在会话列表中看到该话题
                  </span>
                  <Button
                    disabled={mutating}
                    onClick={() => void participate()}
                    type="button"
                  >
                    {mutating && (
                      <LoaderCircle className="size-4 animate-spin" />
                    )}
                    参与话题
                  </Button>
                </div>
              ) : undefined
            }
            readOnly={conversation.topic?.archived}
            replyTarget={replyTarget}
            richTextMode={richTextMode}
            sending={Boolean(messageState?.sending)}
          />
        )}
      </SheetContent>
      <TopicArchiveConfirmDialog
        onConfirm={() => void archive()}
        onOpenChange={setArchiveConfirmOpen}
        open={archiveConfirmOpen}
        saving={mutating}
      />
    </Sheet>
  )
}

function TopicRemovalSync({
  conversationId,
  onRemoved,
  parentConversationId,
}: {
  conversationId: string
  onRemoved: () => void
  parentConversationId?: string
}) {
  const { subscribeRealtimeEvent } = useRealtime()

  React.useEffect(
    () =>
      subscribeRealtimeEvent("conversation.removed", (payload) => {
        try {
          const event = normalizeConversationRemovedEventPayload(payload)
          if (
            event.conversationId === conversationId ||
            event.conversationId === parentConversationId
          ) {
            onRemoved()
          }
        } catch {
          // Ignore malformed realtime events. The websocket remains usable.
        }
      }),
    [conversationId, onRemoved, parentConversationId, subscribeRealtimeEvent]
  )

  return null
}

export function TopicArchiveAction({
  conversationId,
}: {
  conversationId: string
}) {
  const { getConversation, refreshConversations, updateMessageTopic } =
    useClientData()
  const [detail, setDetail] = React.useState<ClientTopicDetail | null>(null)
  const [saving, setSaving] = React.useState(false)
  const [confirmOpen, setConfirmOpen] = React.useState(false)

  React.useEffect(() => {
    let active = true
    void getConversationTopic(conversationId)
      .then((value) => {
        if (active) setDetail(value)
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [conversationId])

  if (!detail?.canArchive || getConversation(conversationId)?.topic?.archived) {
    return null
  }

  async function archive() {
    const currentDetail = detail
    if (!currentDetail) return
    setSaving(true)
    try {
      await archiveConversationTopic(conversationId)
      updateMessageTopic?.(
        currentDetail.parentConversation.id,
        currentDetail.sourceMessage.id,
        { archived: true, conversationId }
      )
      setDetail({
        ...currentDetail,
        canArchive: false,
        canParticipate: false,
      })
      setConfirmOpen(false)
      toast.success("话题已关闭")
      void refreshConversations().catch(() => undefined)
    } catch (error) {
      toast.error(getClientDataErrorMessage(error, "关闭话题失败"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <TopicArchiveMenu
        disabled={saving}
        onSelect={() => setConfirmOpen(true)}
      />
      <TopicArchiveConfirmDialog
        onConfirm={() => void archive()}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        saving={saving}
      />
    </>
  )
}

function TopicArchiveMenu({
  disabled,
  onSelect,
}: {
  disabled: boolean
  onSelect: () => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label="更多操作"
          disabled={disabled}
          size="icon-sm"
          title="更多操作"
          type="button"
          variant="ghost"
        >
          <Ellipsis />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-36">
        <DropdownMenuItem
          disabled={disabled}
          onSelect={onSelect}
          variant="destructive"
        >
          <MessageSquareOff />
          关闭讨论
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TopicArchiveConfirmDialog({
  onConfirm,
  onOpenChange,
  open,
  saving,
}: {
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  saving: boolean
}) {
  return (
    <AlertDialog
      onOpenChange={(nextOpen) => {
        if (!saving) onOpenChange(nextOpen)
      }}
      open={open}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>确认关闭讨论</AlertDialogTitle>
          <AlertDialogDescription>
            关闭后仍可查看话题，但无法继续发言，其他人也无法再参与。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={saving}>取消</AlertDialogCancel>
          <AlertDialogAction
            disabled={saving}
            onClick={(event) => {
              event.preventDefault()
              onConfirm()
            }}
            variant="destructive"
          >
            {saving && <LoaderCircle className="size-4 animate-spin" />}
            确认关闭
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export function TopicSourceBanner({
  conversationId,
  currentUserId,
  mentionLabelResolver,
  sourceMessage,
}: {
  conversationId?: string
  currentUserId: string
  mentionLabelResolver?: MentionLabelResolver
  sourceMessage?: ClientTopicSourceMessage
}) {
  const [fetchedSource, setFetchedSource] =
    React.useState<ClientTopicSourceMessage | null>(null)
  const loadedSource = sourceMessage ?? fetchedSource

  React.useEffect(() => {
    if (sourceMessage) return
    if (!conversationId) return
    let active = true
    void getConversationTopic(conversationId)
      .then((value) => {
        if (active) setFetchedSource(value.sourceMessage)
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [conversationId, sourceMessage])

  if (!loadedSource) return null

  const fromCurrentUser =
    loadedSource.sender.type === "user" &&
    loadedSource.sender.id === currentUserId
  const avatar = (
    <Avatar className="size-8 rounded-sm bg-muted after:rounded-sm">
      {loadedSource.sender.avatar && (
        <AvatarImage
          alt={loadedSource.sender.name}
          className="rounded-sm"
          src={loadedSource.sender.avatar}
        />
      )}
      <AvatarFallback
        className={cn(
          "rounded-sm",
          fromCurrentUser && "bg-primary text-primary-foreground"
        )}
      >
        {loadedSource.sender.type === "app" ? (
          <Bot className="size-4" />
        ) : fromCurrentUser ? (
          "我"
        ) : (
          getAvatarInitial(loadedSource.sender.name)
        )}
      </AvatarFallback>
    </Avatar>
  )

  return (
    <div
      className={cn(
        "flex gap-3",
        fromCurrentUser ? "justify-end" : "justify-start"
      )}
    >
      {!fromCurrentUser && avatar}
      <div
        className={cn(
          "flex max-w-[min(70%,64rem)] flex-col gap-1",
          fromCurrentUser ? "items-end" : "items-start"
        )}
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>{loadedSource.sender.name}</span>
          <span>{formatTopicSourceTime(loadedSource.createdAt)}</span>
        </div>
        <div
          className={cn(
            "max-w-full min-w-0 rounded-md p-3 text-sm leading-relaxed shadow-sm",
            fromCurrentUser
              ? "bg-teal-100/60 text-foreground dark:bg-teal-950/80"
              : "bg-zinc-100 text-foreground dark:bg-zinc-800"
          )}
          data-testid="topic-source-message-bubble"
        >
          <MessageBodyRenderer
            body={loadedSource.body}
            currentUserId={currentUserId}
            mentionLabelResolver={
              mentionLabelResolver ?? emptyMentionLabelResolver
            }
          />
        </div>
      </div>
      {fromCurrentUser && avatar}
    </div>
  )
}

function formatTopicSourceTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date)
}

function normalizeSingleLinkMessageURL(content: string) {
  if (!content || /\s/.test(content)) return null
  const candidate = content.toLowerCase().startsWith("www.")
    ? `https://${content}`
    : content
  try {
    const url = new URL(candidate)
    return url.protocol === "http:" || url.protocol === "https:"
      ? url.toString()
      : null
  } catch {
    return null
  }
}
