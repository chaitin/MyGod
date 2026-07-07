import { adminFetch } from "@/lib/auth"

type AdminAppsFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type AdminAppsSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type AdminAppsErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type AdminAppResponse = {
  avatar?: string
  callback_secret?: string
  callback_url?: string
  created_at?: string
  creator_user_id?: null | string
  description?: string
  enabled?: boolean
  id?: string
  name?: string
  system?: boolean
  updated_at?: string
  visibility?: string
}

export type AdminAppVisibility = "creator" | "public"

export type AdminApp = {
  avatar: string
  callbackSecret: string
  callbackUrl: string
  createdAt: string
  creatorUserId: null | string
  description: string
  enabled: boolean
  id: string
  name: string
  system: boolean
  updatedAt: string
  visibility: AdminAppVisibility
}

export type AdminAppInput = Pick<
  AdminApp,
  "avatar" | "callbackUrl" | "description" | "name" | "visibility"
>

export class AdminAppsRequestError extends Error {
  code?: string

  constructor(message: string, options?: { code?: string }) {
    super(message)
    this.name = "AdminAppsRequestError"
    this.code = options?.code
  }
}

export async function listAdminApps(fetcher: AdminAppsFetch = adminFetch) {
  const response = await fetcher("/api/admin/apps", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    | AdminAppsErrorEnvelope
    | AdminAppsSuccessEnvelope<{ apps?: AdminAppResponse[] }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载应用失败")
  }

  const apps = (
    payload as
      | AdminAppsSuccessEnvelope<{ apps?: AdminAppResponse[] }>
      | undefined
  )?.data?.apps

  return normalizeAdminAppList(apps)
}

export async function createAdminApp(
  input: AdminAppInput,
  fetcher: AdminAppsFetch = adminFetch
) {
  return saveAdminApp("/api/admin/apps", "POST", input, fetcher)
}

export async function updateAdminApp(
  id: string,
  input: AdminAppInput,
  fetcher: AdminAppsFetch = adminFetch
) {
  return saveAdminApp(
    `/api/admin/apps/${encodeURIComponent(id)}`,
    "PUT",
    input,
    fetcher
  )
}

export async function deleteAdminApp(
  id: string,
  fetcher: AdminAppsFetch = adminFetch
) {
  const response = await fetcher(`/api/admin/apps/${encodeURIComponent(id)}`, {
    credentials: "include",
    method: "DELETE",
  })
  const payload = await readJson<
    AdminAppsErrorEnvelope | AdminAppsSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "删除应用失败")
  }
}

export async function enableAdminApp(
  id: string,
  fetcher: AdminAppsFetch = adminFetch
) {
  return updateAdminAppStatus(id, "enable", fetcher)
}

export async function disableAdminApp(
  id: string,
  fetcher: AdminAppsFetch = adminFetch
) {
  return updateAdminAppStatus(id, "disable", fetcher)
}

export async function regenerateAdminAppSecret(
  id: string,
  fetcher: AdminAppsFetch = adminFetch
) {
  const response = await fetcher(
    `/api/admin/apps/${encodeURIComponent(id)}/secret/regenerate`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminAppsErrorEnvelope
    | AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "生成密钥失败")
  }

  const app = (
    payload as AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }> | undefined
  )?.data?.app

  return normalizeAdminApp(app)
}

async function saveAdminApp(
  path: string,
  method: "POST" | "PUT",
  input: AdminAppInput,
  fetcher: AdminAppsFetch
) {
  const response = await fetcher(path, {
    body: JSON.stringify(toAdminAppRequest(input)),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method,
  })
  const payload = await readJson<
    | AdminAppsErrorEnvelope
    | AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存应用失败")
  }

  const app = (
    payload as AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }> | undefined
  )?.data?.app

  return normalizeAdminApp(app)
}

async function updateAdminAppStatus(
  id: string,
  operation: "disable" | "enable",
  fetcher: AdminAppsFetch
) {
  const response = await fetcher(
    `/api/admin/apps/${encodeURIComponent(id)}/${operation}`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminAppsErrorEnvelope
    | AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "更新应用状态失败")
  }

  const app = (
    payload as AdminAppsSuccessEnvelope<{ app?: AdminAppResponse }> | undefined
  )?.data?.app

  return normalizeAdminApp(app)
}

function toAdminAppRequest(input: AdminAppInput) {
  return {
    avatar: input.avatar.trim(),
    callback_url: input.callbackUrl.trim(),
    description: input.description.trim(),
    name: input.name.trim(),
    visibility: input.visibility,
  }
}

function normalizeAdminAppList(
  apps: AdminAppResponse[] | undefined
): AdminApp[] {
  if (!Array.isArray(apps)) {
    throw new AdminAppsRequestError("应用列表响应格式不正确")
  }

  return apps.map(normalizeAdminApp)
}

function normalizeAdminApp(app: AdminAppResponse | undefined): AdminApp {
  if (
    !app ||
    typeof app.avatar !== "string" ||
    typeof app.callback_secret !== "string" ||
    typeof app.callback_url !== "string" ||
    typeof app.created_at !== "string" ||
    typeof app.description !== "string" ||
    typeof app.enabled !== "boolean" ||
    typeof app.id !== "string" ||
    typeof app.name !== "string" ||
    typeof app.system !== "boolean" ||
    typeof app.updated_at !== "string" ||
    typeof app.visibility !== "string"
  ) {
    throw new AdminAppsRequestError("应用响应格式不正确")
  }
  if (!isAdminAppVisibility(app.visibility)) {
    throw new AdminAppsRequestError("应用可见范围响应格式不正确")
  }
  if (
    app.creator_user_id !== null &&
    app.creator_user_id !== undefined &&
    typeof app.creator_user_id !== "string"
  ) {
    throw new AdminAppsRequestError("应用创建者响应格式不正确")
  }

  return {
    avatar: app.avatar,
    callbackSecret: app.callback_secret,
    callbackUrl: app.callback_url,
    createdAt: app.created_at,
    creatorUserId: app.creator_user_id ?? null,
    description: app.description,
    enabled: app.enabled,
    id: app.id,
    name: app.name,
    system: app.system,
    updatedAt: app.updated_at,
    visibility: app.visibility,
  }
}

function isAdminAppVisibility(value: string): value is AdminAppVisibility {
  return value === "creator" || value === "public"
}

function createRequestError(
  payload:
    | AdminAppsErrorEnvelope
    | AdminAppsSuccessEnvelope<unknown>
    | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as AdminAppsErrorEnvelope | undefined)?.error

  return new AdminAppsRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
    }
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
