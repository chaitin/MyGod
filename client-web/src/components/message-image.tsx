import * as React from "react"
import { ImageOff } from "lucide-react"

import {
  readTemporaryFileURLs,
  type ClientImageMessageBody,
} from "@/lib/client-data-api"
import { cn } from "@/lib/utils"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"

type MessageImageProps = {
  image: ClientImageMessageBody
}

type PreviewOffset = {
  x: number
  y: number
}

type PreviewSize = {
  height: number
  width: number
}

const minPreviewZoom = 0.5
const maxPreviewZoom = 2
const previewZoomStep = 0.1

export function MessageImage({ image }: MessageImageProps) {
  const previewAreaRef = React.useRef<HTMLDivElement | null>(null)
  const previewDragRef = React.useRef<{
    offset: PreviewOffset
    pointerId: number
    x: number
    y: number
  } | null>(null)
  const [open, setOpen] = React.useState(false)
  const [source, setSource] = React.useState<{
    error: boolean
    fileId: string
    loaded: boolean
    url: string | null
  } | null>(null)
  const [previewAreaSize, setPreviewAreaSize] =
    React.useState<PreviewSize | null>(null)
  const [previewImageSize, setPreviewImageSize] =
    React.useState<PreviewSize | null>(null)
  const [previewZoom, setPreviewZoom] = React.useState(1)
  const [previewOffset, setPreviewOffset] = React.useState<PreviewOffset>({
    x: 0,
    y: 0,
  })
  const [previewDragging, setPreviewDragging] = React.useState(false)

  const previewBaseSize = React.useMemo(() => {
    if (!previewAreaSize || !previewImageSize) {
      return null
    }

    return getContainedPreviewSize(previewImageSize, previewAreaSize)
  }, [previewAreaSize, previewImageSize])
  const clampedPreviewOffset =
    previewAreaSize && previewBaseSize
      ? clampPreviewOffset(
          previewOffset,
          previewAreaSize,
          previewBaseSize,
          previewZoom
        )
      : { x: 0, y: 0 }

  React.useEffect(() => {
    let active = true

    readTemporaryFileURLs([image.fileId])
      .then((urls) => {
        if (!active) {
          return
        }

        const readURL =
          urls.find((item) => item.fileId === image.fileId) ?? urls[0]

        if (!readURL) {
          throw new Error("missing read url")
        }

        setSource({
          error: false,
          fileId: image.fileId,
          loaded: false,
          url: readURL.url,
        })
      })
      .catch(() => {
        if (active) {
          setSource({
            error: true,
            fileId: image.fileId,
            loaded: false,
            url: null,
          })
        }
      })

    return () => {
      active = false
    }
  }, [image.fileId])

  React.useEffect(() => {
    if (!open) {
      return
    }

    const previewArea = previewAreaRef.current

    if (!previewArea) {
      return
    }

    const updatePreviewAreaSize = () => {
      setPreviewAreaSize({
        height: previewArea.clientHeight,
        width: previewArea.clientWidth,
      })
    }

    updatePreviewAreaSize()

    const resizeObserver = new ResizeObserver(updatePreviewAreaSize)
    resizeObserver.observe(previewArea)

    return () => {
      resizeObserver.disconnect()
    }
  }, [open])

  const currentSource = source?.fileId === image.fileId ? source : null

  function resetPreviewState() {
    setPreviewZoom(1)
    setPreviewOffset({ x: 0, y: 0 })
    setPreviewDragging(false)
    previewDragRef.current = null
  }

  function handlePreviewOpenChange(nextOpen: boolean) {
    if (nextOpen) {
      resetPreviewState()
    }

    setOpen(nextOpen)
  }

  function handleImageError() {
    setSource({
      error: true,
      fileId: image.fileId,
      loaded: false,
      url: null,
    })
  }

  function handleImageLoad(event: React.SyntheticEvent<HTMLImageElement>) {
    const loadedURL = event.currentTarget.currentSrc || event.currentTarget.src

    setSource((currentSource) => {
      if (
        !currentSource ||
        currentSource.fileId !== image.fileId ||
        currentSource.url !== loadedURL
      ) {
        return currentSource
      }

      return {
        ...currentSource,
        loaded: true,
      }
    })
  }

  function handlePreviewImageLoad(event: React.SyntheticEvent<HTMLImageElement>) {
    const previewImage = event.currentTarget
    setPreviewImageSize({
      height: previewImage.naturalHeight,
      width: previewImage.naturalWidth,
    })
  }

  function handlePreviewWheel(event: React.WheelEvent<HTMLDivElement>) {
    if (event.deltaY === 0) {
      return
    }

    event.preventDefault()
    const nextZoom = clampPreviewZoom(
      previewZoom + (event.deltaY < 0 ? previewZoomStep : -previewZoomStep)
    )

    setPreviewZoom(nextZoom)
    setPreviewOffset((currentOffset) =>
      previewAreaSize && previewBaseSize
        ? clampPreviewOffset(
            currentOffset,
            previewAreaSize,
            previewBaseSize,
            nextZoom
          )
        : { x: 0, y: 0 }
    )
  }

  function handlePreviewClick() {
    resetPreviewState()
    setOpen(true)
  }

  function handlePreviewPointerDown(
    event: React.PointerEvent<HTMLDivElement>
  ) {
    if (!previewAreaSize || !previewBaseSize || previewZoom <= 1) {
      return
    }

    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    previewDragRef.current = {
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
      previewDrag.pointerId !== event.pointerId ||
      !previewAreaSize ||
      !previewBaseSize
    ) {
      return
    }

    const nextOffset = {
      x: previewDrag.offset.x + event.clientX - previewDrag.x,
      y: previewDrag.offset.y + event.clientY - previewDrag.y,
    }

    setPreviewOffset(
      clampPreviewOffset(
        nextOffset,
        previewAreaSize,
        previewBaseSize,
        previewZoom
      )
    )
  }

  function handlePreviewPointerEnd(event: React.PointerEvent<HTMLDivElement>) {
    if (previewDragRef.current?.pointerId !== event.pointerId) {
      return
    }

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    previewDragRef.current = null
    setPreviewDragging(false)
  }

  if (currentSource?.error) {
    return (
      <MessageImageStatus
        icon={<ImageOff className="size-5" />}
        text="图片加载失败"
      />
    )
  }

  if (!currentSource?.url) {
    return <MessageImageLoadingStatus />
  }

  return (
    <>
      <button
        aria-label="预览图片"
        className={cn(
          "relative block max-w-full overflow-hidden rounded-sm text-left",
          !currentSource.loaded && "w-64 max-w-[65vw]"
        )}
        onClick={handlePreviewClick}
        type="button"
      >
        {!currentSource.loaded && <MessageImageLoadingStatus />}
        <img
          alt="图片消息"
          className={cn(
            "block rounded-sm object-contain",
            currentSource.loaded
              ? "h-auto w-64 max-w-[65vw]"
              : "absolute inset-0 h-full w-full opacity-0"
          )}
          onError={handleImageError}
          onLoad={handleImageLoad}
          src={currentSource.url}
        />
      </button>
      <Dialog open={open} onOpenChange={handlePreviewOpenChange}>
        <DialogContent
          className="h-[90vh] w-[90vw] max-w-[90vw] gap-0 overflow-hidden bg-background p-0 sm:max-w-[90vw]"
          onContextMenu={(event) => {
            event.preventDefault()
            event.stopPropagation()
          }}
          showCloseButton={false}
        >
          <DialogHeader className="sr-only">
            <DialogTitle>图片预览</DialogTitle>
            <DialogDescription>查看图片消息大图</DialogDescription>
          </DialogHeader>
          <div
            ref={previewAreaRef}
            className={cn(
              "relative h-full w-full touch-none overflow-hidden bg-background select-none",
              previewZoom > 1 &&
                (previewDragging ? "cursor-grabbing" : "cursor-grab")
            )}
            onPointerCancel={handlePreviewPointerEnd}
            onPointerDown={handlePreviewPointerDown}
            onPointerMove={handlePreviewPointerMove}
            onPointerUp={handlePreviewPointerEnd}
            onWheel={handlePreviewWheel}
          >
            <img
              alt="图片消息预览"
              className={cn(
                "select-none",
                previewBaseSize
                  ? "absolute top-1/2 left-1/2 max-w-none"
                  : "h-full w-full object-contain"
              )}
              draggable={false}
              onError={handleImageError}
              onLoad={handlePreviewImageLoad}
              src={currentSource.url}
              style={
                previewBaseSize
                  ? {
                      height: previewBaseSize.height * previewZoom,
                      transform: `translate(-50%, -50%) translate(${clampedPreviewOffset.x}px, ${clampedPreviewOffset.y}px)`,
                      width: previewBaseSize.width * previewZoom,
                    }
                  : undefined
              }
            />
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function MessageImageLoadingStatus() {
  return (
    <MessageImageStatus
      icon={<Spinner className="size-5 text-muted-foreground" />}
      text="图片正在加载"
    />
  )
}

function MessageImageStatus({
  icon,
  text,
}: {
  icon: React.ReactNode
  text: string
}) {
  return (
    <div className="flex w-64 max-w-[65vw] items-center gap-3 rounded-sm bg-background/50 p-3">
      <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-background/60 text-muted-foreground">
        {icon}
      </div>
      <span className="min-w-0 text-sm font-medium text-foreground">
        {text}
      </span>
    </div>
  )
}

function getContainedPreviewSize(
  imageSize: PreviewSize,
  areaSize: PreviewSize
): PreviewSize | null {
  if (
    imageSize.width <= 0 ||
    imageSize.height <= 0 ||
    areaSize.width <= 0 ||
    areaSize.height <= 0
  ) {
    return null
  }

  const scale = Math.min(
    areaSize.width / imageSize.width,
    areaSize.height / imageSize.height
  )

  return {
    height: imageSize.height * scale,
    width: imageSize.width * scale,
  }
}

function clampPreviewZoom(zoom: number) {
  return Math.min(
    maxPreviewZoom,
    Math.max(minPreviewZoom, Number(zoom.toFixed(2)))
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
