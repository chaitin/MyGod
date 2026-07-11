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
import { ContactsPage } from "@/pages/contacts-page"
import { LoginPage } from "@/pages/login-page"
import { ProjectsPage } from "@/pages/projects-page"

export function App() {
  return (
    <AppInfoProvider>
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
            <>
              <GlobalBeforeUnloadGuard />
              <ClientDataProvider>
                <ClientRealtimeProvider>
                  <ClientConversationRealtimeSync />
                  <ClientMessageNotificationSync />
                  <AppLayout />
                </ClientRealtimeProvider>
              </ClientDataProvider>
            </>
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
            path="/chat/:conversationId?"
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
            path="/projects/:projectId?"
            element={
              <>
                <ClientDocumentTitle title="项目" />
                <ProjectsPage />
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
