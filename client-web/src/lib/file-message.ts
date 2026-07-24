export const fileMessageMaxBytes = 200 * 1024 * 1024

export function getFileMessageUploadError(
  file: Pick<File, "size">
): string | null {
  if (file.size <= 0) {
    return "文件不能为空"
  }

  if (file.size > fileMessageMaxBytes) {
    return "文件不能超过 200MiB"
  }

  return null
}
