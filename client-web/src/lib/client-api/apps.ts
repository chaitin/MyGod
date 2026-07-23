import { ClientDataRequestError, createRequestError, readJson } from "./core"
import type {
  ClientDataErrorEnvelope,
  ClientDataFetch,
  ClientDataSuccessEnvelope,
} from "./types"

export type ClientAppVisibility = "creator" | "public" | "restricted"

export type ClientOwnedApp = {
  avatar: string
  connectionStatus: "disabled" | "offline" | "online"
  createdAt: string
  description: string
  enabled: boolean
  id: string
  name: string
  updatedAt: string
  userIds: string[]
  visibility: ClientAppVisibility
}

export type ClientAppCredentials = {
  app: ClientOwnedApp
  connectionSecret: string
}

export type CreateClientAppInput = {
  description: string
  name: string
  userIds: string[]
  visibility: ClientAppVisibility
}

export type UpdateClientAppInput = {
  description?: string
  name?: string
  userIds?: string[]
  visibility?: ClientAppVisibility
}

type ClientAppResponse = {
  avatar?: string
  connection_status?: string
  created_at?: string
  description?: string
  enabled?: boolean
  id?: string
  name?: string
  updated_at?: string
  user_ids?: string[]
  visibility?: string
}

type ClientAppEnvelope = {
  app?: ClientAppResponse
}

type ClientAppCredentialEnvelope = ClientAppEnvelope & {
  connection_secret?: string
}

export async function createClientApp(
  input: CreateClientAppInput,
  fetcher: ClientDataFetch = fetch
): Promise<ClientAppCredentials> {
  const response = await fetcher("/api/client/apps", {
    body: JSON.stringify({
      description: input.description,
      name: input.name,
      user_ids: input.visibility === "restricted" ? input.userIds : [],
      visibility: input.visibility,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ClientAppCredentialEnvelope>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "创建应用失败")
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<ClientAppCredentialEnvelope> | undefined
  )?.data

  if (!data?.connection_secret) {
    throw new ClientDataRequestError("创建应用响应格式不正确")
  }

  return {
    app: normalizeClientApp(data.app),
    connectionSecret: data.connection_secret,
  }
}

export async function getClientAppCredentials(
  appId: string,
  fetcher: ClientDataFetch = fetch
) {
  return requestClientAppCredentials(
    `/api/client/apps/${encodeURIComponent(appId)}`,
    "GET",
    "加载开发指南失败",
    fetcher
  )
}

export async function getClientAppProfile(
  appId: string,
  fetcher: ClientDataFetch = fetch
) {
  const credentials = await requestClientAppCredentials(
    `/api/client/apps/${encodeURIComponent(appId)}`,
    "GET",
    "加载应用资料失败",
    fetcher
  )

  return credentials.app
}

export async function regenerateClientAppSecret(
  appId: string,
  fetcher: ClientDataFetch = fetch
) {
  return requestClientAppCredentials(
    `/api/client/apps/${encodeURIComponent(appId)}/secret/regenerate`,
    "POST",
    "重置连接密钥失败",
    fetcher
  )
}

export async function updateClientApp(
  appId: string,
  input: UpdateClientAppInput,
  fetcher: ClientDataFetch = fetch
) {
  const body: Record<string, unknown> = {}
  if (input.description !== undefined) {
    body.description = input.description
  }
  if (input.name !== undefined) {
    body.name = input.name
  }
  if (input.userIds !== undefined) {
    body.user_ids = input.userIds
  }
  if (input.visibility !== undefined) {
    body.visibility = input.visibility
  }

  const response = await fetcher(
    `/api/client/apps/${encodeURIComponent(appId)}`,
    {
      body: JSON.stringify(body),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<ClientAppEnvelope>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "保存应用信息失败")
  }

  const data = (
    payload as ClientDataSuccessEnvelope<ClientAppEnvelope> | undefined
  )?.data

  return normalizeClientApp(data?.app)
}

export async function deleteClientApp(
  appId: string,
  fetcher: ClientDataFetch = fetch
) {
  const response = await fetcher(
    `/api/client/apps/${encodeURIComponent(appId)}`,
    {
      credentials: "include",
      method: "DELETE",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "删除应用失败")
  }
}

export async function uploadClientAppAvatar(
  appId: string,
  file: File,
  fetcher: ClientDataFetch = fetch
) {
  const formData = new FormData()
  formData.set("file", file)
  const response = await fetcher(
    `/api/client/apps/${encodeURIComponent(appId)}/avatar`,
    {
      body: formData,
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    ClientDataErrorEnvelope | ClientDataSuccessEnvelope<ClientAppEnvelope>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "上传应用头像失败")
  }

  const data = (
    payload as ClientDataSuccessEnvelope<ClientAppEnvelope> | undefined
  )?.data

  return normalizeClientApp(data?.app)
}

export function buildAppWebSocketURL(
  location: Pick<Location | URL, "host" | "protocol">
) {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"

  return `${protocol}//${location.host}/api/app/ws`
}

async function requestClientAppCredentials(
  url: string,
  method: "GET" | "POST",
  fallbackMessage: string,
  fetcher: ClientDataFetch
): Promise<ClientAppCredentials> {
  const response = await fetcher(url, {
    credentials: "include",
    method,
  })
  const payload = await readJson<
    | ClientDataErrorEnvelope
    | ClientDataSuccessEnvelope<ClientAppCredentialEnvelope>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, fallbackMessage)
  }

  const data = (
    payload as
      ClientDataSuccessEnvelope<ClientAppCredentialEnvelope> | undefined
  )?.data

  if (!data?.connection_secret) {
    throw new ClientDataRequestError("应用接入信息响应格式不正确")
  }

  return {
    app: normalizeClientApp(data.app),
    connectionSecret: data.connection_secret,
  }
}

function normalizeClientApp(
  app: ClientAppResponse | undefined
): ClientOwnedApp {
  if (
    !app?.created_at ||
    !app.id ||
    !app.name ||
    !app.updated_at ||
    !Array.isArray(app.user_ids)
  ) {
    throw new ClientDataRequestError("应用响应格式不正确")
  }

  return {
    avatar: app.avatar ?? "",
    connectionStatus: normalizeConnectionStatus(app.connection_status),
    createdAt: app.created_at,
    description: app.description ?? "",
    enabled: app.enabled !== false,
    id: app.id,
    name: app.name,
    updatedAt: app.updated_at,
    userIds: app.user_ids.filter((value): value is string =>
      Boolean(value && typeof value === "string")
    ),
    visibility: normalizeAppVisibility(app.visibility),
  }
}

function normalizeConnectionStatus(
  value: string | undefined
): ClientOwnedApp["connectionStatus"] {
  if (value === "online" || value === "disabled") {
    return value
  }

  return "offline"
}

function normalizeAppVisibility(
  value: string | undefined
): ClientAppVisibility {
  if (value === "public" || value === "restricted") {
    return value
  }

  return "creator"
}
