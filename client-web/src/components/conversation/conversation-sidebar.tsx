import { Bot, Plus } from "lucide-react"

import { ConversationListItemMenu } from "@/components/conversation-list-item-menu"
import { ConversationSearchPopover } from "@/components/conversation/conversation-search-popover"
import { GroupAvatar } from "@/components/group-avatar"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { createConversationMentionLabelResolver } from "@/lib/conversation-mention-labels"
import type { ConversationDrafts } from "@/lib/conversation-drafts"
import {
  formatMentionTemplateText,
  type MentionLabelResolver,
} from "@/lib/message-mentions"

export function ConversationSidebar({
  activeConversationId,
  appsById,
  contactsById,
  conversations,
  currentUser,
  drafts,
  onCreateGroup,
  onSelectConversation,
}: {
  activeConversationId: string
  appsById: ReadonlyMap<string, ContactApp>
  contactsById: ReadonlyMap<string, ContactUser>
  conversations: ClientConversation[]
  currentUser: ClientUser
  drafts: ConversationDrafts
  onCreateGroup: () => void
  onSelectConversation: (conversationId: string) => void
}) {
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
      })
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
              draftText: drafts[conversation.id]?.text,
              hasUnreadMention,
              messageDescription: getConversationListDescription(
                conversation,
                mentionLabelResolver
              ),
              selected,
            })

            return (
              <ConversationListItemMenu key={conversation.id}>
                <SidebarMenuItem data-conversation-list-item-trigger>
                  <SidebarMenuButton
                    className="h-16 gap-3 py-2 data-active:bg-teal-100 data-active:hover:bg-teal-100 dark:data-active:bg-teal-900 dark:data-active:hover:bg-teal-900"
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
                            {conversation.name}
                          </span>
                        </span>
                        {lastMessageTime && (
                          <span className="shrink-0 pr-2 text-xs font-normal text-muted-foreground">
                            {lastMessageTime}
                          </span>
                        )}
                      </div>
                      <p className="w-full min-w-0 truncate text-left text-xs leading-normal font-normal text-muted-foreground">
                        {preview.alertLabel && (
                          <span className="mr-1 font-medium text-rose-700 dark:text-rose-300">
                            {preview.alertLabel}
                          </span>
                        )}
                        <span>{preview.description}</span>
                      </p>
                    </div>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </ConversationListItemMenu>
            )
          })}
        </SidebarMenu>
      </SidebarContent>
    </Sidebar>
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
            {conversation.type === "app" ? (
              <Bot className="size-4" />
            ) : (
              getConversationInitial(conversation.name)
            )}
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
