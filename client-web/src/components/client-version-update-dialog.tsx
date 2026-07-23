import * as React from "react"

import {
  canCheckForClientUpdates,
  clientVersionCheckIntervalMs,
  currentClientBuildCommit,
  fetchLatestClientBuildCommit,
  isClientVersionReminderSnoozed,
  snoozeClientVersionReminder,
} from "@/lib/client-version"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

type ClientVersionUpdateDialogProps = {
  currentCommit?: string
  fetchLatestCommit?: () => Promise<string>
  reload?: () => void
}

export function ClientVersionUpdateDialog({
  currentCommit = currentClientBuildCommit,
  fetchLatestCommit = fetchLatestClientBuildCommit,
  reload = reloadPage,
}: ClientVersionUpdateDialogProps) {
  const [latestCommit, setLatestCommit] = React.useState("")

  React.useEffect(() => {
    if (!canCheckForClientUpdates(currentCommit)) {
      return
    }

    let disposed = false
    let checking = false

    async function checkForUpdate() {
      if (checking) {
        return
      }
      checking = true
      try {
        const nextCommit = await fetchLatestCommit()
        if (
          !disposed &&
          nextCommit !== currentCommit &&
          !isClientVersionReminderSnoozed(nextCommit)
        ) {
          setLatestCommit(nextCommit)
        }
      } catch {
        // Version checks are best-effort and should not interrupt the client.
      } finally {
        checking = false
      }
    }

    const interval = window.setInterval(
      () => void checkForUpdate(),
      clientVersionCheckIntervalMs
    )
    return () => {
      disposed = true
      window.clearInterval(interval)
    }
  }, [currentCommit, fetchLatestCommit])

  function snoozeReminder() {
    if (latestCommit) {
      snoozeClientVersionReminder(latestCommit)
    }
    setLatestCommit("")
  }

  return (
    <AlertDialog
      onOpenChange={(open) => {
        if (!open) {
          setLatestCommit("")
        }
      }}
      open={Boolean(latestCommit)}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>发现新版本</AlertDialogTitle>
          <AlertDialogDescription>
            系统已发布新版本，刷新页面后即可使用最新功能。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={snoozeReminder} variant="secondary">
            一小时内不再提醒
          </AlertDialogCancel>
          <AlertDialogAction onClick={reload}>确认更新</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function reloadPage() {
  window.location.reload()
}
