import * as React from "react"
import { useNavigate } from "react-router"
import { toast } from "sonner"
import {
  Bot,
  Loader2Icon,
  Mail,
  MessageCircle,
  Phone,
  RefreshCw,
  Search,
  UserPen,
  UserRound,
  UsersRound,
} from "lucide-react"

import { cn } from "@/lib/utils"
import { formatContactPhone } from "@/lib/contact-format"
import { sortContactsByDisplayName } from "@/lib/contact-sort"
import { useClientData } from "@/lib/client-data-context"
import type {
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/lib/client-data-api"
import { GroupAvatar } from "@/components/group-avatar"
import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarImage,
} from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInput,
  SidebarInset,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

const CONTACT_DETAIL_PANEL_CLASS = "mt-30 w-full max-w-sm"

type DirectorySelection =
  | { id: string; type: "app" }
  | { id: string; type: "group" }
  | { id: string; type: "user" }

type ActiveDirectoryItem =
  | { app: ContactApp; type: "app" }
  | { group: ContactGroup; type: "group" }
  | { contact: ContactUser; type: "user" }

type DirectoryTab = DirectorySelection["type"]

export function ContactsPage() {
  const {
    contactApps,
    contactGroups,
    contacts,
    contactsRefreshing,
    joinGroupConversation,
    me,
    openAppConversation,
    openDirectConversation,
    refreshContacts,
  } = useClientData()
  const navigate = useNavigate()
  const [activeSelection, setActiveSelection] =
    React.useState<DirectorySelection | null>(null)
  const [openingDirectoryItemKey, setOpeningDirectoryItemKey] =
    React.useState("")
  const [activeTab, setActiveTab] = React.useState<DirectoryTab>("user")
  const [keywords, setKeywords] = React.useState<Record<DirectoryTab, string>>({
    app: "",
    group: "",
    user: "",
  })
  const activeKeyword = keywords[activeTab]
  const activeTabLabel = getDirectoryTabLabel(activeTab)
  const normalizedAppKeyword = keywords.app.trim().toLowerCase()
  const normalizedContactKeyword = keywords.user.trim().toLowerCase()
  const normalizedGroupKeyword = keywords.group.trim().toLowerCase()
  const filteredApps = React.useMemo(() => {
    if (!normalizedAppKeyword) {
      return contactApps
    }

    return contactApps.filter((app) =>
      [app.name, app.description].some((value) =>
        value.toLowerCase().includes(normalizedAppKeyword)
      )
    )
  }, [contactApps, normalizedAppKeyword])
  const filteredContacts = React.useMemo(() => {
    if (!normalizedContactKeyword) {
      return sortContactsByDisplayName(contacts)
    }

    return sortContactsByDisplayName(
      contacts.filter((contact) =>
        [
          contact.email,
          contact.name,
          contact.nickname,
          contact.phone,
          formatContactPhone(contact.phone),
        ].some((value) =>
          value.toLowerCase().includes(normalizedContactKeyword)
        )
      )
    )
  }, [contacts, normalizedContactKeyword])
  const filteredGroups = React.useMemo(() => {
    if (!normalizedGroupKeyword) {
      return contactGroups
    }

    return contactGroups.filter((group) =>
      group.name.toLowerCase().includes(normalizedGroupKeyword)
    )
  }, [contactGroups, normalizedGroupKeyword])
  const activeItem = resolveActiveDirectoryItem(
    activeSelection,
    contactApps,
    contacts,
    contactGroups
  )

  async function startDirectConversation(contact: ContactUser) {
    if (contact.id === me.id) {
      return
    }

    const itemKey = directoryItemKey("user", contact.id)
    setOpeningDirectoryItemKey(itemKey)

    try {
      const conversation = await openDirectConversation(contact.id)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法发起私聊")
    } finally {
      setOpeningDirectoryItemKey((currentItemKey) =>
        currentItemKey === itemKey ? "" : currentItemKey
      )
    }
  }

  async function startAppConversation(app: ContactApp) {
    const itemKey = directoryItemKey("app", app.id)
    setOpeningDirectoryItemKey(itemKey)

    try {
      const conversation = await openAppConversation(app.id)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法发起应用会话")
    } finally {
      setOpeningDirectoryItemKey((currentItemKey) =>
        currentItemKey === itemKey ? "" : currentItemKey
      )
    }
  }

  async function openOrJoinGroupConversation(group: ContactGroup) {
    const itemKey = directoryItemKey("group", group.id)

    if (group.joined) {
      navigate(`/chat/${encodeURIComponent(group.id)}`)
      return
    }

    setOpeningDirectoryItemKey(itemKey)

    try {
      const conversation = await joinGroupConversation(group.id)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法加入群聊")
    } finally {
      setOpeningDirectoryItemKey((currentItemKey) =>
        currentItemKey === itemKey ? "" : currentItemKey
      )
    }
  }

  function updateActiveKeyword(nextKeyword: string) {
    setKeywords((currentKeywords) => ({
      ...currentKeywords,
      [activeTab]: nextKeyword,
    }))
  }

  return (
    <SidebarProvider
      className="min-h-0 min-w-0 flex-1"
      style={
        {
          "--sidebar-width": "18rem",
        } as React.CSSProperties
      }
    >
      <Sidebar className="border-r bg-background" collapsible="none">
        <SidebarHeader className="gap-0 p-0">
          <div className="flex h-14 items-center justify-between px-4">
            <h1 className="text-base font-medium">通讯录</h1>
            <Button
              aria-label="刷新"
              disabled={contactsRefreshing}
              onClick={() => void refreshContacts()}
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
          onValueChange={(value) => setActiveTab(value as DirectoryTab)}
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
                onChange={(event) => updateActiveKeyword(event.target.value)}
                placeholder={`搜索${activeTabLabel}`}
                type="search"
                value={activeKeyword}
              />
            </div>
          </div>
          <TabsContent className="min-h-0 flex-1" value="user">
            <SidebarContent className="h-full">
              <DirectoryList ariaLabel="联系人列表">
                {filteredContacts.map((contact) => (
                  <ContactListItem
                    key={contact.id}
                    contact={contact}
                    canStartConversation={contact.id !== me.id}
                    size="sm"
                    selected={isDirectorySelection(
                      activeSelection,
                      "user",
                      contact.id
                    )}
                    onSelect={() =>
                      setActiveSelection({ id: contact.id, type: "user" })
                    }
                    onStartConversation={() =>
                      void startDirectConversation(contact)
                    }
                    startingConversation={
                      openingDirectoryItemKey ===
                      directoryItemKey("user", contact.id)
                    }
                  />
                ))}
                {filteredContacts.length === 0 && (
                  <DirectoryEmptyState label="联系人" />
                )}
              </DirectoryList>
            </SidebarContent>
          </TabsContent>
          <TabsContent className="min-h-0 flex-1" value="app">
            <SidebarContent className="h-full">
              <DirectoryList ariaLabel="应用列表">
                {filteredApps.map((app) => (
                  <AppListItem
                    key={app.id}
                    app={app}
                    selected={isDirectorySelection(
                      activeSelection,
                      "app",
                      app.id
                    )}
                    onSelect={() =>
                      setActiveSelection({ id: app.id, type: "app" })
                    }
                    onStartConversation={() => void startAppConversation(app)}
                    startingConversation={
                      openingDirectoryItemKey ===
                      directoryItemKey("app", app.id)
                    }
                  />
                ))}
                {filteredApps.length === 0 && (
                  <DirectoryEmptyState label="应用" />
                )}
              </DirectoryList>
            </SidebarContent>
          </TabsContent>
          <TabsContent className="min-h-0 flex-1" value="group">
            <SidebarContent className="h-full">
              <DirectoryList ariaLabel="群组列表">
                {filteredGroups.map((group) => (
                  <GroupListItem
                    key={group.id}
                    group={group}
                    selected={isDirectorySelection(
                      activeSelection,
                      "group",
                      group.id
                    )}
                    onSelect={() =>
                      setActiveSelection({ id: group.id, type: "group" })
                    }
                    onStartConversation={() =>
                      void openOrJoinGroupConversation(group)
                    }
                    startingConversation={
                      openingDirectoryItemKey ===
                      directoryItemKey("group", group.id)
                    }
                  />
                ))}
                {filteredGroups.length === 0 && (
                  <DirectoryEmptyState label="群组" />
                )}
              </DirectoryList>
            </SidebarContent>
          </TabsContent>
        </Tabs>
      </Sidebar>

      <SidebarInset className="min-w-0 overflow-hidden">
        <div
          className={cn(
            "flex min-h-0 flex-1 items-start justify-center px-6",
            activeItem ? "bg-background" : "bg-muted"
          )}
          data-testid="contact-detail-shell"
        >
          {activeItem?.type === "app" ? (
            <AppDetailPanel
              app={activeItem.app}
              onStartConversation={() =>
                void startAppConversation(activeItem.app)
              }
              startingConversation={
                openingDirectoryItemKey ===
                directoryItemKey("app", activeItem.app.id)
              }
            />
          ) : activeItem?.type === "group" ? (
            <GroupDetailPanel
              group={activeItem.group}
              onStartConversation={() =>
                void openOrJoinGroupConversation(activeItem.group)
              }
              startingConversation={
                openingDirectoryItemKey ===
                directoryItemKey("group", activeItem.group.id)
              }
            />
          ) : activeItem?.type === "user" ? (
            <ContactDetailPanel
              contact={activeItem.contact}
              canStartConversation={activeItem.contact.id !== me.id}
              onStartConversation={() =>
                void startDirectConversation(activeItem.contact)
              }
              startingConversation={
                openingDirectoryItemKey ===
                directoryItemKey("user", activeItem.contact.id)
              }
            />
          ) : (
            <ContactEmptyState />
          )}
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}

function DirectoryList({
  ariaLabel,
  children,
}: {
  ariaLabel: string
  children: React.ReactNode
}) {
  return (
    <SidebarMenu aria-label={ariaLabel} className="px-2 pb-3" role="listbox">
      {children}
    </SidebarMenu>
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

function AppDetailPanel({
  app,
  onStartConversation,
  startingConversation,
}: {
  app: ContactApp
  onStartConversation: () => void
  startingConversation: boolean
}) {
  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center gap-3 text-center">
          <Avatar
            className="size-20 rounded-sm bg-muted after:rounded-sm"
            data-testid="contact-detail-avatar"
          >
            {app.avatar && (
              <AvatarImage
                alt={app.name}
                className="rounded-sm"
                src={app.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              <Bot className="size-7" />
            </AvatarFallback>
          </Avatar>
          <div>
            <div className="text-base font-medium">{app.name}</div>
            {app.description && (
              <div className="mt-1 text-sm text-muted-foreground">
                {app.description}
              </div>
            )}
          </div>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<Bot className="size-4 text-muted-foreground" />}
            label="类型"
            value="应用"
          />
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="状态"
            value={app.online ? "在线" : "离线"}
          />
        </div>
        <Button
          className="w-full"
          disabled={startingConversation}
          onClick={onStartConversation}
          type="button"
        >
          {startingConversation && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          发消息
        </Button>
      </div>
    </div>
  )
}

function GroupDetailPanel({
  group,
  onStartConversation,
  startingConversation,
}: {
  group: ContactGroup
  onStartConversation: () => void
  startingConversation: boolean
}) {
  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center gap-3 text-center">
          <GroupAvatar
            avatar={group.avatar}
            className="size-20"
            name={group.name}
          />
          <div>
            <div className="text-base font-medium">{group.name}</div>
            <div className="mt-1 text-sm text-muted-foreground">
              {group.memberCount} 人群聊
            </div>
          </div>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<UsersRound className="size-4 text-muted-foreground" />}
            label="类型"
            value="群组"
          />
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="状态"
            value={group.joined ? "已加入" : "未加入"}
          />
        </div>
        <Button
          className="w-full"
          disabled={startingConversation}
          onClick={onStartConversation}
          type="button"
        >
          {startingConversation && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          {group.joined ? "发消息" : "加入群聊"}
        </Button>
      </div>
    </div>
  )
}

function ContactDetailPanel({
  canStartConversation,
  contact,
  onStartConversation,
  startingConversation,
}: {
  canStartConversation: boolean
  contact: ContactUser
  onStartConversation: () => void
  startingConversation: boolean
}) {
  const displayName = getContactDisplayName(contact)

  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center text-center">
          <Avatar
            className="size-20 rounded-sm bg-muted after:rounded-sm"
            data-testid="contact-detail-avatar"
          >
            {contact.avatar && (
              <AvatarImage
                alt={displayName}
                className="rounded-sm"
                src={contact.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              {getContactInitial(displayName)}
            </AvatarFallback>
          </Avatar>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="姓名"
            value={contact.name}
          />
          <ContactDetailRow
            icon={<UserPen className="size-4 text-muted-foreground" />}
            label="昵称"
            value={contact.nickname}
          />
          <ContactDetailRow
            icon={<Mail className="size-4 text-muted-foreground" />}
            label="邮箱"
            value={contact.email}
          />
          <ContactDetailRow
            icon={<Phone className="size-4 text-muted-foreground" />}
            label="手机"
            value={contact.phone ? formatContactPhone(contact.phone) : ""}
          />
        </div>
        <Button
          className="w-full"
          disabled={startingConversation || !canStartConversation}
          onClick={onStartConversation}
          type="button"
        >
          {startingConversation && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          发消息
        </Button>
      </div>
    </div>
  )
}

function ContactEmptyState() {
  return (
    <div
      className="flex flex-1 items-center justify-center self-stretch text-sm text-muted-foreground"
      data-testid="contact-empty-state"
    >
      选择一个联系人查看详情
    </div>
  )
}

function ContactDetailRow({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode
  label: string
  value: string
}) {
  const hasValue = Boolean(value.trim())
  const displayValue = hasValue ? value : "未设置"

  return (
    <div className="flex items-center gap-3 border-b py-2 last:border-b-0">
      {icon}
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span
        className={cn("min-w-0 truncate", !hasValue && "text-muted-foreground")}
      >
        {displayValue}
      </span>
    </div>
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
      actionDisabled={!canStartConversation}
      actionLabel={`与 ${title} 对话`}
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
      onAction={onStartConversation}
      onSelect={onSelect}
      selected={selected}
      size={size}
      title={title}
    />
  )
}

function DirectoryListItem({
  actionDisabled,
  actionLabel,
  actionLoading,
  media,
  onAction,
  onSelect,
  selected,
  size = "sm",
  title,
}: {
  actionDisabled: boolean
  actionLabel: string
  actionLoading: boolean
  media: React.ReactNode
  onAction: () => void
  onSelect: () => void
  selected: boolean
  size?: "default" | "sm"
  title: string
}) {
  function handleActionClick(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!actionDisabled) {
      onAction()
    }
  }

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        aria-label={title}
        aria-selected={selected}
        className={cn(
          "gap-2.5 pr-8 data-active:bg-foreground/10 data-active:hover:bg-foreground/10",
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

function resolveActiveDirectoryItem(
  selection: DirectorySelection | null,
  apps: ContactApp[],
  contacts: ContactUser[],
  groups: ContactGroup[]
): ActiveDirectoryItem | null {
  if (!selection) {
    return null
  }

  if (selection.type === "app") {
    const app = apps.find((item) => item.id === selection.id)
    return app ? { app, type: "app" } : null
  }

  if (selection.type === "group") {
    const group = groups.find((item) => item.id === selection.id)
    return group ? { group, type: "group" } : null
  }

  const contact = contacts.find((item) => item.id === selection.id)
  return contact ? { contact, type: "user" } : null
}

function isDirectorySelection(
  selection: DirectorySelection | null,
  type: DirectorySelection["type"],
  id: string
) {
  return selection?.type === type && selection.id === id
}

function directoryItemKey(type: DirectorySelection["type"], id: string) {
  return `${type}:${id}`
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
