import * as React from "react"
import { LoaderCircle } from "lucide-react"

import { cn } from "@/lib/utils"
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

type PreviewOffset = {
  x: number
  y: number
}

type PreviewSize = {
  height: number
  width: number
}

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
  const previewDragRef = React.useRef<{
    imageKey: string
    offset: PreviewOffset
    pointerId: number
    x: number
    y: number
  } | null>(null)
  const imageKey = previewURL ?? ""
  const [previewAreaElement, setPreviewAreaElement] =
    React.useState<HTMLDivElement | null>(null)
  const [previewAreaSize, setPreviewAreaSize] =
    React.useState<PreviewSize | null>(null)
  const [zoomState, setZoomState] = React.useState({
    imageKey: "",
    value: 1,
  })
  const [imageSize, setImageSize] = React.useState<{
    height: number
    imageKey: string
    width: number
  } | null>(null)
  const [previewOffset, setPreviewOffset] = React.useState<PreviewOffset>({
    x: 0,
    y: 0,
  })
  const [previewDragging, setPreviewDragging] = React.useState(false)
  const zoom = zoomState.imageKey === imageKey ? zoomState.value : 1
  const currentImageSize =
    imageSize?.imageKey === imageKey ? imageSize : null
  const previewSize = currentImageSize && previewAreaSize
    ? getContainedSize(
        currentImageSize,
        previewAreaSize.width,
        previewAreaSize.height
      )
    : null
  const clampedPreviewOffset =
    previewAreaSize && previewSize
      ? clampPreviewOffset(previewOffset, previewAreaSize, previewSize, zoom)
      : { x: 0, y: 0 }

  const handlePreviewWheel = React.useCallback(
    (event: WheelEvent) => {
      if (!image || event.deltaY === 0) {
        return
      }

      event.preventDefault()
      const nextZoom = clampPreviewZoom(
        zoom +
          (event.deltaY < 0 ? imagePreviewZoomStep : -imagePreviewZoomStep)
      )

      setZoomState({ imageKey, value: nextZoom })
      setPreviewOffset((currentOffset) => {
        return previewAreaSize && previewSize
          ? clampPreviewOffset(
              currentOffset,
              previewAreaSize,
              previewSize,
              nextZoom
            )
          : { x: 0, y: 0 }
      })
    },
    [image, imageKey, previewAreaSize, previewSize, zoom]
  )

  React.useEffect(() => {
    if (!open || !previewAreaElement) {
      return
    }

    const updatePreviewAreaSize = () => {
      setPreviewAreaSize({
        height: previewAreaElement.clientHeight,
        width: previewAreaElement.clientWidth,
      })
    }

    updatePreviewAreaSize()

    const resizeObserver = new ResizeObserver(updatePreviewAreaSize)
    resizeObserver.observe(previewAreaElement)

    return () => {
      resizeObserver.disconnect()
    }
  }, [open, previewAreaElement])

  React.useEffect(() => {
    if (!open || !image || !previewURL || !previewAreaElement) {
      return
    }

    previewAreaElement.addEventListener("wheel", handlePreviewWheel, {
      passive: false,
    })

    return () => {
      previewAreaElement.removeEventListener("wheel", handlePreviewWheel)
    }
  }, [handlePreviewWheel, image, open, previewAreaElement, previewURL])

  function handlePreviewPointerDown(
    event: React.PointerEvent<HTMLDivElement>
  ) {
    if (
      event.button !== 0 ||
      !previewAreaSize ||
      !previewSize ||
      zoom <= 1
    ) {
      return
    }

    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    previewDragRef.current = {
      imageKey,
      offset: clampedPreviewOffset,
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
    }
    setPreviewDragging(true)
  }

  function handlePreviewPointerMove(
    event: React.PointerEvent<HTMLDivElement>
  ) {
    const previewDrag = previewDragRef.current

    if (
      !previewDrag ||
      previewDrag.imageKey !== imageKey ||
      previewDrag.pointerId !== event.pointerId ||
      !previewAreaSize ||
      !previewSize
    ) {
      return
    }

    setPreviewOffset(
      clampPreviewOffset(
        {
          x: previewDrag.offset.x + event.clientX - previewDrag.x,
          y: previewDrag.offset.y + event.clientY - previewDrag.y,
        },
        previewAreaSize,
        previewSize,
        zoom
      )
    )
  }

  function handlePreviewPointerEnd(event: React.PointerEvent<HTMLDivElement>) {
    if (
      previewDragRef.current?.imageKey !== imageKey ||
      previewDragRef.current.pointerId !== event.pointerId
    ) {
      return
    }

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    previewDragRef.current = null
    setPreviewDragging(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="h-[60vh] w-[60vw] max-w-[60vw] grid-rows-[auto_minmax(0,1fr)_auto] gap-5 overflow-hidden sm:max-w-[60vw]"
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
          <div className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-3">
            <div
              ref={setPreviewAreaElement}
              className={cn(
                "relative min-h-0 min-w-0 touch-none overflow-hidden rounded-md border bg-muted/20 select-none",
                zoom > 1 &&
                  (previewDragging ? "cursor-grabbing" : "cursor-grab")
              )}
              onPointerCancel={handlePreviewPointerEnd}
              onPointerDown={handlePreviewPointerDown}
              onPointerMove={handlePreviewPointerMove}
              onPointerUp={handlePreviewPointerEnd}
            >
              <div className="relative h-full w-full">
                <img
                  alt="待发送图片预览"
                  className="absolute top-1/2 left-1/2 max-w-none rounded-sm object-contain select-none"
                  draggable={false}
                  onLoad={(event) => {
                    const target = event.currentTarget
                    setImageSize({
                      height: target.naturalHeight,
                      imageKey,
                      width: target.naturalWidth,
                    })
                  }}
                  src={previewURL}
                  style={
                    previewSize
                      ? {
                          height: previewSize.height * zoom,
                          transform: `translate(-50%, -50%) translate(${clampedPreviewOffset.x}px, ${clampedPreviewOffset.y}px)`,
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

function clampPreviewOffset(
  offset: PreviewOffset,
  areaSize: PreviewSize,
  baseSize: PreviewSize,
  zoom: number
): PreviewOffset {
  const maxX = Math.max(0, (baseSize.width * zoom - areaSize.width) / 2)
  const maxY = Math.max(0, (baseSize.height * zoom - areaSize.height) / 2)

  return {
    x: Math.min(maxX, Math.max(-maxX, offset.x)),
    y: Math.min(maxY, Math.max(-maxY, offset.y)),
  }
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
