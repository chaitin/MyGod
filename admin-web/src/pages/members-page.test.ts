import { describe, expect, it } from "vitest"

import membersPageSourceText from "./members-page.tsx?raw"

import {
  formatMemberPhone,
  getColumns,
  getMemberAvatarFallback,
  getMemberActionConfirmation,
  getMemberAvatarClassName,
  getMemberColumnClassName,
  getMemberOnlineStatusText,
  getMemberOptionalDisplayValue,
  getMembersTableContainerClassName,
  getMembersTableClassName,
  getResetPasswordPendingDialogState,
} from "@/pages/members-page"

describe("members page reset password dialog state", () => {
  it("opens the reset password dialog in a pending state before the API returns", () => {
    const member = {
      avatar: "/assets/avatars/builtin/01.webp",
      email: "alice@example.com",
      id: "user-1",
      joinedAt: "2026-07-01",
      lastOnlineAt: "2026-07-03T01:45:00Z",
      name: "Alice",
      nickname: "",
      online: true,
      phone: "",
      status: "enabled" as const,
    }

    expect(getResetPasswordPendingDialogState(member)).toEqual({
      isPending: true,
      member,
      newPassword: "",
      open: true,
    })
  })
})

describe("members page phone formatting", () => {
  it("omits +86 when displaying mainland China phone numbers", () => {
    expect(formatMemberPhone("+8613812345678")).toBe("13812345678")
    expect(formatMemberPhone("+862112345678")).toBe("2112345678")
  })

  it("keeps non +86 phone numbers unchanged", () => {
    expect(formatMemberPhone("+14155552671")).toBe("+14155552671")
  })
})

describe("members page optional display values", () => {
  it("shows a dash placeholder for missing values", () => {
    expect(getMemberOptionalDisplayValue("")).toBe("-")
    expect(getMemberOptionalDisplayValue("   ")).toBe("-")
  })

  it("keeps present values trimmed", () => {
    expect(getMemberOptionalDisplayValue("  小爱  ")).toBe("小爱")
  })
})

describe("members page online status text", () => {
  const now = new Date("2026-07-03T02:00:00Z")

  it("shows current online text for online members", () => {
    expect(
      getMemberOnlineStatusText(
        {
          lastOnlineAt: "2026-07-03T01:45:00Z",
          online: true,
        },
        now
      )
    ).toBe("当前在线")
  })

  it("shows relative last online text for offline members", () => {
    expect(
      getMemberOnlineStatusText(
        {
          lastOnlineAt: "2026-07-03T01:55:00Z",
          online: false,
        },
        now
      )
    ).toBe("5分钟前在线")
  })

  it("shows a fallback when an offline member has no online record", () => {
    expect(
      getMemberOnlineStatusText(
        {
          lastOnlineAt: "",
          online: false,
        },
        now
      )
    ).toBe("从未在线")
  })
})

describe("members page avatar fallback", () => {
  it("uses the nickname initial before name or email", () => {
    expect(
      getMemberAvatarFallback({
        email: "alice@example.com",
        name: "Alice",
        nickname: "小爱",
      })
    ).toBe("小")
  })

  it("falls back to the name initial when nickname is missing", () => {
    expect(
      getMemberAvatarFallback({
        email: "alice@example.com",
        name: "Alice",
        nickname: "",
      })
    ).toBe("A")
  })

  it("falls back to the email initial when nickname and name are missing", () => {
    expect(
      getMemberAvatarFallback({
        email: "alice@example.com",
        name: "",
        nickname: "",
      })
    ).toBe("A")
  })
})

describe("members page avatar styling", () => {
  it("uses a rounded square avatar in the members table", () => {
    expect(getMemberAvatarClassName()).toBe(
      "rounded-sm after:rounded-sm [&_[data-slot=avatar-fallback]]:rounded-sm [&_[data-slot=avatar-image]]:rounded-sm"
    )
  })
})

