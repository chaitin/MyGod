import * as React from "react"

import { useClientData } from "@/lib/client-data-context"
import { AppConversationInfo } from "@/components/app-conversation-info"
import { DirectConversationInfo } from "@/components/direct-conversation-info"
import { GroupConversationInfo } from "@/components/group-conversation-info"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"

type ConversationInfoDrawerProps = {
  children: React.ReactNode
  conversationId: string
}

export function ConversationInfoDrawer({
  children,
  conversationId,
}: ConversationInfoDrawerProps) {
  return (
    <Sheet>
      <SheetTrigger asChild>{children}</SheetTrigger>
      <SheetContent className="gap-0 p-0 sm:max-w-sm" side="right">
        <ConversationInfoContent conversationId={conversationId} />
      </SheetContent>
    </Sheet>
  )
}

function ConversationInfoContent({
  conversationId,
}: {
  conversationId: string
}) {
  const { getConversation, me } = useClientData()
  const conversation = getConversation(conversationId)

  if (!conversation) {
    return (
      <>
        <SheetHeader className="border-b">
          <SheetTitle>会话信息</SheetTitle>
          <SheetDescription>会话</SheetDescription>
        </SheetHeader>
        <div className="px-4 py-6 text-sm text-muted-foreground">
          会话信息不可用
        </div>
      </>
    )
  }

  if (conversation.type === "direct") {
    const otherUserId =
      conversation.members?.find((member) => member.id !== me.id)?.id ?? ""

    if (otherUserId) {
      return <DirectConversationInfo userId={otherUserId} />
    }

    return (
      <>
        <SheetHeader className="border-b">
          <SheetTitle>会话信息</SheetTitle>
          <SheetDescription>私聊</SheetDescription>
        </SheetHeader>
        <div className="px-4 py-6 text-sm text-muted-foreground">
          用户信息不可用
        </div>
      </>
    )
  }

  if (conversation.type === "group") {
    return <GroupConversationInfo conversationId={conversationId} />
  }

  return <AppConversationInfo conversationId={conversationId} />
}
