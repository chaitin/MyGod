import * as React from "react"
import { LoaderCircle } from "lucide-react"
import { toast } from "sonner"

import { MessageReactionPicker } from "@/components/conversation/message-reaction-picker"
import { messageReactionChipToneClassName } from "@/components/conversation/message-reaction-styles"
import { UserProfilePopover } from "@/components/user-profile-popover"
import {
  listConversationMessageReactionUsers,
  type ClientMessageReaction,
  type ClientMessageReactionUser,
} from "@/lib/client-data-api"
import { cn } from "@/lib/utils"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

type MessageReactionChipsProps = {
  align?: "start" | "end"
  canAdd: boolean
  conversationId: string
  enabled?: boolean
  messageId: string
  onSetReaction?: (text: string, reacted: boolean) => Promise<unknown>
  reactions: ClientMessageReaction[]
}

export function MessageReactionChips({
  align = "start",
  canAdd,
  conversationId,
  enabled = true,
  messageId,
  onSetReaction,
  reactions,
}: MessageReactionChipsProps) {
  const [pendingTexts, setPendingTexts] = React.useState<ReadonlySet<string>>(
    new Set()
  )

  if (reactions.length === 0) {
    return null
  }

  async function setReaction(text: string, reacted: boolean) {
    if (!enabled || !onSetReaction || pendingTexts.has(text)) return
    setPendingTexts((current) => new Set(current).add(text))
    try {
      await onSetReaction(text, reacted)
    } catch {
      toast.error("更新表情失败")
    } finally {
      setPendingTexts((current) => {
        const next = new Set(current)
        next.delete(text)
        return next
      })
    }
  }

  return (
    <div
      className="flex max-w-full flex-wrap items-center justify-start gap-1"
      data-slot="message-reactions"
    >
      {reactions.map((reaction) => {
        const canToggle =
          enabled && Boolean(onSetReaction) && (canAdd || reaction.reactedByMe)
        return (
          <div
            className={cn(
              "inline-flex min-h-6 max-w-full items-center gap-1 rounded-md px-2 text-xs whitespace-nowrap transition-colors",
              messageReactionChipToneClassName[align],
              !canToggle && "opacity-80"
            )}
            data-slot="message-reaction-chip"
            key={reaction.text}
          >
            <button
              aria-label={`${reaction.reactedByMe ? "移除" : "添加"}表情 ${reaction.text}`}
              className={cn(
                "shrink-0 cursor-pointer rounded-sm transition-opacity outline-none hover:opacity-70 focus-visible:ring-[3px] focus-visible:ring-ring/50",
                !canToggle && "cursor-default"
              )}
              disabled={!canToggle || pendingTexts.has(reaction.text)}
              onClick={() =>
                void setReaction(reaction.text, !reaction.reactedByMe)
              }
              type="button"
            >
              {reaction.text}
            </button>
            <ReactionParticipantSummary
              conversationId={conversationId}
              messageId={messageId}
              reaction={reaction}
            />
          </div>
        )
      })}
    </div>
  )
}

function ReactionParticipantSummary({
  conversationId,
  messageId,
  reaction,
}: {
  conversationId: string
  messageId: string
  reaction: ClientMessageReaction
}) {
  if (reaction.users.length === 0) {
    return (
      <MessageReactionUsersPopover
        conversationId={conversationId}
        messageId={messageId}
        reaction={reaction}
      />
    )
  }

  const hasMoreUsers = reaction.count > reaction.users.length
  return (
    <span className="inline-flex items-center whitespace-nowrap">
      {reaction.users.map((user, index) => (
        <React.Fragment key={user.id}>
          {index > 0 && <span>,&nbsp;</span>}
          <ReactionUserName user={user} />
        </React.Fragment>
      ))}
      {hasMoreUsers && (
        <>
          <span>等&nbsp;</span>
          <MessageReactionUsersPopover
            conversationId={conversationId}
            messageId={messageId}
            reaction={reaction}
          />
          <span>&nbsp;人</span>
        </>
      )}
    </span>
  )
}

function ReactionUserName({ user }: { user: ClientMessageReactionUser }) {
  return (
    <UserProfilePopover
      triggerAriaLabel={`${user.name}资料`}
      triggerClassName="transition-colors hover:text-sky-500 focus-visible:text-sky-500 data-[state=open]:text-sky-500"
      userId={user.id}
    >
      <span>{user.name}</span>
    </UserProfilePopover>
  )
}

function MessageReactionUsersPopover({
  conversationId,
  messageId,
  reaction,
}: {
  conversationId: string
  messageId: string
  reaction: ClientMessageReaction
}) {
  const [open, setOpen] = React.useState(false)
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState("")
  const [users, setUsers] = React.useState<ClientMessageReactionUser[]>([])
  const requestVersionRef = React.useRef(0)

  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen)
    const requestVersion = ++requestVersionRef.current
    if (!nextOpen) return
    setLoading(true)
    setError("")
    void listConversationMessageReactionUsers(
      conversationId,
      messageId,
      reaction.text
    )
      .then((nextUsers) => {
        if (requestVersionRef.current === requestVersion) setUsers(nextUsers)
      })
      .catch(() => {
        if (requestVersionRef.current === requestVersion) {
          setError("加载参与者失败")
        }
      })
      .finally(() => {
        if (requestVersionRef.current === requestVersion) setLoading(false)
      })
  }

  return (
    <Popover onOpenChange={handleOpenChange} open={open}>
      <PopoverTrigger asChild>
        <button
          aria-label={`查看表情 ${reaction.text} 的 ${reaction.count} 位参与者`}
          className="cursor-pointer rounded-sm font-medium text-sky-500 transition-colors outline-none hover:text-sky-600 focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:text-sky-400 dark:hover:text-sky-300"
          type="button"
        >
          {reaction.count}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        <div className="border-b px-4 py-3 text-sm font-medium">
          {reaction.text} 的参与者（{reaction.count}）
        </div>
        <div className="max-h-72 overflow-y-auto p-2">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" />
              正在加载
            </div>
          ) : error ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              {error}
            </div>
          ) : users.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              暂无参与者
            </div>
          ) : (
            <div className="grid gap-0.5">
              {users.map((user) => (
                <UserProfilePopover
                  key={user.id}
                  triggerAriaLabel={`${user.name}资料`}
                  triggerClassName="w-full rounded-md px-2 py-2 text-sm transition-colors hover:bg-accent hover:text-sky-500 focus-visible:text-sky-500 data-[state=open]:bg-accent data-[state=open]:text-sky-500"
                  userId={user.id}
                >
                  <span className="truncate">{user.name}</span>
                </UserProfilePopover>
              ))}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function MessageReactionAddButton({
  align,
  onSetReaction,
}: {
  align: "start" | "end"
  onSetReaction: (text: string, reacted: boolean) => Promise<unknown>
}) {
  const [pending, setPending] = React.useState(false)

  async function addReaction(text: string) {
    if (pending) return
    setPending(true)
    try {
      await onSetReaction(text, true)
    } catch {
      toast.error("更新表情失败")
    } finally {
      setPending(false)
    }
  }

  return (
    <MessageReactionPicker
      align={align}
      disabled={pending}
      onSelect={(text) => void addReaction(text)}
    />
  )
}
