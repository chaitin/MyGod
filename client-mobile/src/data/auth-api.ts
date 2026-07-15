import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import type { AuthenticatedUser } from "@/data/models"

type LoginResponse = {
  user?: {
    email?: string
    id?: string
    name?: string
  }
}

export async function login(
  serverUrl: string,
  input: { account: string; password: string },
  options: { fetcher?: ApiFetch } = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<
    LoginResponse
  >("/api/client/auth/login", {
    body: JSON.stringify({
      email: input.account.trim(),
      password: input.password,
    }),
    errorMessage: "登录失败",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const user = data?.user

  if (!user?.email || !user.id || !user.name) {
    throw new ApiRequestError("登录响应格式不正确")
  }

  return {
    email: user.email,
    id: user.id,
    name: user.name,
  } satisfies AuthenticatedUser
}
