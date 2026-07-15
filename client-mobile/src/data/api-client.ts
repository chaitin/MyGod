const DEFAULT_REQUEST_TIMEOUT_MS = 10_000

export type ApiFetch = (
  input: string,
  init?: RequestInit
) => Promise<Response>

type ApiErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type ApiSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type ApiRequestOptions = Omit<RequestInit, "credentials"> & {
  errorMessage: string
  timeoutMs?: number
}

export class ApiRequestError extends Error {
  code?: string
  status?: number

  constructor(
    message: string,
    options: { code?: string; status?: number } = {}
  ) {
    super(message)
    this.name = "ApiRequestError"
    this.code = options.code
    this.status = options.status
  }
}

export function createApiClient(
  serverUrl: string,
  fetcher: ApiFetch = fetch
) {
  const baseUrl = `${serverUrl.replace(/\/+$/, "")}/`

  return {
    async request<T>(path: string, options: ApiRequestOptions) {
      const {
        errorMessage,
        signal: parentSignal,
        timeoutMs = DEFAULT_REQUEST_TIMEOUT_MS,
        ...requestInit
      } = options
      const controller = new AbortController()
      let didTimeout = false

      const handleParentAbort = () => controller.abort()
      if (parentSignal?.aborted) {
        controller.abort()
      } else {
        parentSignal?.addEventListener("abort", handleParentAbort, {
          once: true,
        })
      }

      const timeout = setTimeout(() => {
        didTimeout = true
        controller.abort()
      }, timeoutMs)

      try {
        const endpoint = new URL(path.replace(/^\/+/, ""), baseUrl).toString()
        const response = await fetcher(endpoint, {
          ...requestInit,
          credentials: "include",
          signal: controller.signal,
        })
        const payload = await readJson<
          ApiErrorEnvelope | ApiSuccessEnvelope<T>
        >(response)

        if (!response.ok || payload?.success === false) {
          const error = (payload as ApiErrorEnvelope | undefined)?.error

          throw new ApiRequestError(
            error?.message ?? `${errorMessage}（HTTP ${response.status}）`,
            {
              code: error?.code,
              status: response.status,
            }
          )
        }

        return (payload as ApiSuccessEnvelope<T> | undefined)?.data
      } catch (error: unknown) {
        if (error instanceof ApiRequestError || parentSignal?.aborted) {
          throw error
        }

        if (didTimeout) {
          throw new ApiRequestError(`${errorMessage}：请求超时`)
        }

        throw new ApiRequestError(`${errorMessage}：无法连接到服务器`)
      } finally {
        clearTimeout(timeout)
        parentSignal?.removeEventListener("abort", handleParentAbort)
      }
    },
  }
}

export function isUnauthorizedError(error: unknown) {
  return (
    error instanceof ApiRequestError &&
    (error.status === 401 || error.code === "unauthorized")
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type") ?? ""

  if (!contentType.includes("application/json")) {
    return undefined
  }

  try {
    return (await response.json()) as T
  } catch {
    throw new ApiRequestError("服务器响应格式不正确", {
      status: response.status,
    })
  }
}
