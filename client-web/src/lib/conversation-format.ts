export function formatConversationLastMessageTime(
  lastMessageAt: string | null,
  now = new Date()
) {
  if (!lastMessageAt) {
    return ""
  }

  const date = new Date(lastMessageAt)

  if (Number.isNaN(date.getTime())) {
    return ""
  }

  if (!isSameLocalDay(date, now)) {
    return `${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}`
  }

  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    hour12: false,
    minute: "2-digit",
  }).format(date)
}

function isSameLocalDay(date: Date, otherDate: Date) {
  return (
    date.getFullYear() === otherDate.getFullYear() &&
    date.getMonth() === otherDate.getMonth() &&
    date.getDate() === otherDate.getDate()
  )
}

function padDatePart(value: number) {
  return String(value).padStart(2, "0")
}
