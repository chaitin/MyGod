import type { ClientUser, ContactApp, ContactUser } from "@/lib/client-data-api"

export type ClientUserProfile = ClientUser | ContactUser

export type ClientProfileSnapshot = {
  contactApps: ContactApp[]
  contacts: ContactUser[]
  me: ClientUser
}

type Listener = () => void

export class ClientProfileStore {
  private appListeners = new Map<string, Set<Listener>>()
  private apps = new Map<string, ContactApp>()
  private currentUserId = ""
  private currentUserIdListeners = new Set<Listener>()
  private userListeners = new Map<string, Set<Listener>>()
  private users = new Map<string, ClientUserProfile>()

  constructor(snapshot: ClientProfileSnapshot) {
    this.replace(snapshot, false)
  }

  getApp(appId: string | null | undefined) {
    return this.apps.get(normalizeProfileId(appId))
  }

  getCurrentUserId() {
    return this.currentUserId
  }

  getUser(userId: string | null | undefined) {
    return this.users.get(normalizeProfileId(userId))
  }

  replace(snapshot: ClientProfileSnapshot, notify = true) {
    const nextUsers = new Map<string, ClientUserProfile>()
    for (const contact of snapshot.contacts) {
      nextUsers.set(normalizeProfileId(contact.id), contact)
    }
    nextUsers.set(normalizeProfileId(snapshot.me.id), snapshot.me)

    const nextApps = new Map<string, ContactApp>()
    for (const app of snapshot.contactApps) {
      nextApps.set(normalizeProfileId(app.id), app)
    }

    const reconciledUsers = reconcileProfiles(
      this.users,
      nextUsers,
      areUserProfilesEqual
    )
    const reconciledApps = reconcileProfiles(
      this.apps,
      nextApps,
      areAppProfilesEqual
    )
    const nextCurrentUserId = normalizeProfileId(snapshot.me.id)
    const currentUserIdChanged = this.currentUserId !== nextCurrentUserId

    this.users = reconciledUsers.profiles
    this.apps = reconciledApps.profiles
    this.currentUserId = nextCurrentUserId

    if (!notify) {
      return
    }
    for (const userId of reconciledUsers.changedIds) {
      notifyListeners(this.userListeners.get(userId))
    }
    for (const appId of reconciledApps.changedIds) {
      notifyListeners(this.appListeners.get(appId))
    }
    if (currentUserIdChanged) {
      notifyListeners(this.currentUserIdListeners)
    }
  }

  subscribeApp(appId: string | null | undefined, listener: Listener) {
    return subscribeById(this.appListeners, normalizeProfileId(appId), listener)
  }

  subscribeCurrentUserId(listener: Listener) {
    this.currentUserIdListeners.add(listener)
    return () => this.currentUserIdListeners.delete(listener)
  }

  subscribeUser(userId: string | null | undefined, listener: Listener) {
    return subscribeById(
      this.userListeners,
      normalizeProfileId(userId),
      listener
    )
  }
}

function reconcileProfiles<T>(
  current: ReadonlyMap<string, T>,
  incoming: ReadonlyMap<string, T>,
  equal: (left: T, right: T) => boolean
) {
  const profiles = new Map<string, T>()
  const changedIds = new Set(current.keys())

  for (const [id, profile] of incoming) {
    const previous = current.get(id)
    if (previous && equal(previous, profile)) {
      profiles.set(id, previous)
      changedIds.delete(id)
      continue
    }
    profiles.set(id, profile)
    changedIds.add(id)
  }

  return { changedIds, profiles }
}

function areUserProfilesEqual(
  left: ClientUserProfile,
  right: ClientUserProfile
) {
  if (
    left.avatar !== right.avatar ||
    left.email !== right.email ||
    left.id !== right.id ||
    left.lastOnlineAt !== right.lastOnlineAt ||
    left.name !== right.name ||
    left.nickname !== right.nickname ||
    left.phone !== right.phone
  ) {
    return false
  }

  if ("status" in left || "status" in right) {
    return (
      "status" in left &&
      "status" in right &&
      left.createdAt === right.createdAt &&
      left.status === right.status
    )
  }

  return left.online === right.online && left.type === right.type
}

function areAppProfilesEqual(left: ContactApp, right: ContactApp) {
  return (
    left.avatar === right.avatar &&
    left.creatorUserId === right.creatorUserId &&
    left.description === right.description &&
    left.id === right.id &&
    left.name === right.name &&
    left.online === right.online &&
    left.type === right.type
  )
}

function normalizeProfileId(profileId: string | null | undefined) {
  return profileId?.trim().toLowerCase() ?? ""
}

function notifyListeners(listeners: ReadonlySet<Listener> | undefined) {
  if (!listeners) {
    return
  }
  for (const listener of [...listeners]) {
    listener()
  }
}

function subscribeById(
  listenersById: Map<string, Set<Listener>>,
  id: string,
  listener: Listener
) {
  if (!id) {
    return () => undefined
  }

  const listeners = listenersById.get(id) ?? new Set<Listener>()
  listeners.add(listener)
  listenersById.set(id, listeners)

  return () => {
    listeners.delete(listener)
    if (listeners.size === 0) {
      listenersById.delete(id)
    }
  }
}
