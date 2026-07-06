import { describe, expect, it } from "vitest"

import settingsPageSourceText from "./settings-page.tsx?raw"
import {
  createDefaultOIDCProviderForm,
  getOIDCProviderTextClassName,
  getSettingsCardClassName,
  getSettingsPageLayoutClassName,
  oidcProviderFormToInput,
  oidcProviderToForm,
  sortOIDCProvidersForDisplay,
} from "@/pages/settings-page"

describe("settings page OIDC provider form", () => {
  it("uses sensible defaults for new providers", () => {
    expect(createDefaultOIDCProviderForm()).toEqual({
      avatarField: "picture",
      authorizeUrl: "",
      clientId: "",
      clientSecret: "",
      emailField: "email",
      name: "",
      nameField: "name",
      nicknameField: "nickname",
      phoneField: "phone",
      scopesText: "email,profile",
      tokenUrl: "",
      userinfoUrl: "",
    })
  })

  it("converts providers to editable form state", () => {
    expect(
      oidcProviderToForm({
        avatarField: "avatar_url",
        authorizeUrl: "https://sso.example.com/authorize",
        clientId: "client-id",
        clientSecret: "client-secret",
        emailField: "mail",
        enabled: false,
        id: "provider-1",
        key: "company-sso",
        name: "企业 SSO",
        nameField: "real_name",
        nicknameField: "nick",
        phoneField: "mobile",
        scopes: ["openid", "email"],
        sortOrder: 3,
        tokenUrl: "https://sso.example.com/token",
        userinfoUrl: "https://sso.example.com/userinfo",
      })
    ).toEqual({
      avatarField: "avatar_url",
      authorizeUrl: "https://sso.example.com/authorize",
      clientId: "client-id",
      clientSecret: "client-secret",
      emailField: "mail",
      name: "企业 SSO",
      nameField: "real_name",
      nicknameField: "nick",
      phoneField: "mobile",
      scopesText: "openid,email",
      tokenUrl: "https://sso.example.com/token",
      userinfoUrl: "https://sso.example.com/userinfo",
    })
  })

  it("converts form state to a trimmed API input", () => {
    expect(
      oidcProviderFormToInput({
        avatarField: " picture ",
        authorizeUrl: " https://sso.example.com/authorize ",
        clientId: " client-id ",
        clientSecret: " client-secret ",
        emailField: " mail ",
        name: " 企业 SSO ",
        nameField: " real_name ",
        nicknameField: " nick ",
        phoneField: " mobile ",
        scopesText: "email, profile,,custom",
        tokenUrl: " https://sso.example.com/token ",
        userinfoUrl: " https://sso.example.com/userinfo ",
      })
    ).toEqual({
      avatarField: "picture",
      authorizeUrl: "https://sso.example.com/authorize",
      clientId: "client-id",
      clientSecret: "client-secret",
      emailField: "mail",
      name: "企业 SSO",
      nameField: "real_name",
      nicknameField: "nick",
      phoneField: "mobile",
      scopes: ["email", "profile", "custom"],
      tokenUrl: "https://sso.example.com/token",
      userinfoUrl: "https://sso.example.com/userinfo",
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
})

describe("settings page layout", () => {
  it("places basic info and OIDC login in two columns on wide screens", () => {
    expect(getSettingsPageLayoutClassName()).toContain("lg:grid-cols-2")
    expect(getSettingsCardClassName()).toBe("w-full")
  })

  it("uses a dialog for the OIDC provider form", () => {
    const settingsPageSource = getSourceBetween(
      settingsPageSourceText,
      "export default function SettingsPage()",
      "function OIDCProviderDialog("
    )

    expect(settingsPageSource).toContain("OIDCProviderDialog")
    expect(settingsPageSource).not.toContain("onSubmit={handleOIDCSubmit}")
    expect(settingsPageSource).not.toContain('id="oidc-client-secret"')
  })

  it("uses short add copy for the dialog trigger", () => {
    const triggerSource = getSourceBetween(
      settingsPageSourceText,
      "<DialogTrigger",
      "</DialogTrigger>"
    )

    expect(triggerSource).toContain("添加")
    expect(triggerSource).not.toContain("添加登录方式")
    expect(triggerSource).not.toContain("新增")
  })

  it("keeps generated fields and enable state out of the provider dialog", () => {
    const dialogSource = getSourceBetween(
      settingsPageSourceText,
      "function OIDCProviderDialog(",
      "export function getSettingsPageLayoutClassName()"
    )

    expect(dialogSource).not.toContain("Checkbox")
    expect(dialogSource).not.toContain(">启用<")
    expect(dialogSource).not.toContain(">Key<")
    expect(dialogSource).not.toContain(">排序<")
    expect(dialogSource).not.toContain("Textarea")
    expect(dialogSource).toContain("htmlFor={oidcScopesId}>Scope")
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
    avatarField: "picture",
    authorizeUrl: "https://sso.example.com/authorize",
    clientId: "client-id",
    clientSecret: "client-secret",
    emailField: "email",
    enabled: true,
    id,
    key: id,
    name,
    nameField: "name",
    nicknameField: "nickname",
    phoneField: "phone",
    scopes: ["email"],
    sortOrder,
    tokenUrl: "https://sso.example.com/token",
    userinfoUrl: "https://sso.example.com/userinfo",
  }
}
