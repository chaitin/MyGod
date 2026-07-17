export const imageMessageMaxBytes = 2 * 1024 * 1024

const imageMessageMaxDimension = 1920
const imageMessageOutputType = "image/webp"
const imageMessageFallbackType = "image/png"
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

  const encodedImage = await encodeMessageImage(canvas)

  return new File(
    [encodedImage.blob],
    createImageMessageFileName(sourceFile.name, encodedImage.extension),
    {
      lastModified: Date.now(),
      type: encodedImage.type,
    }
  )
}

export function isAcceptedImageMessageFile(file: File) {
  if (isAcceptedImageMessageMimeType(file.type)) {
    return true
  }

  return /\.(jpe?g|png|webp)$/i.test(file.name)
}

export function isAcceptedImageMessageMimeType(type: string) {
  return acceptedImageMessageTypes.has(type.toLowerCase())
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
  quality?: number
) {
  return new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, type, quality)
  })
}

async function encodeMessageImage(canvas: HTMLCanvasElement) {
  const webPBlob = await canvasToBlob(
    canvas,
    imageMessageOutputType,
    imageMessageOutputQuality
  )
  if (webPBlob && (await isWebPBlob(webPBlob))) {
    return {
      blob: webPBlob,
      extension: "webp",
      type: imageMessageOutputType,
    }
  }
  if (webPBlob?.type.toLowerCase() === imageMessageFallbackType) {
    return {
      blob: webPBlob,
      extension: "png",
      type: imageMessageFallbackType,
    }
  }

  const webPDataURLBlob = dataUrlToBlob(
    canvas.toDataURL(imageMessageOutputType, imageMessageOutputQuality)
  )
  if (await isWebPBlob(webPDataURLBlob)) {
    return {
      blob: webPDataURLBlob,
      extension: "webp",
      type: imageMessageOutputType,
    }
  }
  if (webPDataURLBlob.type.toLowerCase() === imageMessageFallbackType) {
    return {
      blob: webPDataURLBlob,
      extension: "png",
      type: imageMessageFallbackType,
    }
  }

  const pngBlob =
    (await canvasToBlob(canvas, imageMessageFallbackType)) ??
    dataUrlToBlob(canvas.toDataURL(imageMessageFallbackType))
  if (pngBlob.type.toLowerCase() !== imageMessageFallbackType) {
    throw new Error("转换图片失败")
  }

  return {
    blob: pngBlob,
    extension: "png",
    type: imageMessageFallbackType,
  }
}

async function isWebPBlob(blob: Blob) {
  if (blob.type.toLowerCase() !== imageMessageOutputType || blob.size < 12) {
    return false
  }

  const header = new Uint8Array(await blob.slice(0, 12).arrayBuffer())
  return (
    bytesEqualASCII(header, 0, "RIFF") && bytesEqualASCII(header, 8, "WEBP")
  )
}

function bytesEqualASCII(bytes: Uint8Array, offset: number, value: string) {
  return Array.from(value).every(
    (character, index) => bytes[offset + index] === character.charCodeAt(0)
  )
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

function createImageMessageFileName(fileName: string, extension: string) {
  const baseName = fileName.trim().replace(/\.[^.]+$/, "") || "image"

  return `${baseName}.${extension}`
}
