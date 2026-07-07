import { describe, expect, it } from "vitest"

import settingsPageSourceText from "./settings-page.tsx?raw"
import {
  createDefaultOIDCProviderForm,
  getThirdPartyCallbackURL,
  getOIDCProviderTextClassName,
  getSettingsCardClassName,
  getSettingsPageLayoutClassName,
  oidcProviderFormToInput,
  oidcProviderToForm,
  sortOIDCProvidersForDisplay,
} from "@/pages/settings-page"

describe("settings page third-party provider form", () => {
  it("uses sensible defaults for new providers", () => {
    expect(createDefaultOIDCProviderForm()).toEqual({
      agentId: "",
      avatarField: "picture",
      authorizeUrl: "",
      clientId: "",
      clientSecret: "",
      emailField: "email",
      externalIdField: "sub",
      name: "通用 OIDC",
      nameField: "name",
      nicknameField: "nickname",
      phoneField: "phone",
      scopesText: "openid,email,profile",
      tokenUrl: "",
      type: "oidc",
      userinfoUrl: "",
    })
  })

  it("converts providers to editable form state", () => {
    expect(
      oidcProviderToForm({
        clientId: "client-id",
        clientSecret: "client-secret",
        config: {
          avatar_field: "avatar_url",
          authorize_url: "https://sso.example.com/authorize",
          email_field: "mail",
          external_id_field: "sub",
          name_field: "real_name",
          nickname_field: "nick",
          phone_field: "mobile",
          token_url: "https://sso.example.com/token",
          userinfo_url: "https://sso.example.com/userinfo",
        },
        enabled: false,
        id: "provider-1",
        key: "company-sso",
        name: "企业 SSO",
        scopes: ["openid", "email"],
        sortOrder: 3,
        type: "oidc",
      })
    ).toEqual({
      agentId: "",
      avatarField: "avatar_url",
      authorizeUrl: "https://sso.example.com/authorize",
      clientId: "client-id",
      clientSecret: "client-secret",
      emailField: "mail",
      externalIdField: "sub",
      name: "企业 SSO",
      nameField: "real_name",
      nicknameField: "nick",
      phoneField: "mobile",
      scopesText: "openid,email",
      tokenUrl: "https://sso.example.com/token",
      type: "oidc",
      userinfoUrl: "https://sso.example.com/userinfo",
    })
  })

  it("converts form state to a trimmed API input", () => {
    expect(
      oidcProviderFormToInput({
        agentId: "",
        avatarField: " picture ",
        authorizeUrl: " https://sso.example.com/authorize ",
        clientId: " client-id ",
        clientSecret: " client-secret ",
        emailField: " mail ",
        externalIdField: " sub ",
        name: " 企业 SSO ",
        nameField: " real_name ",
        nicknameField: " nick ",
        phoneField: " mobile ",
        scopesText: "email, profile,,custom",
        tokenUrl: " https://sso.example.com/token ",
        type: "oidc",
        userinfoUrl: " https://sso.example.com/userinfo ",
      })
    ).toEqual({
      clientId: "client-id",
      clientSecret: "client-secret",
      config: {
        avatar_field: "picture",
        authorize_url: "https://sso.example.com/authorize",
        email_field: "mail",
        external_id_field: "sub",
        name_field: "real_name",
        nickname_field: "nick",
        phone_field: "mobile",
        token_url: "https://sso.example.com/token",
        userinfo_url: "https://sso.example.com/userinfo",
      },
      name: "企业 SSO",
      scopes: ["email", "profile", "custom"],
      type: "oidc",
    })
  })

  it("sorts providers by sort order and name for display", () => {
    const providers = [
      createProvider({ id: "provider-b", name: "Beta", sortOrder: 2 }),
      createProvider({ id: "provider-c", name: "Alpha", sortOrder: 1 }),
      createProvider({ id: "provider-a", name: "Gamma", sortOrder: 1 }),
    ]

    expect(
      sortOIDCProvidersForDisplay(providers).map((provider) => provider.id)
    ).toEqual(["provider-c", "provider-a", "provider-b"])
    expect(providers.map((provider) => provider.id)).toEqual([
      "provider-b",
      "provider-c",
      "provider-a",
    ])
  })

  it("dims provider text when the login method is disabled", () => {
    expect(getOIDCProviderTextClassName(true)).not.toContain(
      "text-muted-foreground"
    )
    expect(getOIDCProviderTextClassName(false)).toContain(
      "text-muted-foreground"
    )
  })

  it("uses the server-provided callback URL", () => {
    expect(
      getThirdPartyCallbackURL(
        createProvider({
          id: "provider-1",
          name: "企业 SSO",
          sortOrder: 1,
        })
      )
    ).toBe(
      "https://client.example.com/api/client/auth/third-party/provider-1/callback"
    )
  })
})

