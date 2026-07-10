# Sidebar Chat Unread Dot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a reusable red notification dot on the sidebar chat button whenever any conversation has unread messages, and flash it once whenever the global unread total increases.

**Architecture:** Keep the presentation-only `NotificationDot` primitive responsible for dot appearance but not positioning or chat state. `AppLayout` derives the global unread total, compares it with the previous rendered total, and passes visibility plus a restartable animation version to the chat navigation item. CSS owns the 520ms keyframes and the operating-system/browser reduced-motion fallback.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, React Router, Vitest, Testing Library.

---

## File Structure

- Create `client-web/src/components/ui/notification-dot.tsx`: reusable visual notification-dot primitive with overridable `span` props and styles.
- Modify `client-web/src/components/app-layout.tsx`: derive global unread state, compare unread totals, and render a restartable one-shot animation on the chat navigation entry.
- Modify `client-web/src/components/app-layout.test.tsx`: verify visible/hidden states, accessible labeling, 4px positioning, and animation trigger/restart rules.
- Modify `client-web/src/index.css`: define the 520ms flash keyframes and the `prefers-reduced-motion` override.

`client-web/src/components/app-layout.tsx` already contains unrelated uncommitted navigation changes. Preserve those changes and stage only this feature's hunks if committing.

Tasks 1 and 2 were completed in `c4de4f3`. The follow-up positioning change and Tasks 3 and 4 complete the approved animation extension.

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
          <NotificationDot className="absolute top-1 right-1 ring-sidebar" />
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

### Task 3: Add a Restartable One-Shot Flash on Unread Increases

**Files:**
- Modify: `client-web/src/components/app-layout.test.tsx`
- Modify: `client-web/src/components/app-layout.tsx:1-80,279-325`
- Modify: `client-web/src/index.css`

- [ ] **Step 1: Write failing tests for the animation trigger rules**

Add `fireEvent` to the Testing Library import:

```tsx
import { fireEvent, render, screen, within } from "@testing-library/react"
```

Add these tests inside the existing `describe("AppLayout", ...)` block:

```tsx
it("does not flash unread messages already present on initial load", () => {
  mocks.clientData.conversations = [{ unreadCount: 2 }]

  render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const notificationDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(notificationDot).not.toHaveClass("notification-dot-flash")
})

it("flashes when the global unread total increases", () => {
  mocks.clientData.conversations = [{ unreadCount: 0 }]
  const view = render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  mocks.clientData.conversations = [{ unreadCount: 1 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const notificationDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(notificationDot).toHaveClass("notification-dot-flash")
})

it("restarts the flash when another unread message arrives", () => {
  mocks.clientData.conversations = [{ unreadCount: 0 }]
  const view = render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  mocks.clientData.conversations = [{ unreadCount: 1 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )
  const firstDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  mocks.clientData.conversations = [{ unreadCount: 2 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )
  const restartedDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(restartedDot).not.toBe(firstDot)
  expect(restartedDot).toHaveClass("notification-dot-flash")
})

it("returns the dot to its static state when the flash ends", () => {
  mocks.clientData.conversations = [{ unreadCount: 0 }]
  const view = render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  mocks.clientData.conversations = [{ unreadCount: 1 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )
  const notificationDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(notificationDot).toHaveClass("notification-dot-flash")
  fireEvent.animationEnd(notificationDot!)
  expect(notificationDot).not.toHaveClass("notification-dot-flash")
})

it("does not flash when the global unread total decreases", () => {
  mocks.clientData.conversations = [{ unreadCount: 2 }]
  const view = render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  mocks.clientData.conversations = [{ unreadCount: 1 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const notificationDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(notificationDot).not.toHaveClass("notification-dot-flash")
})

it("does not flash when a message leaves the global unread total unchanged", () => {
  mocks.clientData.conversations = [{ unreadCount: 1 }]
  const view = render(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  mocks.clientData.conversations = [{ unreadCount: 1 }]
  view.rerender(
    <MemoryRouter initialEntries={["/chat"]}>
      <AppLayout />
    </MemoryRouter>
  )

  const notificationDot = screen
    .getByRole("link", { name: "聊天，有未读消息" })
    .querySelector('[data-slot="notification-dot"]')

  expect(notificationDot).not.toHaveClass("notification-dot-flash")
})
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd client-web
pnpm exec vitest run src/components/app-layout.test.tsx
```

Expected: the initial-load, decrease, and unchanged-total tests remain green, while the increase, restart, and animation-end tests fail because `AppLayout` does not yet add `notification-dot-flash` or replace the dot on consecutive increases.

- [ ] **Step 3: Track unread increases in one animation state snapshot**

Keep the existing React state import:

```tsx
import { useState } from "react"
```

