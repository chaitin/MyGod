import * as React from "react"
import { useSearchParams } from "react-router"
import { Loader2Icon, Plus, Search } from "lucide-react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { createConversationMentionLabelResolver } from "@/lib/conversation-mention-labels"
import { sortContactsByDisplayName } from "@/lib/contact-sort"
import { useClientData } from "@/lib/client-data-context"
import {
  formatClientMessageBodySummary,
  type ClientConversation,
  type ClientMessage,
  type ClientUser,
  type ContactApp,
  type ContactUser,
} from "@/lib/client-data-api"
import { formatConversationLastMessageTime } from "@/lib/conversation-format"
import {
  formatMentionTemplateText,
  type MentionLabelResolver,
} from "@/lib/message-mentions"
import { ConversationListItemMenu } from "@/components/conversation-list-item-menu"
import {
  ConversationPanel,
  type ConversationPanelMessage,
  type ConversationPanelReplyTarget,
} from "@/components/conversation-panel"
import { GroupAvatar } from "@/components/group-avatar"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"

const emptyClientMessages: ClientMessage[] = []

function normalizeSingleLinkMessageURL(content: string) {
  const value = content.trim()
  if (!value || /\s/.test(value)) {
    return null
  }

  const linkCandidate = value.toLowerCase().startsWith("www.")
    ? `https://${value}`
    : value

  try {
    const url = new URL(linkCandidate)
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return null
    }
    if (!url.hostname) {
      return null
    }

    return url.toString()
  } catch {
    return null
  }
}

function getMessageTime(createdAt: string) {
  const date = new Date(createdAt)

  if (Number.isNaN(date.getTime())) {
    return ""
  }

  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date)
}

