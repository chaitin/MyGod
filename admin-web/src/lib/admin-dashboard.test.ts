import { describe, expect, it, vi } from "vitest"

import { getAdminDashboardStats } from "@/lib/admin-dashboard"

describe("admin dashboard", () => {
  it("loads dashboard statistics", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            total_users: 120,
            visited_users_24_hours: 32,
            visited_users_7_days: 86,
            online_users: 18,
            messages_24_hours: 326,
            messages_7_days: 1842,
            active_conversations_24_hours: 27,
            active_conversations_7_days: 74,
          },
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        }
      )
    )

    await expect(getAdminDashboardStats(fetcher)).resolves.toEqual({
      totalUsers: 120,
      visitedUsers24Hours: 32,
      visitedUsers7Days: 86,
      onlineUsers: 18,
      messages24Hours: 326,
      messages7Days: 1842,
      activeConversations24Hours: 27,
      activeConversations7Days: 74,
    })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/dashboard", {
      credentials: "include",
      method: "GET",
    })
  })

  it("rejects an incomplete dashboard response", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: {} }), {
        headers: { "content-type": "application/json" },
        status: 200,
      })
    )

    await expect(getAdminDashboardStats(fetcher)).rejects.toThrow(
      "仪表盘统计响应格式不正确"
    )
  })
})
