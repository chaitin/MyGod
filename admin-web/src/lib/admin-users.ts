import { adminFetch } from "@/lib/auth"

type AdminUsersFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type AdminUsersSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type AdminUsersErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type AdminUserResponse = {
  created_at?: string
  email?: string
  id?: string
  name?: string
  status?: string
}

type CreateAdminUserResponse = {
  initial_password?: string
  user?: AdminUserResponse
}

type ListAdminUsersResponse = {
  order?: string
  page?: number
  page_size?: number
  sort?: string
  total?: number
  users?: AdminUserResponse[]
}

type ResetAdminUserPasswordResponse = {
  new_password?: string
  user?: AdminUserResponse
}

type UpdateAdminUserStatusResponse = {
  user?: AdminUserResponse
}

export type AdminUser = {
  createdAt: string
  email: string
  id: string
  name: string
  status: "active" | "disabled"
}

export type AdminUsersSort = "created_at" | "email" | "status"
export type AdminUsersSortOrder = "asc" | "desc"

export type CreateAdminUserInput = {
  email: string
  name: string
}

export type ListAdminUsersInput = {
  keyword?: string
  order?: AdminUsersSortOrder
  page?: number
  pageSize?: number
  sort?: AdminUsersSort
}

export class AdminUserRequestError extends Error {
  code?: string

  constructor(message: string, options?: { code?: string }) {
    super(message)
    this.name = "AdminUserRequestError"
    this.code = options?.code
  }
}

export async function listAdminUsers(
  input: ListAdminUsersInput = {},
  fetcher: AdminUsersFetch = adminFetch
) {
  const params = new URLSearchParams()
  const keyword = input.keyword?.trim()

  if (keyword) {
    params.set("keyword", keyword)
  }

  if (input.page !== undefined) {
    params.set("page", String(input.page))
  }

  if (input.pageSize !== undefined) {
    params.set("page_size", String(input.pageSize))
  }

  if (input.sort) {
    params.set("sort", input.sort)
  }

  if (input.order) {
    params.set("order", input.order)
  }

  const query = params.toString()
  const response = await fetcher(
    `/api/admin/users${query ? `?${query}` : ""}`,
    {
      credentials: "include",
      method: "GET",
    }
  )
  const payload = await readJson<
    AdminUsersErrorEnvelope | AdminUsersSuccessEnvelope<ListAdminUsersResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "加载成员失败")
  }

  const data = (
    payload as AdminUsersSuccessEnvelope<ListAdminUsersResponse> | undefined
  )?.data

  if (!data?.users) {
    throw new AdminUserRequestError("成员列表响应格式不正确")
  }

  return {
    order: data.order ?? input.order ?? "desc",
    page: data.page ?? input.page ?? 1,
    pageSize: data.page_size ?? input.pageSize ?? data.users.length,
    sort: data.sort ?? input.sort ?? "created_at",
    total: data.total ?? data.users.length,
    users: data.users.map(normalizeAdminUser),
  }
}

export async function createAdminUser(
  input: CreateAdminUserInput,
  fetcher: AdminUsersFetch = adminFetch
) {
  const response = await fetcher("/api/admin/users", {
    body: JSON.stringify({
      email: input.email.trim(),
      name: input.name.trim(),
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    AdminUsersErrorEnvelope | AdminUsersSuccessEnvelope<CreateAdminUserResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "添加成员失败")
  }

  const data = (
    payload as AdminUsersSuccessEnvelope<CreateAdminUserResponse> | undefined
  )?.data
  const user = data?.user

  if (!user) {
    throw new AdminUserRequestError("添加成员响应格式不正确")
  }

  return {
    initialPassword: data?.initial_password ?? "",
    user: normalizeAdminUser(user),
  }
}

export async function resetAdminUserPassword(
  id: string,
  fetcher: AdminUsersFetch = adminFetch
) {
  const response = await fetcher(
    `/api/admin/users/${encodeURIComponent(id)}/reset-password`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminUsersErrorEnvelope
    | AdminUsersSuccessEnvelope<ResetAdminUserPasswordResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "重置密码失败")
  }

  const data = (
    payload as
      | AdminUsersSuccessEnvelope<ResetAdminUserPasswordResponse>
      | undefined
  )?.data
  const user = data?.user

  if (!user || !data?.new_password) {
    throw new AdminUserRequestError("重置密码响应格式不正确")
  }

  return {
    newPassword: data.new_password,
    user: normalizeAdminUser(user),
  }
}

export async function updateAdminUserStatus(
  id: string,
  status: AdminUser["status"],
  fetcher: AdminUsersFetch = adminFetch
) {
  const action = status === "active" ? "enable" : "disable"
  const response = await fetcher(
    `/api/admin/users/${encodeURIComponent(id)}/${action}`,
    {
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    | AdminUsersErrorEnvelope
    | AdminUsersSuccessEnvelope<UpdateAdminUserStatusResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createRequestError(payload, response, "更新成员状态失败")
  }

  const user = (
    payload as
      | AdminUsersSuccessEnvelope<UpdateAdminUserStatusResponse>
      | undefined
  )?.data?.user

  if (!user) {
    throw new AdminUserRequestError("成员状态响应格式不正确")
  }

  return normalizeAdminUser(user)
}

function createRequestError(
  payload:
    | AdminUsersErrorEnvelope
    | AdminUsersSuccessEnvelope<unknown>
    | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as AdminUsersErrorEnvelope | undefined)?.error

  return new AdminUserRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
    }
  )
}

function normalizeAdminUser(user: AdminUserResponse): AdminUser {
  if (!user.created_at || !user.email || !user.id || !user.name) {
    throw new AdminUserRequestError("添加成员响应格式不正确")
  }

  return {
    createdAt: user.created_at,
    email: user.email,
    id: user.id,
    name: user.name,
    status: user.status === "disabled" ? "disabled" : "active",
  }
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
