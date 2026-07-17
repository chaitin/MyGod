import {
  ArrowDownIcon,
  ArrowUpIcon,
  EyeIcon,
  EyeOffIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  SaveIcon,
  SendIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useId, useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { useProductInfo } from "@/components/product-info-provider"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  AdminSettingsRequestError,
  createThirdPartyLoginProvider,
  deleteThirdPartyLoginProvider,
  disableThirdPartyLoginProvider,
  enableThirdPartyLoginProvider,
  getEmailLoginSettings,
  getInfoSettings,
  getPasswordLoginSettings,
  listThirdPartyLoginProviders,
  moveThirdPartyLoginProvider,
  type ThirdPartyLoginProvider,
  type ThirdPartyLoginProviderInput,
  type ThirdPartyLoginProviderMoveDirection,
  type ThirdPartyLoginProviderType,
  type EmailLoginSettings,
  type PasswordLoginSettings,
  type SMTPSecurity,
  testEmailLoginSettings,
  updateEmailLoginSettings,
  updateInfoSettings,
  updatePasswordLoginSettings,
  updateThirdPartyLoginProvider,
} from "@/lib/admin-settings"

type ThirdPartyLoginProviderOption = {
  clientIdLabel: string
  label: string
  name: string
  secretLabel: string
  type: ThirdPartyLoginProviderType
}

export type ThirdPartyLoginProviderForm = {
  agentId: string
  avatarField: string
  authorizeUrl: string
  clientId: string
  clientSecret: string
  emailField: string
  externalIdField: string
  name: string
  nameField: string
  nicknameField: string
  phoneField: string
  scopesText: string
  tokenUrl: string
  type: ThirdPartyLoginProviderType
  userinfoUrl: string
}

const thirdPartyLoginProviderOptions: ThirdPartyLoginProviderOption[] = [
  {
    clientIdLabel: "AppKey / Client ID",
    label: "钉钉",
    name: "钉钉",
    secretLabel: "AppSecret / Client Secret",
    type: "dingtalk",
  },
  {
    clientIdLabel: "Corp ID",
    label: "企业微信",
    name: "企业微信",
    secretLabel: "Secret",
    type: "wecom",
  },
  {
    clientIdLabel: "App ID",
    label: "飞书",
    name: "飞书",
    secretLabel: "App Secret",
    type: "feishu",
  },
  {
    clientIdLabel: "Client ID",
    label: "GitHub",
    name: "GitHub",
    secretLabel: "Client Secret",
    type: "github",
  },
  {
    clientIdLabel: "Client ID",
    label: "Google",
    name: "Google",
    secretLabel: "Client Secret",
    type: "google",
  },
  {
    clientIdLabel: "Client ID",
    label: "通用 OIDC",
    name: "通用 OIDC",
    secretLabel: "Client Secret",
    type: "oidc",
  },
]

