import { MoreHorizontalIcon, PlusIcon, SaveIcon, XIcon } from "lucide-react"
import { useEffect, useId, useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
  AdminSettingsRequestError,
  createOIDCProvider,
  deleteOIDCProvider,
  disableOIDCProvider,
  enableOIDCProvider,
  getInfoSettings,
  listOIDCProviders,
  moveOIDCProvider,
  type OIDCProvider,
  type OIDCProviderMoveDirection,
  type OIDCProviderInput,
  updateOIDCProvider,
  updateInfoSettings,
} from "@/lib/admin-settings"

export type OIDCProviderForm = {
  avatarField: string
  authorizeUrl: string
  clientId: string
  clientSecret: string
  emailField: string
  name: string
  nameField: string
  nicknameField: string
  phoneField: string
  scopesText: string
  tokenUrl: string
  userinfoUrl: string
}

export default function SettingsPage() {
  const appNameId = useId()
  const organizationNameId = useId()
  const [appName, setAppName] = useState("")
  const [editingOIDCProvider, setEditingOIDCProvider] =
    useState<OIDCProvider | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isOIDCDialogOpen, setIsOIDCDialogOpen] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [oidcProviders, setOIDCProviders] = useState<OIDCProvider[]>([])
  const [updatingOIDCProviderId, setUpdatingOIDCProviderId] = useState<
    string | null
  >(null)
  const [organizationName, setOrganizationName] = useState("")
  const isSubmitDisabled =
    isLoading ||
    isSaving ||
    appName.trim() === "" ||
    organizationName.trim() === ""

  useEffect(() => {
    let ignore = false

    async function loadSettings() {
      setIsLoading(true)

      try {
        const [settings, providers] = await Promise.all([
          getInfoSettings(),
          listOIDCProviders(),
        ])

        if (ignore) {
          return
        }

        setAppName(settings.appName)
        setOrganizationName(settings.organizationName)
        setOIDCProviders(sortOIDCProvidersForDisplay(providers))
      } catch (error) {
        if (ignore) {
          return
        }

        toast.error(
          error instanceof AdminSettingsRequestError
            ? error.message
            : "加载系统设置失败"
        )
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void loadSettings()

    return () => {
      ignore = true
    }
  }, [])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (isSubmitDisabled) {
      return
    }

    setIsSaving(true)

    try {
      const settings = await updateInfoSettings({
        appName,
        organizationName,
      })

      setAppName(settings.appName)
      setOrganizationName(settings.organizationName)
      toast.success("系统设置已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存系统设置失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  async function handleOIDCDelete(provider: OIDCProvider) {
    setUpdatingOIDCProviderId(provider.id)

    try {
      await deleteOIDCProvider(provider.id)
      setOIDCProviders((providers) =>
        providers.filter(
          (currentProvider) => currentProvider.id !== provider.id
        )
      )
      if (editingOIDCProvider?.id === provider.id) {
        handleOIDCDialogOpenChange(false)
      }
      toast.success("OIDC 登录方式已删除")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "删除 OIDC 登录方式失败"
      )
    } finally {
      setUpdatingOIDCProviderId(null)
    }
  }

  async function handleOIDCStatusChange(
    provider: OIDCProvider,
    enabled: boolean
  ) {
    setUpdatingOIDCProviderId(provider.id)

    try {
      const updatedProvider = enabled
        ? await enableOIDCProvider(provider.id)
        : await disableOIDCProvider(provider.id)

      setOIDCProviders((providers) =>
        sortOIDCProvidersForDisplay(
          providers.map((currentProvider) =>
            currentProvider.id === updatedProvider.id
              ? updatedProvider
              : currentProvider
          )
        )
      )
      toast.success(enabled ? "OIDC 登录方式已启用" : "OIDC 登录方式已禁用")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "更新 OIDC 登录方式状态失败"
      )
    } finally {
      setUpdatingOIDCProviderId(null)
    }
  }

  async function handleOIDCMove(
    provider: OIDCProvider,
    direction: OIDCProviderMoveDirection
  ) {
    setUpdatingOIDCProviderId(provider.id)

    try {
      const providers = await moveOIDCProvider(provider.id, direction)
      setOIDCProviders(sortOIDCProvidersForDisplay(providers))
      toast.success("OIDC 登录方式排序已更新")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "移动 OIDC 登录方式失败"
      )
    } finally {
      setUpdatingOIDCProviderId(null)
    }
  }

  function handleOIDCDialogOpenChange(open: boolean) {
    setIsOIDCDialogOpen(open)
    if (!open) {
      setEditingOIDCProvider(null)
    }
  }

  function openEditOIDCForm(provider: OIDCProvider) {
    setEditingOIDCProvider(provider)
    setIsOIDCDialogOpen(true)
  }

  function handleOIDCProviderSaved(provider: OIDCProvider) {
    setOIDCProviders((providers) =>
      sortOIDCProvidersForDisplay([
        provider,
        ...providers.filter(
          (currentProvider) => currentProvider.id !== provider.id
        ),
      ])
    )
    handleOIDCDialogOpenChange(false)
  }

  return (
    <div className={getSettingsPageLayoutClassName()}>
      <Card className={getSettingsCardClassName()}>
        <CardHeader>
          <CardTitle>基础信息</CardTitle>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
            <FieldGroup className="gap-4">
              <Field>
                <FieldLabel htmlFor={appNameId}>应用名称</FieldLabel>
                <Input
                  disabled={isLoading || isSaving}
                  id={appNameId}
                  onChange={(event) => setAppName(event.target.value)}
                  required
                  value={appName}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={organizationNameId}>组织名称</FieldLabel>
                <Input
                  disabled={isLoading || isSaving}
                  id={organizationNameId}
                  onChange={(event) => setOrganizationName(event.target.value)}
                  required
                  value={organizationName}
                />
              </Field>
            </FieldGroup>

            <div className="flex justify-end">
              <Button disabled={isSubmitDisabled} type="submit">
                {isSaving ? (
                  <Spinner data-icon="inline-start" />
                ) : (
                  <SaveIcon data-icon="inline-start" />
                )}
                保存设置
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      <Card className={getSettingsCardClassName()}>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <CardTitle>OIDC 登录</CardTitle>
            <OIDCProviderDialog
              disabled={isLoading}
              editingProvider={editingOIDCProvider}
              onOpenChange={handleOIDCDialogOpenChange}
              onProviderSaved={handleOIDCProviderSaved}
              open={isOIDCDialogOpen}
            />
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            {oidcProviders.length === 0 ? (
              <div className="text-sm text-muted-foreground">
                暂无 OIDC 登录方式
              </div>
            ) : (
              oidcProviders.map((provider, index) => (
                <div
                  className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
                  key={provider.id}
                >
                  <div className={getOIDCProviderTextClassName(provider.enabled)}>
                    <div className="truncate text-sm font-medium">
                      {provider.name}
                    </div>
                    <div className="truncate text-xs text-muted-foreground">
                      {provider.key}
                      {provider.enabled ? "" : " · 已禁用"}
                    </div>
                  </div>
                  <OIDCProviderActions
                    isFirst={index === 0}
                    isLast={index === oidcProviders.length - 1}
                    isUpdating={updatingOIDCProviderId === provider.id}
                    onDelete={handleOIDCDelete}
                    onEdit={openEditOIDCForm}
                    onMove={handleOIDCMove}
                    onStatusChange={handleOIDCStatusChange}
                    provider={provider}
                  />
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function OIDCProviderActions({
  isFirst,
  isLast,
  isUpdating,
  onDelete,
  onEdit,
  onMove,
  onStatusChange,
  provider,
}: {
  isFirst: boolean
  isLast: boolean
  isUpdating: boolean
  onDelete: (provider: OIDCProvider) => void
  onEdit: (provider: OIDCProvider) => void
  onMove: (provider: OIDCProvider, direction: OIDCProviderMoveDirection) => void
  onStatusChange: (provider: OIDCProvider, enabled: boolean) => void
  provider: OIDCProvider
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={`打开 ${provider.name} 的操作菜单`}
            size="icon-xs"
            type="button"
            variant="ghost"
          />
        }
      >
        <span className="sr-only">Open menu</span>
        <MoreHorizontalIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onEdit(provider)}
          >
            编辑
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || provider.enabled}
            onClick={() => onStatusChange(provider, true)}
          >
            启用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || !provider.enabled}
            onClick={() => onStatusChange(provider, false)}
          >
            禁用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || isFirst}
            onClick={() => onMove(provider, "up")}
          >
            上移
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || isLast}
            onClick={() => onMove(provider, "down")}
          >
            下移
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onDelete(provider)}
            variant="destructive"
          >
            删除
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function OIDCProviderDialog({
  disabled,
  editingProvider,
  onOpenChange,
  onProviderSaved,
  open,
}: {
  disabled: boolean
  editingProvider: OIDCProvider | null
  onOpenChange: (open: boolean) => void
  onProviderSaved: (provider: OIDCProvider) => void
  open: boolean
}) {
  const oidcAvatarFieldId = useId()
  const oidcAuthorizeUrlId = useId()
  const oidcClientIdId = useId()
  const oidcClientSecretId = useId()
  const oidcEmailFieldId = useId()
  const oidcNameFieldId = useId()
  const oidcNameId = useId()
  const oidcNicknameFieldId = useId()
  const oidcPhoneFieldId = useId()
  const oidcScopesId = useId()
  const oidcTokenUrlId = useId()
  const oidcUserinfoUrlId = useId()
  const [form, setForm] = useState<OIDCProviderForm>(
    createDefaultOIDCProviderForm
  )
  const [isSaving, setIsSaving] = useState(false)
  const isEditing = editingProvider !== null
  const isSubmitDisabled =
    disabled ||
    isSaving ||
    form.name.trim() === "" ||
    form.authorizeUrl.trim() === "" ||
    form.tokenUrl.trim() === "" ||
    form.userinfoUrl.trim() === "" ||
    form.clientId.trim() === "" ||
    form.clientSecret.trim() === "" ||
    form.emailField.trim() === "" ||
    form.nameField.trim() === "" ||
    oidcProviderFormToInput(form).scopes.length === 0

  useEffect(() => {
    if (!open) {
      return
    }

    setForm(
      editingProvider
        ? oidcProviderToForm(editingProvider)
        : createDefaultOIDCProviderForm()
    )
  }, [editingProvider, open])

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
      const input = oidcProviderFormToInput(form)
      const provider = editingProvider
        ? await updateOIDCProvider(editingProvider.id, input)
        : await createOIDCProvider(input)

      onProviderSaved(provider)
      toast.success("OIDC 登录方式已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存 OIDC 登录方式失败"
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
        className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-2xl"
        showCloseButton={!isSaving}
      >
        <DialogHeader>
          <DialogTitle>
            {isEditing ? "编辑登录方式" : "添加登录方式"}
          </DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
          <FieldGroup className="gap-4">
            <Field>
              <FieldLabel htmlFor={oidcNameId}>名称</FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcNameId}
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
              <FieldLabel htmlFor={oidcAuthorizeUrlId}>
                Authorize URL
              </FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcAuthorizeUrlId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    authorizeUrl: event.target.value,
                  }))
                }
                value={form.authorizeUrl}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={oidcTokenUrlId}>Access Token URL</FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcTokenUrlId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    tokenUrl: event.target.value,
                  }))
                }
                value={form.tokenUrl}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={oidcUserinfoUrlId}>用户信息 URL</FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcUserinfoUrlId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    userinfoUrl: event.target.value,
                  }))
                }
                value={form.userinfoUrl}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={oidcClientIdId}>Client ID</FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcClientIdId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    clientId: event.target.value,
                  }))
                }
                value={form.clientId}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={oidcClientSecretId}>
                Client Secret
              </FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcClientSecretId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    clientSecret: event.target.value,
                  }))
                }
                value={form.clientSecret}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={oidcScopesId}>Scope</FieldLabel>
              <Input
                disabled={isSaving}
                id={oidcScopesId}
                onChange={(event) =>
                  setForm((currentForm) => ({
                    ...currentForm,
                    scopesText: event.target.value,
                  }))
                }
                placeholder="email,profile"
                value={form.scopesText}
              />
            </Field>
            <div className="grid gap-4 md:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={oidcEmailFieldId}>邮箱字段</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={oidcEmailFieldId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      emailField: event.target.value,
                    }))
                  }
                  value={form.emailField}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={oidcPhoneFieldId}>手机字段</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={oidcPhoneFieldId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      phoneField: event.target.value,
                    }))
                  }
                  value={form.phoneField}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={oidcNameFieldId}>姓名字段</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={oidcNameFieldId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      nameField: event.target.value,
                    }))
                  }
                  value={form.nameField}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={oidcNicknameFieldId}>昵称字段</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={oidcNicknameFieldId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      nicknameField: event.target.value,
                    }))
                  }
                  value={form.nicknameField}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={oidcAvatarFieldId}>头像字段</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={oidcAvatarFieldId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      avatarField: event.target.value,
                    }))
                  }
                  value={form.avatarField}
                />
              </Field>
            </div>
          </FieldGroup>
          <DialogFooter>
            <Button
              disabled={isSaving}
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="ghost"
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

