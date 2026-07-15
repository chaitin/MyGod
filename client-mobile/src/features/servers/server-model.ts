import { appConfig } from "@/config/app-config"

export type ServerConfig = {
  id: string
  isBuiltIn: boolean
  name: string
  url: string
}

export const OFFICIAL_SERVER_ID = "magicchat-official"

export const officialServer: ServerConfig = {
  id: OFFICIAL_SERVER_ID,
  isBuiltIn: true,
  name: "MagicChat 官方服务器",
  url: appConfig.officialServerUrl,
}

export function isValidServerUrl(value: string) {
  try {
    const url = new URL(value.trim())

    return (
      (url.protocol === "http:" || url.protocol === "https:") &&
      url.hostname.length > 0
    )
  } catch {
    return false
  }
}

export function normalizeServerUrl(value: string) {
  const url = new URL(value.trim())
  url.hash = ""

  return url.toString().replace(/\/$/, "")
}
