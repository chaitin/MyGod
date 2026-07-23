import { describe, expect, it, vi } from "vitest"

import {
  buildAppWebSocketURL,
  createClientApp,
  deleteClientApp,
  getClientAppCredentials,
  getClientAppProfile,
  regenerateClientAppSecret,
  updateClientApp,
  uploadClientAppAvatar,
} from "@/lib/client-api/apps"

describe("client app API", () => {
  it("creates an application and returns its connection secret", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: createAppResponse(),
          connection_secret: "app-secret",
        },
      })
    )

    await expect(
      createClientApp(
        {
          description: "生成业务报表",
          name: "报表机器人",
          userIds: ["user-2"],
          visibility: "restricted",
        },
        fetcher
      )
    ).resolves.toEqual({
      app: {
        avatar: "",
        connectionStatus: "offline",
        createdAt: "2026-07-17T07:00:00Z",
        description: "生成业务报表",
        enabled: true,
        id: "app-1",
        name: "报表机器人",
        updatedAt: "2026-07-17T07:00:00Z",
        userIds: ["user-2"],
        visibility: "restricted",
      },
      connectionSecret: "app-secret",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/apps", {
      body: JSON.stringify({
        description: "生成业务报表",
        name: "报表机器人",
        user_ids: ["user-2"],
        visibility: "restricted",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("uploads an application avatar through the dedicated endpoint", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: {
            ...createAppResponse(),
            avatar: "https://files.example.test/apps/app-1.webp",
          },
        },
      })
    )
    const file = new File(["avatar"], "avatar.webp", {
      type: "image/webp",
    })

    await expect(
      uploadClientAppAvatar("app/1", file, fetcher)
    ).resolves.toMatchObject({
      avatar: "https://files.example.test/apps/app-1.webp",
      id: "app-1",
    })
    const [, init] = fetcher.mock.calls[0]
    expect(fetcher.mock.calls[0][0]).toBe("/api/client/apps/app%2F1/avatar")
    expect(init).toMatchObject({
      credentials: "include",
      method: "POST",
    })
    expect(init.body).toBeInstanceOf(FormData)
    expect((init.body as FormData).get("file")).toBe(file)
  })

  it("loads the current secret for an owned application", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: createAppResponse(),
          connection_secret: "current-secret",
        },
      })
    )

    await expect(
      getClientAppCredentials("app/1", fetcher)
    ).resolves.toMatchObject({
      app: { id: "app-1" },
      connectionSecret: "current-secret",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/apps/app%2F1", {
      credentials: "include",
      method: "GET",
    })
  })

  it("loads an owned application profile with a profile-specific error", async () => {
    const successFetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: createAppResponse(),
          connection_secret: "current-secret",
        },
      })
    )

    await expect(
      getClientAppProfile("app/1", successFetcher)
    ).resolves.toMatchObject({ id: "app-1" })

    const failureFetcher = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 500 }))
    await expect(getClientAppProfile("app-1", failureFetcher)).rejects.toThrow(
      "加载应用资料失败"
    )
  })

  it("regenerates an owned application secret", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: createAppResponse(),
          connection_secret: "new-secret",
        },
      })
    )

    await expect(
      regenerateClientAppSecret("app-1", fetcher)
    ).resolves.toMatchObject({
      connectionSecret: "new-secret",
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/apps/app-1/secret/regenerate",
      {
        credentials: "include",
        method: "POST",
      }
    )
  })

  it("updates only the provided owned application fields", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {
          app: {
            ...createAppResponse(),
            name: "新版报表机器人",
            visibility: "public",
            user_ids: [],
          },
        },
      })
    )

    await expect(
      updateClientApp(
        "app-1",
        {
          name: "新版报表机器人",
          userIds: [],
          visibility: "public",
        },
        fetcher
      )
    ).resolves.toMatchObject({
      name: "新版报表机器人",
      userIds: [],
      visibility: "public",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/apps/app-1", {
      body: JSON.stringify({
        name: "新版报表机器人",
        user_ids: [],
        visibility: "public",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    })
  })

  it("deletes an owned application", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      createJSONResponse({
        success: true,
        data: {},
      })
    )

    await expect(deleteClientApp("app/1", fetcher)).resolves.toBeUndefined()
    expect(fetcher).toHaveBeenCalledWith("/api/client/apps/app%2F1", {
      credentials: "include",
      method: "DELETE",
    })
  })

  it("builds the app WebSocket URL from the current server origin", () => {
    expect(
      buildAppWebSocketURL({
        host: "chat.example.com",
        protocol: "https:",
      })
    ).toBe("wss://chat.example.com/api/app/ws")
  })
})

function createAppResponse() {
  return {
    avatar: "",
    connection_status: "offline",
    created_at: "2026-07-17T07:00:00Z",
    description: "生成业务报表",
    enabled: true,
    id: "app-1",
    name: "报表机器人",
    updated_at: "2026-07-17T07:00:00Z",
    user_ids: ["user-2"],
    visibility: "restricted",
  }
}

function createJSONResponse(payload: unknown) {
  return new Response(JSON.stringify(payload), {
    headers: {
      "content-type": "application/json",
    },
    status: 200,
  })
}
