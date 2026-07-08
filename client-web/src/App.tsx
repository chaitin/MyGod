import { Navigate, Route, Routes } from "react-router"

import { AppLayout } from "@/components/app-layout"
import { ClientConversationRealtimeSync } from "@/components/client-conversation-realtime-sync"
import { ClientDataProvider } from "@/components/client-data-provider"
import { ClientDocumentTitle } from "@/components/client-document-title"
import { ClientMessageNotificationSync } from "@/components/client-message-notification-sync"
import { ClientRealtimeProvider } from "@/components/client-realtime-provider"
import { GlobalBeforeUnloadGuard } from "@/components/global-before-unload-guard"
import { AppInfoProvider } from "@/components/app-info-provider"
import { ChatPage } from "@/pages/chat-page"
import { ConnectionsPage } from "@/pages/connections-page"
import { ContactsPage } from "@/pages/contacts-page"
import { LoginPage } from "@/pages/login-page"
import { TasksPage } from "@/pages/tasks-page"

export function App() {
  return (
    <AppInfoProvider>
      <GlobalBeforeUnloadGuard />
      <Routes>
        <Route path="/" element={<Navigate to="/login" replace />} />
        <Route
          path="/login"
          element={
            <>
              <ClientDocumentTitle title="登录" disableMessageAlert />
              <LoginPage />
            </>
          }
        />
        <Route
          element={
            <ClientDataProvider>
              <ClientRealtimeProvider>
                <ClientConversationRealtimeSync />
                <ClientMessageNotificationSync />
                <AppLayout />
              </ClientRealtimeProvider>
            </ClientDataProvider>
          }
        >
          <Route
            path="/init"
            element={
              <>
                <ClientDocumentTitle title="正在加载" disableMessageAlert />
                <InitPage />
              </>
            }
          />
          <Route
            path="/chat"
            element={
              <>
                <ClientDocumentTitle title="聊天" />
                <ChatPage />
              </>
            }
          />
          <Route
            path="/contacts"
            element={
              <>
                <ClientDocumentTitle title="联系人" />
                <ContactsPage />
              </>
            }
          />
          <Route
            path="/tasks"
            element={
              <>
                <ClientDocumentTitle title="任务" />
                <TasksPage />
              </>
            }
          />
          <Route
            path="/connections"
            element={
              <>
                <ClientDocumentTitle title="连接" />
                <ConnectionsPage />
              </>
            }
          />
        </Route>
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </AppInfoProvider>
  )
}

export default App

function InitPage() {
  return <Navigate to="/chat" replace />
}
