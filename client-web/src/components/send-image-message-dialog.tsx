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

const imagePreviewBaseMaxWidth = 384
const imagePreviewBaseMaxHeight = 288
const minImagePreviewZoom = 0.5
const maxImagePreviewZoom = 3
const imagePreviewZoomStep = 0.1

export function SendImageMessageDialog({
  conversationName,
  image,
  onConfirm,
  onOpenChange,
  open,
  sending,
}: SendImageMessageDialogProps) {
  const previewURL = useObjectURL(image)
  const confirmButtonRef = React.useRef<HTMLButtonElement | null>(null)
  const [zoom, setZoom] = React.useState(1)
  const [imageSize, setImageSize] = React.useState<{
    height: number
    width: number
  } | null>(null)
  const previewSize = imageSize
    ? getContainedSize(
        imageSize,
        imagePreviewBaseMaxWidth,
        imagePreviewBaseMaxHeight
      )
    : null

  React.useEffect(() => {
    if (open) {
      setZoom(1)
    }
  }, [image, open])

  React.useEffect(() => {
    setImageSize(null)
  }, [previewURL])

  function handlePreviewWheel(event: React.WheelEvent<HTMLDivElement>) {
    if (!image || event.deltaY === 0) {
      return
    }

    event.preventDefault()
    setZoom((currentZoom) =>
      clampPreviewZoom(
        currentZoom +
          (event.deltaY < 0 ? imagePreviewZoomStep : -imagePreviewZoomStep)
      )
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[calc(100vh-2rem)] w-fit max-w-[calc(100vw-2rem)] gap-5 overflow-hidden sm:max-w-[calc(100vw-2rem)]"
        onOpenAutoFocus={(event) => {
          if (!image || sending) {
            return
          }

          event.preventDefault()
          confirmButtonRef.current?.focus()
        }}
      >
        <DialogHeader>
          <DialogTitle className="text-base">发送图片</DialogTitle>
          <DialogDescription className="sr-only">
            确认发送图片到当前会话
          </DialogDescription>
        </DialogHeader>
        {image && previewURL && (
          <div className="grid gap-3">
            <div
              className="max-h-[calc(100vh-14rem)] max-w-[calc(100vw-5rem)] min-h-44 justify-self-center overflow-auto rounded-md border bg-muted/20 p-2"
              onWheel={handlePreviewWheel}
            >
              <div className="flex min-h-40 min-w-full">
                <img
                  alt="待发送图片预览"
                  className="m-auto max-w-none shrink-0 rounded-sm object-contain"
                  draggable={false}
                  onLoad={(event) => {
                    const target = event.currentTarget
                    setImageSize({
                      height: target.naturalHeight,
                      width: target.naturalWidth,
                    })
                  }}
                  src={previewURL}
                  style={
                    previewSize
                      ? {
                          height: previewSize.height * zoom,
                          width: previewSize.width * zoom,
                        }
                      : undefined
                  }
                />
              </div>
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
            ref={confirmButtonRef}
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

function getContainedSize(
  size: { height: number; width: number },
  maxWidth: number,
  maxHeight: number
) {
  const scale = Math.min(1, maxWidth / size.width, maxHeight / size.height)

  return {
    height: Math.max(1, Math.round(size.height * scale)),
    width: Math.max(1, Math.round(size.width * scale)),
  }
}

function clampPreviewZoom(zoom: number) {
  return Math.min(
    maxImagePreviewZoom,
    Math.max(minImagePreviewZoom, Number(zoom.toFixed(2)))
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
