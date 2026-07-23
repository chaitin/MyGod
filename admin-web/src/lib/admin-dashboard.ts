import { adminFetch } from "@/lib/auth"

type AdminDashboardFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type DashboardStatsResponse = {
  total_users?: number
  visited_users_24_hours?: number
  visited_users_7_days?: number
  online_users?: number
  messages_24_hours?: number
  messages_7_days?: number
  active_conversations_24_hours?: number
  active_conversations_7_days?: number
}

type DashboardSuccessEnvelope = {
  data?: DashboardStatsResponse
  success?: boolean
}

type DashboardErrorEnvelope = {
  error?: { code?: string; message?: string }
  success?: boolean
}

export type AdminDashboardStats = {
  totalUsers: number
  visitedUsers24Hours: number
  visitedUsers7Days: number
  onlineUsers: number
  messages24Hours: number
  messages7Days: number
  activeConversations24Hours: number
  activeConversations7Days: number
}

export class AdminDashboardRequestError extends Error {
  code?: string

  constructor(message: string, options?: { code?: string }) {
    super(message)
    this.name = "AdminDashboardRequestError"
    this.code = options?.code
  }
}

export async function getAdminDashboardStats(
  fetcher: AdminDashboardFetch = adminFetch
): Promise<AdminDashboardStats> {
  const response = await fetcher("/api/admin/dashboard", {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    DashboardErrorEnvelope | DashboardSuccessEnvelope
  >(response)

  if (!response.ok || payload?.success === false) {
    const error = (payload as DashboardErrorEnvelope | undefined)?.error
    throw new AdminDashboardRequestError(
      error?.message ?? `加载仪表盘统计失败（HTTP ${response.status}）`,
      { code: error?.code }
    )
  }

  const data = (payload as DashboardSuccessEnvelope | undefined)?.data
  if (!isDashboardStatsResponse(data)) {
    throw new AdminDashboardRequestError("仪表盘统计响应格式不正确")
  }

  return {
    totalUsers: data.total_users,
    visitedUsers24Hours: data.visited_users_24_hours,
    visitedUsers7Days: data.visited_users_7_days,
    onlineUsers: data.online_users,
    messages24Hours: data.messages_24_hours,
    messages7Days: data.messages_7_days,
    activeConversations24Hours: data.active_conversations_24_hours,
    activeConversations7Days: data.active_conversations_7_days,
  }
}

function isDashboardStatsResponse(
  value: DashboardStatsResponse | undefined
): value is Required<DashboardStatsResponse> {
  return (
    value !== undefined &&
    [
      value.total_users,
      value.visited_users_24_hours,
      value.visited_users_7_days,
      value.online_users,
      value.messages_24_hours,
      value.messages_7_days,
      value.active_conversations_24_hours,
      value.active_conversations_7_days,
    ].every((item) => typeof item === "number" && Number.isFinite(item))
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")
  if (!contentType?.includes("application/json")) {
    return undefined
  }
  return response.json() as Promise<T>
}