describe("settings page layout", () => {
  it("places basic info and third-party login in two columns on wide screens", () => {
    expect(getSettingsPageLayoutClassName()).toContain("lg:grid-cols-2")
    expect(getSettingsCardClassName()).toBe("w-full")
  })

  it("uses a dialog for the third-party provider form", () => {
    const settingsPageSource = getSourceBetween(
      settingsPageSourceText,
      "export default function SettingsPage()",
      "function ThirdPartyProviderAddMenu("
    )

    expect(settingsPageSource).toContain("ThirdPartyProviderDialog")
    expect(settingsPageSource).not.toContain("onSubmit={handleOIDCSubmit}")
  })

  it("uses a provider menu for adding login methods", () => {
    const triggerSource = getSourceBetween(
      settingsPageSourceText,
      "function ThirdPartyProviderAddMenu(",
      "function ThirdPartyProviderActions("
    )

    expect(triggerSource).toContain("添加")
    expect(settingsPageSourceText).toContain("企业微信")
    expect(settingsPageSourceText).toContain("通用 OIDC")
  })

  it("keeps generated fields and enable state out of the provider dialog", () => {
    const dialogSource = getSourceBetween(
      settingsPageSourceText,
      "function ThirdPartyProviderDialog(",
      "export function getSettingsPageLayoutClassName()"
    )

    expect(dialogSource).not.toContain("Checkbox")
    expect(dialogSource).not.toContain(">启用<")
    expect(dialogSource).not.toContain(">Key<")
    expect(dialogSource).not.toContain(">排序<")
    expect(dialogSource).not.toContain("Textarea")
    expect(dialogSource).toContain("htmlFor={scopesId}>Scope")
    expect(dialogSource).toContain("<Input")
  })

  it("uses the provider operation menu for edit, status, order, and delete actions", () => {
    expect(settingsPageSourceText).toContain("DropdownMenu")
    expect(settingsPageSourceText).toContain("编辑")
    expect(settingsPageSourceText).toContain("启用")
    expect(settingsPageSourceText).toContain("禁用")
    expect(settingsPageSourceText).toContain("上移")
    expect(settingsPageSourceText).toContain("下移")
    expect(settingsPageSourceText).toContain("删除")
  })
})

function getSourceBetween(source: string, start: string, end: string) {
  const startIndex = source.indexOf(start)
  const endIndex = source.indexOf(end)

  if (startIndex === -1 || endIndex === -1 || endIndex <= startIndex) {
    throw new Error(`Could not find source section between ${start} and ${end}`)
  }

  return source.slice(startIndex, endIndex)
}

function createProvider({
  id,
  name,
  sortOrder,
}: {
  id: string
  name: string
  sortOrder: number
}) {
  return {
    clientId: "client-id",
    clientSecret: "client-secret",
    callbackUrl: `https://client.example.com/api/client/auth/third-party/${id}/callback`,
    config: {
      authorize_url: "https://sso.example.com/authorize",
      email_field: "email",
      name_field: "name",
      token_url: "https://sso.example.com/token",
      userinfo_url: "https://sso.example.com/userinfo",
    },
    enabled: true,
    id,
    key: id,
    name,
    scopes: ["email"],
    sortOrder,
    type: "oidc" as const,
  }
}
