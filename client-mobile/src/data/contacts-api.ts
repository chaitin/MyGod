import { ApiRequestError, createApiClient, type ApiFetch } from "@/data/api-client"
import type {
  ClientContacts,
  ContactApp,
  ContactGroup,
  ContactGroupAvatarMember,
  ContactUser,
} from "@/data/models"

type ContactUserResponse = {
  avatar?: string
  email?: string
  id?: string
  last_online_at?: string | null
  name?: string
  nickname?: string
  online?: boolean
  phone?: string
}

type ContactAppResponse = {
  avatar?: string
  description?: string
  id?: string
  name?: string
  online?: boolean
}

type ContactGroupAvatarMemberResponse = {
  avatar?: string
  name?: string
  nickname?: string
  role?: string
}

type ContactGroupResponse = {
  avatar?: string
  avatar_members?: ContactGroupAvatarMemberResponse[]
  id?: string
  joined?: boolean
  member_count?: number
  name?: string
  visibility?: string
}

type ContactsResponse = {
  apps?: ContactAppResponse[]
  groups?: ContactGroupResponse[]
  users?: ContactUserResponse[]
}

export async function fetchContacts(
  serverUrl: string,
  options: { fetcher?: ApiFetch; signal?: AbortSignal } = {}
) {
  const data = await createApiClient(serverUrl, options.fetcher).request<
    ContactsResponse
  >("/api/client/contacts", {
    errorMessage: "加载通讯录失败",
    method: "GET",
    signal: options.signal,
  })

  if (
    !data ||
    !Array.isArray(data.apps) ||
    !Array.isArray(data.groups) ||
    !Array.isArray(data.users)
  ) {
    throw new ApiRequestError("通讯录响应格式不正确")
  }

  return {
    apps: data.apps.map(normalizeContactApp),
    groups: data.groups.map(normalizeContactGroup),
    users: data.users.map(normalizeContactUser),
  } satisfies ClientContacts
}

function normalizeContactUser(contact: ContactUserResponse): ContactUser {
  if (!contact.email || !contact.id || !contact.name) {
    throw new ApiRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: contact.avatar ?? "",
    email: contact.email,
    id: contact.id,
    lastOnlineAt: contact.last_online_at ?? null,
    name: contact.name,
    nickname: contact.nickname ?? "",
    online: Boolean(contact.online),
    phone: contact.phone ?? "",
    type: "user",
  }
}

function normalizeContactApp(app: ContactAppResponse): ContactApp {
  if (!app.id || !app.name) {
    throw new ApiRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: app.avatar ?? "",
    description: app.description ?? "",
    id: app.id,
    name: app.name,
    online: Boolean(app.online),
    type: "app",
  }
}

function normalizeContactGroup(group: ContactGroupResponse): ContactGroup {
  if (!group.id || !group.name) {
    throw new ApiRequestError("通讯录响应格式不正确")
  }

  return {
    avatar: group.avatar ?? "",
    avatarMembers: (group.avatar_members ?? []).map(
      normalizeContactGroupAvatarMember
    ),
    id: group.id,
    joined: Boolean(group.joined),
    memberCount: group.member_count ?? 0,
    name: group.name,
    type: "group",
    visibility: group.visibility === "public" ? "public" : "private",
  }
}

function normalizeContactGroupAvatarMember(
  member: ContactGroupAvatarMemberResponse
): ContactGroupAvatarMember {
  if (!member.name) {
    throw new ApiRequestError("通讯录群头像成员响应格式不正确")
  }

  return {
    avatar: member.avatar ?? "",
    name: member.name,
    nickname: member.nickname ?? "",
    role:
      member.role === "owner" || member.role === "admin"
        ? member.role
        : "member",
  }
}
