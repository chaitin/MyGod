import { adminFetch } from "@/lib/auth"

type AdminSettingsFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type AdminSettingsSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type AdminSettingsErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type InfoSettingsResponse = {
  app_name?: string
  organization_name?: string
}

type PasswordLoginSettingsResponse = {
  enabled?: boolean
}

type EmailLoginSettingsResponse = {
  enabled?: boolean
  from_email?: string
  from_name?: string
  smtp_host?: string
  smtp_password?: string
  smtp_password_configured?: boolean
  smtp_port?: number
  smtp_security?: SMTPSecurity
  smtp_username?: string
}

type ThirdPartyLoginProviderResponse = {
  callback_url?: string
  client_id?: string
  client_secret?: string
  config?: Record<string, unknown>
  enabled?: boolean
  id?: string
  key?: string
  name?: string
  scopes?: string[]
  sort_order?: number
  type?: ThirdPartyLoginProviderType
}

export type InfoSettings = {
  appName: string
  organizationName: string
}

export type PasswordLoginSettings = {
  enabled: boolean
}

export type SMTPSecurity = "none" | "starttls" | "tls"

export type EmailLoginSettings = {
  enabled: boolean
  fromEmail: string
  fromName: string
  smtpHost: string
  smtpPassword: string
  smtpPasswordConfigured: boolean
  smtpPort: number
  smtpSecurity: SMTPSecurity
  smtpUsername: string
}

export type UpdateEmailLoginSettingsInput = Omit<
  EmailLoginSettings,
  "smtpPasswordConfigured"
>

export type ThirdPartyLoginProviderType =
  | "dingtalk"
  | "feishu"
  | "github"
  | "google"
  | "oidc"
  | "wecom"

export type ThirdPartyLoginProvider = {
  callbackUrl: string
  clientId: string
  clientSecret: string
  config: Record<string, string>
  enabled: boolean
  id: string
  key: string
  name: string
  scopes: string[]
  sortOrder: number
  type: ThirdPartyLoginProviderType
}

export type UpdateInfoSettingsInput = {
  appName: string
  organizationName: string
}

export type ThirdPartyLoginProviderInput = Omit<
  ThirdPartyLoginProvider,
  "callbackUrl" | "enabled" | "id" | "key" | "sortOrder"
>
export type ThirdPartyLoginProviderMoveDirection = "down" | "up"

export class AdminSettingsRequestError extends Error {
  code?: string

  constructor(message: string, options?: { code?: string }) {
    super(message)
    this.name = "AdminSettingsRequestError"
    this.code = options?.code
  }
}

export async function getInfoSettings(
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/info", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<InfoSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载系统设置失败")
  }

  const data = (
    payload as AdminSettingsSuccessEnvelope<InfoSettingsResponse> | undefined
  )?.data

  return normalizeInfoSettings(data)
}

export async function updateInfoSettings(
  input: UpdateInfoSettingsInput,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/info", {
    body: JSON.stringify({
      app_name: input.appName.trim(),
      organization_name: input.organizationName.trim(),
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "PUT",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<InfoSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存系统设置失败")
  }

  const data = (
    payload as AdminSettingsSuccessEnvelope<InfoSettingsResponse> | undefined
  )?.data

  return normalizeInfoSettings(data)
}

export async function getEmailLoginSettings(
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/email-login", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<EmailLoginSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载邮箱登录设置失败")
  }

  const data = (
    payload as
      | AdminSettingsSuccessEnvelope<EmailLoginSettingsResponse>
      | undefined
  )?.data

  return normalizeEmailLoginSettings(data)
}

export async function getPasswordLoginSettings(
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/password-login", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<PasswordLoginSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载密码登录设置失败")
  }

  const data = (
    payload as
      | AdminSettingsSuccessEnvelope<PasswordLoginSettingsResponse>
      | undefined
  )?.data

  return normalizePasswordLoginSettings(data)
}

export async function updatePasswordLoginSettings(
  input: PasswordLoginSettings,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/password-login", {
    body: JSON.stringify({ enabled: input.enabled }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "PUT",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<PasswordLoginSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存密码登录设置失败")
  }

  const data = (
    payload as
      | AdminSettingsSuccessEnvelope<PasswordLoginSettingsResponse>
      | undefined
  )?.data

  return normalizePasswordLoginSettings(data)
}

export async function updateEmailLoginSettings(
  input: UpdateEmailLoginSettingsInput,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/email-login", {
    body: JSON.stringify({
      enabled: input.enabled,
      from_email: input.fromEmail.trim(),
      from_name: input.fromName.trim(),
      smtp_host: input.smtpHost.trim(),
      smtp_password: input.smtpPassword,
      smtp_port: input.smtpPort,
      smtp_security: input.smtpSecurity,
      smtp_username: input.smtpUsername.trim(),
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "PUT",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<EmailLoginSettingsResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存邮箱登录设置失败")
  }

  const data = (
    payload as
      | AdminSettingsSuccessEnvelope<EmailLoginSettingsResponse>
      | undefined
  )?.data

  return normalizeEmailLoginSettings(data)
}

export async function testEmailLoginSettings(
  recipientEmail: string,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/settings/email-login/test", {
    body: JSON.stringify({ recipient_email: recipientEmail.trim() }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "发送 SMTP 测试邮件失败")
  }
}