export function ChatPage() {
  const {
    contactApps,
    contacts,
    conversations,
    createGroupConversation,
    ensureConversationMessages,
    getConversation,
    getConversationMessageState,
    loadBeforeConversationMessages,
    markConversationRead,
    me,
    revokeConversationMessage,
    sendConversationFile,
    sendConversationImage,
    sendConversationLink,
    sendConversationMarkdown,
    sendConversationText,
  } = useClientData()
  const [searchParams, setSearchParams] = useSearchParams()
  const [draftState, setDraftState] = React.useState({
    conversationId: "",
    value: "",
  })
  const [replyTargetState, setReplyTargetState] = React.useState<{
    conversationId: string
    target: ConversationPanelReplyTarget
  } | null>(null)
  const [richTextMode, setRichTextMode] = React.useState(false)
  const [createGroupDialogOpen, setCreateGroupDialogOpen] =
    React.useState(false)
  const requestedConversationId = searchParams.get("conversation_id") ?? ""

  const activeConversation = React.useMemo(
    () =>
      requestedConversationId ? getConversation(requestedConversationId) : null,
    [getConversation, requestedConversationId]
  )

  const activeConversationId = activeConversation?.id ?? ""
  const activeConversationIdRef = React.useRef(activeConversationId)
  const draft =
    draftState.conversationId === activeConversationId ? draftState.value : ""
  const replyTarget =
    replyTargetState?.conversationId === activeConversationId
      ? replyTargetState.target
      : null
  const activeMessageState = activeConversationId
    ? getConversationMessageState(activeConversationId)
    : undefined
  const activeConversationHasUnreadProgress = Boolean(
    activeConversation &&
    (activeConversation.unreadCount > 0 ||
      activeConversation.lastReadSeq < activeConversation.lastMessageSeq)
  )
  const historyLoading = Boolean(
    activeConversation &&
    activeMessageState &&
    !activeMessageState.loaded &&
    !activeMessageState.error
  )
  const activeClientMessages =
    activeMessageState?.messages ?? emptyClientMessages
  const activeClientMessagesById = React.useMemo(
    () => new Map(activeClientMessages.map((message) => [message.id, message])),
    [activeClientMessages]
  )
  const contactsById = React.useMemo(
    () => new Map(contacts.map((contact) => [contact.id, contact])),
    [contacts]
  )
  const contactAppsByLookup = React.useMemo(() => {
    const appsByLookup = new Map<string, ContactApp>()

    for (const app of contactApps) {
      appsByLookup.set(app.id, app)
      appsByLookup.set(app.name, app)
    }

    return appsByLookup
  }, [contactApps])
  const activeMentionLabelResolver = React.useMemo(
    () =>
      createConversationMentionLabelResolver({
        appsById: contactAppsByLookup,
        contactsById,
        conversation: activeConversation,
        currentUser: me,
      }),
    [activeConversation, contactAppsByLookup, contactsById, me]
  )
  const activeConversationOnline = activeConversation
    ? getConversationOnlineStatus(
        activeConversation,
        me.id,
        contactsById,
        contactAppsByLookup
      )
    : undefined
  const activeMessages = React.useMemo(
    () =>
      activeConversation
        ? activeClientMessages.map((message) =>
            toConversationPanelMessage(
              message,
              activeConversation,
              me,
              contactsById,
              activeClientMessagesById,
              activeMentionLabelResolver
            )
          )
        : [],
    [
      activeClientMessages,
      activeClientMessagesById,
      activeConversation,
      activeMentionLabelResolver,
      contactsById,
      me,
    ]
  )

  const setDraft = React.useCallback(
    (nextDraft: string) => {
      setDraftState({
        conversationId: activeConversationId,
        value: nextDraft,
      })
    },
    [activeConversationId]
  )

  React.useEffect(() => {
    activeConversationIdRef.current = activeConversationId
  }, [activeConversationId])

  React.useEffect(() => {
    if (!activeConversationId) {
      return
    }

    ensureConversationMessages(activeConversationId)
  }, [activeConversationId, ensureConversationMessages])

  React.useEffect(() => {
    if (!activeConversationId || !activeConversationHasUnreadProgress) {
      return
    }

    function markActiveConversationRead() {
      if (document.visibilityState !== "visible") {
        return
      }

      void markConversationRead(activeConversationId).catch(() => undefined)
    }

    markActiveConversationRead()
    const interval = window.setInterval(markActiveConversationRead, 20_000)

    function handleVisibilityChange() {
      markActiveConversationRead()
    }

    document.addEventListener("visibilitychange", handleVisibilityChange)

    return () => {
      window.clearInterval(interval)
      document.removeEventListener("visibilitychange", handleVisibilityChange)
    }
  }, [
    activeConversationId,
    activeConversationHasUnreadProgress,
    markConversationRead,
  ])

  const loadBeforeMessages = React.useCallback(() => {
    if (!activeConversationId) {
      return
    }

    loadBeforeConversationMessages(activeConversationId)
  }, [activeConversationId, loadBeforeConversationMessages])

  const clearReplyTarget = React.useCallback(() => {
    setReplyTargetState((currentReplyTargetState) =>
      currentReplyTargetState?.conversationId === activeConversationId
        ? null
        : currentReplyTargetState
    )
  }, [activeConversationId])

  const replyToMessage = React.useCallback(
    (message: ConversationPanelMessage) => {
      setReplyTargetState({
        conversationId: activeConversationId,
        target: {
          id: message.id,
          author: message.author,
          summary: formatConversationMessageSummary(
            message.body,
            activeMentionLabelResolver
          ),
        },
      })
    },
    [activeConversationId, activeMentionLabelResolver]
  )

  const revokeMessage = React.useCallback(
    (message: ConversationPanelMessage) => {
      if (!activeConversationId || !message.canRevoke) {
        return
      }

      void revokeConversationMessage(activeConversationId, message.id).catch(
        () => {
          toast.error("撤回消息失败")
        }
      )
    },
    [activeConversationId, revokeConversationMessage]
  )

  function sendMessage(contentOverride?: string) {
    const visibleContent = draft.trim()
    const content = (contentOverride ?? draft).trim()
    if (!content || !activeConversationId || activeMessageState?.sending) {
      return
    }

    const sendingConversationId = activeConversationId
    const sendingReplyToMessageId = replyTarget?.id
    const linkURL = normalizeSingleLinkMessageURL(visibleContent)
    const sendConversation = linkURL
      ? sendConversationLink
      : richTextMode
        ? sendConversationMarkdown
        : sendConversationText
    const sendContent = linkURL ?? content

    void sendConversation(sendingConversationId, sendContent, {
      replyToMessageId: sendingReplyToMessageId,
    }).then((message) => {
      if (
        message &&
        activeConversationIdRef.current === sendingConversationId
      ) {
        setDraftState((currentDraftState) =>
          currentDraftState.conversationId === sendingConversationId
            ? { ...currentDraftState, value: "" }
            : currentDraftState
        )
        setReplyTargetState((currentReplyTargetState) =>
          currentReplyTargetState?.conversationId === sendingConversationId &&
          currentReplyTargetState.target.id === sendingReplyToMessageId
            ? null
            : currentReplyTargetState
        )
      }
    })
  }

  async function sendFileMessage(file: File) {
    if (!activeConversationId || activeMessageState?.sending) {
      return null
    }

    const sendingConversationId = activeConversationId
    const sendingReplyToMessageId = replyTarget?.id
    const message = await sendConversationFile(sendingConversationId, file, {
      replyToMessageId: sendingReplyToMessageId,
    })
    if (message && activeConversationIdRef.current === sendingConversationId) {
      setReplyTargetState((currentReplyTargetState) =>
        currentReplyTargetState?.conversationId === sendingConversationId &&
        currentReplyTargetState.target.id === sendingReplyToMessageId
          ? null
          : currentReplyTargetState
      )
    }

    return message
  }

  async function sendImageMessage(image: File) {
    if (!activeConversationId || activeMessageState?.sending) {
      return null
    }

    const sendingConversationId = activeConversationId
    const sendingReplyToMessageId = replyTarget?.id
    const message = await sendConversationImage(sendingConversationId, image, {
      replyToMessageId: sendingReplyToMessageId,
    })
    if (message && activeConversationIdRef.current === sendingConversationId) {
      setReplyTargetState((currentReplyTargetState) =>
        currentReplyTargetState?.conversationId === sendingConversationId &&
        currentReplyTargetState.target.id === sendingReplyToMessageId
          ? null
          : currentReplyTargetState
      )
    }

    return message
  }

  function selectConversation(conversationId: string) {
    setSearchParams({ conversation_id: conversationId }, { replace: true })
  }

  async function startGroupConversation(name: string, memberIds: string[]) {
    const conversation = await createGroupConversation(name, memberIds)
    setSearchParams({ conversation_id: conversation.id })
  }

  function handleConversationListContextMenu(
    event: React.MouseEvent<HTMLDivElement>
  ) {
    const target = event.target

    if (
      target instanceof Element &&
      target.closest("[data-conversation-list-item-trigger]")
    ) {
      return
    }

    event.preventDefault()
  }

  return (
    <>
      <aside className="flex w-72 shrink-0 flex-col border-r bg-background">
        <div className="flex h-14 items-center justify-between px-4">
          <h1 className="text-base font-medium">消息</h1>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                aria-label="新建 Agent"
                size="icon-sm"
                title="新建 Agent"
                type="button"
                variant="ghost"
              >
                <Plus className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-32">
              <DropdownMenuItem onSelect={() => setCreateGroupDialogOpen(true)}>
                发起群聊
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="px-4 pb-3">
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-8" placeholder="搜索" type="search" />
          </div>
        </div>
        <ScrollArea
          className="min-h-0 flex-1 overflow-hidden"
          viewportProps={{
            className:
              "[&>div]:!block [&>div]:!min-w-0 [&>div]:!w-full [&>div]:!max-w-full",
            onContextMenu: handleConversationListContextMenu,
          }}
        >
          <ItemGroup className="gap-1 px-2 pb-3 has-data-[size=sm]:gap-1">
            {conversations.length === 0 && (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                暂无会话
              </div>
            )}
            {conversations.map((conversation) => {
              const selected = conversation.id === activeConversation?.id
              const lastMessageTime = formatConversationLastMessageTime(
                conversation.lastMessageAt ?? conversation.createdAt
              )
              const mentionLabelResolver =
                createConversationMentionLabelResolver({
                  appsById: contactAppsByLookup,
                  contactsById,
                  conversation,
                  currentUser: me,
                })
              const hasUnreadMention =
                conversation.lastMentionedSeq > conversation.lastReadSeq
              const description = getConversationListDescription(
                conversation,
                mentionLabelResolver
              )

              return (
                <ConversationListItemMenu key={conversation.id}>
                  <Item
                    asChild
                    data-conversation-list-item-trigger
                    size="sm"
                    className={cn(
                      "min-h-16 w-full max-w-full flex-nowrap overflow-hidden px-2 py-2",
                      selected
                        ? "bg-primary/10 text-foreground"
                        : "hover:bg-muted"
                    )}
                  >
                    <Button
                      className="h-auto w-full max-w-full min-w-0 shrink justify-start overflow-hidden text-left whitespace-normal"
                      type="button"
                      onClick={() => selectConversation(conversation.id)}
                      variant="ghost"
                    >
                      <ItemMedia>
                        <ConversationListAvatar conversation={conversation} />
                      </ItemMedia>
                      <ItemContent className="min-w-0 flex-1 overflow-hidden">
                        <div
                          data-slot="item-title"
                          className="flex w-full min-w-0 items-center justify-between gap-2 overflow-hidden text-sm leading-snug font-medium underline-offset-4"
                        >
                          <span className="flex min-w-0 flex-1 items-center overflow-hidden">
                            <span className="block min-w-0 flex-1 truncate">
                              {conversation.name}
                            </span>
                          </span>
                          {lastMessageTime && (
                            <span className="shrink-0 pr-2 text-xs font-normal text-muted-foreground">
                              {lastMessageTime}
                            </span>
                          )}
                        </div>
                        <p
                          data-slot="item-description"
                          className="w-full min-w-0 truncate text-left text-xs leading-normal font-normal text-muted-foreground"
                        >
                          {hasUnreadMention && (
                            <span className="mr-1 font-medium text-rose-700 dark:text-rose-300">
                              [有人 @ 我]
                            </span>
                          )}
                          <span>{description}</span>
                        </p>
                      </ItemContent>
                    </Button>
                  </Item>
                </ConversationListItemMenu>
              )
            })}
          </ItemGroup>
        </ScrollArea>
      </aside>

      <ConversationPanel
        key={activeConversationId || "empty"}
        conversation={activeConversation}
        conversationOnline={activeConversationOnline}
        currentUserId={me.id}
        draft={draft}
        historyError={activeMessageState?.error ?? null}
        historyLoading={historyLoading}
        historyLoadingBefore={Boolean(activeMessageState?.loadingBefore)}
        mentionLabelResolver={activeMentionLabelResolver}
        messages={activeMessages}
        onCancelReply={clearReplyTarget}
        onDraftChange={setDraft}
        onReplyToMessage={replyToMessage}
        onRevokeMessage={revokeMessage}
        onRichTextModeChange={setRichTextMode}
        onSendFile={sendFileMessage}
        onSendImage={sendImageMessage}
        onLoadBeforeMessages={loadBeforeMessages}
        onSendMessage={sendMessage}
        replyTarget={replyTarget}
        richTextMode={richTextMode}
        sending={Boolean(activeMessageState?.sending)}
      />
      <CreateGroupConversationDialog
        contacts={contacts}
        currentUserId={me.id}
        open={createGroupDialogOpen}
        onCreate={startGroupConversation}
        onOpenChange={setCreateGroupDialogOpen}
      />
    </>
  )
}

