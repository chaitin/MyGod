import type {
  ClientContacts,
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/data/models"

export type DirectoryTab = "user" | "app" | "group"

export type DirectoryItem =
  | { key: string; type: "user"; value: ContactUser }
  | { key: string; type: "app"; value: ContactApp }
  | { key: string; type: "group"; value: ContactGroup }

export type DirectorySection = {
  count: number
  data: DirectoryItem[]
  title?: string
}

const contactNameCollator = new Intl.Collator("zh-CN-u-co-pinyin", {
  numeric: true,
  sensitivity: "base",
  usage: "sort",
})

export function buildDirectorySections({
  activeTab,
  contacts,
  keyword,
  organizationName,
}: {
  activeTab: DirectoryTab
  contacts: ClientContacts
  keyword: string
  organizationName: string
}): DirectorySection[] {
  const normalizedKeyword = keyword.trim().toLocaleLowerCase()

  if (activeTab === "app") {
    const apps = contacts.apps.filter((app) =>
      matchesKeyword([app.name, app.description], normalizedKeyword)
    )

    return apps.length > 0
      ? [
          {
            count: apps.length,
            data: apps.map((app) => ({
              key: `app:${app.id}`,
              type: "app",
              value: app,
            })),
          },
        ]
      : []
  }

  if (activeTab === "group") {
    const groups = contacts.groups.filter((group) =>
      matchesKeyword([group.name], normalizedKeyword)
    )
    const joinedGroups = groups.filter((group) => group.joined)
    const publicGroups = groups.filter(
      (group) => group.visibility === "public"
    )

    return [
      createGroupSection("我加入的", "joined", joinedGroups),
      createGroupSection("公开群组", "public", publicGroups),
    ].filter((section) => section.data.length > 0)
  }

  const users = [...contacts.users]
    .filter((contact) =>
      matchesKeyword(
        [
          contact.email,
          contact.name,
          contact.nickname,
          contact.phone,
          formatContactPhone(contact.phone),
        ],
        normalizedKeyword
      )
    )
    .sort(compareContactsByDisplayName)

  return users.length > 0
    ? [
        {
          count: users.length,
          data: users.map((contact) => ({
            key: `user:${contact.id}`,
            type: "user",
            value: contact,
          })),
          title: organizationName,
        },
      ]
    : []
}

export function getContactDisplayName(contact: {
  name: string
  nickname: string
}) {
  return contact.nickname.trim() || contact.name.trim()
}

export function getContactInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}

export function formatContactPhone(phone: string) {
  return phone.startsWith("+86") ? phone.slice(3) : phone
}

function createGroupSection(
  title: string,
  sectionKey: string,
  groups: ContactGroup[]
): DirectorySection {
  return {
    count: groups.length,
    data: groups.map((group) => ({
      key: `group:${sectionKey}:${group.id}`,
      type: "group",
      value: group,
    })),
    title,
  }
}

function matchesKeyword(values: string[], keyword: string) {
  return (
    keyword.length === 0 ||
    values.some((value) => value.toLocaleLowerCase().includes(keyword))
  )
}

function compareContactsByDisplayName(left: ContactUser, right: ContactUser) {
  return (
    contactNameCollator.compare(
      getContactDisplayName(left),
      getContactDisplayName(right)
    ) ||
    contactNameCollator.compare(left.email, right.email) ||
    contactNameCollator.compare(left.id, right.id)
  )
}
