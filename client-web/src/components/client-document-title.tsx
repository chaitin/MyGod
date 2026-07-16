import { useContext, useEffect, useMemo, useRef } from "react"

import { useAppInfo } from "@/lib/app-info-context"
import { ClientDataContext } from "@/lib/client-data-context"

type ClientDocumentTitleProps = {
  disableMessageAlert?: boolean
  title: string
}

const faviconBlinkIntervalMs = 500
const transparentFavicon = {
  href: "/transparent-favicon.svg",
  type: "image/svg+xml",
}

export function ClientDocumentTitle({
  disableMessageAlert = false,
  title,
}: ClientDocumentTitleProps) {
  const { appName } = useAppInfo()
  const clientData = useContext(ClientDataContext)
  const defaultFaviconRef = useRef<Favicon | null>(null)
  const conversations = clientData?.conversations
  const unreadCount = useMemo(() => {
    if (disableMessageAlert || !conversations) {
      return 0
    }

    return conversations.reduce(
      (total, conversation) => total + conversation.unreadCount,
      0
    )
  }, [conversations, disableMessageAlert])
  const hasMessageAlert = unreadCount > 0
  const pageTitle = `${title} - ${appName}`

  useEffect(() => {
    const faviconLink = getFaviconLink()
    if (!defaultFaviconRef.current) {
      defaultFaviconRef.current = {
        href: faviconLink.getAttribute("href") ?? "/favicon.webp",
        type: faviconLink.getAttribute("type") ?? "image/webp",
      }
    }
    const defaultFavicon = defaultFaviconRef.current

    document.title = pageTitle

    if (!hasMessageAlert) {
      setFavicon(defaultFavicon)
      return
    }

    let showingDefaultFavicon = true
    setFavicon(defaultFavicon)
    const intervalId = window.setInterval(() => {
      showingDefaultFavicon = !showingDefaultFavicon
      setFavicon(showingDefaultFavicon ? defaultFavicon : transparentFavicon)
    }, faviconBlinkIntervalMs)

    return () => {
      window.clearInterval(intervalId)
      setFavicon(defaultFavicon)
    }
  }, [hasMessageAlert, pageTitle])

  return null
}

function getFaviconLink() {
  let faviconLink = document.querySelector<HTMLLinkElement>('link[rel~="icon"]')
  if (faviconLink) {
    return faviconLink
  }

  faviconLink = document.createElement("link")
  faviconLink.rel = "icon"
  faviconLink.href = "/favicon.webp"
  faviconLink.type = "image/webp"
  document.head.appendChild(faviconLink)

  return faviconLink
}

type Favicon = {
  href: string
  type: string
}

function setFavicon(favicon: Favicon) {
  const faviconLink = getFaviconLink()
  faviconLink.setAttribute("href", favicon.href)
  faviconLink.setAttribute("type", favicon.type)
}
