import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react"

export type AuthSession = {
  serverUrl: string
}

type AuthContextValue = {
  isAuthenticated: boolean
  session: AuthSession | null
  signIn: (session: AuthSession) => void
  signOut: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: React.PropsWithChildren) {
  const [session, setSession] = useState<AuthSession | null>(null)
  const signIn = useCallback((nextSession: AuthSession) => {
    setSession(nextSession)
  }, [])
  const signOut = useCallback(() => {
    setSession(null)
  }, [])
  const value = useMemo(
    () => ({
      isAuthenticated: session !== null,
      session,
      signIn,
      signOut,
    }),
    [session, signIn, signOut]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)

  if (!value) {
    throw new Error("useAuth 必须在 AuthProvider 内使用")
  }

  return value
}
