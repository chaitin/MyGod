import * as React from "react"
import { LoaderCircle } from "lucide-react"

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

type SendImageMessageDialogProps = {
  conversationName: string
  image: File | null
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  sending: boolean
}

export function SendImageMessageDialog({
  conversationName,
  image,
  onConfirm,
  onOpenChange,
  open,
  sending,
}: SendImageMessageDialogProps) {
  const previewURL = useObjectURL(image)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-5 sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base">发送图片</DialogTitle>
          <DialogDescription className="sr-only">
            确认发送图片到当前会话
          </DialogDescription>
        </DialogHeader>
        {image && previewURL && (
          <div className="grid gap-3">
            <div className="flex max-h-80 min-h-44 items-center justify-center overflow-hidden rounded-md border bg-muted/20 p-2">
              <img
                alt="待发送图片预览"
                className="max-h-72 max-w-full rounded-sm object-contain"
                src={previewURL}
              />
            </div>
            <p className="min-w-0 text-sm text-muted-foreground">
              将要发送到{" "}
              <span className="font-medium text-foreground">
                {conversationName}
              </span>
            </p>
          </div>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button disabled={sending} type="button" variant="outline">
              取消
            </Button>
          </DialogClose>
          <Button
            disabled={!image || sending}
            onClick={onConfirm}
            type="button"
          >
            {sending && <LoaderCircle className="size-4 animate-spin" />}
            发送
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function useObjectURL(file: File | null) {
  const [source, setSource] = React.useState<{
    file: File | null
    url: string | null
  } | null>(null)

  React.useEffect(() => {
    let active = true

    if (!file) {
      queueMicrotask(() => {
        if (active) {
          setSource({ file: null, url: null })
        }
      })
      return () => {
        active = false
      }
    }

    const objectURL = URL.createObjectURL(file)

    queueMicrotask(() => {
      if (active) {
        setSource({ file, url: objectURL })
      }
    })

    return () => {
      active = false
      URL.revokeObjectURL(objectURL)
    }
  }, [file])

  return source?.file === file ? source.url : null
}
