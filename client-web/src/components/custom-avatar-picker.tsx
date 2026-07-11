import * as React from "react"
import { Loader2Icon, Minus, Plus, RotateCcw, Upload } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

const avatarOutputSize = 256
const avatarMinSourceSize = 64
const avatarMaxSourceSize = 4096
const avatarMaxSourceBytes = 5 * 1024 * 1024
const avatarMaxOutputBytes = 1 * 1024 * 1024
const avatarOutputType = "image/webp"
const avatarOutputQuality = 0.9
const avatarZoomMin = 1
const avatarZoomMax = 3
const avatarZoomStep = 0.1
const acceptedImageTypes = new Set(["image/jpeg", "image/png", "image/webp"])

type Point = {
  x: number
  y: number
}

type ImageSize = {
  height: number
  width: number
}

type ImageLayout = {
  displayHeight: number
  displayWidth: number
}

export type CroppedAvatar = {
  file: File
  previewUrl: string
}

type CustomAvatarPickerProps = {
  disabled?: boolean
  onSave: (avatar: CroppedAvatar) => Promise<void> | void
  saving?: boolean
}

export function CustomAvatarPicker({
  disabled = false,
  onSave,
  saving = false,
}: CustomAvatarPickerProps) {
  const cropFrameRef = React.useRef<HTMLDivElement>(null)
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const imageRef = React.useRef<HTMLImageElement>(null)
  const previousCropSizeRef = React.useRef(avatarOutputSize)
  const dragStartRef = React.useRef<{
    offset: Point
    pointer: Point
  } | null>(null)
  const [cropFrameSize, setCropFrameSize] = React.useState(avatarOutputSize)
  const [error, setError] = React.useState("")
  const [imageSize, setImageSize] = React.useState<ImageSize | null>(null)
  const [sourceFile, setSourceFile] = React.useState<File | null>(null)
  const [sourceUrl, setSourceUrl] = React.useState("")
  const [dragging, setDragging] = React.useState(false)
  const [offset, setOffset] = React.useState<Point>({ x: 0, y: 0 })
  const [zoom, setZoom] = React.useState(1)
  const imageLayout = imageSize
    ? getImageLayout(imageSize, zoom, cropFrameSize)
    : null
  const interactionsDisabled = disabled || saving
  const cropControlsDisabled =
    interactionsDisabled || !sourceUrl || !imageLayout

  React.useEffect(() => {
    if (!sourceUrl) {
      return
    }

    return () => URL.revokeObjectURL(sourceUrl)
  }, [sourceUrl])

  React.useLayoutEffect(() => {
    const cropFrame = cropFrameRef.current

    if (!cropFrame) {
      return
    }

    function updateCropFrameSize() {
      if (!cropFrame) {
        return
      }

      const nextSize = cropFrame.getBoundingClientRect().width

      if (nextSize > 0) {
        setCropFrameSize(nextSize)
      }
    }

    updateCropFrameSize()

    const resizeObserver = new ResizeObserver(updateCropFrameSize)

    resizeObserver.observe(cropFrame)

    return () => resizeObserver.disconnect()
  }, [sourceUrl])

  React.useEffect(() => {
    const previousCropSize = previousCropSizeRef.current

    if (previousCropSize === cropFrameSize) {
      return
    }

    previousCropSizeRef.current = cropFrameSize

    if (!imageSize) {
      return
    }

    const ratio = cropFrameSize / previousCropSize
    const nextLayout = getImageLayout(imageSize, zoom, cropFrameSize)

    setOffset((currentOffset) =>
      clampOffset(
        {
          x: currentOffset.x * ratio,
          y: currentOffset.y * ratio,
        },
        nextLayout,
        cropFrameSize
      )
    )
  }, [cropFrameSize, imageSize, zoom])

  function openFilePicker() {
    if (interactionsDisabled) {
      return
    }

    fileInputRef.current?.click()
  }

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]

    event.target.value = ""

    if (!file) {
      return
    }

    if (!acceptedImageTypes.has(file.type)) {
      resetImage()
      setError("请选择 PNG、JPG 或 WebP 图片")
      return
    }

    if (file.size > avatarMaxSourceBytes) {
      resetImage()
      setError("图片文件不能超过 5MiB")
      return
    }

    setError("")
    setImageSize(null)
    setZoom(1)
    setOffset({ x: 0, y: 0 })
    setSourceFile(file)
    setSourceUrl(URL.createObjectURL(file))
  }

  function handleImageLoad(event: React.SyntheticEvent<HTMLImageElement>) {
    const nextImageSize = {
      height: event.currentTarget.naturalHeight,
      width: event.currentTarget.naturalWidth,
    }

    if (
      nextImageSize.width < avatarMinSourceSize ||
      nextImageSize.height < avatarMinSourceSize
    ) {
      resetImage()
      setError("图片尺寸不能小于 64x64")
      return
    }

    if (
      nextImageSize.width > avatarMaxSourceSize ||
      nextImageSize.height > avatarMaxSourceSize
    ) {
      resetImage()
      setError("图片尺寸不能超过 4096x4096")
      return
    }

    const nextLayout = getImageLayout(nextImageSize, 1, cropFrameSize)

    setImageSize(nextImageSize)
    setZoom(1)
    setOffset({
      x: (cropFrameSize - nextLayout.displayWidth) / 2,
      y: (cropFrameSize - nextLayout.displayHeight) / 2,
    })
  }

  function handleImageError() {
    resetImage()
    setError("图片读取失败")
  }

  function handlePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (interactionsDisabled || !imageLayout) {
      return
    }

    event.currentTarget.setPointerCapture(event.pointerId)
    dragStartRef.current = {
      offset,
      pointer: {
        x: event.clientX,
        y: event.clientY,
      },
    }
    setDragging(true)
  }

  function handlePointerMove(event: React.PointerEvent<HTMLDivElement>) {
    if (!dragStartRef.current || !imageLayout) {
      return
    }

    const delta = {
      x: event.clientX - dragStartRef.current.pointer.x,
      y: event.clientY - dragStartRef.current.pointer.y,
    }

    setOffset(
      clampOffset(
        {
          x: dragStartRef.current.offset.x + delta.x,
          y: dragStartRef.current.offset.y + delta.y,
        },
        imageLayout,
        cropFrameSize
      )
    )
  }

  function handlePointerEnd(event: React.PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }

    dragStartRef.current = null
    setDragging(false)
  }

  function handleZoomChange(nextZoomValue: number) {
    const nextZoom = clamp(nextZoomValue, avatarZoomMin, avatarZoomMax)

    if (!imageSize || !imageLayout) {
      setZoom(nextZoom)
      return
    }

    const nextLayout = getImageLayout(imageSize, nextZoom, cropFrameSize)
    const cropCenter = cropFrameSize / 2

    setOffset((currentOffset) => {
      const imageFocus = {
        x: (cropCenter - currentOffset.x) / imageLayout.displayWidth,
        y: (cropCenter - currentOffset.y) / imageLayout.displayHeight,
      }

      return clampOffset(
        {
          x: cropCenter - imageFocus.x * nextLayout.displayWidth,
          y: cropCenter - imageFocus.y * nextLayout.displayHeight,
        },
        nextLayout,
        cropFrameSize
      )
    })
    setZoom(nextZoom)
  }

  function resetImage() {
    setImageSize(null)
    setSourceFile(null)
    setSourceUrl("")
    setDragging(false)
    setOffset({ x: 0, y: 0 })
    setZoom(1)
    dragStartRef.current = null
  }

  async function handleSave() {
    if (!sourceFile || !imageLayout || !imageRef.current) {
      return
    }

    const avatar = await createCroppedAvatar({
      image: imageRef.current,
      layout: imageLayout,
      offset,
      viewportSize: cropFrameSize,
      sourceFile,
    })

    if (avatar.file.size > avatarMaxOutputBytes) {
      setError("裁切后的头像文件不能超过 1MiB")
      return
    }

    await onSave(avatar)
  }

  return (
    <div className="grid gap-4">
      <input
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={handleFileChange}
        ref={fileInputRef}
        type="file"
      />

      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto]">
        <div
          aria-label={sourceUrl ? "头像裁切区域" : undefined}
          className={cn(
            "relative aspect-square w-full overflow-hidden rounded-md bg-muted",
            sourceUrl
              ? "touch-none"
              : "bg-muted transition-colors hover:bg-muted/20",
            sourceUrl &&
              (interactionsDisabled
                ? "cursor-default opacity-60"
                : "cursor-grab"),
            dragging && "cursor-grabbing"
          )}
          onPointerCancel={sourceUrl ? handlePointerEnd : undefined}
          onPointerDown={sourceUrl ? handlePointerDown : undefined}
          onPointerMove={sourceUrl ? handlePointerMove : undefined}
          onPointerUp={sourceUrl ? handlePointerEnd : undefined}
          ref={cropFrameRef}
          role={sourceUrl ? "img" : undefined}
        >
          {sourceUrl ? (
            <img
              alt=""
              className="absolute top-0 left-0 max-w-none select-none"
              draggable={false}
              onError={handleImageError}
              onLoad={handleImageLoad}
              ref={imageRef}
              src={sourceUrl}
              style={
                imageLayout
                  ? {
                      height: imageLayout.displayHeight,
                      transform: `translate(${offset.x}px, ${offset.y}px)`,
                      width: imageLayout.displayWidth,
                    }
                  : undefined
              }
            />
          ) : (
            <button
              className={cn(
                "absolute inset-0 flex flex-col items-center justify-center gap-3 px-4 text-center text-sm text-muted-foreground",
                interactionsDisabled && "pointer-events-none opacity-50"
              )}
              disabled={interactionsDisabled}
              onClick={openFilePicker}
              type="button"
            >
              <span className="flex size-12 items-center justify-center rounded-md bg-background text-foreground shadow-xs">
                <Upload className="size-5" />
              </span>
              <span className="text-foreground">选择图片</span>
              <span>PNG、JPG、WebP，最大 5MiB</span>
            </button>
          )}
          {!sourceUrl && (
            <div className="pointer-events-none absolute inset-0 rounded-md ring-1 ring-foreground/15 ring-inset" />
          )}
        </div>

        <div className="flex min-h-48 w-max items-stretch justify-between gap-3 sm:min-h-0 sm:flex-col sm:items-center">
          <div className="grid gap-2">
            <Button
              aria-label="放大图片"
              disabled={cropControlsDisabled || zoom >= avatarZoomMax}
              onClick={() => handleZoomChange(zoom + avatarZoomStep)}
              size="icon-sm"
              title="放大图片"
              type="button"
              variant="outline"
            >
              <Plus className="size-4" />
            </Button>
            <Button
              aria-label="缩小图片"
              disabled={cropControlsDisabled || zoom <= avatarZoomMin}
              onClick={() => handleZoomChange(zoom - avatarZoomStep)}
              size="icon-sm"
              title="缩小图片"
              type="button"
              variant="outline"
            >
              <Minus className="size-4" />
            </Button>
          </div>
          <Button
            aria-label="清除图片"
            disabled={interactionsDisabled || !sourceUrl}
            onClick={resetImage}
            size="icon-sm"
            title="清除图片"
            type="button"
            variant="outline"
          >
            <RotateCcw className="size-4" />
          </Button>
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          disabled={interactionsDisabled || !sourceUrl || !imageLayout}
          onClick={() => void handleSave()}
          type="button"
        >
          {saving && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          保存
        </Button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}