export default function SettingsPage() {
  const { setAppName: setProductName } = useProductInfo()
  const appNameId = useId()
  const organizationNameId = useId()
  const [appName, setAppName] = useState("")
  const [callbackProvider, setCallbackProvider] =
    useState<ThirdPartyLoginProvider | null>(null)
  const [dialogProviderType, setDialogProviderType] =
    useState<ThirdPartyLoginProviderType>("oidc")
  const [editingProvider, setEditingProvider] =
    useState<ThirdPartyLoginProvider | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isProviderDialogOpen, setIsProviderDialogOpen] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [organizationName, setOrganizationName] = useState("")
  const [providers, setProviders] = useState<ThirdPartyLoginProvider[]>([])
  const [updatingProviderId, setUpdatingProviderId] = useState<string | null>(
    null
  )
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
        const [settings, loadedProviders] = await Promise.all([
          getInfoSettings(),
          listThirdPartyLoginProviders(),
        ])

        if (ignore) {
          return
        }

        setAppName(settings.appName)
        setOrganizationName(settings.organizationName)
        setProviders(sortThirdPartyProvidersForDisplay(loadedProviders))
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
      setProductName(settings.appName)
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

  async function handleProviderDelete(provider: ThirdPartyLoginProvider) {
    setUpdatingProviderId(provider.id)

    try {
      await deleteThirdPartyLoginProvider(provider.id)
      setProviders((currentProviders) =>
        currentProviders.filter(
          (currentProvider) => currentProvider.id !== provider.id
        )
      )
      if (editingProvider?.id === provider.id) {
        handleProviderDialogOpenChange(false)
      }
      toast.success("第三方登录方式已删除")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "删除第三方登录方式失败"
      )
    } finally {
      setUpdatingProviderId(null)
    }
  }

  async function handleProviderStatusChange(
    provider: ThirdPartyLoginProvider,
    enabled: boolean
  ) {
    setUpdatingProviderId(provider.id)

    try {
      const updatedProvider = enabled
        ? await enableThirdPartyLoginProvider(provider.id)
        : await disableThirdPartyLoginProvider(provider.id)

      setProviders((currentProviders) =>
        sortThirdPartyProvidersForDisplay(
          currentProviders.map((currentProvider) =>
            currentProvider.id === updatedProvider.id
              ? updatedProvider
              : currentProvider
          )
        )
      )
      toast.success(enabled ? "第三方登录方式已启用" : "第三方登录方式已禁用")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "更新第三方登录方式状态失败"
      )
    } finally {
      setUpdatingProviderId(null)
    }
  }

  async function handleProviderMove(
    provider: ThirdPartyLoginProvider,
    direction: ThirdPartyLoginProviderMoveDirection
  ) {
    setUpdatingProviderId(provider.id)

    try {
      const updatedProviders = await moveThirdPartyLoginProvider(
        provider.id,
        direction
      )
      setProviders(sortThirdPartyProvidersForDisplay(updatedProviders))
      toast.success("第三方登录方式排序已更新")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "移动第三方登录方式失败"
      )
    } finally {
      setUpdatingProviderId(null)
    }
  }

  function handleProviderDialogOpenChange(open: boolean) {
    setIsProviderDialogOpen(open)
    if (!open) {
      setEditingProvider(null)
    }
  }

  function openCreateProviderForm(type: ThirdPartyLoginProviderType) {
    setDialogProviderType(type)
    setEditingProvider(null)
    setIsProviderDialogOpen(true)
  }

  function openEditProviderForm(provider: ThirdPartyLoginProvider) {
    setDialogProviderType(provider.type)
    setEditingProvider(provider)
    setIsProviderDialogOpen(true)
  }

  function handleCallbackDialogOpenChange(open: boolean) {
    if (!open) {
      setCallbackProvider(null)
    }
  }

  function handleProviderSaved(provider: ThirdPartyLoginProvider) {
    setProviders((currentProviders) =>
      sortThirdPartyProvidersForDisplay([
        provider,
        ...currentProviders.filter(
          (currentProvider) => currentProvider.id !== provider.id
        ),
      ])
    )
    handleProviderDialogOpenChange(false)
  }

  return (
    <div className={getSettingsPageLayoutClassName()}>
      <div className="flex min-w-0 flex-col gap-4">
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
                    onChange={(event) =>
                      setOrganizationName(event.target.value)
                    }
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
        <PasswordLoginSettingsCard />
        <Card className={getSettingsCardClassName()}>
          <CardHeader>
            <div className="flex items-center justify-between gap-3">
              <CardTitle>第三方登录</CardTitle>
              <ThirdPartyProviderAddMenu
                disabled={isLoading}
                onSelect={openCreateProviderForm}
              />
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              {providers.length === 0 ? (
                <div className="text-sm text-muted-foreground">
                  暂无第三方登录方式
                </div>
              ) : (
                providers.map((provider, index) => (
                  <div
                    className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
                    key={provider.id}
                  >
                    <div
                      className={getThirdPartyProviderTextClassName(
                        provider.enabled
                      )}
                    >
                      <div className="truncate text-sm font-medium">
                        {provider.name}
                        {provider.enabled ? "" : " · 已禁用"}
                      </div>
                    </div>
                    <ThirdPartyProviderActions
                      isFirst={index === 0}
                      isLast={index === providers.length - 1}
                      isUpdating={updatingProviderId === provider.id}
                      onDelete={handleProviderDelete}
                      onEdit={openEditProviderForm}
                      onMove={handleProviderMove}
                      onStatusChange={handleProviderStatusChange}
                      onViewCallback={setCallbackProvider}
                      provider={provider}
                    />
                  </div>
                ))
              )}
            </div>
            <ThirdPartyCallbackURLDialog
              onOpenChange={handleCallbackDialogOpenChange}
              provider={callbackProvider}
            />
            <ThirdPartyProviderDialog
              disabled={isLoading}
              editingProvider={editingProvider}
              onOpenChange={handleProviderDialogOpenChange}
              onProviderSaved={handleProviderSaved}
              open={isProviderDialogOpen}
              providerType={dialogProviderType}
            />
          </CardContent>
        </Card>
      </div>
      <EmailLoginSettingsCard />
    </div>
  )
}

