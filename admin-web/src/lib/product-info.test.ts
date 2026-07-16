import { describe, expect, it, vi } from "vitest"

import { getPublicProductInfo, normalizeProductName } from "@/lib/product-info"

describe("public product info", () => {
  it("loads the configured app name from the client info API", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            app_name: "星环协作",
          },
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        }
      )
    )

    await expect(getPublicProductInfo(fetcher)).resolves.toEqual({
      appName: "星环协作",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/info", {
      credentials: "include",
      method: "GET",
    })
  })

  it("uses the default name when the configured name is empty", () => {
    expect(normalizeProductName("  ")).toBe("即应")
  })
})