async function createCroppedAvatar({
  image,
  layout,
  offset,
  sourceFile,
  viewportSize,
}: {
  image: HTMLImageElement
  layout: ImageLayout
  offset: Point
  sourceFile: File
  viewportSize: number
}) {
  const canvas = document.createElement("canvas")
  const outputScale = avatarOutputSize / viewportSize

  canvas.height = avatarOutputSize
  canvas.width = avatarOutputSize

  const context = canvas.getContext("2d")

  if (!context) {
    throw new Error("canvas context unavailable")
  }

  context.drawImage(
    image,
    offset.x * outputScale,
    offset.y * outputScale,
    layout.displayWidth * outputScale,
    layout.displayHeight * outputScale
  )

  const previewUrl = canvas.toDataURL(avatarOutputType, avatarOutputQuality)
  const blob =
    (await canvasToBlob(canvas, avatarOutputType, avatarOutputQuality)) ??
    dataUrlToBlob(previewUrl)
  const file = new File([blob], createAvatarFileName(sourceFile.name), {
    type: blob.type || avatarOutputType,
  })

  return {
    file,
    previewUrl,
  }
}

function canvasToBlob(
  canvas: HTMLCanvasElement,
  type: string,
  quality: number
) {
  return new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, type, quality)
  })
}

function dataUrlToBlob(dataUrl: string) {
  const [metadata, content = ""] = dataUrl.split(",")
  const mimeType = metadata.match(/^data:(.*?);/)?.[1] || avatarOutputType
  const binary = atob(content)
  const bytes = new Uint8Array(binary.length)

  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }

  return new Blob([bytes], { type: mimeType })
}

function createAvatarFileName(fileName: string) {
  const baseName = fileName.trim().replace(/\.[^.]+$/, "") || "avatar"

  return `${baseName}.webp`
}

function getImageLayout(
  imageSize: ImageSize,
  zoom: number,
  viewportSize: number
): ImageLayout {
  const scale =
    (viewportSize / Math.min(imageSize.width, imageSize.height)) * zoom

  return {
    displayHeight: imageSize.height * scale,
    displayWidth: imageSize.width * scale,
  }
}

function clampOffset(
  offset: Point,
  layout: ImageLayout,
  viewportSize: number
): Point {
  return {
    x: clamp(offset.x, viewportSize - layout.displayWidth, 0),
    y: clamp(offset.y, viewportSize - layout.displayHeight, 0),
  }
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}
