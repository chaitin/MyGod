import {
  ApiRequestError,
  createApiClient,
  type ApiFetch,
} from "@/data/api-client"
import type { AppInfo } from "@/data/models"

type AppInfoResponse = {
  app_name?: string
  authenticated?: boolean
  organization_name?: string
}

export async function fetchAppInfo(
  serverUrl: string,
  options: { fetcher?: ApiFetch; signal?: AbortSignal } = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<
    AppInfoResponse
  >("/api/client/info", {
    errorMessage: "加载服务器信息失败",
    method: "GET",
    signal: options.signal,
  })
  const appName = data?.app_name?.trim()
  const organizationName = data?.organization_name?.trim()

  if (!appName || !organizationName) {
    throw new ApiRequestError("服务器信息响应格式不正确")
  }

  return {
    appName,
    authenticated: data?.authenticated === true,
    organizationName,
  } satisfies AppInfo
}
