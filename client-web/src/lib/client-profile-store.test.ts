import { describe, expect, it, vi } from "vitest"

import { ClientProfileStore } from "@/lib/client-profile-store"

describe("ClientProfileStore", () => {
  it("reuses equal profile objects without notifying subscribers", () => {
    const snapshot = createSnapshot()
    const store = new ClientProfileStore(snapshot)
    const originalUser = store.getUser("user-2")
    const originalApp = store.getApp("app-1")
    const userListener = vi.fn()
    const appListener = vi.fn()
    store.subscribeUser("user-2", userListener)
    store.subscribeApp("app-1", appListener)

    store.replace({
      contactApps: snapshot.contactApps.map((app) => ({ ...app })),
      contacts: snapshot.contacts.map((contact) => ({ ...contact })),
      me: { ...snapshot.me },
    })

    expect(store.getUser("user-2")).toBe(originalUser)
    expect(store.getApp("app-1")).toBe(originalApp)
    expect(userListener).not.toHaveBeenCalled()
    expect(appListener).not.toHaveBeenCalled()
  })

  it("notifies changed and removed profile IDs independently", () => {
    const snapshot = createSnapshot()
    const store = new ClientProfileStore(snapshot)
    const userListener = vi.fn()
    const appListener = vi.fn()
    store.subscribeUser("USER-2", userListener)
    store.subscribeApp("APP-1", appListener)

    store.replace({
      contactApps: [],
      contacts: [{ ...snapshot.contacts[0], online: true }],
      me: snapshot.me,
    })

    expect(userListener).toHaveBeenCalledOnce()
    expect(appListener).toHaveBeenCalledOnce()
    expect(store.getApp("app-1")).toBeUndefined()
  })
})

function createSnapshot() {
  return {
    contactApps: [
      {
        avatar: "",
        creatorUserId: null,
        description: "AI 助手",
        id: "app-1",
        name: "茉莉",
        online: true,
        type: "app" as const,
      },
    ],
    contacts: [
      {
        avatar: "",
        email: "bob@example.com",
        id: "user-2",
        lastOnlineAt: null,
        name: "Bob",
        nickname: "",
        online: false,
        phone: "",
        type: "user" as const,
      },
    ],
    me: {
      avatar: "",
      createdAt: "2026-07-22T00:00:00Z",
      email: "alice@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "Alice",
      nickname: "",
      phone: "",
      status: "active" as const,
    },
  }
}
