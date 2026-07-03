import * as React from "react"
import { useNavigate } from "react-router"
import { toast } from "sonner"
import {
  Loader2Icon,
  Mail,
  MessageCircle,
  Phone,
  RefreshCw,
  Search,
  UserPen,
  UserRound,
} from "lucide-react"

import { cn } from "@/lib/utils"
import { formatContactPhone } from "@/lib/contact-format"
import { useClientData } from "@/lib/client-data-context"
import type { ContactUser } from "@/lib/client-data-api"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { ScrollArea } from "@/components/ui/scroll-area"

const CONTACT_DETAIL_PANEL_CLASS = "mt-30 w-full max-w-sm"

export function ContactsPage() {
  const {
    contacts,
    contactsRefreshing,
    me,
    openDirectConversation,
    refreshContacts,
  } = useClientData()
  const navigate = useNavigate()
  const [activeContactId, setActiveContactId] = React.useState("")
  const [openingConversationContactId, setOpeningConversationContactId] =
    React.useState("")
  const [keyword, setKeyword] = React.useState("")
  const filteredContacts = React.useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()

    if (!normalizedKeyword) {
      return contacts
    }

    return contacts.filter((contact) =>
      [
        contact.email,
        contact.name,
        contact.nickname,
        contact.phone,
        formatContactPhone(contact.phone),
      ].some((value) => value.toLowerCase().includes(normalizedKeyword))
    )
  }, [contacts, keyword])
  const activeContact =
    filteredContacts.find((contact) => contact.id === activeContactId) ?? null

  async function startDirectConversation(contact: ContactUser) {
    if (contact.id === me.id) {
      return
    }

    setOpeningConversationContactId(contact.id)

    try {
      const conversation = await openDirectConversation(contact.id)
      navigate(`/chat?conversation_id=${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法发起私聊")
    } finally {
      setOpeningConversationContactId((currentContactId) =>
        currentContactId === contact.id ? "" : currentContactId
      )
    }
  }

  return (
    <>
      <aside className="flex w-80 shrink-0 flex-col border-r bg-background">
        <div className="flex h-14 items-center justify-between px-4">
          <h1 className="text-base font-medium">联系人</h1>
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
        <div className="px-4 pb-3">
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              onChange={(event) => setKeyword(event.target.value)}
              placeholder="搜索联系人"
              type="search"
              value={keyword}
            />
          </div>
        </div>
        <ScrollArea className="min-h-0 flex-1">
          <ItemGroup
            aria-label="联系人列表"
            className="gap-1 px-2 pb-3 has-data-[size=sm]:gap-1"
            role="listbox"
          >
            {filteredContacts.map((contact) => (
              <ContactListItem
                key={contact.id}
                contact={contact}
                canStartConversation={contact.id !== me.id}
                size="sm"
                selected={contact.id === activeContact?.id}
                onSelect={() => setActiveContactId(contact.id)}
                onStartConversation={() =>
                  void startDirectConversation(contact)
                }
                startingConversation={
                  contact.id === openingConversationContactId
                }
              />
            ))}
            {filteredContacts.length === 0 && (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                没有匹配的联系人
              </div>
            )}
          </ItemGroup>
        </ScrollArea>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col bg-background">
        <div
          className={cn(
            "flex min-h-0 flex-1 items-start justify-center px-6",
            activeContact ? "bg-background" : "bg-muted"
          )}
          data-testid="contact-detail-shell"
        >
          {activeContact ? (
            <ContactDetailPanel
              contact={activeContact}
              canStartConversation={activeContact.id !== me.id}
              onStartConversation={() =>
                void startDirectConversation(activeContact)
              }
              startingConversation={
                activeContact.id === openingConversationContactId
              }
            />
          ) : (
            <ContactEmptyState />
          )}
        </div>
      </main>
    </>
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

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget) {
      return
    }

    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault()
      onSelect()
    }
  }

  function handleConversationClick(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!canStartConversation) {
      return
    }

    onStartConversation()
  }

  return (
    <Item
      aria-label={title}
      aria-selected={selected}
      className={cn(
        "group/contact-item cursor-pointer px-2 py-1.5",
        selected ? "bg-primary/10 text-foreground" : "hover:bg-muted"
      )}
      onClick={onSelect}
      onKeyDown={handleKeyDown}
      role="option"
      size={size}
      tabIndex={0}
    >
      <ItemMedia>
        <Avatar
          className="rounded-sm bg-muted after:rounded-sm"
          data-size={size}
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
        </Avatar>
      </ItemMedia>
      <ItemContent className="min-w-0">
        <ItemTitle className="w-full truncate">
          <span className="min-w-0 truncate">{title}</span>
          <span
            aria-label={contact.online ? "在线" : "离线"}
            className={cn(
              "size-1.5 shrink-0 rounded-full",
              contact.online ? "bg-emerald-500" : "bg-muted-foreground/30"
            )}
          />
        </ItemTitle>
      </ItemContent>
      <ItemActions
        className={cn(
          "transition-opacity",
          selected
            ? "opacity-100"
            : "pointer-events-none opacity-0 group-focus-within/contact-item:pointer-events-auto group-focus-within/contact-item:opacity-100 group-hover/contact-item:pointer-events-auto group-hover/contact-item:opacity-100"
        )}
      >
        <Button
          aria-label={`与 ${title} 对话`}
          disabled={startingConversation || !canStartConversation}
          onClick={handleConversationClick}
          size="icon-xs"
          type="button"
          variant="ghost"
        >
          {startingConversation ? (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          ) : (
            <MessageCircle />
          )}
        </Button>
      </ItemActions>
    </Item>
  )
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
