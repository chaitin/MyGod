import { useContext, useEffect, useMemo } from "react"

import { useAppInfo } from "@/lib/app-info-context"
import { ClientDataContext } from "@/lib/client-data-context"

type ClientDocumentTitleProps = {
  alertEmojis?: [string, string]
  alertTitle?: string
  disableMessageAlert?: boolean
  title: string
}

const alertEmojiBlinkIntervalMs = 500
const defaultAlertEmojis: [string, string] = ["🔔", "💬"]

export function ClientDocumentTitle({
  alertEmojis = defaultAlertEmojis,
  alertTitle = "新消息",
  disableMessageAlert = false,
  title,
}: ClientDocumentTitleProps) {
  const { appName } = useAppInfo()
  const clientData = useContext(ClientDataContext)
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
  const firstAlertEmoji = alertEmojis[0]
  const secondAlertEmoji = alertEmojis[1]

  useEffect(() => {
    if (!hasMessageAlert) {
      document.title = pageTitle
      return
    }

    let alertEmojiIndex = 0
    const alertEmojiOptions = [firstAlertEmoji, secondAlertEmoji]
    document.title = `${alertEmojiOptions[alertEmojiIndex]} 【${alertTitle}】- ${appName}`
    const intervalId = window.setInterval(() => {
      alertEmojiIndex = alertEmojiIndex === 0 ? 1 : 0
      document.title = `${alertEmojiOptions[alertEmojiIndex]} 【${alertTitle}】- ${appName}`
    }, alertEmojiBlinkIntervalMs)

    return () => window.clearInterval(intervalId)
  }, [
    alertTitle,
    appName,
    firstAlertEmoji,
    hasMessageAlert,
    pageTitle,
    secondAlertEmoji,
  ])

  return null
}
