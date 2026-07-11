import * as React from "react"
import { useNavigate } from "react-router"
import { Bot, Loader2Icon, UserRound } from "lucide-react"
import { toast } from "sonner"

import { useClientData } from "@/lib/client-data-context"
import { cn } from "@/lib/utils"
import { AvatarPreviewDialog } from "@/components/avatar-preview-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

type AppProfilePopoverProps = {
  appId: string | null
  children: React.ReactNode
  fallbackProfile?: AppProfile | null
  triggerAriaLabel?: string
  triggerClassName?: string
}

type AppProfile = {
  avatar: string
  description: string
  id: string
  name: string
  online: boolean
}

export function AppProfilePopover({
  appId,
  children,
  fallbackProfile = null,
  triggerAriaLabel,
  triggerClassName,
}: AppProfilePopoverProps) {
  const { contactApps, openAppConversation } = useClientData()
  const navigate = useNavigate()
  const [open, setOpen] = React.useState(false)
  const [avatarPreviewOpen, setAvatarPreviewOpen] = React.useState(false)
  const [openingConversation, setOpeningConversation] = React.useState(false)
  const app = React.useMemo(
    () => resolveAppProfile(appId, contactApps, fallbackProfile),
    [appId, contactApps, fallbackProfile]
  )

  if (!app) {
    return <>{children}</>
  }

  const profile = app

  async function handleStartConversation() {
    if (openingConversation) {
      return
    }

    setOpeningConversation(true)

    try {
      const conversation = await openAppConversation(profile.id)
      setOpen(false)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法发起应用会话")
    } finally {
      setOpeningConversation(false)
    }
  }

  function handleAvatarPreview() {
    setOpen(false)
    setAvatarPreviewOpen(true)
  }

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          aria-label={triggerAriaLabel}
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
                aria-label={`预览${profile.name}头像`}
                className="shrink-0 cursor-pointer rounded-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                onClick={handleAvatarPreview}
                type="button"
              >
                <Avatar className="size-14 rounded-sm bg-muted after:rounded-sm">
                  {profile.avatar && (
                    <AvatarImage
                      alt={profile.name}
                      className="rounded-sm"
                      src={profile.avatar}
                    />
                  )}
                  <AvatarFallback className="rounded-sm">
                    <Bot className="size-5" />
                  </AvatarFallback>
                </Avatar>
              </button>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {profile.name}
                </div>
                <div className="truncate text-xs text-muted-foreground">
                  {profile.description || "应用资料"}
                </div>
              </div>
            </div>

            <div className="grid gap-1 text-sm">
              <AppProfileRow
                icon={<Bot className="size-4 text-muted-foreground" />}
                label="类型"
                value="应用"
              />
              <AppProfileRow
                icon={<UserRound className="size-4 text-muted-foreground" />}
                label="状态"
                value={profile.online ? "在线" : "离线"}
              />
            </div>

            <Button
              className="w-full"
              disabled={openingConversation}
              onClick={() => void handleStartConversation()}
              type="button"
            >
              {openingConversation && (
                <Loader2Icon aria-hidden="true" className="animate-spin" />
              )}
              发消息
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      <AvatarPreviewDialog
        label={`${profile.name}头像预览`}
        onOpenChange={setAvatarPreviewOpen}
        open={avatarPreviewOpen}
      >
        <Avatar className="size-full rounded-sm bg-muted after:rounded-sm">
          {profile.avatar && (
            <AvatarImage
              alt={profile.name}
              className="rounded-sm"
              src={profile.avatar}
            />
          )}
          <AvatarFallback className="rounded-sm">
            <Bot className="size-20" />
          </AvatarFallback>
        </Avatar>
      </AvatarPreviewDialog>
    </>
  )
}

function resolveAppProfile(
  appId: string | null,
  apps: AppProfile[],
  fallbackProfile: AppProfile | null
) {
  if (!appId) {
    return null
  }

  return (
    apps.find((app) => app.id === appId) ??
    (fallbackProfile?.id === appId ? fallbackProfile : null)
  )
}

function AppProfileRow({
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
