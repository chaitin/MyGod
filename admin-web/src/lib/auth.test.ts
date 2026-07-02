import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  AUTH_SESSION_KEY,
  ADMIN_UNAUTHORIZED_EVENT,
  AdminLoginRequestError,
  addAdminUnauthorizedListener,
  adminFetch,
  adminLogin,
  clearAuthSession,
  isAuthenticated,
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

describe("auth", () => {
  let storage: ReturnType<typeof createStorage>

  beforeEach(() => {
    storage = createStorage()
  })

  it("logs in through the admin API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            admin: {
              email: "admin",
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

    const admin = await adminLogin(
      {
        account: " admin ",
        password: "secret",
      },
      fetcher
    )

    expect(admin).toEqual({ email: "admin" })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/auth/login", {
      body: JSON.stringify({
        email: "admin",
        password: "secret",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("throws the admin API error message when login fails", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "unauthorized",
            message: "账号或密码不正确",
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

    await expect(
      adminLogin(
        {
          account: "admin",
          password: "wrong-password",
        },
        fetcher
      )
    ).rejects.toMatchObject({
      code: "unauthorized",
      message: "账号或密码不正确",
      name: "AdminLoginRequestError",
    } satisfies AdminLoginRequestError)
  })

  it("persists and clears the authenticated session", () => {
    expect(isAuthenticated(storage)).toBe(false)

    setAuthSession(storage)

    expect(storage.getItem(AUTH_SESSION_KEY)).toBe("true")
    expect(isAuthenticated(storage)).toBe(true)

    clearAuthSession(storage)

    expect(isAuthenticated(storage)).toBe(false)
  })

  it("clears the authenticated session and emits an event when an admin API returns 401", async () => {
    const eventTarget = new EventTarget()
    const listener = vi.fn()
    const fetcher = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 401 }))
    eventTarget.addEventListener(ADMIN_UNAUTHORIZED_EVENT, listener)
    setAuthSession(storage)

    const response = await adminFetch("/api/admin/users", undefined, {
      eventTarget,
      fetcher,
      storage,
    })

    expect(response.status).toBe(401)
    expect(isAuthenticated(storage)).toBe(false)
    expect(listener).toHaveBeenCalledOnce()
  })

  it("registers and removes admin unauthorized listeners", () => {
    const eventTarget = new EventTarget()
    const listener = vi.fn()
    const unsubscribe = addAdminUnauthorizedListener(listener, eventTarget)

    eventTarget.dispatchEvent(new Event(ADMIN_UNAUTHORIZED_EVENT))
    unsubscribe()
    eventTarget.dispatchEvent(new Event(ADMIN_UNAUTHORIZED_EVENT))

    expect(listener).toHaveBeenCalledOnce()
  })
})
