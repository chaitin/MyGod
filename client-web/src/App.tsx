import { useEffect, type ReactNode } from "react"
import { Navigate, Route, Routes } from "react-router"

import { AppLayout } from "@/components/app-layout"
import { ClientConversationRealtimeSync } from "@/components/client-conversation-realtime-sync"
import { ClientDataProvider } from "@/components/client-data-provider"
import { ClientRealtimeProvider } from "@/components/client-realtime-provider"
import { AppInfoProvider } from "@/components/app-info-provider"
import { useAppInfo } from "@/lib/app-info-context"
import { ChatPage } from "@/pages/chat-page"
import { ConnectionsPage } from "@/pages/connections-page"
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
                <ClientConversationRealtimeSync />
                <AppLayout />
              </ClientRealtimeProvider>
            </ClientDataProvider>
          }
        >
          <Route
            path="/init"
            element={
              <PageTitle title="加载">
                <InitPage />
              </PageTitle>
            }
          />
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
          <Route
            path="/connections"
            element={
              <PageTitle title="连接">
                <ConnectionsPage />
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

function InitPage() {
  return <Navigate to="/chat" replace />
}
