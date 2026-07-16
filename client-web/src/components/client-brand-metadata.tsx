import { useEffect } from "react"

import { useAppInfo } from "@/lib/app-info-context"

export function ClientBrandMetadata() {
  const { appName } = useAppInfo()

  useEffect(() => {
    setMetaContent('meta[name="application-name"]', appName)
    setMetaContent('meta[property="og:site_name"]', appName)
    setMetaContent('meta[property="og:title"]', `${appName} AI 企业 IM`)
    setMetaContent('meta[name="twitter:title"]', `${appName} AI 企业 IM`)
    setMetaContent(
      'meta[name="description"]',
      `${appName}是面向企业团队的 AI 原生即时通讯与协作平台，提供企业聊天、AI 助手、通讯录、项目管理和任务协作能力。`
    )
    setMetaContent(
      'meta[name="keywords"]',
      `${appName},AI 企业 IM,企业即时通讯,AI 助手,团队协作,企业聊天,项目管理,任务管理`
    )
  }, [appName])

  return null
}

function setMetaContent(selector: string, content: string) {
  document
    .querySelector<HTMLMetaElement>(selector)
    ?.setAttribute("content", content)
}
