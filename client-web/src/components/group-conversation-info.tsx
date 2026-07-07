import { useState } from "react"

import { Camera, LogOut, X } from "lucide-react"
import { toast } from "sonner"

import type { ClientConversationMember } from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import {
  CustomAvatarPicker,
  type CroppedAvatar,
} from "@/components/custom-avatar-picker"
import { GroupAvatar } from "@/components/group-avatar"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog"
import { UserProfilePopover } from "@/components/user-profile-popover"
import {
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

type GroupConversationInfoProps = {
  conversationId: string
}

export function GroupConversationInfo({
  conversationId,
}: GroupConversationInfoProps) {
  const { getConversation, me, updateGroupConversationAvatar } = useClientData()
  const conversation = getConversation(conversationId)
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false)
  const [avatarSaving, setAvatarSaving] = useState(false)
  const [draftAvatarOverride, setDraftAvatarOverride] = useState<{
    avatar: string
    baseAvatar: string
    conversationId: string
  } | null>(null)

  if (!conversation) {
    return (
      <>
        <SheetHeader className="border-b">
          <SheetTitle>群聊信息</SheetTitle>
          <SheetDescription>群聊</SheetDescription>
        </SheetHeader>
        <div className="px-4 py-6 text-sm text-muted-foreground">
          会话信息不可用
        </div>
      </>
    )
  }

  const activeConversation = conversation
  const members = [...(activeConversation.members ?? [])].sort(
    compareConversationMembers
  )
  const currentMember = members.find((member) => member.id === me.id)
  const canChangeAvatar = canManageGroupAvatar(currentMember?.role)
  const conversationAvatar = activeConversation.avatar
  const draftAvatar =
    draftAvatarOverride?.conversationId === activeConversation.id &&
    draftAvatarOverride.baseAvatar === conversationAvatar
      ? draftAvatarOverride.avatar
      : conversationAvatar

  async function handleAvatarSave(avatar: CroppedAvatar) {
    if (!canChangeAvatar || avatarSaving) {
      return
    }

    setAvatarSaving(true)
    try {
      const updatedConversation = await updateGroupConversationAvatar(
        activeConversation.id,
        avatar.file
      )
      setDraftAvatarOverride({
        avatar: updatedConversation.avatar,
        baseAvatar: updatedConversation.avatar,
        conversationId: updatedConversation.id,
      })
      setAvatarPickerOpen(false)
      toast.success("群头像已保存")
    } catch (error) {
      toast.error(getErrorMessage(error, "上传群头像失败"))
    } finally {
      setAvatarSaving(false)
    }
  }

  return (
    <>
      <SheetHeader className="border-b">
        <SheetTitle>群聊信息</SheetTitle>
        <SheetDescription>
          {activeConversation.memberCount} 人群聊
        </SheetDescription>
      </SheetHeader>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div className="flex flex-col gap-5">
          <div className="flex justify-center">
            <GroupConversationAvatarControl
              avatar={draftAvatar}
              canChangeAvatar={canChangeAvatar}
              members={members}
              name={activeConversation.name}
              onClick={() => setAvatarPickerOpen(true)}
            />
          </div>

          <div className="grid gap-2">
            <div className="text-sm font-medium">
              群成员（{activeConversation.memberCount}）
            </div>
            <div className="grid gap-1">
              {members.map((member) => (
                <GroupMemberItem key={member.id} member={member} />
              ))}
              {members.length === 0 && (
                <div className="rounded-md border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
                  暂无成员信息
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
      <Dialog
        open={avatarPickerOpen}
        onOpenChange={(open) => {
          if (!avatarSaving) {
            setAvatarPickerOpen(open)
          }
        }}
      >
        <DialogContent
          showCloseButton={false}
          className="flex w-[calc(100vw-2rem)] max-w-2xl flex-col gap-4 rounded-md border bg-background p-5 text-foreground shadow-lg ring-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
        >
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <DialogTitle className="text-base font-medium">
                修改群头像
              </DialogTitle>
              <DialogDescription className="sr-only">
                上传并裁切一张图片作为群聊头像
              </DialogDescription>
            </div>
            <DialogClose asChild>
              <Button
                aria-label="关闭群头像选择"
                disabled={avatarSaving}
                size="icon-sm"
                type="button"
                variant="ghost"
              >
                <X className="size-4" />
              </Button>
            </DialogClose>
          </div>
          <CustomAvatarPicker onSave={handleAvatarSave} saving={avatarSaving} />
        </DialogContent>
      </Dialog>
      <SheetFooter className="border-t">
        <Button type="button" variant="destructive">
          <LogOut aria-hidden="true" className="size-4" />
          退出群聊
        </Button>
      </SheetFooter>
    </>
  )
}

function GroupConversationAvatarControl({
  avatar,
  canChangeAvatar,
  members,
  name,
  onClick,
}: {
  avatar: string
  canChangeAvatar: boolean
  members: ClientConversationMember[]
  name: string
  onClick: () => void
}) {
  const avatarNode = (
    <GroupAvatar
      avatar={avatar}
      className="size-20"
      members={members}
      name={name}
    />
  )

  if (!canChangeAvatar) {
    return avatarNode
  }

  return (
    <Button
      aria-haspopup="dialog"
      aria-label="更换群头像"
      className="group/group-avatar-change relative h-auto overflow-hidden rounded-sm bg-muted p-0 hover:bg-background"
      onClick={onClick}
      type="button"
      variant="ghost"
    >
      {avatarNode}
      <span
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-sm bg-foreground/40 text-background opacity-0 transition-opacity group-hover/group-avatar-change:opacity-100 group-focus-visible/group-avatar-change:opacity-100"
      >
        <Camera className="size-5" />
      </span>
    </Button>
  )
}

function GroupMemberItem({ member }: { member: ClientConversationMember }) {
  return (
    <UserProfilePopover
      fallbackProfile={member}
      triggerClassName="flex w-full min-w-0 items-center gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
      userId={member.id}
    >
      <GroupMemberItemContent member={member} />
    </UserProfilePopover>
  )
}

function GroupMemberItemContent({
  member,
}: {
  member: ClientConversationMember
}) {
  const displayName = getMemberDisplayName(member)

  return (
    <>
      <Avatar className="size-8 rounded-sm bg-muted after:rounded-sm">
        {member.avatar && (
          <AvatarImage
            alt={displayName}
            className="rounded-sm"
            src={member.avatar}
          />
        )}
        <AvatarFallback className="rounded-sm">
          {getInitial(displayName)}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="truncate">{displayName}</div>
        <div className="truncate text-xs text-muted-foreground">
          {getMemberRoleLabel(member.role)}
        </div>
      </div>
    </>
  )
}

function getMemberDisplayName(
  member: Pick<ClientConversationMember, "name" | "nickname">
) {
  return member.nickname.trim() || member.name.trim()
}

function getMemberRoleLabel(role: ClientConversationMember["role"]) {
  if (role === "owner") {
    return "群主"
  }
  if (role === "admin") {
    return "管理员"
  }

  return "成员"
}

const memberRoleOrder: Record<ClientConversationMember["role"], number> = {
  owner: 0,
  admin: 1,
  member: 2,
}

function compareConversationMembers(
  left: ClientConversationMember,
  right: ClientConversationMember
) {
  return memberRoleOrder[left.role] - memberRoleOrder[right.role]
}

function canManageGroupAvatar(
  role: ClientConversationMember["role"] | undefined
) {
  return role === "owner" || role === "admin"
}

function getErrorMessage(error: unknown, fallbackMessage: string) {
  return error instanceof Error ? error.message : fallbackMessage
}

function getInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