export function getSettingsPageLayoutClassName() {
  return "grid min-w-0 flex-1 gap-4 p-4 pt-0 lg:grid-cols-2 lg:items-start"
}

export function getSettingsCardClassName() {
  return "w-full"
}

export function getOIDCProviderTextClassName(enabled: boolean) {
  return enabled ? "min-w-0" : "min-w-0 text-muted-foreground"
}

export function createDefaultOIDCProviderForm(): OIDCProviderForm {
  return {
    avatarField: "picture",
    authorizeUrl: "",
    clientId: "",
    clientSecret: "",
    emailField: "email",
    name: "",
    nameField: "name",
    nicknameField: "nickname",
    phoneField: "phone",
    scopesText: "email,profile",
    tokenUrl: "",
    userinfoUrl: "",
  }
}

export function oidcProviderToForm(provider: OIDCProvider): OIDCProviderForm {
  return {
    avatarField: provider.avatarField,
    authorizeUrl: provider.authorizeUrl,
    clientId: provider.clientId,
    clientSecret: provider.clientSecret,
    emailField: provider.emailField,
    name: provider.name,
    nameField: provider.nameField,
    nicknameField: provider.nicknameField,
    phoneField: provider.phoneField,
    scopesText: provider.scopes.join(","),
    tokenUrl: provider.tokenUrl,
    userinfoUrl: provider.userinfoUrl,
  }
}

export function oidcProviderFormToInput(
  form: OIDCProviderForm
): OIDCProviderInput {
  return {
    avatarField: form.avatarField.trim(),
    authorizeUrl: form.authorizeUrl.trim(),
    clientId: form.clientId.trim(),
    clientSecret: form.clientSecret.trim(),
    emailField: form.emailField.trim(),
    name: form.name.trim(),
    nameField: form.nameField.trim(),
    nicknameField: form.nicknameField.trim(),
    phoneField: form.phoneField.trim(),
    scopes: form.scopesText
      .split(",")
      .map((scope) => scope.trim())
      .filter(Boolean),
    tokenUrl: form.tokenUrl.trim(),
    userinfoUrl: form.userinfoUrl.trim(),
  }
}

export function sortOIDCProvidersForDisplay(providers: OIDCProvider[]) {
  return [...providers].sort((firstProvider, secondProvider) => {
    if (firstProvider.sortOrder !== secondProvider.sortOrder) {
      return firstProvider.sortOrder - secondProvider.sortOrder
    }

    const nameCompare = firstProvider.name.localeCompare(secondProvider.name)
    if (nameCompare !== 0) {
      return nameCompare
    }

    return firstProvider.id.localeCompare(secondProvider.id)
  })
}
