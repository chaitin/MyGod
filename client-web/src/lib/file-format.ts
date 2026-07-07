const fileSizeUnits = ["B", "KB", "MB", "GB", "TB"]

export function formatFileSize(sizeBytes: number) {
  const normalizedSize = Math.max(0, Number.isFinite(sizeBytes) ? sizeBytes : 0)

  if (normalizedSize < 1024) {
    return `${Math.round(normalizedSize)} B`
  }

  let value = normalizedSize
  let unitIndex = 0

  while (value >= 1024 && unitIndex < fileSizeUnits.length - 1) {
    value = value / 1024
    unitIndex += 1
  }

  return `${new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: value >= 10 ? 0 : 1,
  }).format(value)} ${fileSizeUnits[unitIndex]}`
}
