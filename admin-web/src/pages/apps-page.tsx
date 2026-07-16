import {
  BanIcon,
  CheckIcon,
  CopyIcon,
  LinkIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  SaveIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useId, useState, type FormEvent } from "react"
import { toast } from "sonner"

import { useProductInfo } from "@/components/product-info-provider"
import {
  AdminAppsRequestError,
  createAdminApp,
  deleteAdminApp,
  disableAdminApp,
  enableAdminApp,
  type AdminApp,
  type AdminAppInput,
  type AdminAppVisibility,
  listAdminApps,
  regenerateAdminAppSecret,
  updateAdminApp,
  uploadAdminAppAvatar,
} from "@/lib/admin-apps"
import {
  CustomAvatarPicker,
  type CroppedAvatar,
} from "@/components/custom-avatar-picker"
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
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"

type AdminAppForm = {
  avatar: string
  description: string
  name: string
  visibility: AdminAppVisibility
}

export default function AppsPage() {
  const [apps, setApps] = useState<AdminApp[]>([])
  const [deleteConfirmationApp, setDeleteConfirmationApp] =
    useState<AdminApp | null>(null)
  const [connectionApp, setConnectionApp] = useState<AdminApp | null>(null)
  const [editingApp, setEditingApp] = useState<AdminApp | null>(null)
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [updatingAppId, setUpdatingAppId] = useState<string | null>(null)
  const isDeletingApp =
    deleteConfirmationApp !== null && updatingAppId === deleteConfirmationApp.id

  useEffect(() => {
    let ignore = false

    async function loadApps() {
      setIsLoading(true)

      try {
        const loadedApps = await listAdminApps()

        if (ignore) {
          return
        }

        setApps(sortAdminAppsForDisplay(loadedApps))
      } catch (error) {
        if (ignore) {
          return
        }

        toast.error(
          error instanceof AdminAppsRequestError
            ? error.message
            : "加载应用失败"
        )
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void loadApps()

    return () => {
      ignore = true
    }
  }, [])

  function handleDialogOpenChange(open: boolean) {
    setIsDialogOpen(open)
    if (!open) {
      setEditingApp(null)
    }
  }

  function openEditForm(app: AdminApp) {
    setEditingApp(app)
    setIsDialogOpen(true)
  }

  function openDeleteConfirmation(app: AdminApp) {
    setDeleteConfirmationApp(app)
  }

  function handleDeleteConfirmationOpenChange(open: boolean) {
    if (!open && !isDeletingApp) {
      setDeleteConfirmationApp(null)
    }
  }

  function handleAppSaved(app: AdminApp) {
    setApps((currentApps) =>
      sortAdminAppsForDisplay([
        app,
        ...currentApps.filter((currentApp) => currentApp.id !== app.id),
      ])
    )
    handleDialogOpenChange(false)
  }

  async function handleAppStatusChange(app: AdminApp, enabled: boolean) {
    setUpdatingAppId(app.id)

    try {
      const updatedApp = enabled
        ? await enableAdminApp(app.id)
        : await disableAdminApp(app.id)

      upsertApp(updatedApp)
      toast.success(enabled ? "应用已启用" : "应用已禁用")
    } catch (error) {
      toast.error(
        error instanceof AdminAppsRequestError
          ? error.message
          : "更新应用状态失败"
      )
    } finally {
      setUpdatingAppId(null)
    }
  }

  async function handleAppSecretRegenerate(app: AdminApp) {
    setUpdatingAppId(app.id)

    try {
      const updatedApp = await regenerateAdminAppSecret(app.id)

      upsertApp(updatedApp)
      toast.success("应用密钥已生成")
    } catch (error) {
      toast.error(
        error instanceof AdminAppsRequestError ? error.message : "生成密钥失败"
      )
    } finally {
      setUpdatingAppId(null)
    }
  }

  async function handleAppDelete(app: AdminApp) {
    setUpdatingAppId(app.id)

    try {
      await deleteAdminApp(app.id)
      setApps((currentApps) =>
        currentApps.filter((currentApp) => currentApp.id !== app.id)
      )
      if (editingApp?.id === app.id) {
        handleDialogOpenChange(false)
      }
      setDeleteConfirmationApp(null)
      toast.success("应用已删除")
    } catch (error) {
      toast.error(
        error instanceof AdminAppsRequestError ? error.message : "删除应用失败"
      )
    } finally {
      setUpdatingAppId(null)
    }
  }

  async function handleConfirmAppDelete() {
    if (deleteConfirmationApp === null) {
      return
    }

    await handleAppDelete(deleteConfirmationApp)
  }

  function upsertApp(app: AdminApp) {
    setApps((currentApps) =>
      sortAdminAppsForDisplay(
        currentApps.map((currentApp) =>
          currentApp.id === app.id ? app : currentApp
        )
      )
    )
  }

  return (
    <>
      <div className="grid min-w-0 flex-1 items-start gap-4 p-4 pt-0">
        <Card className="w-full">
          <CardHeader>
            <div className="flex items-center justify-between gap-3">
              <CardTitle>应用</CardTitle>
              <AdminAppDialog
                disabled={isLoading}
                editingApp={editingApp}
                onAppSaved={handleAppSaved}
                onOpenChange={handleDialogOpenChange}
                open={isDialogOpen}
              />
            </div>
          </CardHeader>
          <CardContent>
            <div className={getAdminAppsTableContainerClassName()}>
              <Table className={getAdminAppsTableClassName()}>
                <TableHeader>
                  <TableRow>
                    <TableHead className={getAdminAppColumnClassName("name")}>
                      名称
                    </TableHead>
                    <TableHead className={getAdminAppColumnClassName("status")}>
                      状态
                    </TableHead>
                    <TableHead
                      className={getAdminAppColumnClassName("visibility")}
                    >
                      可见范围
                    </TableHead>
                    <TableHead
                      className={getAdminAppColumnClassName("actions")}
                    >
                      操作
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {apps.length === 0 ? (
                    <TableRow>
                      <TableCell className="h-24 text-center" colSpan={4}>
                        {isLoading ? "加载中" : "暂无应用"}
                      </TableCell>
                    </TableRow>
                  ) : (
                    apps.map((app) => (
                      <TableRow
                        className={getAdminAppRowClassName(app.enabled)}
                        key={app.id}
                      >
                        <TableCell
                          className={getAdminAppColumnClassName("name")}
                        >
                          <div className="flex min-w-0 items-center gap-3">
                            <AdminAppAvatar app={app} />
                            <div className="min-w-0">
                              <div className="flex min-w-0 items-center gap-2">
                                <span className="truncate font-medium">
                                  {app.name}
                                </span>
                                {app.system && (
                                  <Badge variant="secondary">系统</Badge>
                                )}
                              </div>
                              <div className="truncate text-xs text-muted-foreground">
                                {app.description || app.id}
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell
                          className={getAdminAppColumnClassName("status")}
                        >
                          <Badge
                            className={getAdminAppStatusBadgeClassName(app)}
                            variant={getAdminAppStatusBadgeVariant(app)}
                          >
                            {getAdminAppStatusLabel(app)}
                          </Badge>
                        </TableCell>
                        <TableCell
                          className={getAdminAppColumnClassName("visibility")}
                        >
                          {getAdminAppVisibilityLabel(app.visibility)}
                        </TableCell>
                        <TableCell
                          className={getAdminAppColumnClassName("actions")}
                        >
                          <AdminAppActions
                            app={app}
                            isUpdating={updatingAppId === app.id}
                            onCopy={copyAppField}
                            onDelete={openDeleteConfirmation}
                            onEdit={openEditForm}
                            onSecretRegenerate={handleAppSecretRegenerate}
                            onShowConnection={setConnectionApp}
                            onStatusChange={handleAppStatusChange}
                          />
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </div>
      <AlertDialog
        onOpenChange={handleDeleteConfirmationOpenChange}
        open={deleteConfirmationApp !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除应用</AlertDialogTitle>
            <AlertDialogDescription>
              删除后将无法继续使用
              {deleteConfirmationApp
                ? ` ${deleteConfirmationApp.name} `
                : "这个应用"}
              ，这个操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeletingApp}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={isDeletingApp}
              onClick={handleConfirmAppDelete}
              variant="destructive"
            >
              {isDeletingApp ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Trash2Icon data-icon="inline-start" />
              )}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AdminAppConnectionDialog
        app={connectionApp}
        onOpenChange={(open) => {
          if (!open) {
            setConnectionApp(null)
          }
        }}
      />
    </>
  )
}

function AdminAppActions({
  app,
  isUpdating,
  onCopy,
  onDelete,
  onEdit,
  onSecretRegenerate,
  onShowConnection,
  onStatusChange,
}: {
  app: AdminApp
  isUpdating: boolean
  onCopy: (value: string, label: string) => void
  onDelete: (app: AdminApp) => void
  onEdit: (app: AdminApp) => void
  onSecretRegenerate: (app: AdminApp) => void
  onShowConnection: (app: AdminApp) => void
  onStatusChange: (app: AdminApp, enabled: boolean) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={`打开 ${app.name} 的操作菜单`}
            size="icon-xs"
            type="button"
            variant="ghost"
          />
        }
      >
        <span className="sr-only">Open menu</span>
        {isUpdating ? <Spinner /> : <MoreHorizontalIcon />}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuItem disabled={isUpdating} onClick={() => onEdit(app)}>
            <PencilIcon />
            编辑
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onShowConnection(app)}
          >
            <LinkIcon />
            查看连接地址
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onCopy(app.id, "App ID")}
          >
            <CopyIcon />
            复制 App ID
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onCopy(app.connectionSecret, "连接密钥")}
          >
            <CopyIcon />
            复制密钥
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem
            disabled={isUpdating || app.enabled}
            onClick={() => onStatusChange(app, true)}
          >
            <CheckIcon />
            启用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || !app.enabled}
            onClick={() => onStatusChange(app, false)}
          >
            <BanIcon />
            禁用
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem
            disabled={isUpdating || app.system}
            onClick={() => onSecretRegenerate(app)}
          >
            <RefreshCwIcon />
            生成密钥
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || app.system}
            onClick={() => onDelete(app)}
            variant="destructive"
          >
            <Trash2Icon />
            删除
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function AdminAppConnectionDialog({
  app,
  onOpenChange,
}: {
  app: AdminApp | null
  onOpenChange: (open: boolean) => void
}) {
  const { appName } = useProductInfo()
  const urlId = useId()
  const appIdId = useId()
  const authorizationId = useId()

  if (app === null) {
    return null
  }

  const connectionURL = getAdminAppConnectionURL()
  const authorization = `Bearer ${app.connectionSecret}`

  return (
    <Dialog onOpenChange={onOpenChange} open>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>连接地址</DialogTitle>
          <DialogDescription>
            应用启动后使用这个地址和请求头连接到{appName}。
          </DialogDescription>
        </DialogHeader>
        <FieldGroup className="gap-4">
          <Field>
            <FieldLabel htmlFor={urlId}>URL</FieldLabel>
            <div className="flex gap-2">
              <Input id={urlId} readOnly value={connectionURL} />
              <Button
                onClick={() => copyAppField(connectionURL, "连接地址")}
                size="icon"
                title="复制连接地址"
                type="button"
                variant="outline"
              >
                <CopyIcon />
              </Button>
            </div>
          </Field>
          <Field>
            <FieldLabel htmlFor={appIdId}>X-MagicChat-App-ID</FieldLabel>
            <div className="flex gap-2">
              <Input id={appIdId} readOnly value={app.id} />
              <Button
                onClick={() => copyAppField(app.id, "App ID")}
                size="icon"
                title="复制 App ID"
                type="button"
                variant="outline"
              >
                <CopyIcon />
              </Button>
            </div>
          </Field>
          <Field>
            <FieldLabel htmlFor={authorizationId}>Authorization</FieldLabel>
            <div className="flex gap-2">
              <Input id={authorizationId} readOnly value={authorization} />
              <Button
                onClick={() => copyAppField(authorization, "Authorization")}
                size="icon"
                title="复制 Authorization"
                type="button"
                variant="outline"
              >
                <CopyIcon />
              </Button>
            </div>
          </Field>
        </FieldGroup>
      </DialogContent>
    </Dialog>
  )
}

function AdminAppDialog({
  disabled,
  editingApp,
  onAppSaved,
  onOpenChange,
  open,
}: {
  disabled: boolean
  editingApp: AdminApp | null
  onAppSaved: (app: AdminApp) => void
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const idId = useId()
  const secretId = useId()
  const nameId = useId()
  const descriptionId = useId()
  const visibilityId = useId()
  const [form, setForm] = useState<AdminAppForm>(createDefaultAdminAppForm)
  const [isAvatarCropPending, setIsAvatarCropPending] = useState(false)
  const [pendingAvatar, setPendingAvatar] = useState<CroppedAvatar | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const isEditing = editingApp !== null
  const isSubmitDisabled =
    disabled || isSaving || isAvatarCropPending || form.name.trim() === ""

  useEffect(() => {
    if (!open) {
      return
    }

    setForm(
      editingApp ? adminAppToForm(editingApp) : createDefaultAdminAppForm()
    )
    setIsAvatarCropPending(false)
    setPendingAvatar(null)
  }, [editingApp, open])

  function handleOpenChange(nextOpen: boolean) {
    if (isSaving) {
      return
    }

    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (isSubmitDisabled) {
      return
    }

    setIsSaving(true)

    try {
      const input = adminAppFormToInput(form)
      let app = editingApp
        ? await updateAdminApp(editingApp.id, input)
        : await createAdminApp(input)

      if (pendingAvatar) {
        try {
          app = await uploadAdminAppAvatar(app.id, pendingAvatar.file)
        } catch (error) {
          onAppSaved(app)
          toast.error(
            error instanceof AdminAppsRequestError
              ? `应用已保存，但${error.message}`
              : "应用已保存，但头像上传失败"
          )
          return
        }
      }

      onAppSaved(app)
      toast.success("应用已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminAppsRequestError ? error.message : "保存应用失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogTrigger
        render={
          <Button
            disabled={disabled}
            size="sm"
            type="button"
            variant="outline"
          />
        }
      >
        <PlusIcon data-icon="inline-start" />
        添加
      </DialogTrigger>
      <DialogContent
        className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-xl"
        showCloseButton={!isSaving}
      >
        <DialogHeader>
          <DialogTitle>{isEditing ? "编辑应用" : "添加应用"}</DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
          <FieldGroup className="gap-4">
            {editingApp && (
              <>
                <Field>
                  <FieldLabel htmlFor={idId}>App ID</FieldLabel>
                  <div className="flex gap-2">
                    <Input id={idId} readOnly value={editingApp.id} />
                    <Button
                      onClick={() => copyAppField(editingApp.id, "App ID")}
                      size="icon"
                      title="复制 App ID"
                      type="button"
                      variant="outline"
                    >
                      <CopyIcon />
                    </Button>
                  </div>
                </Field>
                <Field>
                  <FieldLabel htmlFor={secretId}>连接密钥</FieldLabel>
                  <div className="flex gap-2">
                    <Input
                      id={secretId}
                      readOnly
                      type="text"
                      value={editingApp.connectionSecret}
                    />
                    <Button
                      onClick={() =>
                        copyAppField(editingApp.connectionSecret, "连接密钥")
                      }
                      size="icon"
                      title="复制连接密钥"
                      type="button"
                      variant="outline"
                    >
                      <CopyIcon />
                    </Button>
                  </div>
                </Field>
              </>
            )}
            <Field>
              <FieldLabel htmlFor={nameId}>名称</FieldLabel>
              <Input
                disabled={isSaving}
                id={nameId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    name: event.target.value,
                  }))
                }
                value={form.name}
              />
            </Field>
            <Field>
              <FieldLabel>头像</FieldLabel>
              {(pendingAvatar || form.avatar) && (
                <Avatar className="size-16 rounded-sm bg-muted after:rounded-sm">
                  <AvatarImage
                    alt="应用头像预览"
                    className="rounded-sm"
                    src={pendingAvatar?.previewUrl ?? form.avatar}
                  />
                  <AvatarFallback className="rounded-sm">
                    {getAdminAppInitial(form.name)}
                  </AvatarFallback>
                </Avatar>
              )}
              <CustomAvatarPicker
                disabled={isSaving}
                onChange={setPendingAvatar}
                onEditingChange={setIsAvatarCropPending}
                saving={isSaving}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={descriptionId}>简介</FieldLabel>
              <Textarea
                disabled={isSaving}
                id={descriptionId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    description: event.target.value,
                  }))
                }
                value={form.description}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={visibilityId}>可见范围</FieldLabel>
              <Input
                id={visibilityId}
                readOnly
                value={getAdminAppVisibilityLabel(form.visibility)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              disabled={isSaving}
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              <XIcon data-icon="inline-start" />
              取消
            </Button>
            <Button disabled={isSubmitDisabled} type="submit">
              {isSaving ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SaveIcon data-icon="inline-start" />
              )}
              保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function AdminAppAvatar({ app }: { app: AdminApp }) {
  return (
    <Avatar className="size-9 rounded-sm bg-muted after:rounded-sm">
      {app.avatar && (
        <AvatarImage alt={app.name} className="rounded-sm" src={app.avatar} />
      )}
      <AvatarFallback className="rounded-sm">
        {getAdminAppInitial(app.name)}
      </AvatarFallback>
    </Avatar>
  )
}

