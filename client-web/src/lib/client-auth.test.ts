import { describe, expect, it, vi } from "vitest"

import { ClientLoginRequestError, clientLogin } from "@/lib/client-auth"

describe("client auth", () => {
  it("logs in through the client auth API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              created_at: "2026-07-01T00:00:00Z",
              email: "alice@example.com",
              id: "user-1",
              name: "Alice",
              status: "active",
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

    const user = await clientLogin(
      {
        account: " Alice@Example.com ",
        password: "secret",
      },
      fetcher
    )

    expect(user).toEqual({
      email: "alice@example.com",
      id: "user-1",
      name: "Alice",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/auth/login", {
      body: JSON.stringify({
        email: "Alice@Example.com",
        password: "secret",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("throws the client API error message when login fails", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "invalid_credentials",
            message: "邮箱或密码错误",
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
      clientLogin(
        {
          account: "alice@example.com",
          password: "wrong",
        },
        fetcher
      )
    ).rejects.toMatchObject({
      code: "invalid_credentials",
      message: "邮箱或密码错误",
      name: "ClientLoginRequestError",
    } satisfies ClientLoginRequestError)
  })
})
