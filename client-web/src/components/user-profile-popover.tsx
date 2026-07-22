import * as React from "react"
import { useNavigate } from "react-router"
import { Loader2Icon, Mail, Phone, UserPen, UserRound } from "lucide-react"
import { toast } from "sonner"

import { formatContactPhone } from "@/lib/contact-format"
import { useClientData } from "@/lib/client-data-context"
import {
  type ClientProfileContextValue,
  useClientCurrentUserId,
  useClientUserProfile,
  useOptionalClientProfileContext,
} from "@/lib/client-profile-context"
import { cn } from "@/lib/utils"
import { AvatarPreviewDialog } from "@/components/avatar-preview-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

type UserProfilePopoverProps = {
  children: React.ReactNode
  fallbackProfile?: UserProfile | null
  triggerAriaLabel?: string
  triggerClassName?: string
  userId: string | null
}

export type UserProfile = {
  avatar: string
  email: string
  id: string
  name: string
  nickname: string
  phone: string
}

export function UserProfilePopoverLink({
  profile,
  triggerClassName,
}: {
  profile: UserProfile
  triggerClassName?: string
}) {
  const displayName = getUserDisplayName(profile)

  return (
    <UserProfilePopover
      fallbackProfile={profile}
      triggerAriaLabel={`${displayName}资料`}
      triggerClassName={cn(
        "max-w-full truncate transition-colors hover:text-sky-500 focus-visible:text-sky-500 data-[state=open]:text-sky-500",
        triggerClassName
      )}
      userId={profile.id}
    >
      <span className="truncate">{displayName}</span>
    </UserProfilePopover>
  )
}

export function UserProfilePopover(props: UserProfilePopoverProps) {
  const profileContext = useOptionalClientProfileContext()

  return profileContext ? (
    <StoredUserProfilePopover
      {...props}
      openDirectConversation={profileContext.openDirectConversation}
    />
  ) : (
    <LegacyUserProfilePopover {...props} />
  )
}

function StoredUserProfilePopover({
  fallbackProfile = null,
  openDirectConversation,
  userId,
  ...props
}: UserProfilePopoverProps & {
  openDirectConversation: ClientProfileContextValue["openDirectConversation"]
}) {
  const currentUserId = useClientCurrentUserId()
  const storedProfile = useClientUserProfile(userId)
  const profile =
    storedProfile ?? (fallbackProfile?.id === userId ? fallbackProfile : null)

  return (
    <UserProfilePopoverContent
      {...props}
      currentUserId={currentUserId}
      openDirectConversation={openDirectConversation}
      profile={profile}
    />
  )
}

function LegacyUserProfilePopover(props: UserProfilePopoverProps) {
  const { contacts, me, openDirectConversation } = useClientData()
  const profile = resolveUserProfile(
    props.userId,
    me,
    contacts,
    props.fallbackProfile ?? null
  )

  return (
    <UserProfilePopoverContent
      {...props}
      currentUserId={me.id}
      openDirectConversation={openDirectConversation}
      profile={profile}
    />
  )
}

function UserProfilePopoverContent({
  children,
  currentUserId,
  openDirectConversation,
  profile,
  triggerAriaLabel,
  triggerClassName,
}: Omit<UserProfilePopoverProps, "fallbackProfile" | "userId"> & {
  currentUserId: string
  openDirectConversation: ClientProfileContextValue["openDirectConversation"]
  profile: UserProfile | null
}) {
  const navigate = useNavigate()
  const [open, setOpen] = React.useState(false)
  const [avatarPreviewOpen, setAvatarPreviewOpen] = React.useState(false)
  const [openingConversation, setOpeningConversation] = React.useState(false)

  if (!profile) {
    return <>{children}</>
  }

  const currentProfile = profile
  const displayName = getUserDisplayName(currentProfile)
  const canStartConversation = currentProfile.id !== currentUserId

  async function handleStartConversation() {
    if (!canStartConversation || openingConversation) {
      return
    }

    setOpeningConversation(true)

    try {
      const conversation = await openDirectConversation(currentProfile.id)
      setOpen(false)
      navigate(`/chat/${encodeURIComponent(conversation.id)}`)
    } catch {
      toast.error("无法发起私聊")
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
                aria-label={`预览${displayName}头像`}
                className="shrink-0 cursor-pointer rounded-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                onClick={handleAvatarPreview}
                type="button"
              >
                <Avatar className="size-14 rounded-sm bg-muted after:rounded-sm">
                  {currentProfile.avatar && (
                    <AvatarImage
                      alt={displayName}
                      className="rounded-sm"
                      src={currentProfile.avatar}
                    />
                  )}
                  <AvatarFallback className="rounded-sm text-lg">
                    {getUserInitial(displayName)}
                  </AvatarFallback>
                </Avatar>
              </button>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {displayName}
                </div>
                <div className="truncate text-xs text-muted-foreground">
                  用户资料
                </div>
              </div>
            </div>

            <div className="grid gap-1 text-sm">
              <UserProfileRow
                icon={<UserRound className="size-4 text-muted-foreground" />}
                label="姓名"
                value={currentProfile.name}
              />
              <UserProfileRow
                icon={<UserPen className="size-4 text-muted-foreground" />}
                label="昵称"
                value={currentProfile.nickname}
              />
              <UserProfileRow
                icon={<Mail className="size-4 text-muted-foreground" />}
                label="邮箱"
                value={currentProfile.email}
              />
              <UserProfileRow
                icon={<Phone className="size-4 text-muted-foreground" />}
                label="手机"
                value={
                  currentProfile.phone
                    ? formatContactPhone(currentProfile.phone)
                    : ""
                }
              />
            </div>

            <Button
              className="w-full"
              disabled={!canStartConversation || openingConversation}
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
        label={`${displayName}头像预览`}
        onOpenChange={setAvatarPreviewOpen}
        open={avatarPreviewOpen}
      >
        <Avatar className="size-full rounded-sm bg-muted after:rounded-sm">
          {currentProfile.avatar && (
            <AvatarImage
              alt={displayName}
              className="rounded-sm"
              src={currentProfile.avatar}
            />
          )}
          <AvatarFallback className="rounded-sm text-6xl">
            {getUserInitial(displayName)}
          </AvatarFallback>
        </Avatar>
      </AvatarPreviewDialog>
    </>
  )
}

function resolveUserProfile(
  userId: string | null,
  me: UserProfile,
  contacts: UserProfile[],
  fallbackProfile: UserProfile | null
) {
  if (!userId) {
    return null
  }

  if (me.id === userId) {
    return me
  }

  return (
    contacts.find((contact) => contact.id === userId) ??
    (fallbackProfile?.id === userId ? fallbackProfile : null)
  )
}

function UserProfileRow({
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
      <span className="w-12 shrink-0 text-muted-foreground">{label}</span>
      <span
        className={cn("min-w-0 truncate", !hasValue && "text-muted-foreground")}
      >
        {displayValue}
      </span>
    </div>
  )
}

function getUserDisplayName(user: Pick<UserProfile, "name" | "nickname">) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name || "未命名用户"
}

function getUserInitial(displayName: string) {
  return Array.from(displayName.trim())[0]?.toUpperCase() ?? "?"
}
