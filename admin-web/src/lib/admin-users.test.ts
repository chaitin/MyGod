import { afterEach, describe, expect, it, vi } from "vitest"

import {
  AdminUserRequestError,
  createAdminUser,
  listAdminUsers,
  resetAdminUserPassword,
  updateAdminUserStatus,
} from "@/lib/admin-users"
import {
  ADMIN_UNAUTHORIZED_EVENT,
  AUTH_SESSION_KEY,
  setAuthSession,
} from "@/lib/auth"

function createStorage() {
  const values = new Map<string, string>()

  return {
    getItem: (key: string) => values.get(key) ?? null,
    removeItem: (key: string) => {
      values.delete(key)
    },
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

describe("admin users", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("lists members through the admin users API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            users: [
              {
                id: "user-1",
                email: "alice@example.com",
                name: "Alice",
                status: "disabled",
                created_at: "2026-07-01T12:34:56Z",
              },
            ],
            total: 12,
            page: 2,
            page_size: 50,
            sort: "email",
            order: "asc",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    const result = await listAdminUsers(
      {
        keyword: " alice ",
        order: "asc",
        page: 2,
        pageSize: 50,
        sort: "email",
      },
      fetcher
    )

    expect(result).toEqual({
      order: "asc",
      page: 2,
      pageSize: 50,
      sort: "email",
      total: 12,
      users: [
        {
          createdAt: "2026-07-01T12:34:56Z",
          email: "alice@example.com",
          id: "user-1",
          name: "Alice",
          status: "disabled",
        },
      ],
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/admin/users?keyword=alice&page=2&page_size=50&sort=email&order=asc",
      {
        credentials: "include",
        method: "GET",
      }
    )
  })

  it("creates a member through the admin users API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              id: "user-1",
              email: "new@example.com",
              name: "New User",
              status: "active",
              created_at: "2026-07-01T12:34:56Z",
            },
            initial_password: "initial-secret",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 201,
        }
      )
    )

    const created = await createAdminUser(
      {
        email: " new@example.com ",
        name: " New User ",
      },
      fetcher
    )

    expect(created).toEqual({
      initialPassword: "initial-secret",
      user: {
        createdAt: "2026-07-01T12:34:56Z",
        email: "new@example.com",
        id: "user-1",
        name: "New User",
        status: "active",
      },
    })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/users", {
      body: JSON.stringify({
        email: "new@example.com",
        name: "New User",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("throws the admin API error message when member creation fails", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "conflict",
            message: "邮箱已存在",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 409,
        }
      )
    )

    await expect(
      createAdminUser(
        {
          email: "duplicate@example.com",
          name: "Duplicate User",
        },
        fetcher
      )
    ).rejects.toMatchObject({
      code: "conflict",
      message: "邮箱已存在",
      name: "AdminUserRequestError",
    } satisfies AdminUserRequestError)
  })

  it("uses the shared admin fetcher for unauthorized admin API responses", async () => {
    const storage = createStorage()
    const eventTarget = new EventTarget()
    const listener = vi.fn()
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "unauthorized",
            message: "未登录",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 401,
        }
      )
    )
    eventTarget.addEventListener(ADMIN_UNAUTHORIZED_EVENT, listener)
    setAuthSession(storage)
    vi.stubGlobal(
      "window",
      Object.assign(eventTarget, {
        localStorage: storage,
      })
    )
    vi.stubGlobal("fetch", fetcher)

    await expect(listAdminUsers()).rejects.toMatchObject({
      code: "unauthorized",
      message: "未登录",
      name: "AdminUserRequestError",
    } satisfies AdminUserRequestError)

    expect(storage.getItem(AUTH_SESSION_KEY)).toBeNull()
    expect(listener).toHaveBeenCalledOnce()
  })

  it("enables a member through the admin users API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              id: "user-1",
              email: "alice@example.com",
              name: "Alice",
              status: "active",
              created_at: "2026-07-01T12:34:56Z",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    const user = await updateAdminUserStatus("user-1", "active", fetcher)

    expect(user).toEqual({
      createdAt: "2026-07-01T12:34:56Z",
      email: "alice@example.com",
      id: "user-1",
      name: "Alice",
      status: "active",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/users/user-1/enable", {
      credentials: "include",
      method: "POST",
    })
  })

  it("disables a member through the admin users API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              id: "user-1",
              email: "alice@example.com",
              name: "Alice",
              status: "disabled",
              created_at: "2026-07-01T12:34:56Z",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await updateAdminUserStatus("user-1", "disabled", fetcher)

    expect(fetcher).toHaveBeenCalledWith("/api/admin/users/user-1/disable", {
      credentials: "include",
      method: "POST",
    })
  })

  it("resets a member password through the admin users API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              id: "user-1",
              email: "alice@example.com",
              name: "Alice",
              status: "active",
              created_at: "2026-07-01T12:34:56Z",
            },
            new_password: "new-secret",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    const result = await resetAdminUserPassword("user-1", fetcher)

    expect(result).toEqual({
      newPassword: "new-secret",
      user: {
        createdAt: "2026-07-01T12:34:56Z",
        email: "alice@example.com",
        id: "user-1",
        name: "Alice",
        status: "active",
      },
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/admin/users/user-1/reset-password",
      {
        credentials: "include",
        method: "POST",
      }
    )
  })
})
