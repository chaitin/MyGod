import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import type { ClientUser } from "@/data/models"

type CurrentUserResponse = {
  user?: {
    avatar?: string
    created_at?: string
    email?: string
    id?: string
    last_online_at?: string | null
    name?: string
    nickname?: string
    phone?: string
    status?: string
  }
}

export async function fetchCurrentUser(
  serverUrl: string,
  options: { fetcher?: ApiFetch; signal?: AbortSignal } = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<
    CurrentUserResponse
  >("/api/client/me", {
    errorMessage: "加载当前用户失败",
    method: "GET",
    signal: options.signal,
  })
  const user = data?.user

  if (!user?.created_at || !user.email || !user.id || !user.name) {
    throw new ApiRequestError("当前用户响应格式不正确")
  }

  return {
    avatar: user.avatar ?? "",
    createdAt: user.created_at,
    email: user.email,
    id: user.id,
    lastOnlineAt: user.last_online_at ?? null,
    name: user.name,
    nickname: user.nickname ?? "",
    phone: user.phone ?? "",
    status: user.status === "disabled" ? "disabled" : "active",
  } satisfies ClientUser
}
