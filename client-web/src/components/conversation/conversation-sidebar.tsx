import * as React from "react"
import { BellOff, Pin, Plus } from "lucide-react"
import { toast } from "sonner"

import { ConversationListItemMenu } from "@/components/conversation-list-item-menu"
import { ConversationAvatar } from "@/components/conversation/conversation-avatar"
import { ConversationSearchPopover } from "@/components/conversation/conversation-search-popover"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { formatActivityTime } from "@/lib/activity-time"
import type {
  ClientConversation,
  ClientUser,
  ContactApp,
  ContactUser,
} from "@/lib/client-data-api"
import { getConversationDisplayName } from "@/lib/conversation-avatar-presentation"
import {
  getClientDataErrorMessage,
  isBuiltinAssistantConversation,
} from "@/lib/client-data-state"
import { createConversationMentionLabelResolver } from "@/lib/conversation-mention-labels"
import type { ConversationDrafts } from "@/lib/conversation-drafts"
import {
  formatMentionTemplateText,
  type MentionLabelResolver,
} from "@/lib/message-mentions"
import { cn } from "@/lib/utils"

export function ConversationSidebar({
  activeConversationId,
  appsById,
  contactsById,
  conversations,
  currentUser,
  drafts,
  onCreateGroup,
  onDismissConversation,
  onSelectConversation,
  onSetConversationMuted,
  onSetConversationPinned,
}: {
  activeConversationId: string
  appsById: ReadonlyMap<string, ContactApp>
  contactsById: ReadonlyMap<string, ContactUser>
  conversations: ClientConversation[]
  currentUser: ClientUser
  drafts: ConversationDrafts
  onCreateGroup: () => void
  onDismissConversation?: (conversationId: string) => Promise<void>
  onSelectConversation: (conversationId: string) => void
  onSetConversationMuted?: (
    conversationId: string,
    muted: boolean
  ) => Promise<void>
  onSetConversationPinned: (
    conversationId: string,
    pinned: boolean
  ) => Promise<void>
}) {
  const [pinningConversationId, setPinningConversationId] = React.useState("")
  const [mutingConversationId, setMutingConversationId] = React.useState("")
  const [dismissingConversationId, setDismissingConversationId] =
    React.useState("")
  const [dismissCandidate, setDismissCandidate] =
    React.useState<ClientConversation | null>(null)

  async function handlePinnedChange(
    conversation: ClientConversation,
    pinned: boolean
  ) {
    if (pinningConversationId) {
      return
    }
    setPinningConversationId(conversation.id)
    try {
      await onSetConversationPinned(conversation.id, pinned)
      toast.success(pinned ? "会话已置顶" : "已取消置顶")
    } catch (error) {
      toast.error(
        getClientDataErrorMessage(
          error,
          pinned ? "置顶会话失败" : "取消置顶失败"
        )
      )
    } finally {
      setPinningConversationId("")
    }
  }

  async function handleMutedChange(
    conversation: ClientConversation,
    muted: boolean
  ) {
    if (mutingConversationId || !onSetConversationMuted) {
      return
    }
    setMutingConversationId(conversation.id)
    try {
      await onSetConversationMuted(conversation.id, muted)
      toast.success(muted ? "已开启消息免打扰" : "已取消消息免打扰")
    } catch (error) {
      toast.error(
        getClientDataErrorMessage(
          error,
          muted ? "开启消息免打扰失败" : "取消消息免打扰失败"
        )
      )
    } finally {
      setMutingConversationId("")
    }
  }

  async function handleDismissConversation() {
    if (
      !dismissCandidate ||
      dismissingConversationId ||
      !onDismissConversation
    ) {
      return
    }
    const conversation = dismissCandidate
    setDismissingConversationId(conversation.id)
    try {
      await onDismissConversation(conversation.id)
      setDismissCandidate(null)
      toast.success("对话已删除")
    } catch (error) {
      toast.error(getClientDataErrorMessage(error, "删除对话失败"))
    } finally {
      setDismissingConversationId("")
    }
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

  function getSearchConversationDescription(conversation: ClientConversation) {
    return getConversationListDescription(
      conversation,
      createConversationMentionLabelResolver({
        appsById,
        contactsById,
        conversation,
        currentUser,
      }),
      currentUser.id
    )
  }

  return (
    <Sidebar className="border-r bg-background" collapsible="none">
      <SidebarHeader className="gap-0 p-0">
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
              <DropdownMenuItem onSelect={onCreateGroup}>
                发起群聊
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="px-4 pb-3">
          <ConversationSearchPopover
            conversations={conversations}
            currentUserId={currentUser.id}
            getConversationDescription={getSearchConversationDescription}
            onSelectConversation={onSelectConversation}
          />
        </div>
      </SidebarHeader>
      <SidebarContent onContextMenu={handleConversationListContextMenu}>
        <SidebarMenu className="px-2 pb-3">
          {conversations.length === 0 && (
            <SidebarMenuItem>
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                暂无会话
              </div>
            </SidebarMenuItem>
          )}
          {conversations.map((conversation) => {
            const selected = conversation.id === activeConversationId
            const lastMessageTime = formatActivityTime(
              conversation.lastMessageAt ?? conversation.createdAt
            )
            const mentionLabelResolver = createConversationMentionLabelResolver(
              {
                appsById,
                contactsById,
                conversation,
                currentUser,
              }
            )
            const hasUnreadMention =
              conversation.lastMentionedSeq > conversation.lastReadSeq
            const preview = getConversationListPreview({
              draftText: conversation.topic?.archived
                ? undefined
                : drafts[conversation.id]?.text,
              hasUnreadMention,
              messageDescription: getConversationListDescription(
                conversation,
                mentionLabelResolver,
                currentUser.id
              ),
              selected,
            })

            return (
              <ConversationListItemMenu
                dismissing={dismissingConversationId === conversation.id}
                key={conversation.id}
                muted={Boolean(conversation.notificationMuted)}
                muting={mutingConversationId === conversation.id}
                onDismiss={() => setDismissCandidate(conversation)}
                onMutedChange={(muted) =>
                  void handleMutedChange(conversation, muted)
                }
                onPinnedChange={(pinned) =>
                  void handlePinnedChange(conversation, pinned)
                }
                pinned={Boolean(conversation.pinned)}
                pinning={pinningConversationId === conversation.id}
                showPinAction={!isBuiltinAssistantConversation(conversation)}
              >
                <SidebarMenuItem data-conversation-list-item-trigger>
                  <SidebarMenuButton
                    className={cn(
                      "h-16 gap-3 py-2 data-active:bg-teal-100 data-active:hover:bg-teal-100 dark:data-active:bg-teal-900 dark:data-active:hover:bg-teal-900",
                      conversation.pinned &&
                        "bg-neutral-100 hover:bg-neutral-100 dark:bg-neutral-900 dark:hover:bg-neutral-900"
                    )}
                    isActive={selected}
                    onClick={() => onSelectConversation(conversation.id)}
                    size="lg"
                    type="button"
                  >
                    <ConversationListAvatar conversation={conversation} />
                    <div className="min-w-0 flex-1 overflow-hidden">
                      <div className="flex w-full min-w-0 items-center justify-between gap-2 overflow-hidden text-sm leading-snug font-medium underline-offset-4">
                        <span className="flex min-w-0 flex-1 items-center overflow-hidden">
                          <span className="block min-w-0 flex-1 truncate">
                            {getConversationDisplayName(conversation)}
                          </span>
                          {conversation.topic?.archived && (
                            <span className="ml-1.5 shrink-0 text-[10px] font-normal text-muted-foreground">
                              已关闭
                            </span>
                          )}
                        </span>
                        {lastMessageTime && (
                          <span className="shrink-0 pr-2 text-xs font-normal text-muted-foreground">
                            {lastMessageTime}
                          </span>
                        )}
                      </div>
                      <p className="flex w-full min-w-0 items-center gap-0.5 text-left text-xs leading-normal font-normal text-muted-foreground">
                        <span className="min-w-0 flex-1 truncate">
                          {preview.alertLabel && (
                            <span className="mr-1 font-medium text-rose-700 dark:text-rose-300">
                              {preview.alertLabel}
                            </span>
                          )}
                          <span>{preview.description}</span>
                        </span>
                        {(conversation.pinned ||
                          conversation.notificationMuted) && (
                          <span className="mr-2 flex shrink-0 items-center gap-0.5">
                            {conversation.pinned && (
                              <Pin
                                aria-label="已置顶"
                                className="size-3! shrink-0"
                              />
                            )}
                            {conversation.notificationMuted && (
                              <BellOff
                                aria-label="消息免打扰"
                                className="size-3! shrink-0"
                              />
                            )}
                          </span>
                        )}
                      </p>
                    </div>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </ConversationListItemMenu>
            )
          })}
        </SidebarMenu>
      </SidebarContent>
      <AlertDialog
        onOpenChange={(open) => {
          if (!open && !dismissingConversationId) {
            setDismissCandidate(null)
          }
        }}
        open={Boolean(dismissCandidate)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除对话？</AlertDialogTitle>
            <AlertDialogDescription>
              删除后，该对话将暂时从列表中移除。收到新消息后会重新显示，聊天记录不会删除，也不会退出群聊。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(dismissingConversationId)}>
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={Boolean(dismissingConversationId)}
              onClick={(event) => {
                event.preventDefault()
                void handleDismissConversation()
              }}
              variant="destructive"
            >
              {dismissingConversationId ? "删除中..." : "删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Sidebar>
  )
}

function getConversationListDescription(
  conversation: ClientConversation,
  mentionLabelResolver: MentionLabelResolver,
  currentUserId: string
) {
  const summary = conversation.lastMessageSummary.trim()
  if (!summary) {
    return "暂无消息"
  }

  const description = formatMentionTemplateText(summary, mentionLabelResolver)
  if (conversation.type === "direct" || conversation.type === "app") {
    return description
  }
  const senderName = getLastMessageSenderName(conversation, currentUserId)
  return senderName ? `${senderName}：${description}` : description
}

function getLastMessageSenderName(
  conversation: ClientConversation,
  currentUserId: string
) {
  const sender = conversation.lastMessageSender
  if (!sender) {
    return ""
  }
  if (sender.type === "system") {
    return "系统"
  }
  if (sender.type === "user" && sender.id === currentUserId) {
    return "我"
  }
  return sender.nickname.trim() || sender.name.trim()
}

function getConversationListPreview({
  draftText,
  hasUnreadMention,
  messageDescription,
  selected,
}: {
  draftText: string | undefined
  hasUnreadMention: boolean
  messageDescription: string
  selected: boolean
}) {
  if (selected) {
    return {
      alertLabel: null,
      description: messageDescription,
    }
  }

  if (hasUnreadMention) {
    return {
      alertLabel: "[有人 @ 我]",
      description: messageDescription,
    }
  }

  if (draftText !== undefined) {
    return {
      alertLabel: "[草稿]",
      description: draftText,
    }
  }

  return {
    alertLabel: null,
    description: messageDescription,
  }
}

function ConversationListAvatar({
  conversation,
}: {
  conversation: ClientConversation
}) {
  return (
    <div className="relative shrink-0">
      <ConversationAvatar
        className="size-10"
        conversation={conversation}
        sourceAvatarClassName="size-5"
      />
      {conversation.unreadCount > 0 && (
        <span className="absolute top-0 right-0 z-10 translate-x-1/3 -translate-y-1/3">
          {conversation.notificationMuted ? (
            <span
              aria-label="有未读消息"
              className="block size-2 rounded-full bg-rose-700"
            />
          ) : (
            <ConversationUnreadBadge count={conversation.unreadCount} />
          )}
        </span>
      )}
    </div>
  )
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
