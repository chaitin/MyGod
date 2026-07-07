import { describe, expect, it, vi } from "vitest"

import {
  AdminSettingsRequestError,
  createThirdPartyProvider,
  deleteThirdPartyProvider,
  disableThirdPartyProvider,
  enableThirdPartyProvider,
  getInfoSettings,
  listThirdPartyProviders,
  moveThirdPartyProvider,
  updateThirdPartyProvider,
  updateInfoSettings,
  type ThirdPartyProviderInput,
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

  it("lists ThirdParty providers through the admin API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            providers: [
              {
                id: "provider-1",
                name: "企业 SSO",
                key: "company-sso",
                callback_url:
                  "https://client.example.com/api/client/auth/third-party/company-sso/callback",
                enabled: true,
                client_id: "client-id",
                client_secret: "client-secret",
                config: {
                  authorize_url: "https://sso.example.com/oauth/authorize",
                  token_url: "https://sso.example.com/oauth/token",
                  userinfo_url: "https://sso.example.com/oauth/userinfo",
                  external_id_field: "sub",
                  email_field: "mail",
                  phone_field: "mobile",
                  name_field: "real_name",
                  nickname_field: "nick",
                  avatar_field: "picture",
                },
                scopes: ["openid", "email", "profile"],
                sort_order: 2,
                type: "oidc",
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

    const providers = await listThirdPartyProviders(fetcher)

    expect(providers).toEqual([
      {
        id: "provider-1",
        name: "企业 SSO",
        key: "company-sso",
        callbackUrl:
          "https://client.example.com/api/client/auth/third-party/company-sso/callback",
        enabled: true,
        clientId: "client-id",
        clientSecret: "client-secret",
        config: {
          authorize_url: "https://sso.example.com/oauth/authorize",
          token_url: "https://sso.example.com/oauth/token",
          userinfo_url: "https://sso.example.com/oauth/userinfo",
          external_id_field: "sub",
          email_field: "mail",
          phone_field: "mobile",
          name_field: "real_name",
          nickname_field: "nick",
          avatar_field: "picture",
        },
        scopes: ["openid", "email", "profile"],
        sortOrder: 2,
        type: "oidc",
      },
    ])
    expect(fetcher).toHaveBeenCalledWith("/api/admin/third-party/providers", {
      credentials: "include",
      method: "GET",
    })
  })

  it("creates and updates ThirdParty providers with full client secret without hidden fields", async () => {
    const providerResponse = {
      id: "provider-1",
      name: "企业 SSO",
      key: "company-sso",
      callback_url:
        "https://client.example.com/api/client/auth/third-party/company-sso/callback",
      enabled: true,
      client_id: "client-id",
      client_secret: "client-secret",
      config: {
        authorize_url: "https://sso.example.com/oauth/authorize",
        token_url: "https://sso.example.com/oauth/token",
        userinfo_url: "https://sso.example.com/oauth/userinfo",
        external_id_field: "sub",
        email_field: "mail",
        phone_field: "mobile",
        name_field: "real_name",
        nickname_field: "nick",
        avatar_field: "picture",
      },
      scopes: ["openid", "email", "profile"],
      sort_order: 2,
      type: "oidc",
    }
    const fetcher = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            success: true,
            data: {
              provider: providerResponse,
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
    )
    const input = {
      clientId: " client-id ",
      clientSecret: " client-secret ",
      config: {
        authorize_url: " https://sso.example.com/oauth/authorize ",
        avatar_field: " picture ",
        email_field: " mail ",
        external_id_field: " sub ",
        name_field: " real_name ",
        nickname_field: " nick ",
        phone_field: " mobile ",
        token_url: " https://sso.example.com/oauth/token ",
        userinfo_url: " https://sso.example.com/oauth/userinfo ",
      },
      name: " 企业 SSO ",
      scopes: ["email", "profile"],
      type: "oidc",
    } satisfies ThirdPartyProviderInput

    await createThirdPartyProvider(input, fetcher)
    await updateThirdPartyProvider("provider-1", input, fetcher)

    const expectedBody = JSON.stringify({
      client_id: "client-id",
      client_secret: "client-secret",
      config: {
        authorize_url: "https://sso.example.com/oauth/authorize",
        avatar_field: "picture",
        email_field: "mail",
        external_id_field: "sub",
        name_field: "real_name",
        nickname_field: "nick",
        phone_field: "mobile",
        token_url: "https://sso.example.com/oauth/token",
        userinfo_url: "https://sso.example.com/oauth/userinfo",
      },
      name: "企业 SSO",
      scopes: ["email", "profile"],
      type: "oidc",
    })
    expect(fetcher).toHaveBeenNthCalledWith(1, "/api/admin/third-party/providers", {
      body: expectedBody,
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "/api/admin/third-party/providers/provider-1",
      {
        body: expectedBody,
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        method: "PUT",
      }
    )
  })

  it("deletes ThirdParty providers through the admin API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {},
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await deleteThirdPartyProvider("provider-1", fetcher)

    expect(fetcher).toHaveBeenCalledWith(
      "/api/admin/third-party/providers/provider-1",
      {
        credentials: "include",
        method: "DELETE",
      }
    )
  })

  it("enables and disables ThirdParty providers through operation endpoints", async () => {
    const providerResponse = {
      id: "provider-1",
      name: "企业 SSO",
      key: "company-sso",
      callback_url:
        "https://client.example.com/api/client/auth/third-party/company-sso/callback",
      enabled: true,
      client_id: "client-id",
      client_secret: "client-secret",
      config: {
        authorize_url: "https://sso.example.com/oauth/authorize",
        email_field: "mail",
        name_field: "real_name",
        token_url: "https://sso.example.com/oauth/token",
        userinfo_url: "https://sso.example.com/oauth/userinfo",
      },
      scopes: ["email", "profile"],
      sort_order: 10,
      type: "oidc",
    }
    const fetcher = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            success: true,
            data: {
              provider: providerResponse,
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
    )

    await enableThirdPartyProvider("provider-1", fetcher)
    await disableThirdPartyProvider("provider-1", fetcher)

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "/api/admin/third-party/providers/provider-1/enable",
      {
        credentials: "include",
        method: "POST",
      }
    )
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "/api/admin/third-party/providers/provider-1/disable",
      {
        credentials: "include",
        method: "POST",
      }
    )
  })

  it("moves ThirdParty providers and returns the reordered provider list", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            providers: [
              {
                id: "provider-2",
                name: "Beta",
                key: "beta",
                callback_url:
                  "https://client.example.com/api/client/auth/third-party/beta/callback",
                enabled: true,
                client_id: "beta-client",
                client_secret: "beta-secret",
                config: {
                  authorize_url: "https://beta.example.com/authorize",
                  email_field: "email",
                  name_field: "name",
                  token_url: "https://beta.example.com/token",
                  userinfo_url: "https://beta.example.com/userinfo",
                },
                scopes: ["email"],
                sort_order: 10,
                type: "oidc",
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

    const providers = await moveThirdPartyProvider("provider-2", "up", fetcher)

    expect(providers.map((provider) => provider.id)).toEqual(["provider-2"])
    expect(fetcher).toHaveBeenCalledWith(
      "/api/admin/third-party/providers/provider-2/move",
      {
        body: JSON.stringify({
          direction: "up",
        }),
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }
    )
  })
})
