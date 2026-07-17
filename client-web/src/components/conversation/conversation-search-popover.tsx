import * as React from "react"
import { Bot, Search } from "lucide-react"

import { GroupAvatar } from "@/components/group-avatar"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { SidebarInput } from "@/components/ui/sidebar"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  createConversationSearchIndex,
  searchConversationIndex,
  type ConversationSearchEntry,
  type ConversationSearchField,
  type ConversationSearchResult,
} from "@/lib/conversation-search"
import { cn } from "@/lib/utils"

export function ConversationSearchPopover({
  conversations,
  currentUserId,
  getConversationDescription,
  onSelectConversation,
}: {
  conversations: ClientConversation[]
  currentUserId: string
  getConversationDescription: (conversation: ClientConversation) => string
  onSelectConversation: (conversationId: string) => void
}) {
  const [keyword, setKeyword] = React.useState("")
  const [open, setOpen] = React.useState(false)
  const [selectedIndex, setSelectedIndex] = React.useState(0)
  const anchorRef = React.useRef<HTMLDivElement>(null)
  const optionRefs = React.useRef<Array<HTMLButtonElement | null>>([])
  const searchIndexRef = React.useRef<ConversationSearchEntry[]>([])
  const listboxId = React.useId()
  const searchIndex = React.useMemo(() => {
    const nextIndex = createConversationSearchIndex(
      conversations,
      currentUserId,
      searchIndexRef.current
    )
    searchIndexRef.current = nextIndex
    return nextIndex
  }, [conversations, currentUserId])
  const results = React.useMemo(
    () => searchConversationIndex(searchIndex, keyword),
    [keyword, searchIndex]
  )
  const visibleSelectedIndex = getVisibleSelectedIndex(
    selectedIndex,
    results.length
  )

  React.useEffect(() => {
    if (!open || results.length === 0) {
      return
    }

    optionRefs.current[visibleSelectedIndex]?.scrollIntoView?.({
      block: "nearest",
    })
  }, [open, results.length, visibleSelectedIndex])

  function clearSearch() {
    setKeyword("")
    setSelectedIndex(0)
  }

  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen)
    if (!nextOpen) {
      clearSearch()
    }
  }

  function handleKeywordChange(event: React.ChangeEvent<HTMLInputElement>) {
    setKeyword(event.target.value)
    setSelectedIndex(0)
    setOpen(true)
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.nativeEvent.isComposing || event.keyCode === 229) {
      return
    }

    if (event.key === "Escape" && open) {
      event.preventDefault()
      event.stopPropagation()
      handleOpenChange(false)
      return
    }

    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const direction = event.key
      event.preventDefault()
      setOpen(true)

      if (results.length === 0) {
        return
      }

      setSelectedIndex((current) =>
        getNextSelectedIndex(current, results.length, direction)
      )
      return
    }

    if (event.key === "Enter" && open) {
      const result = results[visibleSelectedIndex]
      if (!result) {
        return
      }

      event.preventDefault()
      selectConversation(result.conversation.id)
    }
  }

  function selectConversation(conversationId: string) {
    onSelectConversation(conversationId)
    handleOpenChange(false)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverAnchor asChild>
        <div className="relative" ref={anchorRef}>
          <Search className="pointer-events-none absolute top-1/2 left-2.5 z-10 size-4 -translate-y-1/2 text-muted-foreground" />
          <SidebarInput
            aria-activedescendant={
              open && results.length > 0
                ? getOptionId(listboxId, visibleSelectedIndex)
                : undefined
            }
            aria-autocomplete="list"
            aria-controls={listboxId}
            aria-expanded={open}
            aria-label="搜索消息"
            className="pl-8"
            onChange={handleKeywordChange}
            onClick={() => setOpen(true)}
            onFocus={() => setOpen(true)}
            onKeyDown={handleKeyDown}
            placeholder="搜索"
            role="combobox"
            type="search"
            value={keyword}
          />
        </div>
      </PopoverAnchor>
      <PopoverContent
        align="start"
        className="max-h-80 w-[var(--radix-popover-trigger-width)] overflow-y-auto p-1"
        onInteractOutside={(event) => {
          if (
            event.target instanceof Node &&
            anchorRef.current?.contains(event.target)
          ) {
            event.preventDefault()
          }
        }}
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <div aria-label="搜索会话结果" id={listboxId} role="listbox">
          {results.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-muted-foreground">
              未找到相关会话
            </p>
          ) : (
            results.map((result, index) => {
              const selected = index === visibleSelectedIndex

              return (
                <button
                  aria-selected={selected}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-sm px-2 py-2 text-left outline-none",
                    selected && "bg-accent text-accent-foreground"
                  )}
                  id={getOptionId(listboxId, index)}
                  key={result.conversation.id}
                  onClick={() => selectConversation(result.conversation.id)}
                  onMouseDown={(event) => event.preventDefault()}
                  onMouseEnter={() => setSelectedIndex(index)}
                  ref={(element) => {
                    optionRefs.current[index] = element
                  }}
                  role="option"
                  tabIndex={-1}
                  type="button"
                >
                  <ConversationSearchResultAvatar
                    conversation={result.conversation}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {result.conversation.name}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {getSearchResultDescription(
                        result,
                        keyword,
                        getConversationDescription
                      )}
                    </span>
                  </span>
                </button>
              )
            })
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function ConversationSearchResultAvatar({
  conversation,
}: {
  conversation: ClientConversation
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
      <AvatarFallback className="rounded-sm text-xs">
        {conversation.type === "app" ? (
          <Bot className="size-4" />
        ) : (
          (Array.from(conversation.name.trim())[0]?.toUpperCase() ?? "?")
        )}
      </AvatarFallback>
    </Avatar>
  )
}

function getSearchResultDescription(
  result: ConversationSearchResult,
  keyword: string,
  getConversationDescription: (conversation: ClientConversation) => string
) {
  if (!keyword.trim()) {
    return getConversationDescription(result.conversation)
  }

  const field = result.matchedField
  if (!field || field.kind === "conversation_name") {
    return "匹配会话名称"
  }

  const displayName = field.memberDisplayName
  const value = field.rawValue
  return displayName && displayName !== value
    ? `${getMatchLabel(field)}：${displayName} · ${value}`
    : `${getMatchLabel(field)}：${value}`
}

function getMatchLabel(field: ConversationSearchField) {
  if (field.kind === "member_email") {
    return "匹配邮箱"
  }
  if (field.kind === "member_phone") {
    return "匹配手机号"
  }
  if (field.kind === "app_name") {
    return "匹配应用成员"
  }
  return "匹配成员"
}

function getVisibleSelectedIndex(index: number, length: number) {
  if (length === 0) {
    return 0
  }
  return Math.min(index, length - 1)
}

function getNextSelectedIndex(
  current: number,
  length: number,
  key: "ArrowDown" | "ArrowUp"
) {
  const visibleCurrent = getVisibleSelectedIndex(current, length)

  if (key === "ArrowDown") {
    return (visibleCurrent + 1) % length
  }
  return (visibleCurrent - 1 + length) % length
}

function getOptionId(listboxId: string, index: number) {
  return `${listboxId}-option-${index}`
}