describe("members page columns", () => {
  it("puts name first and keeps nickname in a separate data column", () => {
    const columns = getColumns({
      onResetPassword: (member) => {
        void member
      },
      onStatusChange: (member, status) => {
        void member
        void status
      },
      updatingMemberId: null,
    })

    expect(columns.map(getColumnId)).toEqual([
      "select",
      "name",
      "nickname",
      "email",
      "phone",
      "onlineStatus",
      "joinedAt",
      "status",
      "actions",
    ])
  })
})

describe("members page column widths", () => {
  it("does not force a width for the status column", () => {
    expect(getMemberColumnClassName("status")).toBeUndefined()
  })

  it("keeps the actions column aligned to the right", () => {
    expect(getMemberColumnClassName("actions")).toBe("w-12")
  })

  it("does not force widths for regular text columns", () => {
    expect(getMemberColumnClassName("name")).toBeUndefined()
  })
})

describe("members page table spacing", () => {
  it("keeps row dividers full-width while spacing edge cells", () => {
    expect(getMembersTableContainerClassName()).not.toContain("px-")
    expect(getMembersTableClassName()).toBe(
      "[&_tr>*:first-child]:pl-4 [&_tr>*:last-child]:pr-4"
    )
    expect(getMemberColumnClassName("actions")).not.toContain("pr-")
  })
})

describe("members page add member dialog isolation", () => {
  it("keeps add member form state out of the members page component", () => {
    const membersPageSource = getSourceBetween(
      membersPageSourceText,
      "export default function MembersPage()",
      "function AddMemberDialog("
    )

    expect(membersPageSource).not.toContain("addMemberEmail")
    expect(membersPageSource).not.toContain("addMemberInitialPassword")
    expect(membersPageSource).not.toContain("addMemberName")
    expect(membersPageSource).not.toContain("addMemberOpen")
    expect(membersPageSource).not.toContain("addMemberPhone")
    expect(membersPageSource).not.toContain("isAddingMember")
  })
})

describe("members page table column stability", () => {
  it("keeps table columns stable across table state changes", () => {
    const membersPageSource = getSourceBetween(
      membersPageSourceText,
      "export default function MembersPage()",
      "function AddMemberDialog("
    )

    expect(membersPageSource).toContain(
      "const handleRequestMemberStatusChange = useCallback("
    )
    expect(membersPageSource).toContain(
      "const handleRequestResetMemberPassword = useCallback("
    )
    expect(membersPageSource).toContain("const columns = useMemo(")
    expect(membersPageSource).not.toContain("const columns = getColumns(")
  })
})

describe("members page member action confirmation", () => {
  it("returns confirmation copy for enabling a member", () => {
    expect(getMemberActionConfirmation("enable")).toEqual({
      confirmLabel: "确认启用",
      description: "启用后，该成员可以重新登录系统。",
      title: "确认启用成员",
    })
  })

  it("returns confirmation copy for disabling a member", () => {
    expect(getMemberActionConfirmation("disable")).toEqual({
      confirmLabel: "确认禁用",
      description: "禁用后，该成员将无法继续登录，已有会话也会失效。",
      title: "确认禁用成员",
    })
  })

  it("returns confirmation copy for resetting a member password", () => {
    expect(getMemberActionConfirmation("reset_password")).toEqual({
      confirmLabel: "确认重置",
      description: "重置后旧密码将失效，新密码只会显示一次。",
      title: "确认重置密码",
    })
  })
})

function getColumnId(column: { accessorKey?: unknown; id?: string }) {
  return String(column.id ?? column.accessorKey)
}

function getSourceBetween(
  source: string,
  startMarker: string,
  endMarker: string
) {
  const startIndex = source.indexOf(startMarker)
  const endIndex = source.indexOf(endMarker)

  expect(startIndex).toBeGreaterThanOrEqual(0)
  expect(endIndex).toBeGreaterThan(startIndex)

  return source.slice(startIndex, endIndex)
}
