import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react"
import {
  Bot,
  ChevronRight,
  Loader2Icon,
  MessageCircle,
  RefreshCw,
  Search,
} from "lucide-react"

import type {
  DirectorySelection,
  DirectoryTab,
} from "@/components/contacts/contact-directory"
import { directoryItemKey } from "@/components/contacts/contact-directory"
import { GroupAvatar } from "@/components/group-avatar"
import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarImage,
} from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInput,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type {
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/lib/client-data-api"
import { cn } from "@/lib/utils"

export function ContactDirectorySidebar({
  activeKeyword,
  activeSelection,
  activeTab,
  apps,
  contacts,
  contactsRefreshing,
  currentUserId,
  groups,
  organizationName,
  onActiveTabChange,
  onKeywordChange,
  onRefresh,
  onSelect,
  onStartAppConversation,
  onStartContactConversation,
  onStartGroupConversation,
  openingDirectoryItemKey,
}: {
  activeKeyword: string
  activeSelection: DirectorySelection | null
  activeTab: DirectoryTab
  apps: ContactApp[]
  contacts: ContactUser[]
  contactsRefreshing: boolean
  currentUserId: string
  groups: ContactGroup[]
  organizationName: string
  onActiveTabChange: (tab: DirectoryTab) => void
  onKeywordChange: (keyword: string) => void
  onRefresh: () => void
  onSelect: (selection: DirectorySelection) => void
  onStartAppConversation: (app: ContactApp) => void
  onStartContactConversation: (contact: ContactUser) => void
  onStartGroupConversation: (group: ContactGroup) => void
  openingDirectoryItemKey: string
}) {
  const activeTabLabel = getDirectoryTabLabel(activeTab)
  const joinedGroups = groups.filter((group) => group.joined)
  const publicGroups = groups.filter(
    (group) => !group.joined && group.visibility === "public"
  )

  return (
    <Sidebar className="border-r bg-background" collapsible="none">
      <SidebarHeader className="gap-0 p-0">
        <div className="flex h-14 items-center justify-between px-4">
          <h1 className="text-base font-medium">通讯录</h1>
          <Button
            aria-label="刷新"
            disabled={contactsRefreshing}
            onClick={onRefresh}
            size="icon-sm"
            title="刷新"
            type="button"
            variant="ghost"
          >
            <RefreshCw
              className={cn("size-4", contactsRefreshing && "animate-spin")}
            />
          </Button>
        </div>
      </SidebarHeader>
      <Tabs
        className="min-h-0 flex-1 gap-0"
        onValueChange={(value) => onActiveTabChange(value as DirectoryTab)}
        value={activeTab}
      >
        <div className="px-4 pb-3">
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="user">联系人</TabsTrigger>
            <TabsTrigger value="app">应用</TabsTrigger>
            <TabsTrigger value="group">群组</TabsTrigger>
          </TabsList>
          <div className="relative mt-3">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 z-10 size-4 -translate-y-1/2 text-muted-foreground" />
            <SidebarInput
              aria-label={`搜索${activeTabLabel}`}
              className="pl-8"
              onChange={(event) => onKeywordChange(event.target.value)}
              placeholder={`搜索${activeTabLabel}`}
              type="search"
              value={activeKeyword}
            />
          </div>
        </div>
        <TabsContent className="min-h-0 flex-1" value="user">
          <SidebarContent className="h-full">
            <DirectorySectionCollapsible
              defaultOpen={contacts.length > 0}
              forceOpen={Boolean(activeKeyword.trim())}
              count={contacts.length}
              title={organizationName}
            >
              <DirectoryList ariaLabel={`${organizationName} 联系人列表`}>
                {contacts.map((contact) => (
                  <ContactListItem
                    key={contact.id}
                    contact={contact}
                    canStartConversation={contact.id !== currentUserId}
                    size="sm"
                    selected={isDirectorySelection(
                      activeSelection,
                      "user",
                      contact.id
                    )}
                    onSelect={() => onSelect({ id: contact.id, type: "user" })}
                    onStartConversation={() =>
                      onStartContactConversation(contact)
                    }
                    startingConversation={
                      openingDirectoryItemKey ===
                      directoryItemKey("user", contact.id)
                    }
                  />
                ))}
                {contacts.length === 0 && (
                  <DirectoryEmptyState label={`${organizationName}联系人`} />
                )}
              </DirectoryList>
            </DirectorySectionCollapsible>
          </SidebarContent>
        </TabsContent>
        <TabsContent className="min-h-0 flex-1" value="app">
          <SidebarContent className="h-full">
            <DirectoryList ariaLabel="应用列表">
              {apps.map((app) => (
                <AppListItem
                  key={app.id}
                  app={app}
                  selected={isDirectorySelection(
                    activeSelection,
                    "app",
                    app.id
                  )}
                  onSelect={() => onSelect({ id: app.id, type: "app" })}
                  onStartConversation={() => onStartAppConversation(app)}
                  startingConversation={
                    openingDirectoryItemKey === directoryItemKey("app", app.id)
                  }
                />
              ))}
              {apps.length === 0 && <DirectoryEmptyState label="应用" />}
            </DirectoryList>
          </SidebarContent>
        </TabsContent>
        <TabsContent className="min-h-0 flex-1" value="group">
          <SidebarContent className="h-full">
            <DirectorySectionCollapsible
              count={joinedGroups.length}
              defaultOpen={joinedGroups.length > 0}
              forceOpen={Boolean(activeKeyword.trim())}
              title="我加入的"
            >
              <GroupDirectoryList
                activeSelection={activeSelection}
                ariaLabel="我加入的群组列表"
                groups={joinedGroups}
                onSelect={onSelect}
                onStartGroupConversation={onStartGroupConversation}
                openingDirectoryItemKey={openingDirectoryItemKey}
              />
            </DirectorySectionCollapsible>

            <DirectorySectionCollapsible
              count={publicGroups.length}
              defaultOpen={publicGroups.length > 0}
              forceOpen={Boolean(activeKeyword.trim())}
              title="公开群组"
            >
              <GroupDirectoryList
                activeSelection={activeSelection}
                ariaLabel="公开群组列表"
                groups={publicGroups}
                onSelect={onSelect}
                onStartGroupConversation={onStartGroupConversation}
                openingDirectoryItemKey={openingDirectoryItemKey}
              />
            </DirectorySectionCollapsible>
          </SidebarContent>
        </TabsContent>
      </Tabs>
    </Sidebar>
  )
}

function DirectoryList({
  ariaLabel,
  children,
}: {
  ariaLabel: string
  children: ReactNode
}) {
  return (
    <SidebarMenu aria-label={ariaLabel} className="px-2 pb-3" role="listbox">
      {children}
    </SidebarMenu>
  )
}

function DirectorySectionCollapsible({
  children,
  count,
  defaultOpen = false,
  forceOpen = false,
  title,
}: {
  children: ReactNode
  count: number
  defaultOpen?: boolean
  forceOpen?: boolean
  title: string
}) {
  const [open, setOpen] = useState(defaultOpen)
  const previousCount = useRef(count)
  const visible = forceOpen || open

  useEffect(() => {
    if (forceOpen) {
      return
    }

    const lastCount = previousCount.current
    previousCount.current = count
    if (lastCount === 0 && count > 0) {
      setOpen(true)
    } else if (lastCount > 0 && count === 0) {
      setOpen(false)
    }
  }, [count, forceOpen])

  return (
    <Collapsible
      className="mx-4 overflow-hidden rounded-md border"
      onOpenChange={(nextOpen) => {
        if (!forceOpen) {
          setOpen(nextOpen)
        }
      }}
      open={visible}
    >
      <CollapsibleTrigger asChild>
        <Button
          aria-label={title}
          className="w-full justify-between rounded-none px-4 hover:bg-transparent aria-expanded:bg-transparent dark:hover:bg-transparent"
          size="lg"
          type="button"
          variant="ghost"
        >
          <span className="flex min-w-0 items-center gap-1.5">
            <ChevronRight
              className={cn(
                "size-4 shrink-0 transition-transform",
                visible && "rotate-90"
              )}
            />
            <span className="truncate">{title}</span>
          </span>
          <Badge variant="secondary">{count}</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>{children}</CollapsibleContent>
    </Collapsible>
  )
}

function GroupDirectoryList({
  activeSelection,
  ariaLabel,
  groups,
  onSelect,
  onStartGroupConversation,
  openingDirectoryItemKey,
}: {
  activeSelection: DirectorySelection | null
  ariaLabel: string
  groups: ContactGroup[]
  onSelect: (selection: DirectorySelection) => void
  onStartGroupConversation: (group: ContactGroup) => void
  openingDirectoryItemKey: string
}) {
  return (
    <DirectoryList ariaLabel={ariaLabel}>
      {groups.map((group) => (
        <GroupListItem
          key={group.id}
          group={group}
          selected={isDirectorySelection(activeSelection, "group", group.id)}
          onSelect={() => onSelect({ id: group.id, type: "group" })}
          onStartConversation={() => onStartGroupConversation(group)}
          startingConversation={
            openingDirectoryItemKey === directoryItemKey("group", group.id)
          }
        />
      ))}
      {groups.length === 0 && <DirectoryEmptyState label={ariaLabel} />}
    </DirectoryList>
  )
}

function DirectoryEmptyState({ label }: { label: string }) {
  return (
    <SidebarMenuItem>
      <div className="px-3 py-8 text-center text-sm text-muted-foreground">
        没有匹配的{label}
      </div>
    </SidebarMenuItem>
  )
}

function AppListItem({
  app,
  onSelect,
  onStartConversation,
  selected,
  startingConversation,
}: {
  app: ContactApp
  onSelect: () => void
  onStartConversation: () => void
  selected: boolean
  startingConversation: boolean
}) {
  return (
    <DirectoryListItem
      actionDisabled={false}
      actionLabel={`与 ${app.name} 对话`}
      actionLoading={startingConversation}
      media={
        <Avatar className="rounded-sm bg-muted after:rounded-sm">
          {app.avatar && (
            <AvatarImage
              alt={app.name}
              className="rounded-sm"
              src={app.avatar}
            />
          )}
          <AvatarFallback className="rounded-sm">
            <Bot className="size-4" />
          </AvatarFallback>
          <ContactAvatarBadge online={app.online} />
        </Avatar>
      }
      onAction={onStartConversation}
      onSelect={onSelect}
      selected={selected}
      title={app.name}
    />
  )
}

function GroupListItem({
  group,
  onSelect,
  onStartConversation,
  selected,
  startingConversation,
}: {
  group: ContactGroup
  onSelect: () => void
  onStartConversation: () => void
  selected: boolean
  startingConversation: boolean
}) {
  return (
    <DirectoryListItem
      actionDisabled={false}
      actionLabel={group.joined ? `进入 ${group.name}` : `加入 ${group.name}`}
      actionLoading={startingConversation}
      media={
        <GroupAvatar
          avatar={group.avatar}
          className="size-8"
          members={group.avatarMembers}
          name={group.name}
        />
      }
      onAction={onStartConversation}
      onSelect={onSelect}
      selected={selected}
      title={group.name}
    />
  )
}

function ContactListItem({
  canStartConversation,
  contact,
  onSelect,
  onStartConversation,
  selected,
  startingConversation,
  size = "default",
}: {
  canStartConversation: boolean
  contact: ContactUser
  onSelect: () => void
  onStartConversation: () => void
  selected: boolean
  startingConversation: boolean
  size?: "default" | "sm"
}) {
  const displayName = getContactDisplayName(contact)
  const title = getContactItemTitle(contact)

  return (
    <DirectoryListItem
      actionLabel={canStartConversation ? `与 ${title} 对话` : undefined}
      actionLoading={startingConversation}
      media={
        <Avatar
          className="rounded-sm bg-muted after:rounded-sm"
          data-testid="contact-avatar"
        >
          {contact.avatar && (
            <AvatarImage
              alt={displayName}
              className="rounded-sm"
              src={contact.avatar}
            />
          )}
          <AvatarFallback className="rounded-sm">
            {getContactInitial(displayName)}
          </AvatarFallback>
          <ContactAvatarBadge online={contact.online} />
        </Avatar>
      }
      onAction={canStartConversation ? onStartConversation : undefined}
      onSelect={onSelect}
      selected={selected}
      size={size}
      title={title}
    />
  )
}

function DirectoryListItem({
  actionDisabled = false,
  actionLabel,
  actionLoading = false,
  media,
  onAction,
  onSelect,
  selected,
  size = "sm",
  title,
}: {
  actionDisabled?: boolean
  actionLabel?: string
  actionLoading?: boolean
  media: ReactNode
  onAction?: () => void
  onSelect: () => void
  selected: boolean
  size?: "default" | "sm"
  title: string
}) {
  function handleActionClick(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!actionDisabled && onAction) {
      onAction()
    }
  }

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        aria-label={title}
        aria-selected={selected}
        className={cn(
          "gap-2.5 data-active:bg-teal-200 data-active:hover:bg-teal-200 dark:data-active:bg-teal-800 dark:data-active:hover:bg-teal-800",
          onAction && "pr-8",
          size === "sm" ? "h-11" : "h-12"
        )}
        isActive={selected}
        onClick={onSelect}
        role="option"
        size="lg"
        type="button"
      >
        {media}
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          <span className="min-w-0 truncate">{title}</span>
        </span>
      </SidebarMenuButton>
      {onAction && actionLabel && (
        <SidebarMenuAction
          aria-label={actionLabel}
          className="right-2 size-6 disabled:pointer-events-none disabled:opacity-50 [&>svg]:size-3"
          disabled={actionLoading || actionDisabled}
          onClick={handleActionClick}
          showOnHover={!selected}
          type="button"
        >
          {actionLoading ? (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          ) : (
            <MessageCircle />
          )}
        </SidebarMenuAction>
      )}
    </SidebarMenuItem>
  )
}

function ContactAvatarBadge({ online }: { online: boolean }) {
  return (
    <AvatarBadge
      aria-label={online ? "在线" : "离线"}
      className={
        online ? "bg-emerald-500" : "bg-neutral-400 dark:bg-neutral-500"
      }
    />
  )
}

function isDirectorySelection(
  selection: DirectorySelection | null,
  type: DirectorySelection["type"],
  id: string
) {
  return selection?.type === type && selection.id === id
}

function getDirectoryTabLabel(tab: DirectoryTab) {
  if (tab === "app") {
    return "应用"
  }

  if (tab === "group") {
    return "群组"
  }

  return "联系人"
}

function getContactItemTitle(contact: { name: string; nickname: string }) {
  const nickname = contact.nickname.trim()

  return nickname || contact.name.trim()
}

function getContactDisplayName(contact: { name: string; nickname: string }) {
  return contact.nickname || contact.name
}

function getContactInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
