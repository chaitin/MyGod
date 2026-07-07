export const imageMessageMaxBytes = 2 * 1024 * 1024

const imageMessageMaxDimension = 1024
const imageMessageOutputType = "image/webp"
const imageMessageOutputQuality = 0.82
const acceptedImageMessageTypes = new Set([
  "image/jpeg",
  "image/png",
  "image/webp",
])

export async function compressImageForMessage(sourceFile: File) {
  if (!isAcceptedImageMessageFile(sourceFile)) {
    throw new Error("请选择 PNG、JPG 或 WebP 图片")
  }

  const image = await loadImage(sourceFile)
  const sourceWidth = image.naturalWidth
  const sourceHeight = image.naturalHeight

  if (sourceWidth <= 0 || sourceHeight <= 0) {
    throw new Error("读取图片失败")
  }

  const scale = Math.min(
    1,
    imageMessageMaxDimension / Math.max(sourceWidth, sourceHeight)
  )
  const outputWidth = Math.max(1, Math.round(sourceWidth * scale))
  const outputHeight = Math.max(1, Math.round(sourceHeight * scale))
  const canvas = document.createElement("canvas")

  canvas.width = outputWidth
  canvas.height = outputHeight

  const context = canvas.getContext("2d")

  if (!context) {
    throw new Error("读取图片失败")
  }

  context.drawImage(image, 0, 0, outputWidth, outputHeight)

  const blob =
    (await canvasToBlob(
      canvas,
      imageMessageOutputType,
      imageMessageOutputQuality
    )) ??
    dataUrlToBlob(
      canvas.toDataURL(imageMessageOutputType, imageMessageOutputQuality)
    )

  return new File([blob], createImageMessageFileName(sourceFile.name), {
    lastModified: Date.now(),
    type: blob.type || imageMessageOutputType,
  })
}

function isAcceptedImageMessageFile(file: File) {
  if (acceptedImageMessageTypes.has(file.type)) {
    return true
  }

  return /\.(jpe?g|png|webp)$/i.test(file.name)
}

function loadImage(file: File) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const image = new Image()

    image.onload = () => {
      URL.revokeObjectURL(url)
      resolve(image)
    }
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error("读取图片失败"))
    }
    image.src = url
  })
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
  const mimeType = metadata.match(/^data:(.*?);/)?.[1] || imageMessageOutputType
  const binary = atob(content)
  const bytes = new Uint8Array(binary.length)

  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }

  return new Blob([bytes], { type: mimeType })
}

function createImageMessageFileName(fileName: string) {
  const baseName = fileName.trim().replace(/\.[^.]+$/, "") || "image"

  return `${baseName}.webp`
}
