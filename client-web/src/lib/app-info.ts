type ClientInfoFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type ClientInfoSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type ClientInfoErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type ClientInfoResponse = {
  app_name?: string
  authenticated?: boolean
  oidc_providers?: ClientInfoThirdPartyProviderResponse[]
  organization_name?: string
  third_party_providers?: ClientInfoThirdPartyProviderResponse[]
}

type ClientInfoThirdPartyProviderResponse = {
  key?: string
  name?: string
}

export type AppInfoThirdPartyProvider = {
  key: string
  name: string
}

export type AppInfo = {
  appName: string
  authenticated: boolean
  oidcProviders: AppInfoThirdPartyProvider[]
  organizationName: string
  thirdPartyProviders: AppInfoThirdPartyProvider[]
}

export const defaultAppInfo: AppInfo = {
  appName: "MyGod",
  authenticated: false,
  oidcProviders: [],
  organizationName: "长亭科技",
  thirdPartyProviders: [],
}

export class ClientInfoRequestError extends Error {
  code?: string

  constructor(message: string, options?: { code?: string }) {
    super(message)
    this.name = "ClientInfoRequestError"
    this.code = options?.code
  }
}

export async function getClientInfo(fetcher: ClientInfoFetch = fetch) {
  const response = await fetcher("/api/client/info", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    ClientInfoErrorEnvelope | ClientInfoSuccessEnvelope<ClientInfoResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载应用信息失败")
  }

  const data = (
    payload as ClientInfoSuccessEnvelope<ClientInfoResponse> | undefined
  )?.data

  return normalizeClientInfo(data)
}

function createRequestError(
  payload:
    ClientInfoErrorEnvelope | ClientInfoSuccessEnvelope<unknown> | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as ClientInfoErrorEnvelope | undefined)?.error

  return new ClientInfoRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
    }
  )
}

function normalizeClientInfo(info: ClientInfoResponse | undefined): AppInfo {
  if (!info?.app_name || !info.organization_name) {
    throw new ClientInfoRequestError("应用信息响应格式不正确")
  }

  const thirdPartyProviders = normalizeThirdPartyProviders(
    info.third_party_providers ?? info.oidc_providers
  )

  return {
    appName: info.app_name,
    authenticated: info.authenticated === true,
    oidcProviders: thirdPartyProviders,
    organizationName: info.organization_name,
    thirdPartyProviders,
  }
}

function normalizeThirdPartyProviders(
  providers: ClientInfoThirdPartyProviderResponse[] | undefined
): AppInfoThirdPartyProvider[] {
  if (!providers) {
    return []
  }

  if (!Array.isArray(providers)) {
    throw new ClientInfoRequestError("应用信息响应格式不正确")
  }

  return providers.map((provider) => {
    if (!provider?.key || !provider.name) {
      throw new ClientInfoRequestError("应用信息响应格式不正确")
    }

    return {
      key: provider.key,
      name: provider.name,
    }
  })
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
