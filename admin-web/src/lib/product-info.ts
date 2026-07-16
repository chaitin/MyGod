export const defaultProductName = "即应"

type ProductInfoFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type ProductInfoEnvelope = {
  data?: {
    app_name?: string
  }
  success?: boolean
}

export async function getPublicProductInfo(
  fetcher: ProductInfoFetch = fetch
): Promise<{ appName: string }> {
  const response = await fetcher("/api/client/info", {
    credentials: "include",
    method: "GET",
  })
  const payload = (await response.json().catch(() => undefined)) as
    | ProductInfoEnvelope
    | undefined

  if (!response.ok || payload?.success === false) {
    throw new Error("加载产品信息失败")
  }

  return {
    appName: normalizeProductName(payload?.data?.app_name),
  }
}

export function normalizeProductName(appName: string | undefined) {
  return appName?.trim() || defaultProductName
}
