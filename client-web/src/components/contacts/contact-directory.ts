import type {
  ContactApp,
  ContactGroup,
  ContactUser,
} from "@/lib/client-data-api"

export type DirectorySelection =
  | { id: string; type: "app" }
  | { id: string; type: "group" }
  | { id: string; type: "user" }

export type ActiveDirectoryItem =
  | { app: ContactApp; type: "app" }
  | { group: ContactGroup; type: "group" }
  | { contact: ContactUser; type: "user" }

export type DirectoryTab = DirectorySelection["type"]

export function createDirectorySelection(
  type: string | undefined,
  id: string | undefined
): DirectorySelection | null {
  if (!id || (type !== "app" && type !== "group" && type !== "user")) {
    return null
  }

  return { id, type }
}

export function getDirectorySelectionPath(selection: DirectorySelection) {
  return `/contacts/${selection.type}/${encodeURIComponent(selection.id)}`
}

export function resolveActiveDirectoryItem(
  selection: DirectorySelection | null,
  apps: ContactApp[],
  contacts: ContactUser[],
  groups: ContactGroup[]
): ActiveDirectoryItem | null {
  if (!selection) {
    return null
  }

  if (selection.type === "app") {
    const app = apps.find((item) => item.id === selection.id)
    return app ? { app, type: "app" } : null
  }

  if (selection.type === "group") {
    const group = groups.find((item) => item.id === selection.id)
    return group ? { group, type: "group" } : null
  }

  const contact = contacts.find((item) => item.id === selection.id)
  return contact ? { contact, type: "user" } : null
}

export function directoryItemKey(
  type: DirectorySelection["type"],
  id: string
) {
  return `${type}:${id}`
}
