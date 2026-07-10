# Sidebar Chat Unread Dot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a reusable red notification dot on the sidebar chat button whenever any conversation has unread messages.

**Architecture:** Add a presentation-only `NotificationDot` UI primitive that owns the dot appearance but not positioning. `AppLayout` derives one boolean from the existing conversation `unreadCount` values and passes it to the chat navigation item, which owns positioning and accessible labeling.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, React Router, Vitest, Testing Library.

---

## File Structure

- Create `client-web/src/components/ui/notification-dot.tsx`: reusable visual notification-dot primitive with overridable `span` props and styles.
- Modify `client-web/src/components/app-layout.tsx`: derive global unread state and render the dot on the chat navigation entry.
- Modify `client-web/src/components/app-layout.test.tsx`: verify visible/hidden states and the accessible chat label.

`client-web/src/components/app-layout.tsx` already contains unrelated uncommitted navigation changes. Preserve those changes and stage only this feature's hunks if committing.

### Task 1: Add the Reusable Notification Dot and Chat Indicator

**Files:**
- Create: `client-web/src/components/ui/notification-dot.tsx`
- Modify: `client-web/src/components/app-layout.tsx:18-75,281-297`
- Test: `client-web/src/components/app-layout.test.tsx`

- [ ] **Step 1: Add unread conversation state to the test mock**

Update the Vitest import and `clientData` mock in `app-layout.test.tsx`:

```tsx
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  clientData: {
    conversations: [] as Array<{ unreadCount: number }>,
    me: {
      avatar: "",
      createdAt: "2026-07-09T00:00:00Z",
      email: "me@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "张三",
      nickname: "三三",
      phone: "",
      status: "active",
    },
    refreshMe: vi.fn(),
  },
  clientLogout: vi.fn(),
  setTheme: vi.fn(),
  updateCurrentClientUser: vi.fn(),
  uploadCurrentClientAvatar: vi.fn(),
}))

beforeEach(() => {
  mocks.clientData.conversations = []
})
```

- [ ] **Step 2: Write failing sidebar unread tests**

Add these tests inside the existing `describe("AppLayout", ...)` block:

```tsx
it("shows a notification dot when any conversation is unread", () => {
  mocks.clientData.conversations = [
    { unreadCount: 0 },
    { unreadCount: 2 },
  ]

  render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const chatLink = screen.getByRole("link", {
    name: "聊天，有未读消息",
  })

  expect(
    chatLink.querySelector('[data-slot="notification-dot"]')
  ).toBeInTheDocument()
})

it("hides the notification dot when every conversation is read", () => {
  mocks.clientData.conversations = [
    { unreadCount: 0 },
    { unreadCount: 0 },
  ]

  render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const chatLink = screen.getByRole("link", { name: "聊天" })

  expect(
    chatLink.querySelector('[data-slot="notification-dot"]')
  ).not.toBeInTheDocument()
})
```

The first test deliberately uses the active `/chat` route, proving that route activity does not suppress a real unread state.

- [ ] **Step 3: Run the test and verify RED**

Run:

```bash
cd client-web
pnpm exec vitest run src/components/app-layout.test.tsx
```

Expected: the unread test fails because the chat link is still named `聊天` and no `[data-slot="notification-dot"]` element exists.

- [ ] **Step 4: Create the reusable NotificationDot primitive**

Create `client-web/src/components/ui/notification-dot.tsx`:

```tsx
import * as React from "react"

import { cn } from "@/lib/utils"

function NotificationDot({
  className,
  "aria-hidden": ariaHidden = true,
  ...props
}: React.ComponentProps<"span">) {
  return (
    <span
      aria-hidden={ariaHidden}
      className={cn(
        "pointer-events-none inline-flex size-2.5 shrink-0 rounded-full bg-red-500 ring-2 ring-background",
        className
      )}
      data-slot="notification-dot"
      {...props}
    />
  )
}

export { NotificationDot }
```

The component owns appearance and default accessibility only. It intentionally does not include `absolute`, `top-*`, or `right-*` classes.

