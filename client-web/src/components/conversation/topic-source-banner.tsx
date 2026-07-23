import * as React from "react"
import { Bot } from "lucide-react"

import { getAvatarInitial } from "@/lib/avatar"
import {
  getConversationTopic,
  type ClientMessageReaction,
  type ClientMessageChoiceState,
  type ClientTopicSourceMessage,
} from "@/lib/client-data-api"
import type { MentionLabelResolver } from "@/lib/message-mentions"
import { isTopicSourceMessageSelectable } from "@/lib/topic-source-message"
import { cn } from "@/lib/utils"
import {
  MessageBodyRenderer,
  MessageChoiceBody,
} from "@/components/conversation/conversation-message"
import {
  MessageReactionAddButton,
  MessageReactionChips,
} from "@/components/conversation/message-reactions"
import {
  MessageActionMenu,
  MessageMoreActionsMenu,
  type MessageActionOptions,
} from "@/components/message-action-menu"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Checkbox } from "@/components/ui/checkbox"

const emptyMentionLabelResolver: MentionLabelResolver = () => undefined

type TopicSourceBannerProps = {
  conversationId?: string
  currentUserId: string
  mentionLabelResolver?: MentionLabelResolver
  onForward?: (message: ClientTopicSourceMessage) => void
  onMultiSelect?: (message: ClientTopicSourceMessage) => void
  onSetReaction?: (text: string, reacted: boolean) => Promise<unknown>
  onRespondToChoice?: (optionIds: string[]) => Promise<void>
  onSourceMessageLoaded?: (message: ClientTopicSourceMessage) => void
  onToggleSelected?: (message: ClientTopicSourceMessage) => void
  reactionConversationId?: string
  reactions?: ClientMessageReaction[]
  selected?: boolean
  selectionMode?: boolean
  sourceMessage?: ClientTopicSourceMessage
  sourceChoice?: ClientMessageChoiceState | null
  sourceChoiceStatus?: "active" | "deleted" | "revoked"
}