function createDefaultAdminAppForm(): AdminAppForm {
  return {
    avatar: "",
    description: "",
    name: "",
    visibility: "public",
  }
}

function adminAppToForm(app: AdminApp): AdminAppForm {
  return {
    avatar: app.avatar,
    description: app.description,
    name: app.name,
    visibility: app.visibility,
  }
}

function adminAppFormToInput(form: AdminAppForm): AdminAppInput {
  return {
    description: form.description,
    name: form.name,
    visibility: form.visibility,
  }
}

function sortAdminAppsForDisplay(apps: AdminApp[]) {
  return [...apps].sort((left, right) => {
    if (left.system && !right.system) {
      return -1
    }
    if (!left.system && right.system) {
      return 1
    }
    return left.name.localeCompare(right.name, "zh-Hans")
  })
}

function getAdminAppVisibilityLabel(visibility: AdminAppVisibility) {
  switch (visibility) {
    case "creator":
      return "仅创建者"
    case "public":
      return "所有人"
  }
}

function getAdminAppStatusLabel(app: AdminApp) {
  if (!app.enabled) {
    return "已禁用"
  }

  return app.connectionStatus === "online" ? "在线" : "离线"
}

function getAdminAppStatusBadgeVariant(app: AdminApp) {
  if (!app.enabled) {
    return "secondary"
  }

  return app.connectionStatus === "online" ? "default" : "outline"
}

