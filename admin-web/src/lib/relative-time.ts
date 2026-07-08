const minuteMs = 60 * 1000
const hourMs = 60 * minuteMs
const dayMs = 24 * hourMs
const yearMs = 365 * dayMs

export function formatRelativeTimeDistance(value: string, now = new Date()) {
  const trimmedValue = value.trim()

  if (!trimmedValue) {
    return ""
  }

  const date = new Date(trimmedValue)

  if (Number.isNaN(date.getTime())) {
    return trimmedValue
  }

  const diffMs = Math.max(0, now.getTime() - date.getTime())

  if (diffMs < minuteMs) {
    return "刚刚"
  }

  if (diffMs < hourMs) {
    return `${Math.floor(diffMs / minuteMs)} 分钟`
  }

  if (diffMs < dayMs) {
    return `${Math.floor(diffMs / hourMs)} 小时`
  }

  if (diffMs < yearMs) {
    return `${Math.floor(diffMs / dayMs)} 天`
  }

  return `${Math.floor(diffMs / yearMs)} 年`
}
