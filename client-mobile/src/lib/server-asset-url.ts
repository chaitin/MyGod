export function resolveServerAssetUrl(serverUrl: string, assetUrl: string) {
  if (!assetUrl.trim()) {
    return ""
  }

  try {
    return new URL(
      assetUrl,
      `${serverUrl.replace(/\/+$/, "")}/`
    ).toString()
  } catch {
    return assetUrl
  }
}
