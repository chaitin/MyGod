import { useEffect, type ReactNode } from "react"
import { Navigate, Route, Routes } from "react-router"

import { AppLayout } from "@/components/app-layout"
import { ClientDataProvider } from "@/components/client-data-provider"
import { ClientRealtimeProvider } from "@/components/client-realtime-provider"
import { AppInfoProvider } from "@/components/app-info-provider"
import { useAppInfo } from "@/lib/app-info-context"
import { ChatPage } from "@/pages/chat-page"
import { ContactsPage } from "@/pages/contacts-page"
import { LoginPage } from "@/pages/login-page"
import { TasksPage } from "@/pages/tasks-page"

export function App() {
  return (
    <AppInfoProvider>
      <Routes>
        <Route path="/" element={<Navigate to="/login" replace />} />
        <Route
          path="/login"
          element={
            <PageTitle title="登录">
              <LoginPage />
            </PageTitle>
          }
        />
        <Route
          element={
            <ClientDataProvider>
              <ClientRealtimeProvider>
                <AppLayout />
              </ClientRealtimeProvider>
            </ClientDataProvider>
          }
        >
          <Route
            path="/chat"
            element={
              <PageTitle title="聊天">
                <ChatPage />
              </PageTitle>
            }
          />
          <Route
            path="/contacts"
            element={
              <PageTitle title="联系人">
                <ContactsPage />
              </PageTitle>
            }
          />
          <Route
            path="/tasks"
            element={
              <PageTitle title="任务">
                <TasksPage />
              </PageTitle>
            }
          />
        </Route>
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </AppInfoProvider>
  )
}

export default App

function PageTitle({
  children,
  title,
}: {
  children: ReactNode
  title: string
}) {
  const { appName } = useAppInfo()

  useEffect(() => {
    document.title = `${title} - ${appName}`
  }, [appName, title])

  return children
}
