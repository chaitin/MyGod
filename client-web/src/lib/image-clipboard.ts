import { readTemporaryFileURLs } from "@/lib/client-data-api"

const clipboardImageType = "image/png"

export async function copyTemporaryImageToClipboard(fileId: string) {
  if (
    !window.isSecureContext ||
    !navigator.clipboard?.write ||
    typeof ClipboardItem === "undefined"
  ) {
    throw new Error("image clipboard is unavailable")
  }

  const imageBlob = readTemporaryImageAsPNG(fileId)
  await navigator.clipboard.write([
    new ClipboardItem({
      [clipboardImageType]: imageBlob,
    }),
  ])
}

async function readTemporaryImageAsPNG(fileId: string) {
  const readURLs = await readTemporaryFileURLs([fileId])
  const readURL =
    readURLs.find((item) => item.fileId === fileId)?.url ?? readURLs[0]?.url

  if (!readURL) {
    throw new Error("missing image read url")
  }

  const response = await fetch(readURL)
  if (!response.ok) {
    throw new Error(`read image: HTTP ${response.status}`)
  }

  const imageBlob = await response.blob()
  if (imageBlob.type.toLowerCase() === clipboardImageType) {
    return imageBlob
  }

  return convertImageBlobToPNG(imageBlob)
}

async function convertImageBlobToPNG(imageBlob: Blob) {
  const objectURL = URL.createObjectURL(imageBlob)

  try {
    const image = await loadImage(objectURL)
    if (image.naturalWidth <= 0 || image.naturalHeight <= 0) {
      throw new Error("invalid image dimensions")
    }

    const canvas = document.createElement("canvas")
    canvas.width = image.naturalWidth
    canvas.height = image.naturalHeight

    const context = canvas.getContext("2d")
    if (!context) {
      throw new Error("canvas is unavailable")
    }

    context.drawImage(image, 0, 0)
    return await canvasToPNGBlob(canvas)
  } finally {
    URL.revokeObjectURL(objectURL)
  }
}

function loadImage(source: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error("decode image failed"))
    image.src = source
  })
}

function canvasToPNGBlob(canvas: HTMLCanvasElement) {
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) {
        resolve(blob)
        return
      }

      reject(new Error("encode image failed"))
    }, clipboardImageType)
  })
}