export async function listThirdPartyLoginProviders(
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher("/api/admin/third-party/providers", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<{
        providers?: ThirdPartyLoginProviderResponse[]
      }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载第三方登录方式失败")
  }

  const providers = (
    payload as
      | AdminSettingsSuccessEnvelope<{
          providers?: ThirdPartyLoginProviderResponse[]
        }>
      | undefined
  )?.data?.providers

  return normalizeThirdPartyLoginProviderList(providers)
}

export async function createThirdPartyLoginProvider(
  input: ThirdPartyLoginProviderInput,
  fetcher: AdminSettingsFetch = adminFetch
) {
  return saveThirdPartyLoginProvider(
    "/api/admin/third-party/providers",
    "POST",
    input,
    fetcher
  )
}

export async function updateThirdPartyLoginProvider(
  id: string,
  input: ThirdPartyLoginProviderInput,
  fetcher: AdminSettingsFetch = adminFetch
) {
  return saveThirdPartyLoginProvider(
    `/api/admin/third-party/providers/${encodeURIComponent(id)}`,
    "PUT",
    input,
    fetcher
  )
}

export async function deleteThirdPartyLoginProvider(
  id: string,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher(
    `/api/admin/third-party/providers/${encodeURIComponent(id)}`,
    {
      credentials: "include",
      method: "DELETE",
    }
  )
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "删除第三方登录方式失败")
  }
}

export async function enableThirdPartyLoginProvider(
  id: string,
  fetcher: AdminSettingsFetch = adminFetch
) {
  return updateThirdPartyLoginProviderStatus(id, "enable", fetcher)
}

export async function disableThirdPartyLoginProvider(
  id: string,
  fetcher: AdminSettingsFetch = adminFetch
) {
  return updateThirdPartyLoginProviderStatus(id, "disable", fetcher)
}

export async function moveThirdPartyLoginProvider(
  id: string,
  direction: ThirdPartyLoginProviderMoveDirection,
  fetcher: AdminSettingsFetch = adminFetch
) {
  const response = await fetcher(
    `/api/admin/third-party/providers/${encodeURIComponent(id)}/move`,
    {
      body: JSON.stringify({
        direction,
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<{
        providers?: ThirdPartyLoginProviderResponse[]
      }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "移动第三方登录方式失败")
  }

  const providers = (
    payload as
      | AdminSettingsSuccessEnvelope<{
          providers?: ThirdPartyLoginProviderResponse[]
        }>
      | undefined
  )?.data?.providers

  return normalizeThirdPartyLoginProviderList(providers)
}

async function saveThirdPartyLoginProvider(
  path: string,
  method: "POST" | "PUT",
  input: ThirdPartyLoginProviderInput,
  fetcher: AdminSettingsFetch
) {
  const response = await fetcher(path, {
    body: JSON.stringify(toThirdPartyLoginProviderRequest(input)),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method,
  })
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<{
        provider?: ThirdPartyLoginProviderResponse
      }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存第三方登录方式失败")
  }

  const provider = (
    payload as
      | AdminSettingsSuccessEnvelope<{
          provider?: ThirdPartyLoginProviderResponse
        }>
      | undefined
  )?.data?.provider

  return normalizeThirdPartyLoginProvider(provider)
}

async function updateThirdPartyLoginProviderStatus(
  id: string,
  action: "disable" | "enable",
  fetcher: AdminSettingsFetch
) {
  const response = await fetcher(
    `/api/admin/third-party/providers/${encodeURIComponent(id)}/${action}`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<{
        provider?: ThirdPartyLoginProviderResponse
      }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "更新第三方登录方式状态失败")
  }

  const provider = (
    payload as
      | AdminSettingsSuccessEnvelope<{
          provider?: ThirdPartyLoginProviderResponse
        }>
      | undefined
  )?.data?.provider

  return normalizeThirdPartyLoginProvider(provider)
}

function createRequestError(
  payload:
    | AdminSettingsErrorEnvelope
    | AdminSettingsSuccessEnvelope<unknown>
    | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as AdminSettingsErrorEnvelope | undefined)?.error

  return new AdminSettingsRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
    }
  )
}

function normalizeInfoSettings(
  settings: InfoSettingsResponse | undefined
): InfoSettings {
  if (!settings?.app_name || !settings.organization_name) {
    throw new AdminSettingsRequestError("系统设置响应格式不正确")
  }

  return {
    appName: settings.app_name,
    organizationName: settings.organization_name,
  }
}

