import { describe, expect, it, vi } from "vitest"

import { getClientInfo } from "@/lib/app-info"

describe("client app info", () => {
  it("loads public app info from the client API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            app_name: "星环协作",
            authenticated: true,
            organization_name: "长亭科技",
            third_party_providers: [
              {
                key: "company-sso",
                name: "企业 SSO",
              },
            ],
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

    const info = await getClientInfo(fetcher)

    expect(info).toEqual({
      appName: "星环协作",
      authenticated: true,
      oidcProviders: [
        {
          key: "company-sso",
          name: "企业 SSO",
        },
      ],
      organizationName: "长亭科技",
      thirdPartyProviders: [
        {
          key: "company-sso",
          name: "企业 SSO",
        },
      ],
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/info", {
      credentials: "include",
      method: "GET",
    })
  })

  it("throws when the response is not successful", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "internal_error",
            message: "服务端错误",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 500,
        }
      )
    )

    await expect(getClientInfo(fetcher)).rejects.toThrow("服务端错误")
  })
})
