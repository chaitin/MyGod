import { useEffect, useState } from "react"
import { Navigate, Route, Routes, useLocation } from "react-router-dom"

import Console from "@/console"
import { addAdminUnauthorizedListener, isAuthenticated } from "@/lib/auth"
import { defaultConsolePage } from "@/lib/console-pages"
import AppsPage from "@/pages/apps-page"
import AssistantPage from "@/pages/assistant-page"
import LoginPage from "@/pages/login-page"
import MembersPage from "@/pages/members-page"
import SettingsPage from "@/pages/settings-page"

export function App() {
  const [authenticated, setAuthenticated] = useState(() => isAuthenticated())

  useEffect(() => {
    return addAdminUnauthorizedListener(() => setAuthenticated(false))
  }, [])

  return (
    <Routes>
      <Route
        element={
          <LoginPage
            authenticated={authenticated}
            onLogin={() => setAuthenticated(true)}
          />
        }
        path="/login"
      />
      <Route
        element={<ProtectedConsole authenticated={authenticated} />}
        path="/"
      >
        <Route element={<Navigate replace to={defaultConsolePage} />} index />
        <Route element={<MembersPage />} path="members" />
        <Route element={<AppsPage />} path="apps" />
        <Route element={<SettingsPage />} path="settings" />
        <Route element={<AssistantPage />} path="assistant" />
      </Route>
      <Route
        element={
          <Navigate
            replace
            to={authenticated ? defaultConsolePage : "/login"}
          />
        }
        path="*"
      />
    </Routes>
  )
}

function ProtectedConsole({ authenticated }: { authenticated: boolean }) {
  const location = useLocation()

  if (!authenticated) {
    return (
      <Navigate
        replace
        state={{
          from: {
            pathname: location.pathname,
            search: location.search,
          },
        }}
        to="/login"
      />
    )
  }

  return <Console />
}

export default App
