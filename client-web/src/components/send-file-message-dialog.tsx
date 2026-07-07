import { LoaderCircle } from "lucide-react"

import { formatFileSize } from "@/lib/file-format"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

type SendFileMessageDialogProps = {
  conversationName: string
  file: File | null
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  sending: boolean
}

export function SendFileMessageDialog({
  conversationName,
  file,
  onConfirm,
  onOpenChange,
  open,
  sending,
}: SendFileMessageDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-5 sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base">发送文件</DialogTitle>
          <DialogDescription className="sr-only">
            确认发送文件到当前会话
          </DialogDescription>
        </DialogHeader>
        {file && (
          <div className="grid gap-3 rounded-md border p-3">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{file.name}</p>
              <p className="text-xs text-muted-foreground">
                {formatFileSize(file.size)}
              </p>
            </div>
            <div className="min-w-0 text-xs text-muted-foreground">
              发送到{" "}
              <span className="font-medium text-foreground">
                {conversationName}
              </span>
            </div>
          </div>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button disabled={sending} type="button" variant="outline">
              取消
            </Button>
          </DialogClose>
          <Button disabled={!file || sending} onClick={onConfirm} type="button">
            {sending && <LoaderCircle className="size-4 animate-spin" />}
            发送
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