const defaultPasswordLoginSettings: PasswordLoginSettings = {
  enabled: true,
}

function PasswordLoginSettingsCard() {
  const enabledId = useId()
  const [settings, setSettings] = useState(defaultPasswordLoginSettings)
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    let ignore = false

    async function load() {
      try {
        const loaded = await getPasswordLoginSettings()
        if (!ignore) {
          setSettings(loaded)
        }
      } catch (error) {
        if (!ignore) {
          toast.error(
            error instanceof AdminSettingsRequestError
              ? error.message
              : "加载密码登录设置失败"
          )
        }
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void load()
    return () => {
      ignore = true
    }
  }, [])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setIsSaving(true)

    try {
      const updated = await updatePasswordLoginSettings(settings)
      setSettings(updated)
      toast.success("密码登录设置已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存密码登录设置失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  const disabled = isLoading || isSaving

  return (
    <Card className={getSettingsCardClassName()}>
      <CardHeader>
        <CardTitle>密码登录</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
          <Field orientation="horizontal">
            <div className="flex flex-1 flex-col gap-1">
              <FieldLabel htmlFor={enabledId}>启用密码登录</FieldLabel>
              <FieldDescription>
                关闭后，普通用户无法通过邮箱和密码登录，不影响验证码和第三方登录。
              </FieldDescription>
            </div>
            <Switch
              checked={settings.enabled}
              disabled={disabled}
              id={enabledId}
              onCheckedChange={(enabled) => setSettings({ enabled })}
            />
          </Field>
          <div className="flex justify-end">
            <Button disabled={disabled} type="submit">
              {isSaving ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SaveIcon data-icon="inline-start" />
              )}
              保存密码登录设置
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

const defaultEmailLoginSettings: EmailLoginSettings = {
  enabled: false,
  fromEmail: "",
  fromName: "",
  smtpHost: "",
  smtpPassword: "",
  smtpPasswordConfigured: false,
  smtpPort: 465,
  smtpSecurity: "tls",
  smtpUsername: "",
}

function EmailLoginSettingsCard() {
  const enabledId = useId()
  const fromEmailId = useId()
  const fromNameId = useId()
  const hostId = useId()
  const passwordId = useId()
  const portId = useId()
  const securityId = useId()
  const usernameId = useId()
  const [settings, setSettings] = useState(defaultEmailLoginSettings)
  const [testRecipient, setTestRecipient] = useState("")
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isTesting, setIsTesting] = useState(false)
  const [showSMTPPassword, setShowSMTPPassword] = useState(false)

  useEffect(() => {
    let ignore = false

    async function load() {
      try {
        const loaded = await getEmailLoginSettings()
        if (!ignore) {
          setSettings(loaded)
          setTestRecipient(loaded.fromEmail)
        }
      } catch (error) {
        if (!ignore) {
          toast.error(
            error instanceof AdminSettingsRequestError
              ? error.message
              : "加载邮箱登录设置失败"
          )
        }
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void load()
    return () => {
      ignore = true
    }
  }, [])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setIsSaving(true)

    try {
      const updated = await updateEmailLoginSettings(settings)
      setSettings(updated)
      toast.success("邮箱登录设置已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存邮箱登录设置失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  async function handleTestSMTP() {
    if (testRecipient.trim() === "") {
      toast.error("请输入测试收件邮箱")
      return
    }
    setIsTesting(true)
    try {
      await testEmailLoginSettings(testRecipient)
      toast.success("SMTP 测试邮件已发送")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "发送 SMTP 测试邮件失败"
      )
    } finally {
      setIsTesting(false)
    }
  }

  const disabled = isLoading || isSaving || isTesting

  return (
    <Card className={getSettingsCardClassName()}>
      <CardHeader>
        <CardTitle>邮箱验证码登录</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
          <Field orientation="horizontal">
            <div className="flex flex-1 flex-col gap-1">
              <FieldLabel htmlFor={enabledId}>启用邮箱验证码登录</FieldLabel>
              <FieldDescription>
                启用后，用户可以通过 8 位邮箱验证码登录，验证码 15 分钟内有效。
              </FieldDescription>
            </div>
            <Switch
              checked={settings.enabled}
              disabled={disabled}
              id={enabledId}
              onCheckedChange={(enabled) =>
                setSettings((current) => ({ ...current, enabled }))
              }
            />
          </Field>

          <FieldGroup className="gap-4">
            <div className="grid gap-4 md:grid-cols-[1fr_9rem]">
              <Field>
                <FieldLabel htmlFor={hostId}>SMTP 主机</FieldLabel>
                <Input
                  disabled={disabled}
                  id={hostId}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      smtpHost: event.target.value,
                    }))
                  }
                  placeholder="smtp.example.com"
                  required={settings.enabled}
                  value={settings.smtpHost}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={portId}>端口</FieldLabel>
                <Input
                  disabled={disabled}
                  id={portId}
                  max={65535}
                  min={1}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      smtpPort: Number(event.target.value),
                    }))
                  }
                  required
                  type="number"
                  value={settings.smtpPort}
                />
              </Field>
            </div>

            <Field>
              <FieldLabel htmlFor={securityId}>连接安全</FieldLabel>
              <Select
                disabled={disabled}
                onValueChange={(smtpSecurity) => {
                  setSettings((current) => ({
                    ...current,
                    smtpSecurity: smtpSecurity as SMTPSecurity,
                    ...(smtpSecurity === "none"
                      ? {
                          smtpPassword: "",
                          smtpPasswordConfigured: false,
                          smtpUsername: "",
                        }
                      : {}),
                  }))
                }}
                value={settings.smtpSecurity}
              >
                <SelectTrigger className="w-full" id={securityId}>
                  <SelectValue>
                    {getSMTPSecurityLabel(settings.smtpSecurity)}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value="tls">TLS（推荐）</SelectItem>
                    <SelectItem value="starttls">STARTTLS</SelectItem>
                    <SelectItem disabled={settings.enabled} value="none">
                      无加密
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>

            <div className="grid gap-4 md:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={usernameId}>SMTP 用户名</FieldLabel>
                <Input
                  autoComplete="username"
                  disabled={disabled || settings.smtpSecurity === "none"}
                  id={usernameId}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      smtpUsername: event.target.value,
                    }))
                  }
                  placeholder="请输入 SMTP 用户名"
                  required={settings.enabled}
                  value={settings.smtpUsername}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={passwordId}>SMTP 密码</FieldLabel>
                <InputGroup>
                  <InputGroupInput
                    autoComplete="new-password"
                    disabled={disabled || settings.smtpSecurity === "none"}
                    id={passwordId}
                    onChange={(event) =>
                      setSettings((current) => ({
                        ...current,
                        smtpPassword: event.target.value,
                        smtpPasswordConfigured: event.target.value !== "",
                      }))
                    }
                    placeholder="请输入 SMTP 密码"
                    required={settings.enabled}
                    type={showSMTPPassword ? "text" : "password"}
                    value={settings.smtpPassword}
                  />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton
                      aria-label={showSMTPPassword ? "隐藏密码" : "显示密码"}
                      aria-pressed={showSMTPPassword}
                      disabled={disabled || settings.smtpSecurity === "none"}
                      onClick={() => setShowSMTPPassword((visible) => !visible)}
                      size="icon-xs"
                    >
                      {showSMTPPassword ? <EyeOffIcon /> : <EyeIcon />}
                    </InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              </Field>
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={fromEmailId}>发件人邮箱</FieldLabel>
                <Input
                  disabled={disabled}
                  id={fromEmailId}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      fromEmail: event.target.value,
                    }))
                  }
                  placeholder="mailer@example.com"
                  required={settings.enabled}
                  type="email"
                  value={settings.fromEmail}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={fromNameId}>发件人名称</FieldLabel>
                <Input
                  disabled={disabled}
                  id={fromNameId}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      fromName: event.target.value,
                    }))
                  }
                  placeholder="留空时使用应用名称"
                  value={settings.fromName}
                />
              </Field>
            </div>

            <Field>
              <FieldLabel htmlFor={`${fromEmailId}-test`}>
                测试收件邮箱
              </FieldLabel>
              <Input
                disabled={disabled}
                id={`${fromEmailId}-test`}
                onChange={(event) => setTestRecipient(event.target.value)}
                placeholder="admin@example.com"
                type="email"
                value={testRecipient}
              />
              <FieldDescription>
                测试使用已经保存的 SMTP 配置；修改配置后请先保存。
              </FieldDescription>
            </Field>
          </FieldGroup>

          <div className="flex justify-end gap-2">
            <Button
              disabled={disabled || testRecipient.trim() === ""}
              onClick={handleTestSMTP}
              type="button"
              variant="outline"
            >
              {isTesting ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SendIcon data-icon="inline-start" />
              )}
              发送测试邮件
            </Button>
            <Button disabled={disabled} type="submit">
              {isSaving ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SaveIcon data-icon="inline-start" />
              )}
              保存邮箱设置
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function getSMTPSecurityLabel(value: SMTPSecurity) {
  switch (value) {
    case "starttls":
      return "STARTTLS"
    case "tls":
      return "TLS（推荐）"
    case "none":
      return "无加密"
  }
}

