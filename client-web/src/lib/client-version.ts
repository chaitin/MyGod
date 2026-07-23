export const clientVersionCheckIntervalMs = 300 * 1000
export const clientVersionRequestTimeoutMs = 15 * 1000
export const clientVersionReminderSnoozeMs = 60 * 60 * 1000

const clientVersionReminderStorageKey =
  "client-web:version-update-reminder-snooze"
const developmentClientCommit = "development"

export const currentClientBuildCommit =
  import.meta.env.VITE_CLIENT_BUILD_COMMIT?.trim() || developmentClientCommit

type ClientVersionManifest = {
  commit: string
}

type ClientVersionReminderSnooze = {
  commit: string
  until: number
}

export function canCheckForClientUpdates(commit = currentClientBuildCommit) {
  return commit !== developmentClientCommit
}

export async function fetchLatestClientBuildCommit(
  fetcher: typeof fetch = fetch
) {
  const controller = new AbortController()
  const timeout = window.setTimeout(
    () => controller.abort(),
    clientVersionRequestTimeoutMs
  )

  try {
    const response = await fetcher("/version.json", {
      cache: "no-store",
      method: "GET",
      signal: controller.signal,
    })
    if (!response.ok) {
      throw new Error(`加载客户端版本失败（HTTP ${response.status}）`)
    }

    const manifest = (await response.json()) as Partial<ClientVersionManifest>
    const commit = manifest.commit?.trim()
    if (!commit) {
      throw new Error("客户端版本信息格式不正确")
    }
    return commit
  } finally {
    window.clearTimeout(timeout)
  }
}

export function isClientVersionReminderSnoozed(
  commit: string,
  now = Date.now(),
  storage: Pick<Storage, "getItem"> = window.localStorage
) {
  try {
    const rawValue = storage.getItem(clientVersionReminderStorageKey)
    if (!rawValue) {
      return false
    }
    const value = JSON.parse(rawValue) as Partial<ClientVersionReminderSnooze>
    return value.commit === commit && Number(value.until) > now
  } catch {
    return false
  }
}

export function snoozeClientVersionReminder(
  commit: string,
  now = Date.now(),
  storage: Pick<Storage, "setItem"> = window.localStorage
) {
  try {
    storage.setItem(
      clientVersionReminderStorageKey,
      JSON.stringify({
        commit,
        until: now + clientVersionReminderSnoozeMs,
      } satisfies ClientVersionReminderSnooze)
    )
  } catch {
    // Storage restrictions must not prevent the dialog from closing.
  }
}
