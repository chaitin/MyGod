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
import { useClientData } from "@/lib/client-data-context"
import { formatContactPhone } from "@/lib/contact-format"
import { sortContactsByDisplayName } from "@/lib/contact-sort"
import { cn } from "@/lib/utils"

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
        apps={filteredApps}
        contacts={filteredContacts}
        contactsRefreshing={contactsRefreshing}
        currentUserId={me.id}
        groups={filteredGroups}
        onActiveTabChange={updateActiveTab}
        onKeywordChange={updateActiveKeyword}
        onRefresh={() => void refreshContacts()}
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
