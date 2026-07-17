import {
  Bot,
  FolderClosed,
  Settings,
  UserRound,
  UsersRound,
} from "lucide-react"
import { getAvatarInitial } from "@/lib/avatar"
import { type ClientConversation } from "@/lib/client-data-api"
import { AddGroupMembersDialog } from "@/components/add-group-members-dialog"
import { AppProfilePopover } from "@/components/app-profile-popover"
import { ConversationInfoDrawer } from "@/components/conversation-info-drawer"
import { GroupAvatar } from "@/components/group-avatar"
import { GroupProfilePopover } from "@/components/group-profile-popover"
import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarImage,
} from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { UserProfilePopover } from "@/components/user-profile-popover"

export function ConversationPanelHeader({
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
          {conversation.type === "app" && (
            <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <Bot className="size-3" />
              应用
            </span>
          )}
          {conversation.type === "direct" && (
            <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <UserRound className="size-3" />
              私聊
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
        {conversation.type === "app" ? (
          <Bot className="size-4" />
        ) : (
          getAvatarInitial(conversation.name)
        )}
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