function CreateGroupConversationDialog({
  contacts,
  currentUserId,
  onCreate,
  onOpenChange,
  open,
}: {
  contacts: ContactUser[]
  currentUserId: string
  onCreate: (name: string, memberIds: string[]) => Promise<void>
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-5 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-base">发起群聊</DialogTitle>
          <DialogDescription className="sr-only">
            输入群聊名称并选择联系人创建群聊
          </DialogDescription>
        </DialogHeader>
        <CreateGroupConversationForm
          contacts={contacts}
          currentUserId={currentUserId}
          onCreate={onCreate}
          onOpenChange={onOpenChange}
        />
      </DialogContent>
    </Dialog>
  )
}

function CreateGroupConversationForm({
  contacts,
  currentUserId,
  onCreate,
  onOpenChange,
}: {
  contacts: ContactUser[]
  currentUserId: string
  onCreate: (name: string, memberIds: string[]) => Promise<void>
  onOpenChange: (open: boolean) => void
}) {
  const [creating, setCreating] = React.useState(false)
  const [keyword, setKeyword] = React.useState("")
  const [name, setName] = React.useState("")
  const [selectedMemberIds, setSelectedMemberIds] = React.useState<Set<string>>(
    () => new Set()
  )
  const selectedCount = selectedMemberIds.size
  const trimmedName = name.trim()
  const canCreate = Boolean(trimmedName) && selectedCount > 0 && !creating
  const filteredContacts = React.useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()

    return sortContactsByDisplayName(
      contacts.filter((contact) => {
        if (contact.id === currentUserId) {
          return false
        }
        if (!normalizedKeyword) {
          return true
        }

        return [
          contact.email,
          contact.name,
          contact.nickname,
          contact.phone,
        ].some((value) => value.toLowerCase().includes(normalizedKeyword))
      })
    )
  }, [contacts, currentUserId, keyword])

  function toggleMember(contactId: string, checked: boolean | string) {
    setSelectedMemberIds((currentIds) => {
      const nextChecked = Boolean(checked)
      const currentChecked = currentIds.has(contactId)

      if (currentChecked === nextChecked) {
        return currentIds
      }

      const nextIds = new Set(currentIds)

      if (nextChecked) {
        nextIds.add(contactId)
      } else {
        nextIds.delete(contactId)
      }

      return nextIds
    })
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!canCreate) {
      return
    }

    setCreating(true)

    try {
      await onCreate(trimmedName, Array.from(selectedMemberIds))
      onOpenChange(false)
    } catch {
      toast.error("创建群聊失败")
    } finally {
      setCreating(false)
    }
  }

  return (
    <form className="grid gap-4" onSubmit={handleSubmit}>
      <div className="grid gap-2">
        <Label htmlFor="create-group-name">群聊名称</Label>
        <Input
          id="create-group-name"
          onChange={(event) => setName(event.target.value)}
          placeholder="输入群聊名称"
          value={name}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="create-group-member-search">选择成员</Label>
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-8"
            id="create-group-member-search"
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索联系人"
            type="search"
            value={keyword}
          />
        </div>
      </div>
      <div className="h-64 overflow-y-auto rounded-md border">
        <ItemGroup
          aria-label="群聊成员"
          className="gap-1 p-2 has-data-[size=sm]:gap-1"
          role="group"
        >
          {filteredContacts.map((contact) => {
            const displayName = getContactDisplayName(contact)

            return (
              <CreateGroupMemberItem
                checked={selectedMemberIds.has(contact.id)}
                contact={contact}
                displayName={displayName}
                key={contact.id}
                onCheckedChange={(checked) => toggleMember(contact.id, checked)}
              />
            )
          })}
          {filteredContacts.length === 0 && (
            <div className="px-3 py-8 text-center text-sm text-muted-foreground">
              没有匹配的联系人
            </div>
          )}
        </ItemGroup>
      </div>
      <DialogFooter>
        <DialogClose asChild>
          <Button disabled={creating} type="button" variant="outline">
            取消
          </Button>
        </DialogClose>
        <Button disabled={!canCreate} type="submit">
          {creating && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          创建
        </Button>
      </DialogFooter>
    </form>
  )
}

