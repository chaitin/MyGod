import AsyncStorage from "@react-native-async-storage/async-storage"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"

import {
  isValidServerUrl,
  normalizeServerUrl,
  OFFICIAL_SERVER_ID,
  officialServer,
  type ServerConfig,
} from "@/features/servers/server-model"
const SERVER_STORAGE_KEY = "@magicchat/servers/v1"

type PersistedServerState = {
  customServers: ServerConfig[]
  selectedServerId: string
}

type AddServerResult =
  | { server: ServerConfig; status: "added" }
  | { status: "duplicate" | "invalid" }

type ServerContextValue = {
  addServer: (name: string, url: string) => AddServerResult
  isHydrated: boolean
  removeServer: (id: string) => void
  selectedServer: ServerConfig
  selectServer: (id: string) => void
  servers: ServerConfig[]
}

const ServerContext = createContext<ServerContextValue | null>(null)

export function ServerProvider({ children }: React.PropsWithChildren) {
  const [customServers, setCustomServers] = useState<ServerConfig[]>([])
  const [selectedServerId, setSelectedServerId] = useState(OFFICIAL_SERVER_ID)
  const [isHydrated, setIsHydrated] = useState(false)
  const servers = useMemo(
    () => [officialServer, ...customServers],
    [customServers]
  )
  const selectedServer =
    servers.find((server) => server.id === selectedServerId) ?? officialServer

  useEffect(() => {
    let isCancelled = false

    async function hydrate() {
      try {
        const storedValue = await AsyncStorage.getItem(SERVER_STORAGE_KEY)
        const storedState = parsePersistedState(storedValue)

        if (!isCancelled && storedState) {
          setCustomServers(storedState.customServers)
          setSelectedServerId(storedState.selectedServerId)
        }
      } catch {
        // Keep the built-in server available when local storage cannot be read.
      } finally {
        if (!isCancelled) {
          setIsHydrated(true)
        }
      }
    }

    void hydrate()

    return () => {
      isCancelled = true
    }
  }, [])

  useEffect(() => {
    if (!isHydrated) {
      return
    }

    const state: PersistedServerState = {
      customServers,
      selectedServerId: selectedServer.id,
    }

    void AsyncStorage.setItem(SERVER_STORAGE_KEY, JSON.stringify(state)).catch(
      () => {
        // The in-memory list remains usable if local persistence is unavailable.
      }
    )
  }, [customServers, isHydrated, selectedServer.id])

  const addServer = useCallback(
    (name: string, url: string): AddServerResult => {
      const normalizedName = name.trim()

      if (!normalizedName || !isValidServerUrl(url)) {
        return { status: "invalid" }
      }

      const normalizedUrl = normalizeServerUrl(url)
      const isDuplicate = servers.some(
        (server) => normalizeServerUrl(server.url) === normalizedUrl
      )

      if (isDuplicate) {
        return { status: "duplicate" }
      }

      const server: ServerConfig = {
        id: createServerId(),
        isBuiltIn: false,
        name: normalizedName,
        url: normalizedUrl,
      }

      setCustomServers((current) => [...current, server])

      return { server, status: "added" }
    },
    [servers]
  )

  const selectServer = useCallback(
    (id: string) => {
      if (servers.some((server) => server.id === id)) {
        setSelectedServerId(id)
      }
    },
    [servers]
  )

  const removeServer = useCallback((id: string) => {
    if (id === OFFICIAL_SERVER_ID) {
      return
    }

    setCustomServers((current) =>
      current.filter((server) => server.id !== id)
    )
    setSelectedServerId((current) =>
      current === id ? OFFICIAL_SERVER_ID : current
    )
  }, [])

  const value = useMemo(
    () => ({
      addServer,
      isHydrated,
      removeServer,
      selectedServer,
      selectServer,
      servers,
    }),
    [
      addServer,
      isHydrated,
      removeServer,
      selectedServer,
      selectServer,
      servers,
    ]
  )

  return (
    <ServerContext.Provider value={value}>{children}</ServerContext.Provider>
  )
}

export function useServers() {
  const value = useContext(ServerContext)

  if (!value) {
    throw new Error("useServers 必须在 ServerProvider 内使用")
  }

  return value
}

function createServerId() {
  return `server-${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 10)}`
}

function parsePersistedState(value: string | null): PersistedServerState | null {
  if (!value) {
    return null
  }

  try {
    const parsed: unknown = JSON.parse(value)

    if (!isRecord(parsed) || !Array.isArray(parsed.customServers)) {
      return null
    }

    const seenIds = new Set<string>()
    const seenUrls = new Set<string>([normalizeServerUrl(officialServer.url)])
    const customServers = parsed.customServers.flatMap((candidate) => {
      if (!isStoredServer(candidate)) {
        return []
      }

      const url = normalizeServerUrl(candidate.url)

      if (
        candidate.id === OFFICIAL_SERVER_ID ||
        seenIds.has(candidate.id) ||
        seenUrls.has(url)
      ) {
        return []
      }

      seenIds.add(candidate.id)
      seenUrls.add(url)

      return [
        {
          id: candidate.id,
          isBuiltIn: false,
          name: candidate.name.trim(),
          url,
        },
      ]
    })
    const selectedServerId =
      typeof parsed.selectedServerId === "string" &&
      customServers.some((server) => server.id === parsed.selectedServerId)
        ? parsed.selectedServerId
        : OFFICIAL_SERVER_ID

    return { customServers, selectedServerId }
  } catch {
    return null
  }
}

function isStoredServer(value: unknown): value is ServerConfig {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    value.id.length > 0 &&
    typeof value.name === "string" &&
    value.name.trim().length > 0 &&
    typeof value.url === "string" &&
    isValidServerUrl(value.url)
  )
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}
