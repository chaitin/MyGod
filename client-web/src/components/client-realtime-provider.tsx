import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useNavigate } from "react-router"
import { toast } from "sonner"

import { RealtimeClient, type RealtimeSnapshot } from "@/lib/realtime-client"
import {
  RealtimeContext,
  type RealtimeContextValue,
} from "@/lib/realtime-context"
import { ClientLoadingPage } from "@/components/client-loading-page"

export function ClientRealtimeProvider({
  children,
  client: providedClient,
}: {
  children: ReactNode
  client?: RealtimeClient
}) {
  const navigate = useNavigate()
  const [client] = useState(
    () =>
      providedClient ??
      new RealtimeClient({
        authCheck: checkClientSession,
        onUnauthorized: () => {
          navigate("/login", { replace: true })
        },
      })
  )
  const [snapshot, setSnapshot] = useState<RealtimeSnapshot>(() =>
    client.getSnapshot()
  )
  const [hasReadyOnce, setHasReadyOnce] = useState(snapshot.ready)
  const reconnectingToastShownRef = useRef(false)

  useEffect(() => {
    return client.subscribe(() => {
      const nextSnapshot = client.getSnapshot()
      setSnapshot(nextSnapshot)
      if (nextSnapshot.ready) {
        setHasReadyOnce(true)
      }
    })
  }, [client])

  useEffect(() => {
    client.connect()

    function handleVisibilityChange() {
      if (document.visibilityState === "visible") {
        client.connect()
      }
    }

    document.addEventListener("visibilitychange", handleVisibilityChange)

    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange)
      client.disconnect()
    }
  }, [client])

  useEffect(() => {
    if (snapshot.ready) {
      if (hasReadyOnce && reconnectingToastShownRef.current) {
        toast.success("网络已恢复连接")
      }
      reconnectingToastShownRef.current = false
      return
    }

    if (
      snapshot.status === "reconnecting" &&
      hasReadyOnce &&
      !reconnectingToastShownRef.current
    ) {
      reconnectingToastShownRef.current = true
      toast.warning("网络断开，正在尝试重新连接")
    }
  }, [hasReadyOnce, snapshot.ready, snapshot.status])

  const sendRealtimeRequest = useCallback(
    (method: string, payload: unknown) => client.sendRequest(method, payload),
    [client]
  )

  const value = useMemo<RealtimeContextValue>(
    () => ({
      sendRealtimeRequest,
      status: snapshot.status,
    }),
    [sendRealtimeRequest, snapshot.status]
  )

  const ready = snapshot.ready || hasReadyOnce
  if (!ready) {
    return <ClientLoadingPage />
  }

  return (
    <RealtimeContext.Provider value={value}>
      {children}
    </RealtimeContext.Provider>
  )
}

async function checkClientSession() {
  const response = await fetch("/api/client/me", {
    credentials: "include",
  })

  return response.status !== 401
}
