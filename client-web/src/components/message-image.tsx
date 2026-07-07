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

export function MessageImage({ image }: MessageImageProps) {
  const [open, setOpen] = React.useState(false)
  const [source, setSource] = React.useState<{
    error: boolean
    fileId: string
    loaded: boolean
    url: string | null
  } | null>(null)

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

  const currentSource = source?.fileId === image.fileId ? source : null

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
        onClick={() => setOpen(true)}
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
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-[min(96vw,72rem)] gap-3 p-3">
          <DialogHeader className="sr-only">
            <DialogTitle>图片预览</DialogTitle>
            <DialogDescription>查看图片消息大图</DialogDescription>
          </DialogHeader>
          <div className="flex max-h-[82vh] items-center justify-center overflow-hidden rounded-md bg-black/5 dark:bg-white/5">
            <img
              alt="图片消息预览"
              className="max-h-[82vh] max-w-full object-contain"
              onError={handleImageError}
              src={currentSource.url}
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