function normalizePasswordLoginSettings(
  settings: PasswordLoginSettingsResponse | undefined
): PasswordLoginSettings {
  if (!settings || typeof settings.enabled !== "boolean") {
    throw new AdminSettingsRequestError("密码登录设置响应格式不正确")
  }

  return { enabled: settings.enabled }
}

function normalizeEmailLoginSettings(
  settings: EmailLoginSettingsResponse | undefined
): EmailLoginSettings {
  if (
    !settings ||
    typeof settings.enabled !== "boolean" ||
    typeof settings.smtp_host !== "string" ||
    typeof settings.smtp_port !== "number" ||
    !isSMTPSecurity(settings.smtp_security) ||
    typeof settings.smtp_username !== "string" ||
    typeof settings.smtp_password !== "string" ||
    typeof settings.smtp_password_configured !== "boolean" ||
    typeof settings.from_email !== "string" ||
    typeof settings.from_name !== "string"
  ) {
    throw new AdminSettingsRequestError("邮箱登录设置响应格式不正确")
  }

  return {
    enabled: settings.enabled,
    fromEmail: settings.from_email,
    fromName: settings.from_name,
    smtpHost: settings.smtp_host,
    smtpPassword: settings.smtp_password,
    smtpPasswordConfigured: settings.smtp_password_configured,
    smtpPort: settings.smtp_port,
    smtpSecurity: settings.smtp_security,
    smtpUsername: settings.smtp_username,
  }
}

function isSMTPSecurity(value: unknown): value is SMTPSecurity {
  return value === "none" || value === "starttls" || value === "tls"
}

function normalizeThirdPartyLoginProviderList(
  providers: ThirdPartyLoginProviderResponse[] | undefined
) {
  if (!Array.isArray(providers)) {
    throw new AdminSettingsRequestError("第三方登录方式响应格式不正确")
  }

  return providers.map(normalizeThirdPartyLoginProvider)
}

function normalizeThirdPartyLoginProvider(
  provider: ThirdPartyLoginProviderResponse | undefined
): ThirdPartyLoginProvider {
  if (
    !provider?.id ||
    !provider.name ||
    !provider.key ||
    !provider.callback_url ||
    !provider.type ||
    typeof provider.enabled !== "boolean" ||
    !provider.client_id ||
    !provider.client_secret ||
    !Array.isArray(provider.scopes) ||
    typeof provider.sort_order !== "number"
  ) {
    throw new AdminSettingsRequestError("第三方登录方式响应格式不正确")
  }

  return {
    callbackUrl: provider.callback_url,
    clientId: provider.client_id,
    clientSecret: provider.client_secret,
    config: normalizeStringRecord(provider.config ?? {}),
    enabled: provider.enabled,
    id: provider.id,
    key: provider.key,
    name: provider.name,
    scopes: provider.scopes,
    sortOrder: provider.sort_order,
    type: provider.type,
  }
}

function normalizeStringRecord(record: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(record).map(([key, value]) => [
      key,
      typeof value === "string" ? value : String(value ?? ""),
    ])
  )
}

function toThirdPartyLoginProviderRequest(input: ThirdPartyLoginProviderInput) {
  return {
    client_id: input.clientId.trim(),
    client_secret: input.clientSecret.trim(),
    config: Object.fromEntries(
      Object.entries(input.config)
        .map(([key, value]) => [key, value.trim()])
        .filter(([, value]) => value !== "")
    ),
    name: input.name.trim(),
    scopes: input.scopes.map((scope) => scope.trim()).filter(Boolean),
    type: input.type,
  }
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}

export type OIDCProvider = ThirdPartyLoginProvider
export type OIDCProviderInput = ThirdPartyLoginProviderInput
export type OIDCProviderMoveDirection = ThirdPartyLoginProviderMoveDirection
export const createOIDCProvider = createThirdPartyLoginProvider
export const deleteOIDCProvider = deleteThirdPartyLoginProvider
export const disableOIDCProvider = disableThirdPartyLoginProvider
export const enableOIDCProvider = enableThirdPartyLoginProvider
export const listOIDCProviders = listThirdPartyLoginProviders
export const moveOIDCProvider = moveThirdPartyLoginProvider
export const updateOIDCProvider = updateThirdPartyLoginProvider
export type ThirdPartyProvider = ThirdPartyLoginProvider
export type ThirdPartyProviderInput = ThirdPartyLoginProviderInput
export type ThirdPartyProviderMoveDirection =
  ThirdPartyLoginProviderMoveDirection
export const createThirdPartyProvider = createThirdPartyLoginProvider
export const deleteThirdPartyProvider = deleteThirdPartyLoginProvider
export const disableThirdPartyProvider = disableThirdPartyLoginProvider
export const enableThirdPartyProvider = enableThirdPartyLoginProvider
export const listThirdPartyProviders = listThirdPartyLoginProviders
export const moveThirdPartyProvider = moveThirdPartyLoginProvider
export const updateThirdPartyProvider = updateThirdPartyLoginProvider
