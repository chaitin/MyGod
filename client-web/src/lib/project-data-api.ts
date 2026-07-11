import { ClientDataRequestError } from "@/lib/client-data-api"

type ProjectDataFetch = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>

type ProjectDataSuccessEnvelope<T> = {
  data?: T
  success?: boolean
}

type ProjectDataErrorEnvelope = {
  error?: {
    code?: string
    message?: string
  }
  success?: boolean
}

type ProjectUserResponse = {
  avatar?: string
  id?: string
  name?: string
  nickname?: string
}

type ProjectTaskCountsResponse = {
  canceled?: number
  done?: number
  in_progress?: number
  todo?: number
  total?: number
}

type ProjectResponse = {
  avatar?: string
  created_at?: string
  current_user_role?: string
  description?: string
  group_count?: number
  id?: string
  is_personal?: boolean
  member_count?: number
  name?: string
  owner?: ProjectUserResponse
  task_counts?: ProjectTaskCountsResponse
  updated_at?: string
}

type ProjectSummaryResponse = {
  avatar?: string
  description?: string
  id?: string
  is_personal?: boolean
  name?: string
  updated_at?: string
}

type ProjectListResponse = {
  next_cursor?: string | null
  personal_project?: ProjectSummaryResponse
  projects?: ProjectSummaryResponse[]
}

type ProjectGroupResponse = {
  avatar?: string
  created_at?: string
  id?: string
  member_count?: number
  name?: string
  status?: string
}

type ProjectGroupListResponse = {
  groups?: ProjectGroupResponse[]
  next_cursor?: string | null
}

type ProjectMemberResponse = {
  avatar?: string
  display_name?: string
  email?: string
  id?: string
  name?: string
  nickname?: string
  role?: string
  source_group_ids?: string[]
  status?: string
}

type ProjectMemberListResponse = {
  members?: ProjectMemberResponse[]
  next_cursor?: string | null
}

export type ProjectUser = {
  avatar: string
  id: string
  name: string
  nickname: string
}

export type ProjectTaskCounts = {
  canceled: number
  done: number
  inProgress: number
  todo: number
  total: number
}

export type ClientProjectSummary = {
  avatar: string
  description: string
  id: string
  isPersonal: boolean
  name: string
  updatedAt: string
}

export type ClientProjectDetail = ClientProjectSummary & {
  createdAt: string
  currentUserRole: "owner" | "member"
  groupCount: number
  memberCount: number
  owner: ProjectUser
  taskCounts: ProjectTaskCounts
}

export type ClientProjectPage = {
  nextCursor: string | null
  personalProject: ClientProjectSummary
  projects: ClientProjectSummary[]
}

export type CreateClientProjectInput = {
  groupIds?: string[]
  name: string
}

export type UpdateClientProjectInput = {
  avatar?: string
  description?: string
  name?: string
}

export type ClientProjectGroup = {
  avatar: string
  createdAt: string
  id: string
  memberCount: number
  name: string
  status: string
}

export type ClientProjectGroupPage = {
  groups: ClientProjectGroup[]
  nextCursor: string | null
}

export type ClientProjectMember = {
  avatar: string
  displayName: string
  email: string
  id: string
  name: string
  nickname: string
  role: "member" | "owner"
  sourceGroupIds: string[]
  status: string
}

export type ClientProjectMemberPage = {
  members: ClientProjectMember[]
  nextCursor: string | null
}

export async function listClientProjects(
  options: { cursor?: string; limit?: number } = {},
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectPage> {
  const query = new URLSearchParams()

  if (options.limit !== undefined) {
    query.set("limit", String(options.limit))
  }
  if (options.cursor) {
    query.set("cursor", options.cursor)
  }

  const suffix = query.size > 0 ? `?${query.toString()}` : ""
  const response = await fetcher(`/api/client/projects${suffix}`, {
    credentials: "include",
    method: "GET",
  })
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<ProjectListResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "加载项目列表失败")
  }

  const data = (
    payload as ProjectDataSuccessEnvelope<ProjectListResponse> | undefined
  )?.data

  if (
    !data?.personal_project ||
    !Array.isArray(data.projects) ||
    (data.next_cursor !== null &&
      data.next_cursor !== undefined &&
      typeof data.next_cursor !== "string")
  ) {
    throw new ClientDataRequestError("项目列表响应格式不正确")
  }

  return {
    nextCursor: data.next_cursor ?? null,
    personalProject: normalizeProjectSummary(data.personal_project),
    projects: data.projects.map(normalizeProjectSummary),
  }
}

export async function getClientProject(
  projectId: string,
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectDetail> {
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<ProjectResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "加载项目详情失败")
  }

  return normalizeProjectDetail(
    (payload as ProjectDataSuccessEnvelope<ProjectResponse> | undefined)?.data
  )
}

export async function createClientProject(
  input: CreateClientProjectInput,
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectDetail> {
  const response = await fetcher("/api/client/projects", {
    body: JSON.stringify({
      group_ids: input.groupIds ?? [],
      name: input.name,
    }),
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  })
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<ProjectResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "创建项目失败")
  }

  const project = (
    payload as ProjectDataSuccessEnvelope<ProjectResponse> | undefined
  )?.data

  return normalizeProjectDetail(project)
}

