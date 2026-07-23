import * as React from "react"
import { BookOpen, Copy, RotateCcw } from "lucide-react"
import { toast } from "sonner"

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
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
  buildAppWebSocketURL,
  regenerateClientAppSecret,
  type ClientAppCredentials,
} from "@/lib/client-api/apps"

export function AppCredentialsDialog({
  credentials,
  onCredentialsChange,
  onOpenChange,
  open,
}: {
  credentials: ClientAppCredentials | null
  onCredentialsChange: (credentials: ClientAppCredentials) => void
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const [resetOpen, setResetOpen] = React.useState(false)
  const [resetting, setResetting] = React.useState(false)

  if (!credentials) {
    return null
  }

  const { app, connectionSecret } = credentials
  const webSocketURL = buildAppWebSocketURL(window.location)

  async function handleResetSecret() {
    if (resetting) {
      return
    }

    setResetting(true)
    try {
      const nextCredentials = await regenerateClientAppSecret(app.id)
      onCredentialsChange(nextCredentials)
      setResetOpen(false)
      toast.success("连接密钥已重置")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重置连接密钥失败")
    } finally {
      setResetting(false)
    }
  }

  return (
    <>
      <Dialog
        onOpenChange={(nextOpen) => {
          if (!resetting) {
            onOpenChange(nextOpen)
          }
        }}
        open={open}
      >
        <DialogContent
          className="max-h-[calc(100vh-2rem)] gap-5 overflow-y-auto sm:max-w-lg"
          onPointerDownOutside={(event) => event.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>开发指南</DialogTitle>
            <DialogDescription className="sr-only">
              查看应用的信息和连接凭据
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4">
            <CredentialField copyable label="应用 ID" value={app.id} />
            <CredentialField
              copyable
              label="WebSocket 地址"
              value={webSocketURL}
            />
            <CredentialField
              copyable
              label="连接密钥"
              value={connectionSecret}
            />
          </div>

          <DialogFooter className="sm:justify-between">
            <div className="flex gap-2">
              <Button
                disabled={resetting}
                onClick={() => setResetOpen(true)}
                type="button"
                variant="secondary"
              >
                <RotateCcw />
                重置连接密钥
              </Button>
              <Button asChild variant="secondary">
                <a
                  href="https://github.com/chaitin/MagicChat/blob/main/APPLICATION_DEVELOPMENT.md"
                  rel="noreferrer"
                  target="_blank"
                >
                  <BookOpen />
                  开发文档
                </a>
              </Button>
            </div>
            <Button
              disabled={resetting}
              onClick={() => onOpenChange(false)}
              type="button"
            >
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        onOpenChange={(nextOpen) => {
          if (!resetting) {
            setResetOpen(nextOpen)
          }
        }}
        open={resetOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>重置连接密钥</AlertDialogTitle>
            <AlertDialogDescription>
              重置后旧密钥立即失效，应用现有的 WebSocket
              连接也会被断开。确定继续吗？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={resetting}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={resetting}
              onClick={(event) => {
                event.preventDefault()
                void handleResetSecret()
              }}
              variant="destructive"
            >
              {resetting && <Spinner />}
              确认重置
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function CredentialField({
  copyable = false,
  label,
  value,
}: {
  copyable?: boolean
  label: string
  value: string
}) {
  const inputId = React.useId()

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${label}已复制`)
    } catch {
      toast.error(`${label}复制失败`)
    }
  }

  return (
    <div className="grid gap-2">
      <Label htmlFor={inputId}>{label}</Label>
      <div className="flex items-center gap-2">
        <Input
          className="min-w-0 flex-1 font-mono! text-xs"
          id={inputId}
          readOnly
          value={value}
        />
        {copyable && (
          <Button
            aria-label={`复制${label}`}
            onClick={() => void handleCopy()}
            size="icon"
            title={`复制${label}`}
            type="button"
            variant="outline"
          >
            <Copy />
          </Button>
        )}
      </div>
    </div>
  )
}