function getAdminAppStatusBadgeClassName(app: AdminApp) {
  if (!app.enabled || app.connectionStatus !== "online") {
    return undefined
  }

  return "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200"
}

function getAdminAppInitial(name: string) {
  const trimmed = name.trim()
  if (trimmed === "") {
    return "应"
  }

  return trimmed.slice(0, 1).toUpperCase()
}

function getAdminAppRowClassName(enabled: boolean) {
  return enabled ? "" : "text-muted-foreground"
}

export function getAdminAppsTableContainerClassName() {
  return "overflow-hidden rounded-md border"
}

export function getAdminAppsTableClassName() {
  return "table-fixed"
}

export function getAdminAppColumnClassName(
  column: "actions" | "name" | "status" | "visibility"
) {
  switch (column) {
    case "name":
      return "w-[30%] min-w-0"
    case "status":
      return "w-24"
    case "visibility":
      return "w-28"
    case "actions":
      return "w-16 text-right"
  }
}

function getAdminAppConnectionURL() {
  const url = new URL("/api/app/ws", window.location.origin)
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:"

  return url.toString()
}

function copyAppField(value: string, label: string) {
  if (!navigator.clipboard) {
    toast.error("当前浏览器不支持复制")
    return
  }

  void navigator.clipboard.writeText(value).then(
    () => toast.success(`${label} 已复制`),
    () => toast.error("复制失败")
  )
}