function ThirdPartyProviderAddMenu({
  disabled,
  onSelect,
}: {
  disabled: boolean
  onSelect: (type: ThirdPartyLoginProviderType) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
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
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          {thirdPartyLoginProviderOptions.map((option) => (
            <DropdownMenuItem
              key={option.type}
              onClick={() => onSelect(option.type)}
            >
              {option.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ThirdPartyProviderActions({
  isFirst,
  isLast,
  isUpdating,
  onDelete,
  onEdit,
  onMove,
  onStatusChange,
  onViewCallback,
  provider,
}: {
  isFirst: boolean
  isLast: boolean
  isUpdating: boolean
  onDelete: (provider: ThirdPartyLoginProvider) => void
  onEdit: (provider: ThirdPartyLoginProvider) => void
  onMove: (
    provider: ThirdPartyLoginProvider,
    direction: ThirdPartyLoginProviderMoveDirection
  ) => void
  onStatusChange: (provider: ThirdPartyLoginProvider, enabled: boolean) => void
  onViewCallback: (provider: ThirdPartyLoginProvider) => void
  provider: ThirdPartyLoginProvider
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
          <DropdownMenuItem onClick={() => onViewCallback(provider)}>
            <EyeIcon data-icon="inline-start" />
            查看回调
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onEdit(provider)}
          >
            <PencilIcon data-icon="inline-start" />
            编辑
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || provider.enabled}
            onClick={() => onStatusChange(provider, true)}
          >
            <PowerIcon data-icon="inline-start" />
            启用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || !provider.enabled}
            onClick={() => onStatusChange(provider, false)}
          >
            <PowerOffIcon data-icon="inline-start" />
            禁用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || isFirst}
            onClick={() => onMove(provider, "up")}
          >
            <ArrowUpIcon data-icon="inline-start" />
            上移
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || isLast}
            onClick={() => onMove(provider, "down")}
          >
            <ArrowDownIcon data-icon="inline-start" />
            下移
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onDelete(provider)}
            variant="destructive"
          >
            <Trash2Icon data-icon="inline-start" />
            删除
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ThirdPartyCallbackURLDialog({
  onOpenChange,
  provider,
}: {
  onOpenChange: (open: boolean) => void
  provider: ThirdPartyLoginProvider | null
}) {
  const callbackURL = provider ? getThirdPartyCallbackURL(provider) : ""

  return (
    <Dialog onOpenChange={onOpenChange} open={provider !== null}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>第三方登录回调地址</DialogTitle>
        </DialogHeader>
        <div className="rounded-md border bg-muted/40 px-3 py-2 font-mono text-sm break-all">
          {callbackURL}
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button">
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ThirdPartyProviderDialog({
  disabled,
  editingProvider,
  onOpenChange,
  onProviderSaved,
  open,
  providerType,
}: {
  disabled: boolean
  editingProvider: ThirdPartyLoginProvider | null
  onOpenChange: (open: boolean) => void
  onProviderSaved: (provider: ThirdPartyLoginProvider) => void
  open: boolean
  providerType: ThirdPartyLoginProviderType
}) {
  const agentId = useId()
  const avatarFieldId = useId()
  const authorizeUrlId = useId()
  const clientIdId = useId()
  const clientSecretId = useId()
  const emailFieldId = useId()
  const externalIdFieldId = useId()
  const nameFieldId = useId()
  const nameId = useId()
  const nicknameFieldId = useId()
  const phoneFieldId = useId()
  const scopesId = useId()
  const tokenUrlId = useId()
  const userinfoUrlId = useId()
  const [form, setForm] = useState<ThirdPartyLoginProviderForm>(() =>
    createDefaultThirdPartyProviderForm(providerType)
  )
  const [isSaving, setIsSaving] = useState(false)
  const isEditing = editingProvider !== null
  const option = getThirdPartyProviderOption(form.type)
  const isOIDC = form.type === "oidc"
  const isWeCom = form.type === "wecom"
  const isSubmitDisabled =
    disabled ||
    isSaving ||
    form.name.trim() === "" ||
    form.clientId.trim() === "" ||
    form.clientSecret.trim() === "" ||
    (isWeCom && form.agentId.trim() === "") ||
    (isOIDC &&
      (form.authorizeUrl.trim() === "" ||
        form.tokenUrl.trim() === "" ||
        form.userinfoUrl.trim() === "" ||
        form.externalIdField.trim() === ""))

  useEffect(() => {
    if (!open) {
      return
    }

    setForm(
      editingProvider
        ? thirdPartyProviderToForm(editingProvider)
        : createDefaultThirdPartyProviderForm(providerType)
    )
  }, [editingProvider, open, providerType])

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
      const input = thirdPartyProviderFormToInput(form)
      const provider = editingProvider
        ? await updateThirdPartyLoginProvider(editingProvider.id, input)
        : await createThirdPartyLoginProvider(input)

      onProviderSaved(provider)
      toast.success("第三方登录方式已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存第三方登录方式失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent
        className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-2xl"
        showCloseButton={!isSaving}
      >
        <DialogHeader>
          <DialogTitle>
            {isEditing ? "编辑第三方登录" : `添加${option.label}登录`}
          </DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
          <FieldGroup className="gap-4">
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
            <div className="grid gap-4 md:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={clientIdId}>
                  {option.clientIdLabel}
                </FieldLabel>
                <Input
                  disabled={isSaving}
                  id={clientIdId}
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
                <FieldLabel htmlFor={clientSecretId}>
                  {option.secretLabel}
                </FieldLabel>
                <Input
                  disabled={isSaving}
                  id={clientSecretId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      clientSecret: event.target.value,
                    }))
                  }
                  value={form.clientSecret}
                />
              </Field>
            </div>
            {isWeCom && (
              <Field>
                <FieldLabel htmlFor={agentId}>Agent ID</FieldLabel>
                <Input
                  disabled={isSaving}
                  id={agentId}
                  onChange={(event) =>
                    setForm((currentForm) => ({
                      ...currentForm,
                      agentId: event.target.value,
                    }))
                  }
                  value={form.agentId}
                />
              </Field>
            )}
            {isOIDC && (
              <>
                <Field>
                  <FieldLabel htmlFor={authorizeUrlId}>
                    Authorize URL
                  </FieldLabel>
                  <Input
                    disabled={isSaving}
                    id={authorizeUrlId}
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
                  <FieldLabel htmlFor={tokenUrlId}>Access Token URL</FieldLabel>
                  <Input
                    disabled={isSaving}
                    id={tokenUrlId}
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
                  <FieldLabel htmlFor={userinfoUrlId}>用户信息 URL</FieldLabel>
                  <Input
                    disabled={isSaving}
                    id={userinfoUrlId}
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
                  <FieldLabel htmlFor={scopesId}>Scope</FieldLabel>
                  <Input
                    disabled={isSaving}
                    id={scopesId}
                    onChange={(event) =>
                      setForm((currentForm) => ({
                        ...currentForm,
                        scopesText: event.target.value,
                      }))
                    }
                    placeholder="openid,email,profile"
                    value={form.scopesText}
                  />
                </Field>
                <div className="grid gap-4 md:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor={externalIdFieldId}>
                      用户标识字段
                    </FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={externalIdFieldId}
                      onChange={(event) =>
                        setForm((currentForm) => ({
                          ...currentForm,
                          externalIdField: event.target.value,
                        }))
                      }
                      value={form.externalIdField}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={emailFieldId}>邮箱字段</FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={emailFieldId}
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
                    <FieldLabel htmlFor={phoneFieldId}>手机字段</FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={phoneFieldId}
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
                    <FieldLabel htmlFor={nameFieldId}>姓名字段</FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={nameFieldId}
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
                    <FieldLabel htmlFor={nicknameFieldId}>昵称字段</FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={nicknameFieldId}
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
                    <FieldLabel htmlFor={avatarFieldId}>头像字段</FieldLabel>
                    <Input
                      disabled={isSaving}
                      id={avatarFieldId}
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
              </>
            )}
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

export function getThirdPartyProviderTextClassName(enabled: boolean) {
  return enabled ? "min-w-0" : "min-w-0 text-muted-foreground"
}

export function getThirdPartyCallbackURL(
  provider: Pick<ThirdPartyLoginProvider, "callbackUrl">
) {
  return provider.callbackUrl
}

export function createDefaultThirdPartyProviderForm(
  type: ThirdPartyLoginProviderType = "oidc"
): ThirdPartyLoginProviderForm {
  const option = getThirdPartyProviderOption(type)

  return {
    agentId: "",
    avatarField: "picture",
    authorizeUrl: "",
    clientId: "",
    clientSecret: "",
    emailField: "email",
    externalIdField: "sub",
    name: option.name,
    nameField: "name",
    nicknameField: "nickname",
    phoneField: "phone",
    scopesText: type === "oidc" ? "openid,email,profile" : "",
    tokenUrl: "",
    type,
    userinfoUrl: "",
  }
}

export function thirdPartyProviderToForm(
  provider: ThirdPartyLoginProvider
): ThirdPartyLoginProviderForm {
  const baseForm = createDefaultThirdPartyProviderForm(provider.type)

  return {
    ...baseForm,
    agentId: provider.config.agent_id ?? "",
    avatarField: provider.config.avatar_field ?? baseForm.avatarField,
    authorizeUrl: provider.config.authorize_url ?? "",
    clientId: provider.clientId,
    clientSecret: provider.clientSecret,
    emailField: provider.config.email_field ?? baseForm.emailField,
    externalIdField:
      provider.config.external_id_field ?? baseForm.externalIdField,
    name: provider.name,
    nameField: provider.config.name_field ?? baseForm.nameField,
    nicknameField: provider.config.nickname_field ?? baseForm.nicknameField,
    phoneField: provider.config.phone_field ?? baseForm.phoneField,
    scopesText: provider.scopes.join(","),
    tokenUrl: provider.config.token_url ?? "",
    userinfoUrl: provider.config.userinfo_url ?? "",
  }
}

export function thirdPartyProviderFormToInput(
  form: ThirdPartyLoginProviderForm
): ThirdPartyLoginProviderInput {
  const config: Record<string, string> = {}
  if (form.type === "wecom") {
    config.agent_id = form.agentId.trim()
  }
  if (form.type === "oidc") {
    config.authorize_url = form.authorizeUrl.trim()
    config.token_url = form.tokenUrl.trim()
    config.userinfo_url = form.userinfoUrl.trim()
    config.external_id_field = form.externalIdField.trim()
    config.email_field = form.emailField.trim()
    config.phone_field = form.phoneField.trim()
    config.name_field = form.nameField.trim()
    config.nickname_field = form.nicknameField.trim()
    config.avatar_field = form.avatarField.trim()
  }

  return {
    clientId: form.clientId.trim(),
    clientSecret: form.clientSecret.trim(),
    config,
    name: form.name.trim(),
    scopes: form.scopesText
      .split(",")
      .map((scope) => scope.trim())
      .filter(Boolean),
    type: form.type,
  }
}

export function sortThirdPartyProvidersForDisplay(
  providers: ThirdPartyLoginProvider[]
) {
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

function getThirdPartyProviderOption(type: ThirdPartyLoginProviderType) {
  return (
    thirdPartyLoginProviderOptions.find((option) => option.type === type) ??
    thirdPartyLoginProviderOptions[thirdPartyLoginProviderOptions.length - 1]
  )
}

export type OIDCProviderForm = ThirdPartyLoginProviderForm
export const createDefaultOIDCProviderForm = createDefaultThirdPartyProviderForm
export const getOIDCCallbackURL = getThirdPartyCallbackURL
export const getOIDCProviderTextClassName = getThirdPartyProviderTextClassName
export const oidcProviderFormToInput = thirdPartyProviderFormToInput
export const oidcProviderToForm = thirdPartyProviderToForm
export const sortOIDCProvidersForDisplay = sortThirdPartyProvidersForDisplay