function CreateGroupMemberItem({
  checked,
  contact,
  displayName,
  onCheckedChange,
}: {
  checked: boolean
  contact: ContactUser
  displayName: string
  onCheckedChange: (checked: boolean | string) => void
}) {
  const checkboxId = `create-group-member-${contact.id}`

  return (
    <Item
      asChild
      className={cn(
        "cursor-pointer px-2 py-1.5",
        checked ? "bg-primary/10" : "hover:bg-muted"
      )}
      size="sm"
    >
      <Label htmlFor={checkboxId}>
        <ItemMedia>
          <Avatar
            className="rounded-sm bg-muted after:rounded-sm"
            data-size="sm"
          >
            {contact.avatar && (
              <AvatarImage
                alt={displayName}
                className="rounded-sm"
                src={contact.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm">
              {getConversationInitial(displayName)}
            </AvatarFallback>
          </Avatar>
        </ItemMedia>
        <ItemContent className="min-w-0">
          <ItemTitle className="truncate">{displayName}</ItemTitle>
        </ItemContent>
        <ItemActions>
          <Checkbox
            aria-label={displayName}
            checked={checked}
            id={checkboxId}
            onCheckedChange={onCheckedChange}
          />
        </ItemActions>
      </Label>
    </Item>
  )
}
function getConversationListDescription(
  conversation: ClientConversation,
  mentionLabelResolver: MentionLabelResolver
) {
  const summary = conversation.lastMessageSummary.trim()

  return summary
    ? formatMentionTemplateText(summary, mentionLabelResolver)
    : "暂无消息"
}

function ConversationListAvatar({
  conversation,
}: {
  conversation: ClientConversation
}) {
  return (
    <div className="relative shrink-0">
      {conversation.type === "group" ? (
        <GroupAvatar
          avatar={conversation.avatar}
          className="size-10"
          members={conversation.members}
          name={conversation.name}
        />
      ) : (
        <Avatar className="size-10 rounded-sm bg-muted after:rounded-sm">
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
        </Avatar>
      )}
      {conversation.unreadCount > 0 && (
        <span className="absolute top-0 right-0 z-10 translate-x-1/3 -translate-y-1/3">
          <ConversationUnreadBadge count={conversation.unreadCount} />
        </span>
      )}
    </div>
  )
}

function getConversationInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}

function getContactDisplayName(contact: { name: string; nickname: string }) {
  const nickname = contact.nickname.trim()

  return nickname || contact.name.trim()
}

function ConversationUnreadBadge({ count }: { count: number }) {
  return (
    <Badge
      aria-label={`${count} 条未读消息`}
      className="h-4 bg-rose-700 px-1 py-0 text-[10px] leading-4 font-normal text-white dark:bg-rose-700"
      variant="destructive"
    >
      {formatUnreadCount(count)}
    </Badge>
  )
}

function formatUnreadCount(count: number) {
  if (count > 99) {
    return "99+"
  }

  return String(count)
}

function toConversationPanelMessage(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  messagesById: ReadonlyMap<string, ClientMessage>,
  mentionLabelResolver: MentionLabelResolver
): ConversationPanelMessage {
  const fromMe =
    message.sender.type === "user" && message.sender.id === currentUser.id
  const role =
    message.sender.type === "system" ? "system" : fromMe ? "me" : "other"

  return {
    author: getMessageAuthor(message, conversation, currentUser, contactsById),
    avatar: getMessageAvatar(message, conversation, currentUser, contactsById),
    body: message.body,
    canRevoke: canRevokeMessage(message, conversation, currentUser.id),
    delegatedByName: message.delegatedBy?.name ?? "",
    id: message.id,
    replyTo: getMessageReplyTarget(
      message,
      conversation,
      currentUser,
      contactsById,
      messagesById,
      mentionLabelResolver
    ),
    role,
    senderUserId: message.sender.type === "user" ? message.sender.id : null,
    time: getMessageTime(message.createdAt),
  }
}

function canRevokeMessage(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUserId: string
) {
  if (message.sender.type === "system" || message.body.type === "revoked") {
    return false
  }
  if (message.sender.type === "user" && message.sender.id === currentUserId) {
    return true
  }
  if (conversation.type !== "group") {
    return false
  }

  const currentMember = conversation.members?.find(
    (member) => member.id === currentUserId
  )

  return currentMember?.role === "owner" || currentMember?.role === "admin"
}

function getMessageReplyTarget(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>,
  messagesById: ReadonlyMap<string, ClientMessage>,
  mentionLabelResolver: MentionLabelResolver
): ConversationPanelReplyTarget | undefined {
  if (message.replyTo) {
    return {
      id: message.replyTo.id,
      author: getReplyToSenderAuthor(
        message.replyTo.sender,
        conversation,
        currentUser,
        contactsById
      ),
      summary: formatMentionTemplateText(
        message.replyTo.summary,
        mentionLabelResolver
      ),
    }
  }

  if (!message.replyToMessageId) {
    return undefined
  }

  const replyMessage = messagesById.get(message.replyToMessageId)
  if (!replyMessage) {
    return undefined
  }

  return {
    id: replyMessage.id,
    author: getMessageAuthor(
      replyMessage,
      conversation,
      currentUser,
      contactsById
    ),
    summary: formatConversationMessageSummary(
      replyMessage.body,
      mentionLabelResolver
    ),
  }
}

function formatConversationMessageSummary(
  body: ClientMessage["body"],
  mentionLabelResolver: MentionLabelResolver
) {
  return formatMentionTemplateText(
    formatClientMessageBodySummary(body),
    mentionLabelResolver
  )
}

function getReplyToSenderAuthor(
  sender: NonNullable<ClientMessage["replyTo"]>["sender"],
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>
) {
  if (sender.type === "system") {
    return "系统"
  }

  if (sender.type === "app") {
    return sender.name || conversation.name
  }

  if (sender.id === currentUser.id) {
    return formatMessageUserName(currentUser)
  }

  const contact = contactsById.get(sender.id)
  if (contact) {
    return formatMessageUserName(contact)
  }

  return (
    sender.name || (conversation.type === "direct" ? conversation.name : "成员")
  )
}

function getMessageAuthor(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "id" | "name" | "nickname">,
  contactsById: ReadonlyMap<string, ContactUser>
) {
  if (message.sender.type === "system") {
    return "系统"
  }

  if (message.sender.type === "app") {
    return conversation.name
  }

  if (message.sender.type === "user" && message.sender.id === currentUser.id) {
    return formatMessageUserName(currentUser)
  }

  if (message.sender.type === "user") {
    const contact = contactsById.get(message.sender.id)
    if (contact) {
      return formatMessageUserName(contact)
    }
  }

  if (message.sender.type === "user" && conversation.type === "direct") {
    return conversation.name
  }

  return "成员"
}

function getConversationOnlineStatus(
  conversation: ClientConversation,
  currentUserId: string,
  contactsById: ReadonlyMap<string, ContactUser>,
  contactAppsByLookup: ReadonlyMap<string, ContactApp>
) {
  if (conversation.type === "direct") {
    const otherMember = conversation.members?.find(
      (member) => member.id !== currentUserId
    )

    return otherMember
      ? (contactsById.get(otherMember.id)?.online ?? false)
      : false
  }

  if (conversation.type === "app") {
    return (
      contactAppsByLookup.get(conversation.id)?.online ??
      contactAppsByLookup.get(conversation.name)?.online
    )
  }

  return undefined
}

function formatMessageUserName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name
}

function getMessageAvatar(
  message: ClientMessage,
  conversation: ClientConversation,
  currentUser: Pick<ClientUser, "avatar" | "id">,
  contactsById: ReadonlyMap<string, ContactUser>
) {
  if (message.sender.type === "user" && message.sender.id === currentUser.id) {
    return currentUser.avatar
  }

  if (message.sender.type === "user") {
    return (
      contactsById.get(message.sender.id)?.avatar ||
      (conversation.type === "direct" ? conversation.avatar : "")
    )
  }

  if (message.sender.type === "app") {
    return conversation.avatar
  }

  return ""
}