export async function updateClientProject(
  projectId: string,
  input: UpdateClientProjectInput,
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectDetail> {
  return mutateClientProject(projectId, "PATCH", input, "更新项目失败", fetcher)
}

export async function deleteClientProject(
  projectId: string,
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectDetail> {
  return mutateClientProject(
    projectId,
    "DELETE",
    undefined,
    "删除项目失败",
    fetcher
  )
}

export async function uploadClientProjectAvatar(
  projectId: string,
  file: File,
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectDetail> {
  const formData = new FormData()
  formData.set("file", file)
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/avatar`,
    {
      body: formData,
      credentials: "include",
      method: "POST",
    }
  )
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<ProjectResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "上传项目头像失败")
  }

  return normalizeProjectDetail(
    (payload as ProjectDataSuccessEnvelope<ProjectResponse> | undefined)?.data
  )
}

export async function listClientProjectGroups(
  projectId: string,
  options: { cursor?: string; limit?: number } = {},
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectGroupPage> {
  const query = createPageQuery(options)
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/groups${query}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    | ProjectDataErrorEnvelope
    | ProjectDataSuccessEnvelope<ProjectGroupListResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "加载项目群组失败")
  }

  const data = (
    payload as ProjectDataSuccessEnvelope<ProjectGroupListResponse> | undefined
  )?.data
  if (!Array.isArray(data?.groups)) {
    throw new ClientDataRequestError("项目群组响应格式不正确")
  }

  return {
    groups: data.groups.map(normalizeProjectGroup),
    nextCursor: normalizeNextCursor(data.next_cursor),
  }
}

export async function bindClientProjectGroup(
  projectId: string,
  groupId: string,
  fetcher: ProjectDataFetch = fetch
) {
  return mutateClientProjectGroup(
    projectId,
    groupId,
    "PUT",
    "关联群组失败",
    fetcher
  )
}

export async function unbindClientProjectGroup(
  projectId: string,
  groupId: string,
  fetcher: ProjectDataFetch = fetch
) {
  return mutateClientProjectGroup(
    projectId,
    groupId,
    "DELETE",
    "解除群组关联失败",
    fetcher
  )
}

export async function listClientProjectMembers(
  projectId: string,
  options: { cursor?: string; limit?: number } = {},
  fetcher: ProjectDataFetch = fetch
): Promise<ClientProjectMemberPage> {
  const query = createPageQuery(options)
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/members${query}`,
    { credentials: "include", method: "GET" }
  )
  const payload = await readJson<
    | ProjectDataErrorEnvelope
    | ProjectDataSuccessEnvelope<ProjectMemberListResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, "加载项目成员失败")
  }

  const data = (
    payload as ProjectDataSuccessEnvelope<ProjectMemberListResponse> | undefined
  )?.data
  if (!Array.isArray(data?.members)) {
    throw new ClientDataRequestError("项目成员响应格式不正确")
  }

  return {
    members: data.members.map(normalizeProjectMember),
    nextCursor: normalizeNextCursor(data.next_cursor),
  }
}

export async function bindGroupConversationProject(
  conversationId: string,
  projectId: string,
  fetcher: ProjectDataFetch = fetch
) {
  return mutateGroupConversationProject(
    conversationId,
    projectId,
    "PUT",
    "关联项目失败",
    fetcher
  )
}

export async function unbindGroupConversationProject(
  conversationId: string,
  projectId: string,
  fetcher: ProjectDataFetch = fetch
) {
  return mutateGroupConversationProject(
    conversationId,
    projectId,
    "DELETE",
    "解除项目关联失败",
    fetcher
  )
}

async function mutateGroupConversationProject(
  conversationId: string,
  projectId: string,
  method: "DELETE" | "PUT",
  fallbackMessage: string,
  fetcher: ProjectDataFetch
) {
  const response = await fetcher(
    `/api/client/conversations/${encodeURIComponent(conversationId)}/projects/${encodeURIComponent(projectId)}`,
    {
      credentials: "include",
      method,
    }
  )
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, fallbackMessage)
  }
}