export function TopicSourceBanner({
  conversationId,
  currentUserId,
  mentionLabelResolver,
  onForward,
  onMultiSelect,
  onSetReaction,
  onRespondToChoice,
  onSourceMessageLoaded,
  onToggleSelected,
  reactionConversationId,
  reactions = [],
  selected = false,
  selectionMode = false,
  sourceMessage,
  sourceChoice,
  sourceChoiceStatus,
}: TopicSourceBannerProps) {
  const [fetchedSource, setFetchedSource] =
    React.useState<ClientTopicSourceMessage | null>(null)
  const loadedSource = sourceMessage ?? fetchedSource

  React.useEffect(() => {
    if (sourceMessage || !conversationId) return
    let active = true
    void getConversationTopic(conversationId)
      .then((value) => {
        if (active) setFetchedSource(value.sourceMessage)
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [conversationId, sourceMessage])

  React.useEffect(() => {
    if (loadedSource) {
      onSourceMessageLoaded?.(loadedSource)
    }
  }, [loadedSource, onSourceMessageLoaded])

  if (!loadedSource) return null

  const choiceUnavailable =
    loadedSource.body.type === "choice" &&
    (sourceChoiceStatus === "revoked" || sourceChoiceStatus === "deleted")
  const selectable =
    !choiceUnavailable && isTopicSourceMessageSelectable(loadedSource)
  const hasMessageActions = Boolean(onForward || onMultiSelect)
  const canAddReaction = Boolean(
    onSetReaction && !choiceUnavailable && loadedSource.body.type !== "revoked"
  )
  const messageActionOptions: MessageActionOptions = {
    copyDisabled: true,
    onForward: onForward ? () => onForward(loadedSource) : undefined,
    onMultiSelect: onMultiSelect
      ? () => onMultiSelect(loadedSource)
      : undefined,
  }
  const fromCurrentUser =
    loadedSource.sender.type === "user" &&
    loadedSource.sender.id === currentUserId
  const avatar = (
    <Avatar className="size-8 rounded-sm bg-muted after:rounded-sm">
      {loadedSource.sender.avatar && (
        <AvatarImage
          alt={loadedSource.sender.name}
          className="rounded-sm"
          src={loadedSource.sender.avatar}
        />
      )}
      <AvatarFallback
        className={cn(
          "rounded-sm",
          fromCurrentUser && "bg-primary text-primary-foreground"
        )}
      >
        {loadedSource.sender.type === "app" ? (
          <Bot className="size-4" />
        ) : fromCurrentUser ? (
          "我"
        ) : (
          getAvatarInitial(loadedSource.sender.name)
        )}
      </AvatarFallback>
    </Avatar>
  )

  function handleSelectionClick(event: React.MouseEvent<HTMLDivElement>) {
    if (!selectionMode || !selectable || !onToggleSelected) return
    if (
      event.target instanceof Element &&
      event.target.closest("[data-slot=checkbox]")
    ) {
      return
    }
    event.preventDefault()
    event.stopPropagation()
    onToggleSelected(loadedSource!)
  }

  const messageBubble = (
    <div
      className={cn(
        "max-w-full min-w-0 rounded-md p-3 text-sm leading-relaxed shadow-sm",
        fromCurrentUser
          ? "bg-teal-100/60 text-foreground dark:bg-teal-950/80"
          : "bg-zinc-100 text-foreground dark:bg-zinc-800"
      )}
      data-message-action-trigger={
        !selectionMode && selectable && hasMessageActions ? "" : undefined
      }
      data-testid="topic-source-message-bubble"
    >
      {loadedSource.body.type === "choice" ? (
        sourceChoiceStatus === "revoked" || sourceChoiceStatus === "deleted" ? (
          <span className="text-muted-foreground">
            {sourceChoiceStatus === "revoked"
              ? "该消息已被撤回"
              : "该消息已被删除"}
          </span>
        ) : (
          <MessageChoiceBody
            align={fromCurrentUser ? "end" : "start"}
            body={loadedSource.body}
            canRespond={Boolean(onRespondToChoice && sourceChoice)}
            choice={sourceChoice ?? undefined}
            currentUserId={currentUserId}
            mentionLabelResolver={
              mentionLabelResolver ?? emptyMentionLabelResolver
            }
            messageId={loadedSource.id}
            onRespond={onRespondToChoice}
          />
        )
      ) : (
        <MessageBodyRenderer
          body={loadedSource.body}
          currentUserId={currentUserId}
          mentionLabelResolver={
            mentionLabelResolver ?? emptyMentionLabelResolver
          }
        />
      )}
      {!selectionMode && !choiceUnavailable && reactions.length > 0 && (
        <div className="mt-2">
          <MessageReactionChips
            align={fromCurrentUser ? "end" : "start"}
            canAdd={loadedSource.body.type !== "revoked"}
            conversationId={reactionConversationId ?? conversationId ?? ""}
            enabled={loadedSource.body.type !== "revoked"}
            messageId={loadedSource.id}
            onSetReaction={onSetReaction}
            reactions={reactions}
          />
        </div>
      )}
    </div>
  )
  const renderedMessageBubble =
    selectionMode || !selectable || !hasMessageActions ? (
      messageBubble
    ) : (
      <MessageActionMenu {...messageActionOptions}>
        {messageBubble}
      </MessageActionMenu>
    )

  return (
    <div
      className={cn(
        "group/message-row relative rounded-md transition-colors",
        selectionMode && "px-3 py-2 pl-11",
        selected && "bg-primary/5"
      )}
      data-conversation-message-id={loadedSource.id}
      data-message-selection-row
      onClickCapture={handleSelectionClick}
    >
      {selectionMode && (
        <Checkbox
          aria-label={`${selected ? "取消选择" : "选择"}${loadedSource.sender.name}的消息`}
          checked={selected}
          className="absolute top-4 left-3"
          disabled={!selectable}
          onCheckedChange={() => onToggleSelected?.(loadedSource)}
        />
      )}
      <div
        className={cn(
          "flex gap-3",
          fromCurrentUser ? "justify-end" : "justify-start"
        )}
      >
        {!fromCurrentUser && avatar}
        <div
          className={cn(
            "flex max-w-[min(70%,64rem)] flex-col gap-1",
            fromCurrentUser ? "items-end" : "items-start"
          )}
        >
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>{loadedSource.sender.name}</span>
            <span>{formatTopicSourceTime(loadedSource.createdAt)}</span>
          </div>
          <div
            className={cn(
              "flex max-w-full items-end gap-1.5",
              fromCurrentUser && "flex-row-reverse"
            )}
            data-slot="message-bubble-line"
          >
            {renderedMessageBubble}
            {!selectionMode &&
              (canAddReaction || (selectable && hasMessageActions)) && (
                <div
                  className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover/message-row:opacity-100 focus-within:opacity-100 has-[[data-state=open]]:opacity-100"
                  data-slot="message-hover-actions"
                >
                  {canAddReaction && onSetReaction && (
                    <MessageReactionAddButton
                      align={fromCurrentUser ? "end" : "start"}
                      onSetReaction={onSetReaction}
                    />
                  )}
                  {selectable && hasMessageActions && (
                    <MessageMoreActionsMenu
                      {...messageActionOptions}
                      align={fromCurrentUser ? "end" : "start"}
                    />
                  )}
                </div>
              )}
          </div>
        </div>
        {fromCurrentUser && avatar}
      </div>
    </div>
  )
}

function formatTopicSourceTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date)
}
