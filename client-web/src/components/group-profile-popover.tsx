import * as React from "react"
import { UserRound, UsersRound } from "lucide-react"

import type { ClientConversation } from "@/lib/client-data-api"
import { cn } from "@/lib/utils"
import { AvatarPreviewDialog } from "@/components/avatar-preview-dialog"
import { GroupAvatar } from "@/components/group-avatar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

type GroupProfilePopoverProps = {
  children: React.ReactNode
  conversation: ClientConversation
  triggerClassName?: string
}

export function GroupProfilePopover({
  children,
  conversation,
  triggerClassName,
}: GroupProfilePopoverProps) {
  const [open, setOpen] = React.useState(false)
  const [avatarPreviewOpen, setAvatarPreviewOpen] = React.useState(false)
  const memberCount =
    conversation.memberCount || conversation.members?.length || 0

  function handleAvatarPreview() {
    setOpen(false)
    setAvatarPreviewOpen(true)
  }

  return (
    <>
      <Popover onOpenChange={setOpen} open={open}>
        <PopoverTrigger
          aria-label={`${conversation.name}资料`}
          className={cn(
            "inline-flex cursor-pointer appearance-none rounded-sm border-0 bg-transparent p-0 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
            triggerClassName
          )}
          type="button"
        >
          {children}
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-72"
          side="right"
          sideOffset={8}
        >
          <div className="flex flex-col gap-4">
            <div className="flex items-center gap-3">
              <button
                aria-haspopup="dialog"
                aria-label={`预览${conversation.name}头像`}
                className="shrink-0 cursor-pointer rounded-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                onClick={handleAvatarPreview}
                type="button"
              >
                <GroupAvatar
                  avatar={conversation.avatar}
                  className="size-14"
                  members={conversation.members}
                  name={conversation.name}
                />
              </button>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {conversation.name}
                </div>
                <div className="truncate text-xs text-muted-foreground">
                  群聊资料
                </div>
              </div>
            </div>

            <div className="grid gap-1 text-sm">
              <GroupProfileRow
                icon={<UsersRound className="size-4 text-muted-foreground" />}
                label="类型"
                value="群聊"
              />
              <GroupProfileRow
                icon={<UserRound className="size-4 text-muted-foreground" />}
                label="成员"
                value={`${memberCount} 人群聊`}
              />
            </div>
          </div>
        </PopoverContent>
      </Popover>
      <AvatarPreviewDialog
        label={`${conversation.name}头像预览`}
        onOpenChange={setAvatarPreviewOpen}
        open={avatarPreviewOpen}
      >
        <GroupAvatar
          avatar={conversation.avatar}
          className="size-full"
          members={conversation.members}
          name={conversation.name}
        />
      </AvatarPreviewDialog>
    </>
  )
}

function GroupProfileRow({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode
  label: string
  value: string
}) {
  return (
    <div className="flex items-center gap-3 border-b py-2 last:border-b-0">
      {icon}
      <span className="w-12 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate">{value}</span>
    </div>
  )
}
