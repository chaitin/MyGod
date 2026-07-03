import { useState, type FormEvent } from "react"
import { Camera, Loader2Icon, X } from "lucide-react"

import { AvatarPickerDialog } from "@/components/avatar-picker-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { ClientUser } from "@/lib/client-data-api"

type ProfileSettingsDialogProps = {
  onAvatarSave?: (avatar: string) => Promise<void> | void
  onNicknameSave?: (nickname: string) => Promise<void> | void
  onOpenChange: (open: boolean) => void
  open: boolean
  user: ClientUser
}

export function ProfileSettingsDialog({
  onAvatarSave,
  onNicknameSave,
  onOpenChange,
  open,
  user,
}: ProfileSettingsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open && (
        <ProfileSettingsDialogContent
          onAvatarSave={onAvatarSave}
          onNicknameSave={onNicknameSave}
          user={user}
        />
      )}
    </Dialog>
  )
}

function ProfileSettingsDialogContent({
  onAvatarSave,
  onNicknameSave,
  user,
}: {
  onAvatarSave?: (avatar: string) => Promise<void> | void
  onNicknameSave?: (nickname: string) => Promise<void> | void
  user: ClientUser
}) {
  const [avatar, setAvatar] = useState(user.avatar)
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false)
  const [nickname, setNickname] = useState(user.nickname)
  const [nicknameSaving, setNicknameSaving] = useState(false)
  const [savedNickname, setSavedNickname] = useState(user.nickname)
  const displayName = getDisplayName({
    name: user.name,
    nickname,
  })
  const previewAvatar = avatar.trim()
  const trimmedNickname = nickname.trim()
  const nicknameChanged = trimmedNickname !== savedNickname

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void handleNicknameSave()
  }

  async function handleNicknameSave() {
    if (!nicknameChanged || nicknameSaving) {
      return
    }

    setNicknameSaving(true)

    try {
      await onNicknameSave?.(trimmedNickname)
      setNickname(trimmedNickname)
      setSavedNickname(trimmedNickname)
    } finally {
      setNicknameSaving(false)
    }
  }

  return (
    <DialogContent
      showCloseButton={false}
      className="flex w-[calc(100vw-2rem)] max-w-md flex-col gap-5 rounded-md border bg-background p-5 text-foreground shadow-lg ring-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <DialogTitle className="text-base font-medium">设置</DialogTitle>
          <DialogDescription className="sr-only">
            查看个人资料并编辑昵称和头像
          </DialogDescription>
        </div>
        <DialogClose asChild>
          <Button
            aria-label="关闭设置"
            size="icon-sm"
            type="button"
            variant="ghost"
          >
            <X className="size-4" />
          </Button>
        </DialogClose>
      </div>

      <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
        <div
          className="flex items-start gap-4"
          data-testid="profile-settings-identity-row"
        >
          <Button
            aria-haspopup="dialog"
            aria-label="更换头像"
            className="group/avatar-change relative h-auto overflow-hidden rounded-sm bg-muted p-0 hover:bg-background"
            onClick={() => setAvatarPickerOpen(true)}
            type="button"
            variant="ghost"
          >
            <Avatar className="size-17 rounded-sm bg-muted after:rounded-sm">
              {previewAvatar && (
                <AvatarImage
                  alt={displayName}
                  className="rounded-sm"
                  src={previewAvatar}
                />
              )}
              <AvatarFallback className="rounded-sm text-lg">
                {getAvatarInitial(displayName)}
              </AvatarFallback>
            </Avatar>
            <span
              aria-hidden="true"
              className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-sm bg-foreground/40 text-background opacity-0 transition-opacity group-hover/avatar-change:opacity-100 group-focus-visible/avatar-change:opacity-100"
              data-slot="avatar-hover-overlay"
            >
              <Camera className="size-5" />
            </span>
          </Button>
          <Field className="min-w-0 flex-1">
            <FieldLabel htmlFor="profile-settings-nickname">昵称</FieldLabel>
            <div className="flex items-center gap-2">
              <Input
                className="flex-1"
                disabled={nicknameSaving}
                id="profile-settings-nickname"
                onChange={(event) => setNickname(event.target.value)}
                placeholder="输入昵称"
                value={nickname}
              />
              {nicknameChanged && (
                <Button
                  disabled={nicknameSaving}
                  onClick={() => void handleNicknameSave()}
                  type="button"
                >
                  {nicknameSaving && (
                    <Loader2Icon aria-hidden="true" className="animate-spin" />
                  )}
                  提交
                </Button>
              )}
            </div>
          </Field>
        </div>

        <FieldGroup className="gap-4">
          <ReadonlyProfileField
            id="profile-settings-name"
            label="姓名"
            value={user.name}
          />
          <ReadonlyProfileField
            id="profile-settings-email"
            label="邮箱"
            value={user.email}
          />
          <ReadonlyProfileField
            id="profile-settings-phone"
            label="手机号"
            placeholder="未设置"
            value={user.phone}
          />
        </FieldGroup>

        <div className="flex justify-end">
          <DialogClose asChild>
            <Button type="button">关闭</Button>
          </DialogClose>
        </div>
      </form>
      <AvatarPickerDialog
        open={avatarPickerOpen}
        onOpenChange={setAvatarPickerOpen}
        selectedAvatar={avatar}
        onSaveAvatar={async (nextAvatar) => {
          const previousAvatar = avatar

          setAvatar(nextAvatar)

          try {
            await onAvatarSave?.(nextAvatar)
          } catch (error) {
            setAvatar(previousAvatar)
            throw error
          }
        }}
      />
    </DialogContent>
  )
}

function ReadonlyProfileField({
  id,
  label,
  placeholder,
  value,
}: {
  id: string
  label: string
  placeholder?: string
  value: string
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        className="border-muted-foreground/20 bg-muted/50"
        id={id}
        placeholder={placeholder}
        readOnly
        value={value}
      />
    </Field>
  )
}

function getDisplayName(user: { name: string; nickname: string }) {
  return user.nickname.trim() || user.name
}

function getAvatarInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
