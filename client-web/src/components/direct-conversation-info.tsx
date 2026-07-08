import type { ReactNode } from "react"

import { Mail, Phone, UserPen, UserRound } from "lucide-react"

import { formatContactPhone } from "@/lib/contact-format"
import { useClientData } from "@/lib/client-data-context"
import { cn } from "@/lib/utils"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

type DirectConversationInfoProps = {
  userId: string
}

export function DirectConversationInfo({
  userId,
}: DirectConversationInfoProps) {
  const { contacts, me } = useClientData()
  const user =
    me.id === userId ? me : contacts.find((contact) => contact.id === userId)

  if (!user) {
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

  const displayName = getUserDisplayName(user)

  return (
    <>
      <SheetHeader className="border-b">
        <SheetTitle>会话信息</SheetTitle>
        <SheetDescription>私聊</SheetDescription>
      </SheetHeader>
      <div className="flex min-w-0 flex-col gap-5 p-4">
        <div className="flex justify-center">
          <Avatar className="size-20 rounded-sm bg-muted after:rounded-sm">
            {user.avatar && (
              <AvatarImage
                alt={displayName}
                className="rounded-sm"
                src={user.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              {getUserInitial(displayName)}
            </AvatarFallback>
          </Avatar>
        </div>

        <div className="grid min-w-0 gap-1 text-sm">
          <DirectConversationInfoRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="姓名"
            value={user.name}
          />
          <DirectConversationInfoRow
            icon={<UserPen className="size-4 text-muted-foreground" />}
            label="昵称"
            value={user.nickname}
          />
          <DirectConversationInfoRow
            icon={<Mail className="size-4 text-muted-foreground" />}
            label="邮箱"
            value={user.email}
          />
          <DirectConversationInfoRow
            icon={<Phone className="size-4 text-muted-foreground" />}
            label="手机"
            value={user.phone ? formatContactPhone(user.phone) : ""}
          />
        </div>
      </div>
    </>
  )
}

function DirectConversationInfoRow({
  icon,
  label,
  value,
}: {
  icon: ReactNode
  label: string
  value: string
}) {
  const hasValue = Boolean(value.trim())
  const displayValue = hasValue ? value : "未设置"

  return (
    <div className="flex min-w-0 items-center gap-3 border-b py-2 last:border-b-0">
      <span className="shrink-0">{icon}</span>
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span
        className={cn(
          "block min-w-0 flex-1 truncate",
          !hasValue && "text-muted-foreground"
        )}
      >
        {displayValue}
      </span>
    </div>
  )
}

function getUserDisplayName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name || "未命名用户"
}

function getUserInitial(displayName: string) {
  return Array.from(displayName.trim())[0]?.toUpperCase() ?? "?"
}
