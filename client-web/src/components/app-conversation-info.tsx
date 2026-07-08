import type { ReactNode } from "react"

import { Bot, Circle } from "lucide-react"

import { useClientData } from "@/lib/client-data-context"
import { cn } from "@/lib/utils"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

type AppConversationInfoProps = {
  conversationId: string
}

export function AppConversationInfo({
  conversationId,
}: AppConversationInfoProps) {
  const { contactApps, getConversation } = useClientData()
  const conversation = getConversation(conversationId)

  if (!conversation) {
    return (
      <>
        <SheetHeader className="border-b">
          <SheetTitle>会话信息</SheetTitle>
          <SheetDescription>应用</SheetDescription>
        </SheetHeader>
        <div className="px-4 py-6 text-sm text-muted-foreground">
          应用信息不可用
        </div>
      </>
    )
  }

  const app = contactApps.find(
    (candidate) =>
      candidate.id === conversation.id || candidate.name === conversation.name
  )
  const appName = app?.name ?? conversation.name
  const appAvatar = app?.avatar || conversation.avatar
  const appDescription = app?.description.trim() ?? ""
  const appOnline = app?.online ?? false

  return (
    <>
      <SheetHeader className="border-b">
        <SheetTitle>会话信息</SheetTitle>
        <SheetDescription>应用</SheetDescription>
      </SheetHeader>
      <div className="flex min-w-0 flex-col gap-5 p-4">
        <div className="flex flex-col items-center gap-3 text-center">
          <Avatar className="size-20 rounded-sm bg-muted after:rounded-sm">
            {appAvatar && (
              <AvatarImage alt={appName} className="rounded-sm" src={appAvatar} />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              <Bot className="size-7" />
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="truncate text-base font-medium">{appName}</div>
            {appDescription && (
              <div className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                {appDescription}
              </div>
            )}
          </div>
        </div>

        <div className="grid min-w-0 gap-1 text-sm">
          <AppConversationInfoRow
            icon={<Bot className="size-4 text-muted-foreground" />}
            label="类型"
            value="应用"
          />
          <AppConversationInfoRow
            icon={
              <Circle
                className={cn(
                  "size-3 fill-current",
                  appOnline
                    ? "text-emerald-500"
                    : "text-neutral-400 dark:text-neutral-500"
                )}
              />
            }
            label="状态"
            value={appOnline ? "在线" : "离线"}
          />
        </div>
      </div>
    </>
  )
}

function AppConversationInfoRow({
  icon,
  label,
  value,
}: {
  icon: ReactNode
  label: string
  value: string
}) {
  return (
    <div className="flex min-w-0 items-center gap-3 border-b py-2 last:border-b-0">
      <span className="shrink-0">{icon}</span>
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span className="block min-w-0 flex-1 truncate">{value}</span>
    </div>
  )
}
