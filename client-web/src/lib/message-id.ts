type ClientMessageIdCrypto = {
  getRandomValues?: (array: Uint8Array) => Uint8Array
  randomUUID?: () => string
}

export function createClientMessageId(
  randomSource: ClientMessageIdCrypto | undefined = getGlobalCrypto()
) {
  if (typeof randomSource?.randomUUID === "function") {
    return randomSource.randomUUID()
  }

  if (typeof randomSource?.getRandomValues === "function") {
    const bytes = new Uint8Array(16)
    randomSource.getRandomValues(bytes)

    return formatUUIDv4(bytes)
  }

  return formatUUIDv4(createMathRandomBytes())
}

function getGlobalCrypto(): ClientMessageIdCrypto | undefined {
  if (typeof globalThis === "undefined") {
    return undefined
  }

  return globalThis.crypto
}

function createMathRandomBytes() {
  const bytes = new Uint8Array(16)

  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Math.floor(Math.random() * 256)
  }

  return bytes
}

function formatUUIDv4(bytes: Uint8Array) {
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80

  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0"))

  return [
    hex.slice(0, 4).join(""),
    hex.slice(4, 6).join(""),
    hex.slice(6, 8).join(""),
    hex.slice(8, 10).join(""),
    hex.slice(10, 16).join(""),
  ].join("-")
}
