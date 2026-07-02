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

export type InfoSettings = {
  appName: string
  organizationName: string
}

export type UpdateInfoSettingsInput = {
  appName: string
  organizationName: string
}

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

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