Replace the current unread boolean derivation in `AppLayout` with a state snapshot that also stores the most recently observed total. The guarded state update makes React retry `AppLayout` immediately and terminates after `unreadCount` matches `totalUnreadCount`, avoiding a synchronous state update inside an effect:

```tsx
const totalUnreadCount = conversations.reduce(
  (total, conversation) => total + conversation.unreadCount,
  0
)
const hasUnreadMessages = totalUnreadCount > 0
const [notificationAnimation, setNotificationAnimation] = useState({
  active: false,
  unreadCount: totalUnreadCount,
  version: 0,
})

if (notificationAnimation.unreadCount !== totalUnreadCount) {
  const unreadCountIncreased =
    totalUnreadCount > notificationAnimation.unreadCount

  setNotificationAnimation({
    active: unreadCountIncreased,
    unreadCount: totalUnreadCount,
    version: unreadCountIncreased
      ? notificationAnimation.version + 1
      : notificationAnimation.version,
  })
}

function handleNotificationAnimationEnd() {
  setNotificationAnimation((current) =>
    current.active ? { ...current, active: false } : current
  )
}
```

```tsx
<MainNavItem
  key={item.to}
  item={item}
  showNotification={item.to === "/chat" && hasUnreadMessages}
  notificationAnimationActive={
    item.to === "/chat" && notificationAnimation.active
  }
  notificationAnimationVersion={notificationAnimation.version}
  onNotificationAnimationEnd={handleNotificationAnimationEnd}
/>
```

- [ ] **Step 4: Apply and restart the animation in MainNavItem**

Import the existing `cn` helper:

```tsx
import { cn } from "@/lib/utils"
```

Extend `MainNavItem` and render the dot with the animation key and end handler:

```tsx
function MainNavItem({
  item,
  notificationAnimationActive,
  notificationAnimationVersion,
  onNotificationAnimationEnd,
  showNotification,
}: {
  item: (typeof navItems)[number]
  notificationAnimationActive: boolean
  notificationAnimationVersion: number
  onNotificationAnimationEnd: () => void
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
          <NotificationDot
            key={notificationAnimationVersion}
            className={cn(
              "absolute top-1 right-1 ring-sidebar",
              notificationAnimationActive && "notification-dot-flash"
            )}
            onAnimationEnd={onNotificationAnimationEnd}
          />
        )}
      </NavLink>
    </Button>
  )
}
```

- [ ] **Step 5: Add the exact 520ms animation and reduced-motion fallback**

Add the component class inside `@layer components` in `client-web/src/index.css`:

```css
.notification-dot-flash {
  animation: notification-dot-flash 520ms ease-out 1;
}
```

Add the keyframes and media query after the existing keyframes:

```css
@keyframes notification-dot-flash {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }

  30% {
    opacity: 0.3;
    transform: scale(0.8);
  }

  60% {
    opacity: 1;
    transform: scale(1.35);
  }
}

@media (prefers-reduced-motion: reduce) {
  .notification-dot-flash {
    animation: none;
  }
}
```

- [ ] **Step 6: Run the focused test and verify GREEN**

Run:

```bash
cd client-web
pnpm exec vitest run src/components/app-layout.test.tsx
```

Expected: all `app-layout.test.tsx` tests pass, including initial unread, increase, consecutive restart, animation end, decrease, unchanged total, and 4px positioning.

### Task 4: Verify and Commit the Animation Extension

**Files:**
- Verify: `client-web/src/components/app-layout.test.tsx`
- Verify: `client-web/src/components/app-layout.tsx`
- Verify: `client-web/src/index.css`

- [ ] **Step 1: Run the full client verification suite**

Run from `client-web/`:

```bash
pnpm test
pnpm typecheck
```

Expected: all client tests pass and TypeScript exits with status 0.

- [ ] **Step 2: Inspect the complete working and staged diffs**

Run from the repository root:

```bash
git diff --check
git diff -- client-web/src/components/app-layout.test.tsx \
  client-web/src/components/app-layout.tsx \
  client-web/src/index.css
git status --short
```

Expected: the animation changes are limited to the three intended files. Unrelated project-page and navigation changes remain preserved in the working tree.

- [ ] **Step 3: Selectively stage only the feature hunks**

Stage the animation CSS and tests, then selectively stage only the unread-dot positioning and animation hunks from `app-layout.tsx`:

```bash
git add client-web/src/components/app-layout.test.tsx client-web/src/index.css
git add -p -- client-web/src/components/app-layout.tsx
git diff --cached --check
git diff --cached --name-status
git diff --cached
```

Expected: the staged diff contains the 4px position test/change, animation behavior tests, unread-total state and handler, and CSS animation. It does not include deletion of the Tasks/Connections navigation entries or any project-page work.

- [ ] **Step 4: Commit the verified animation extension**

Run:

```bash
git commit -m "feat(chat): flash sidebar unread indicator"
```

Expected: the commit contains only the intended sidebar unread animation extension.