- [ ] **Step 5: Derive global unread state in AppLayout**

Import the component:

```tsx
import { NotificationDot } from "@/components/ui/notification-dot"
```

Replace the existing `useClientData()` destructuring in `AppLayout` with these statements immediately before its current `return` statement:

```tsx
const { conversations, me, refreshMe } = useClientData()
const hasUnreadMessages = conversations.some(
  (conversation) => conversation.unreadCount > 0
)
```

When mapping `navItems`, pass the indicator only to the chat route:

```tsx
{navItems.map((item) => (
  <MainNavItem
    key={item.to}
    item={item}
    showNotification={item.to === "/chat" && hasUnreadMessages}
  />
))}
```

- [ ] **Step 6: Render and label the notification in MainNavItem**

Replace the existing `MainNavItem` signature and JSX with:

```tsx
function MainNavItem({
  item,
  showNotification,
}: {
  item: (typeof navItems)[number]
  showNotification: boolean
}) {
  const active = Boolean(useMatch({ path: item.to, end: true }))
  const Icon = item.icon
  const accessibleLabel = showNotification
    ? `${item.label}，有未读消息`
    : item.label

  return (
    <Button
      asChild
      variant={active ? "default" : "ghost"}
      size="icon-sm"
      className={
        active
          ? "relative rounded-full"
          : "relative rounded-full text-muted-foreground"
      }
    >
      <NavLink to={item.to} aria-label={accessibleLabel} title={item.label}>
        <Icon className="size-4" strokeWidth={active ? 2.5 : 2} />
        {showNotification && (
          <NotificationDot className="absolute top-0 right-0 ring-sidebar" />
        )}
      </NavLink>
    </Button>
  )
}
```

- [ ] **Step 7: Run the focused test and verify GREEN**

Run:

```bash
cd client-web
pnpm exec vitest run src/components/app-layout.test.tsx
```

Expected: all `app-layout.test.tsx` tests pass with no warnings.

- [ ] **Step 8: Run the client verification suite**

Run from `client-web/`:

```bash
pnpm test
pnpm typecheck
```

Expected: all client tests pass and TypeScript exits with status 0.

- [ ] **Step 9: Inspect and commit only the feature changes**

First inspect:

```bash
git diff --check
git diff -- client-web/src/components/ui/notification-dot.tsx \
  client-web/src/components/app-layout.tsx \
  client-web/src/components/app-layout.test.tsx
```

Stage the new component and test file normally, then use selective staging for `app-layout.tsx` so its pre-existing navigation-item removal remains unstaged:

```bash
git add client-web/src/components/ui/notification-dot.tsx \
  client-web/src/components/app-layout.test.tsx
git add -p -- client-web/src/components/app-layout.tsx
git diff --cached --check
git diff --cached
git commit -m "feat(chat): show sidebar unread indicator"
```

Expected: the commit contains only `NotificationDot`, sidebar unread derivation/rendering, and the two regression tests.

### Task 2: Final State Verification

**Files:**
- Verify: `client-web/src/components/ui/notification-dot.tsx`
- Verify: `client-web/src/components/app-layout.tsx`
- Verify: `client-web/src/components/app-layout.test.tsx`

- [ ] **Step 1: Verify the committed change**

Run:

```bash
git show --stat --oneline HEAD
git show --name-only --format= HEAD
git status --short
```

Expected: the feature commit lists exactly the three intended client files. Existing unrelated project/navigation changes remain in the working tree and are not staged.

- [ ] **Step 2: Confirm acceptance criteria against the implementation**

Check the implementation against these exact conditions:

```text
some unreadCount > 0  -> chat link label includes “有未读消息” and dot exists
all unreadCount == 0  -> chat link label is “聊天” and dot does not exist
active /chat route    -> unread state still controls the dot
NotificationDot       -> contains no positioning classes
```

Expected: every condition holds without changes to server APIs, realtime synchronization, or the existing conversation-list unread badges.
