import * as React from "react"
import {
  Camera,
  Ellipsis,
  Link2,
  Loader2,
  Pencil,
  Trash2,
  X,
} from "lucide-react"
import { toast } from "sonner"

import {
  CustomAvatarPicker,
  type CroppedAvatar,
} from "@/components/custom-avatar-picker"
import { ProjectAvatar } from "@/components/projects/project-avatar"
import { ProjectGroupAssociationsDialog } from "@/components/projects/project-group-associations-dialog"
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
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"
import {
  deleteClientProject,
  type ClientProjectDetail,
  updateClientProject,
  uploadClientProjectAvatar,
} from "@/lib/project-data-api"

export function ProjectSettingsMenu({
  groups,
  onProjectDeleted,
  onProjectUpdated,
  onRelationsChanged,
  project,
  user,
}: {
  groups: ClientConversation[]
  onProjectDeleted: () => Promise<void>
  onProjectUpdated: () => Promise<void>
  onRelationsChanged: () => Promise<void>
  project: ClientProjectDetail
  user: ClientUser
}) {
  const [deleteOpen, setDeleteOpen] = React.useState(false)
  const [deleting, setDeleting] = React.useState(false)
  const [editOpen, setEditOpen] = React.useState(false)
  const [groupsOpen, setGroupsOpen] = React.useState(false)
  const canManage = project.currentUserRole === "owner"

  async function handleDelete() {
    if (!canManage || project.isPersonal || deleting) {
      return
    }

    setDeleting(true)
    try {
      await deleteClientProject(project.id)
      setDeleteOpen(false)
      toast.success("项目已删除")
      await onProjectDeleted()
    } catch (error) {
      toast.error(getErrorMessage(error, "删除项目失败"))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label="项目设置"
            size="icon-sm"
            title="项目设置"
            type="button"
            variant="ghost"
          >
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-40">
          <DropdownMenuItem
            disabled={!canManage}
            onSelect={() => setEditOpen(true)}
          >
            <Pencil />
            修改信息
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={!canManage || project.isPersonal}
            onSelect={() => setGroupsOpen(true)}
          >
            <Link2 />
            授权群组
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={!canManage || project.isPersonal}
            onSelect={() => setDeleteOpen(true)}
            variant="destructive"
          >
            <Trash2 />
            删除项目
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <EditProjectDialog
        onOpenChange={setEditOpen}
        onProjectUpdated={onProjectUpdated}
        open={editOpen}
        project={project}
        user={user}
      />
      {!project.isPersonal && (
        <ProjectGroupAssociationsDialog
          groups={groups}
          onOpenChange={setGroupsOpen}
          onRelationsChanged={onRelationsChanged}
          open={groupsOpen}
          project={project}
        />
      )}

      <AlertDialog
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!deleting) {
            setDeleteOpen(open)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除项目</AlertDialogTitle>
            <AlertDialogDescription>
              {`确定删除“${project.name}”吗？项目内的任务也将一并删除，此操作无法撤销。`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={!canManage || project.isPersonal || deleting}
              onClick={(event) => {
                event.preventDefault()
                void handleDelete()
              }}
              variant="destructive"
            >
              {deleting && <Loader2 className="animate-spin" />}
              删除项目
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function EditProjectDialog({
  onOpenChange,
  onProjectUpdated,
  open,
  project,
  user,
}: {
  onOpenChange: (open: boolean) => void
  onProjectUpdated: () => Promise<void>
  open: boolean
  project: ClientProjectDetail
  user: ClientUser
}) {
  const [avatarPickerOpen, setAvatarPickerOpen] = React.useState(false)
  const [description, setDescription] = React.useState(project.description)
  const [name, setName] = React.useState(project.name)
  const [pendingAvatar, setPendingAvatar] =
    React.useState<CroppedAvatar | null>(null)
  const [saving, setSaving] = React.useState(false)
  const trimmedName = name.trim()
  const canSave = !saving && (project.isPersonal || trimmedName.length > 0)

  function handleOpenChange(nextOpen: boolean) {
    if (saving) {
      return
    }
    if (nextOpen) {
      setDescription(project.description)
      setName(project.name)
      setPendingAvatar(null)
    }
    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSave) {
      return
    }

    setSaving(true)
    try {
      await updateClientProject(project.id, {
        description,
        ...(project.isPersonal ? {} : { name: trimmedName }),
      })
      if (pendingAvatar && !project.isPersonal) {
        await uploadClientProjectAvatar(project.id, pendingAvatar.file)
      }
      onOpenChange(false)
      toast.success("项目信息已保存")
      await onProjectUpdated()
    } catch (error) {
      toast.error(getErrorMessage(error, "保存项目信息失败"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="gap-5 sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>修改项目信息</DialogTitle>
            <DialogDescription>修改项目头像、名称和描述。</DialogDescription>
          </DialogHeader>
          <form className="grid gap-5" onSubmit={handleSubmit}>
            <div className="flex items-start gap-4">
              <Button
                aria-label="修改项目头像"
                className="group/avatar-change relative h-auto overflow-hidden rounded-md bg-muted p-0 hover:bg-muted"
                disabled={saving || project.isPersonal}
                onClick={() => setAvatarPickerOpen(true)}
                type="button"
                variant="ghost"
              >
                <ProjectAvatar
                  avatarOverride={pendingAvatar?.previewUrl}
                  className="size-16"
                  project={project}
                  user={user}
                />
                {!project.isPersonal && (
                  <span className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center bg-foreground/40 text-background opacity-0 transition-opacity group-hover/avatar-change:opacity-100 group-focus-visible/avatar-change:opacity-100">
                    <Camera className="size-5" />
                  </span>
                )}
              </Button>
              <div className="grid min-w-0 flex-1 gap-2">
                <Label htmlFor={`edit-project-name-${project.id}`}>
                  项目名称
                </Label>
                <Input
                  disabled={saving || project.isPersonal}
                  id={`edit-project-name-${project.id}`}
                  maxLength={120}
                  onChange={(event) => setName(event.target.value)}
                  value={name}
                />
                {!project.isPersonal && name.length > 0 && !trimmedName && (
                  <p className="text-xs text-destructive">项目名称不能为空</p>
                )}
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor={`edit-project-description-${project.id}`}>
                项目描述
              </Label>
              <Textarea
                className="min-h-28 resize-none"
                disabled={saving}
                id={`edit-project-description-${project.id}`}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="暂无说明"
                value={description}
              />
            </div>
            <DialogFooter>
              <Button
                disabled={saving}
                onClick={() => handleOpenChange(false)}
                type="button"
                variant="outline"
              >
                取消
              </Button>
              <Button disabled={!canSave} type="submit">
                {saving && <Loader2 className="animate-spin" />}
                保存
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={avatarPickerOpen} onOpenChange={setAvatarPickerOpen}>
        <DialogContent
          showCloseButton={false}
          className="flex w-[calc(100vw-2rem)] max-w-2xl flex-col gap-4 rounded-md border bg-background p-5 text-foreground shadow-lg ring-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
        >
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <DialogTitle className="text-base font-medium">
                上传项目头像
              </DialogTitle>
              <DialogDescription className="sr-only">
                上传并裁切一张图片作为项目头像
              </DialogDescription>
            </div>
            <DialogClose asChild>
              <Button
                aria-label="关闭项目头像选择"
                size="icon-sm"
                type="button"
                variant="ghost"
              >
                <X className="size-4" />
              </Button>
            </DialogClose>
          </div>
          <CustomAvatarPicker
            onSave={(avatar) => {
              setPendingAvatar(avatar)
              setAvatarPickerOpen(false)
            }}
            saving={false}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}

function getErrorMessage(error: unknown, fallbackMessage: string) {
  return error instanceof Error ? error.message : fallbackMessage
}
