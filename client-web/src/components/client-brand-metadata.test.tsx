import { render, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { ClientBrandMetadata } from "@/components/client-brand-metadata"
import { AppInfoContext } from "@/lib/app-info-context"

const originalHead = document.head.innerHTML

describe("ClientBrandMetadata", () => {
  afterEach(() => {
    document.head.innerHTML = originalHead
  })

  it("uses the configured app name in document metadata", async () => {
    document.head.innerHTML = `
      <meta name="application-name" content="即应" />
      <meta name="description" content="" />
      <meta name="keywords" content="" />
      <meta property="og:site_name" content="即应" />
      <meta property="og:title" content="即应 AI 企业 IM" />
      <meta name="twitter:title" content="即应 AI 企业 IM" />
    `

    render(
      <AppInfoContext.Provider
        value={{
          appName: "星环协作",
          authenticated: false,
          emailCodeLoginEnabled: false,
          oidcProviders: [],
          organizationName: "长亭科技",
          passwordLoginEnabled: true,
          setAuthenticated: () => undefined,
          thirdPartyProviders: [],
        }}
      >
        <ClientBrandMetadata />
      </AppInfoContext.Provider>
    )

    await waitFor(() => {
      expect(getMetaContent('meta[name="application-name"]')).toBe("星环协作")
      expect(getMetaContent('meta[property="og:site_name"]')).toBe("星环协作")
      expect(getMetaContent('meta[property="og:title"]')).toBe(
        "星环协作 AI 企业 IM"
      )
      expect(getMetaContent('meta[name="description"]')).toContain("星环协作")
      expect(getMetaContent('meta[name="keywords"]')).toContain("星环协作")
    })
  })
})

function getMetaContent(selector: string) {
  return document.querySelector<HTMLMetaElement>(selector)?.content
}