async function mutateClientProject(
  projectId: string,
  method: "DELETE" | "PATCH",
  input: UpdateClientProjectInput | undefined,
  fallbackMessage: string,
  fetcher: ProjectDataFetch
) {
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}`,
    {
      body: input === undefined ? undefined : JSON.stringify(input),
      credentials: "include",
      headers:
        input === undefined
          ? undefined
          : { "Content-Type": "application/json" },
      method,
    }
  )
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<ProjectResponse>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, fallbackMessage)
  }

  return normalizeProjectDetail(
    (payload as ProjectDataSuccessEnvelope<ProjectResponse> | undefined)?.data
  )
}

async function mutateClientProjectGroup(
  projectId: string,
  groupId: string,
  method: "DELETE" | "PUT",
  fallbackMessage: string,
  fetcher: ProjectDataFetch
) {
  const response = await fetcher(
    `/api/client/projects/${encodeURIComponent(projectId)}/groups/${encodeURIComponent(groupId)}`,
    { credentials: "include", method }
  )
  const payload = await readJson<
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<Record<string, never>>
  >(response)

  if (!response.ok || payload?.success === false) {
    throw createProjectRequestError(payload, response, fallbackMessage)
  }
}

function normalizeProjectSummary(
  project: ProjectSummaryResponse | undefined
): ClientProjectSummary {
  if (
    !project ||
    typeof project.id !== "string" ||
    typeof project.name !== "string" ||
    typeof project.updated_at !== "string" ||
    typeof project.is_personal !== "boolean"
  ) {
    throw new ClientDataRequestError("项目摘要响应格式不正确")
  }

  return {
    avatar: project.avatar ?? "",
    description: project.description ?? "",
    id: project.id,
    isPersonal: project.is_personal,
    name: project.name,
    updatedAt: project.updated_at,
  }
}

function normalizeProjectDetail(
  project: ProjectResponse | undefined
): ClientProjectDetail {
  if (
    !project ||
    typeof project.id !== "string" ||
    typeof project.name !== "string" ||
    typeof project.created_at !== "string" ||
    typeof project.updated_at !== "string" ||
    typeof project.is_personal !== "boolean" ||
    !project.owner ||
    typeof project.owner.id !== "string" ||
    typeof project.owner.name !== "string" ||
    !project.task_counts ||
    (project.current_user_role !== "owner" &&
      project.current_user_role !== "member")
  ) {
    throw new ClientDataRequestError("项目响应格式不正确")
  }

  return {
    avatar: project.avatar ?? "",
    createdAt: project.created_at,
    currentUserRole: project.current_user_role,
    description: project.description ?? "",
    groupCount: normalizeCount(project.group_count),
    id: project.id,
    isPersonal: project.is_personal,
    memberCount: normalizeCount(project.member_count),
    name: project.name,
    owner: {
      avatar: project.owner.avatar ?? "",
      id: project.owner.id,
      name: project.owner.name,
      nickname: project.owner.nickname ?? "",
    },
    taskCounts: {
      canceled: normalizeCount(project.task_counts.canceled),
      done: normalizeCount(project.task_counts.done),
      inProgress: normalizeCount(project.task_counts.in_progress),
      todo: normalizeCount(project.task_counts.todo),
      total: normalizeCount(project.task_counts.total),
    },
    updatedAt: project.updated_at,
  }
}

function normalizeCount(value: number | undefined) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0
}

function normalizeProjectGroup(
  group: ProjectGroupResponse
): ClientProjectGroup {
  if (
    typeof group.id !== "string" ||
    typeof group.name !== "string" ||
    typeof group.created_at !== "string"
  ) {
    throw new ClientDataRequestError("项目群组响应格式不正确")
  }

  return {
    avatar: group.avatar ?? "",
    createdAt: group.created_at,
    id: group.id,
    memberCount: normalizeCount(group.member_count),
    name: group.name,
    status: group.status ?? "",
  }
}

function normalizeProjectMember(
  member: ProjectMemberResponse
): ClientProjectMember {
  if (
    typeof member.id !== "string" ||
    typeof member.name !== "string" ||
    typeof member.display_name !== "string" ||
    typeof member.email !== "string" ||
    (member.role !== "owner" && member.role !== "member") ||
    !Array.isArray(member.source_group_ids)
  ) {
    throw new ClientDataRequestError("项目成员响应格式不正确")
  }

  return {
    avatar: member.avatar ?? "",
    displayName: member.display_name,
    email: member.email,
    id: member.id,
    name: member.name,
    nickname: member.nickname ?? "",
    role: member.role,
    sourceGroupIds: member.source_group_ids,
    status: member.status ?? "",
  }
}

function createPageQuery(options: { cursor?: string; limit?: number }) {
  const query = new URLSearchParams()
  if (options.limit !== undefined) {
    query.set("limit", String(options.limit))
  }
  if (options.cursor) {
    query.set("cursor", options.cursor)
  }
  return query.size > 0 ? `?${query.toString()}` : ""
}

function normalizeNextCursor(value: string | null | undefined) {
  if (value !== null && value !== undefined && typeof value !== "string") {
    throw new ClientDataRequestError("分页游标响应格式不正确")
  }
  return value ?? null
}

function createProjectRequestError(
  payload:
    ProjectDataErrorEnvelope | ProjectDataSuccessEnvelope<unknown> | undefined,
  response: Response,
  fallbackMessage: string
) {
  const error = (payload as ProjectDataErrorEnvelope | undefined)?.error

  return new ClientDataRequestError(
    error?.message ?? `${fallbackMessage}（HTTP ${response.status}）`,
    {
      code: error?.code,
      status: response.status,
    }
  )
}

async function readJson<T>(response: Response): Promise<T | undefined> {
  const contentType = response.headers.get("content-type")

  if (!contentType?.includes("application/json")) {
    return undefined
  }

  return response.json() as Promise<T>
}
