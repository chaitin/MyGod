import { describe, expect, it, vi } from "vitest"

import {
  AdminSettingsRequestError,
  getInfoSettings,
  updateInfoSettings,
} from "@/lib/admin-settings"

describe("admin settings", () => {
  it("loads info settings through the admin API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            app_name: "MyGod",
            organization_name: "长亭科技",
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

    const settings = await getInfoSettings(fetcher)

    expect(settings).toEqual({
      appName: "MyGod",
      organizationName: "长亭科技",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/settings/info", {
      credentials: "include",
      method: "GET",
    })
  })

  it("updates info settings through the admin API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            app_name: "星环协作",
            organization_name: "长亭科技企业安全",
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

    const settings = await updateInfoSettings(
      {
        appName: " 星环协作 ",
        organizationName: " 长亭科技企业安全 ",
      },
      fetcher
    )

    expect(settings).toEqual({
      appName: "星环协作",
      organizationName: "长亭科技企业安全",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/admin/settings/info", {
      body: JSON.stringify({
        app_name: "星环协作",
        organization_name: "长亭科技企业安全",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PUT",
    })
  })

  it("throws the admin API error message when update fails", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "invalid_request",
            message: "App 名称不能为空",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 400,
        }
      )
    )

    await expect(
      updateInfoSettings(
        {
          appName: "",
          organizationName: "长亭科技",
        },
        fetcher
      )
    ).rejects.toMatchObject({
      code: "invalid_request",
      message: "App 名称不能为空",
      name: "AdminSettingsRequestError",
    } satisfies AdminSettingsRequestError)
  })
})
