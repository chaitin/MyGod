import * as React from "react"
import { useLocation, useNavigate, useParams } from "react-router"
import { toast } from "sonner"

import {
  createDirectorySelection,
  directoryItemKey,
  getDirectorySelectionPath,
  resolveActiveDirectoryItem,
  type DirectorySelection,
  type DirectoryTab,
} from "@/components/contacts/contact-directory"
import { AppCredentialsDialog } from "@/components/contacts/app-credentials-dialog"
import { AppProfileDialog } from "@/components/contacts/app-profile-dialog"
import { ContactDirectorySidebar } from "@/components/contacts/contact-directory-sidebar"
import {
  AppDetailPanel,
  ContactDetailPanel,
  ContactEmptyState,
  GroupDetailPanel,
} from "@/components/contacts/contact-detail-panels"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import type {
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/lib/client-data-api"
import {
  deleteClientApp,
  getClientAppCredentials,
  getClientAppProfile,
  type ClientAppCredentials,
  type ClientOwnedApp,
} from "@/lib/client-api/apps"
import { useAppInfo } from "@/lib/app-info-context"
import { useClientData } from "@/lib/client-data-context"
import { formatContactPhone } from "@/lib/contact-format"
import { sortContactsByDisplayName } from "@/lib/contact-sort"
import { cn } from "@/lib/utils"

export function ContactsPage() {
  const { organizationName } = useAppInfo()
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
    refreshConversations,
    restoreConversation,
  } = useClientData()
  const location = useLocation()
  const navigate = useNavigate()
  const { directoryId, directoryType } = useParams<{
    directoryId?: string
    directoryType?: string
  }>()
  const activeSelection = React.useMemo(
    () => createDirectorySelection(directoryType, directoryId),
    [directoryId, directoryType]
  )
  const [openingDirectoryItemKey, setOpeningDirectoryItemKey] =
    React.useState("")
  const [appCredentials, setAppCredentials] =
    React.useState<ClientAppCredentials | null>(null)
  const [appProfile, setAppProfile] = React.useState<ClientOwnedApp | null>(
    null
  )
  const [loadingAccessInfoAppId, setLoadingAccessInfoAppId] = React.useState("")
  const [loadingProfileAppId, setLoadingProfileAppId] = React.useState("")
  const [activeTabsByLocation, setActiveTabsByLocation] = React.useState<
    Record<string, DirectoryTab>
  >({})
  const activeTab =
    activeTabsByLocation[location.key] ?? activeSelection?.type ?? "user"
  const [keywords, setKeywords] = React.useState<Record<DirectoryTab, string>>({
    app: "",
    group: "",
    user: "",
  })
  const activeKeyword = keywords[activeTab]
  const normalizedAppKeyword = keywords.app.trim().toLowerCase()
  const normalizedContactKeyword = keywords.user.trim().toLowerCase()
  const normalizedGroupKeyword = keywords.group.trim().toLowerCase()
  const appGrantUsers = React.useMemo(
    () => sortContactsByDisplayName(contacts),
    [contacts]
  )
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

  async function openAppAccessInfo(app: ContactApp) {
    if (
      app.creatorUserId?.toLowerCase() !== me.id.toLowerCase() ||
      loadingAccessInfoAppId
    ) {
      return
    }

    setLoadingAccessInfoAppId(app.id)
    try {
      setAppCredentials(await getClientAppCredentials(app.id))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载开发指南失败")
    } finally {
      setLoadingAccessInfoAppId("")
    }
  }

  async function openAppProfile(app: ContactApp) {
    if (
      app.creatorUserId?.toLowerCase() !== me.id.toLowerCase() ||
      loadingProfileAppId
    ) {
      return
    }

    setLoadingProfileAppId(app.id)
    try {
      setAppProfile(await getClientAppProfile(app.id))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载应用资料失败")
    } finally {
      setLoadingProfileAppId("")
    }
  }

  async function openOrJoinGroupConversation(group: ContactGroup) {
    const itemKey = directoryItemKey("group", group.id)
    setOpeningDirectoryItemKey(itemKey)

    try {
      const conversation = group.joined
        ? await restoreConversation(group.id)
        : await joinGroupConversation(group.id)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error(group.joined ? "无法打开群聊" : "无法加入群聊")
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

  function updateActiveTab(nextTab: DirectoryTab) {
    setActiveTabsByLocation((currentTabs) => ({
      ...currentTabs,
      [location.key]: nextTab,
    }))
  }

  function selectDirectoryItem(selection: DirectorySelection) {
    navigate(getDirectorySelectionPath(selection))
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
      <ContactDirectorySidebar
        activeKeyword={activeKeyword}
        activeSelection={activeSelection}
        activeTab={activeTab}
        appGrantUsers={appGrantUsers}
        apps={filteredApps}
        contacts={filteredContacts}
        contactsRefreshing={contactsRefreshing}
        currentUserId={me.id}
        groups={filteredGroups}
        organizationName={organizationName}
        onActiveTabChange={updateActiveTab}
        onKeywordChange={updateActiveKeyword}
        onRefresh={() => void refreshContacts().catch(() => undefined)}
        onSelect={selectDirectoryItem}
        onStartAppConversation={(app) => void startAppConversation(app)}
        onStartContactConversation={(contact) =>
          void startDirectConversation(contact)
        }
        onStartGroupConversation={(group) =>
          void openOrJoinGroupConversation(group)
        }
        openingDirectoryItemKey={openingDirectoryItemKey}
      />

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
              developer={getAppDeveloper(activeItem.app, contacts, me)}
              editingProfile={loadingProfileAppId === activeItem.app.id}
              onDelete={
                activeItem.app.creatorUserId?.toLowerCase() ===
                me.id.toLowerCase()
                  ? async () => {
                      await deleteClientApp(activeItem.app.id)
                      navigate("/contacts", { replace: true })
                      await Promise.allSettled([
                        refreshContacts(),
                        refreshConversations(),
                      ])
                    }
                  : undefined
              }
              onEditProfile={
                activeItem.app.creatorUserId?.toLowerCase() ===
                me.id.toLowerCase()
                  ? () => void openAppProfile(activeItem.app)
                  : undefined
              }
              onStartConversation={() =>
                void startAppConversation(activeItem.app)
              }
              onViewAccessInfo={
                activeItem.app.creatorUserId?.toLowerCase() ===
                me.id.toLowerCase()
                  ? () => void openAppAccessInfo(activeItem.app)
                  : undefined
              }
              startingConversation={
                openingDirectoryItemKey ===
                directoryItemKey("app", activeItem.app.id)
              }
              viewingAccessInfo={loadingAccessInfoAppId === activeItem.app.id}
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
      <AppCredentialsDialog
        credentials={appCredentials}
        onCredentialsChange={(credentials) => {
          setAppCredentials(credentials)
          void refreshContacts().catch(() => undefined)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setAppCredentials(null)
          }
        }}
        open={appCredentials !== null}
      />
      <AppProfileDialog
        app={appProfile}
        currentUserId={me.id}
        onAppChange={(app) => {
          setAppProfile(app)
          void refreshContacts().catch(() => undefined)
          void refreshConversations().catch(() => undefined)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setAppProfile(null)
          }
        }}
        open={appProfile !== null}
        users={appGrantUsers}
      />
    </SidebarProvider>
  )
}

function getAppDeveloper(
  app: ContactApp,
  contacts: ContactUser[],
  currentUser: Pick<
    ContactUser,
    "avatar" | "email" | "id" | "name" | "nickname" | "phone"
  >
) {
  if (!app.creatorUserId) {
    return undefined
  }

  const normalizedCreatorId = app.creatorUserId.toLowerCase()
  const developer =
    currentUser.id.toLowerCase() === normalizedCreatorId
      ? currentUser
      : contacts.find(
          (contact) => contact.id.toLowerCase() === normalizedCreatorId
        )

  return developer
}
