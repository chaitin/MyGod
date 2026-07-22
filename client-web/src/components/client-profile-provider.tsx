import * as React from "react"

import {
  type ClientProfileData,
  ClientProfileContext,
} from "@/lib/client-profile-context"
import { ClientProfileStore } from "@/lib/client-profile-store"

export function ClientProfileProvider({
  children,
  contactApps,
  contacts,
  me,
  openAppConversation,
  openDirectConversation,
}: ClientProfileData & { children: React.ReactNode }) {
  const [store] = React.useState(
    () => new ClientProfileStore({ contactApps, contacts, me })
  )
  React.useLayoutEffect(() => {
    store.replace({ contactApps, contacts, me })
  }, [contactApps, contacts, me, store])

  const value = React.useMemo(
    () => ({
      openAppConversation,
      openDirectConversation,
      store,
    }),
    [openAppConversation, openDirectConversation, store]
  )

  return (
    <ClientProfileContext.Provider value={value}>
      {children}
    </ClientProfileContext.Provider>
  )
}
