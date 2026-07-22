import * as React from "react"
import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { ClientProfileProvider } from "@/components/client-profile-provider"
import {
  type ClientProfileData,
  useClientAppProfile,
  useClientUserProfile,
} from "@/lib/client-profile-context"

describe("ClientProfileProvider", () => {
  it("does not notify profile consumers for unrelated parent renders", () => {
    const profileData = createProfileData()
    let profileRenderCount = 0

    const ProfileProbe = React.memo(function ProfileProbe() {
      profileRenderCount += 1
      const profile = useClientUserProfile("user-1")

      return <span>{profile?.name}</span>
    })

    function Harness({ revision }: { revision: number }) {
      return (
        <div data-revision={revision}>
          <ClientProfileProvider {...profileData}>
            <ProfileProbe />
          </ClientProfileProvider>
        </div>
      )
    }

    const view = render(<Harness revision={1} />)
    expect(screen.getByText("Alice")).toBeInTheDocument()
    expect(profileRenderCount).toBe(1)

    view.rerender(<Harness revision={2} />)
    expect(profileRenderCount).toBe(1)
  })

  it("preserves user subscriptions for content-identical refreshes", () => {
    const bob = createContact("user-2", "Bob")
    const profileData = createProfileData({ contacts: [bob] })
    let profileRenderCount = 0

    const ProfileProbe = React.memo(function ProfileProbe() {
      profileRenderCount += 1
      const profile = useClientUserProfile(bob.id)

      return <span>{profile?.name}</span>
    })

    const view = render(
      <ClientProfileProvider {...profileData}>
        <ProfileProbe />
      </ClientProfileProvider>
    )
    expect(profileRenderCount).toBe(1)

    view.rerender(
      <ClientProfileProvider
        {...profileData}
        contactApps={profileData.contactApps.map((app) => ({ ...app }))}
        contacts={profileData.contacts.map((contact) => ({ ...contact }))}
        me={{ ...profileData.me }}
      >
        <ProfileProbe />
      </ClientProfileProvider>
    )

    expect(profileRenderCount).toBe(1)
  })

  it("notifies only the user whose profile changed", () => {
    const bob = createContact("user-2", "Bob")
    const carol = createContact("user-3", "Carol")
    const profileData = createProfileData({ contacts: [bob, carol] })
    const renderCounts = { bob: 0, carol: 0 }

    const BobProbe = React.memo(function BobProbe() {
      renderCounts.bob += 1
      const profile = useClientUserProfile(bob.id)
      return <span>{profile?.nickname || profile?.name}</span>
    })
    const CarolProbe = React.memo(function CarolProbe() {
      renderCounts.carol += 1
      return <span>{useClientUserProfile(carol.id)?.name}</span>
    })

    const view = render(
      <ClientProfileProvider {...profileData}>
        <BobProbe />
        <CarolProbe />
      </ClientProfileProvider>
    )

    view.rerender(
      <ClientProfileProvider
        {...profileData}
        contacts={[{ ...bob, nickname: "Bobby" }, { ...carol }]}
      >
        <BobProbe />
        <CarolProbe />
      </ClientProfileProvider>
    )

    expect(renderCounts).toEqual({ bob: 2, carol: 1 })
    expect(screen.getByText("Bobby")).toBeInTheDocument()
  })

  it("notifies only the app whose profile changed", () => {
    const assistant = createApp("app-1", "茉莉")
    const calendar = createApp("app-2", "日历")
    const profileData = createProfileData({
      contactApps: [assistant, calendar],
    })
    const renderCounts = { assistant: 0, calendar: 0 }

    const AssistantProbe = React.memo(function AssistantProbe() {
      renderCounts.assistant += 1
      const profile = useClientAppProfile(assistant.id)
      return (
        <span>
          {profile?.name}:{String(profile?.online)}
        </span>
      )
    })
    const CalendarProbe = React.memo(function CalendarProbe() {
      renderCounts.calendar += 1
      return <span>{useClientAppProfile(calendar.id)?.name}</span>
    })

    const view = render(
      <ClientProfileProvider {...profileData}>
        <AssistantProbe />
        <CalendarProbe />
      </ClientProfileProvider>
    )

    view.rerender(
      <ClientProfileProvider
        {...profileData}
        contactApps={[{ ...assistant, online: false }, { ...calendar }]}
      >
        <AssistantProbe />
        <CalendarProbe />
      </ClientProfileProvider>
    )

    expect(renderCounts).toEqual({ assistant: 2, calendar: 1 })
    expect(screen.getByText("茉莉:false")).toBeInTheDocument()
  })
})

function createProfileData(
  overrides: Partial<ClientProfileData> = {}
): ClientProfileData {
  return {
    contactApps: [],
    contacts: [],
    me: {
      avatar: "",
      createdAt: "2026-07-22T00:00:00Z",
      email: "alice@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "Alice",
      nickname: "",
      phone: "",
      status: "active",
    },
    openAppConversation: vi.fn(async () => {
      throw new Error("not implemented")
    }),
    openDirectConversation: vi.fn(async () => {
      throw new Error("not implemented")
    }),
    ...overrides,
  }
}

function createContact(id: string, name: string) {
  return {
    avatar: "",
    email: `${name.toLowerCase()}@example.com`,
    id,
    lastOnlineAt: null,
    name,
    nickname: "",
    online: false,
    phone: "",
    type: "user" as const,
  }
}

function createApp(id: string, name: string) {
  return {
    avatar: "",
    creatorUserId: null,
    description: "",
    id,
    name,
    online: true,
    type: "app" as const,
  }
}
