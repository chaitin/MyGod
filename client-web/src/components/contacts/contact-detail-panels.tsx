import { useId, useState, type ReactNode } from "react"
import {
  Bot,
  Loader2Icon,
  Mail,
  Phone,
  UserPen,
  UserRound,
  UsersRound,
} from "lucide-react"
import { toast } from "sonner"

import { GroupAvatar } from "@/components/group-avatar"
import {
  UserProfilePopoverLink,
  type UserProfile,
} from "@/components/user-profile-popover"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type {
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/lib/client-data-api"
import { formatContactPhone } from "@/lib/contact-format"
import { cn } from "@/lib/utils"

const CONTACT_DETAIL_PANEL_CLASS = "mt-30 w-full max-w-sm"

export function AppDetailPanel({
  app,
  developer,
  editingProfile = false,
  onDelete,
  onEditProfile,
  onStartConversation,
  onViewAccessInfo,
  startingConversation,
  viewingAccessInfo = false,
}: {
  app: ContactApp
  developer?: UserProfile
  editingProfile?: boolean
  onDelete?: () => Promise<void>
  onEditProfile?: () => void
  onStartConversation: () => void
  onViewAccessInfo?: () => void
  startingConversation: boolean
  viewingAccessInfo?: boolean
}) {
  const deleteConfirmationId = useId()
  const [deleteConfirmation, setDeleteConfirmation] = useState("")
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  async function handleDelete() {
    if (!onDelete || deleting || deleteConfirmation !== app.name) {
      return
    }

    setDeleting(true)
    try {
      await onDelete()
      toast.success("应用已删除")
      setDeleteOpen(false)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除应用失败")
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center gap-3 text-center">
          <Avatar
            className="size-20 rounded-sm bg-muted after:rounded-sm"
            data-testid="contact-detail-avatar"
          >
            {app.avatar && (
              <AvatarImage
                alt={app.name}
                className="rounded-sm"
                src={app.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              <Bot className="size-7" />
            </AvatarFallback>
          </Avatar>
          <div>
            <div className="text-base font-medium">{app.name}</div>
            {app.description && (
              <div className="mt-1 text-sm text-muted-foreground">
                {app.description}
              </div>
            )}
          </div>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<Bot className="size-4 text-muted-foreground" />}
            label="类型"
            value="应用"
          />
          {developer && (
            <ContactDetailRow
              icon={<UserRound className="size-4 text-muted-foreground" />}
              label="开发者"
              value={<UserProfilePopoverLink profile={developer} />}
            />
          )}
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="状态"
            value={app.online ? "在线" : "离线"}
          />
        </div>
        <div className="grid gap-2">
          <Button
            className="w-full"
            disabled={startingConversation}
            onClick={onStartConversation}
            type="button"
          >
            {startingConversation && (
              <Loader2Icon aria-hidden="true" className="animate-spin" />
            )}
            发消息
          </Button>
          {onViewAccessInfo && onEditProfile && (
            <div className="grid gap-2">
              <Button
                className="w-full"
                disabled={editingProfile}
                onClick={onEditProfile}
                type="button"
                variant="secondary"
              >
                {editingProfile ? (
                  <Loader2Icon aria-hidden="true" className="animate-spin" />
                ) : null}
                修改资料
              </Button>
              <Button
                className="w-full"
                disabled={viewingAccessInfo}
                onClick={onViewAccessInfo}
                type="button"
                variant="secondary"
              >
                {viewingAccessInfo ? (
                  <Loader2Icon aria-hidden="true" className="animate-spin" />
                ) : null}
                开发指南
              </Button>
              {onDelete && (
                <Button
                  className="w-full"
                  disabled={deleting}
                  onClick={() => setDeleteOpen(true)}
                  type="button"
                  variant="destructive"
                >
                  删除应用
                </Button>
              )}
            </div>
          )}
        </div>
      </div>
      <AlertDialog
        onOpenChange={(open) => {
          if (deleting) {
            return
          }
          setDeleteOpen(open)
          if (!open) {
            setDeleteConfirmation("")
          }
        }}
        open={deleteOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除应用</AlertDialogTitle>
            <AlertDialogDescription>
              删除后应用将无法继续使用，并会退出所有会话；由应用创建的群聊会转交给可用成员，没有可用成员时将被解散。此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={deleteConfirmationId}>
              {`请输入应用名称“${app.name}”以确认删除`}
            </Label>
            <Input
              autoFocus
              disabled={deleting}
              id={deleteConfirmationId}
              onChange={(event) => setDeleteConfirmation(event.target.value)}
              value={deleteConfirmation}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting || deleteConfirmation !== app.name}
              onClick={(event) => {
                event.preventDefault()
                void handleDelete()
              }}
              variant="destructive"
            >
              {deleting && (
                <Loader2Icon aria-hidden="true" className="animate-spin" />
              )}
              删除应用
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

export function GroupDetailPanel({
  group,
  onStartConversation,
  startingConversation,
}: {
  group: ContactGroup
  onStartConversation: () => void
  startingConversation: boolean
}) {
  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center gap-3 text-center">
          <GroupAvatar
            avatar={group.avatar}
            className="size-20"
            members={group.avatarMembers}
            name={group.name}
          />
          <div>
            <div className="text-base font-medium">{group.name}</div>
            <div className="mt-1 text-sm text-muted-foreground">
              {group.memberCount} 人群聊
            </div>
          </div>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<UsersRound className="size-4 text-muted-foreground" />}
            label="类型"
            value="群组"
          />
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="状态"
            value={group.joined ? "已加入" : "未加入"}
          />
        </div>
        <Button
          className="w-full"
          disabled={startingConversation}
          onClick={onStartConversation}
          type="button"
        >
          {startingConversation && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          {group.joined ? "发消息" : "加入群聊"}
        </Button>
      </div>
    </div>
  )
}

export function ContactDetailPanel({
  canStartConversation,
  contact,
  onStartConversation,
  startingConversation,
}: {
  canStartConversation: boolean
  contact: ContactUser
  onStartConversation: () => void
  startingConversation: boolean
}) {
  const displayName = getContactDisplayName(contact)

  return (
    <div
      className={CONTACT_DETAIL_PANEL_CLASS}
      data-testid="contact-detail-panel"
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-col items-center text-center">
          <Avatar
            className="size-20 rounded-sm bg-muted after:rounded-sm"
            data-testid="contact-detail-avatar"
          >
            {contact.avatar && (
              <AvatarImage
                alt={displayName}
                className="rounded-sm"
                src={contact.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm text-xl">
              {getContactInitial(displayName)}
            </AvatarFallback>
          </Avatar>
        </div>

        <div className="grid gap-1 text-sm">
          <ContactDetailRow
            icon={<UserRound className="size-4 text-muted-foreground" />}
            label="姓名"
            value={contact.name}
          />
          <ContactDetailRow
            icon={<UserPen className="size-4 text-muted-foreground" />}
            label="昵称"
            value={contact.nickname}
          />
          <ContactDetailRow
            icon={<Mail className="size-4 text-muted-foreground" />}
            label="邮箱"
            value={contact.email}
          />
          <ContactDetailRow
            icon={<Phone className="size-4 text-muted-foreground" />}
            label="手机"
            value={contact.phone ? formatContactPhone(contact.phone) : ""}
          />
        </div>
        {canStartConversation && (
          <Button
            className="w-full"
            disabled={startingConversation}
            onClick={onStartConversation}
            type="button"
          >
            {startingConversation && (
              <Loader2Icon aria-hidden="true" className="animate-spin" />
            )}
            发消息
          </Button>
        )}
      </div>
    </div>
  )
}

export function ContactEmptyState() {
  return (
    <div
      className="flex flex-1 items-center justify-center self-stretch text-sm text-muted-foreground"
      data-testid="contact-empty-state"
    >
      选择一个联系人查看详情
    </div>
  )
}

function ContactDetailRow({
  icon,
  label,
  value,
}: {
  icon: ReactNode
  label: string
  value: ReactNode
}) {
  const hasValue = typeof value !== "string" || Boolean(value.trim())
  const displayValue = hasValue ? value : "未设置"

  return (
    <div className="flex items-center gap-3 border-b py-2 last:border-b-0">
      {icon}
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span
        className={cn("min-w-0 truncate", !hasValue && "text-muted-foreground")}
      >
        {displayValue}
      </span>
    </div>
  )
}

function getContactDisplayName(contact: { name: string; nickname: string }) {
  return contact.nickname || contact.name
}

function getContactInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
