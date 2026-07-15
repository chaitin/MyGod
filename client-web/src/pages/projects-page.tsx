import * as React from "react"
import { Loader2, Plus, Search } from "lucide-react"
import { useNavigate, useParams } from "react-router"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { ProjectAvatar } from "@/components/projects/project-avatar"
import { ProjectDocumentsTab } from "@/components/projects/project-documents-tab"
import { ProjectMembersTab } from "@/components/projects/project-members-tab"
import { ProjectSettingsMenu } from "@/components/projects/project-settings-menu"
import { ProjectTasksTab } from "@/components/projects/project-tasks-tab"
import { ProjectTopicsTab } from "@/components/projects/project-topics-tab"
import { GroupAvatar } from "@/components/group-avatar"
import {
  Avatar,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
  AvatarImage,
} from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"
import { formatActivityTime } from "@/lib/activity-time"
import {
  getClientProject,
  type ClientProjectDetail,
  type ClientProjectMember,
  type ClientProjectSummary,
  listClientProjectMembers,
} from "@/lib/project-data-api"

const maxProjectGroupCount = 100

export function ProjectsPage() {
  const navigate = useNavigate()
  const { projectId } = useParams<{ projectId?: string }>()
  const {
    conversations,
    createProject,
    loadMoreProjects,
    me,
    personalProject,
    projects,
    projectsLoadingMore,
    projectsNextCursor,
    refreshConversations,
    refreshProjects,
  } = useClientData()
  const [createDialogOpen, setCreateDialogOpen] = React.useState(false)
  const [keyword, setKeyword] = React.useState("")
  const groupConversations = React.useMemo(
    () => conversations.filter((conversation) => conversation.type === "group"),
    [conversations]
  )
  const normalizedKeyword = keyword.trim().toLowerCase()
  const visiblePersonalWorkspace = normalizedKeyword
    ? [personalProject.name, personalProject.description].some((value) =>
        value.toLowerCase().includes(normalizedKeyword)
      )
    : true
  const visibleProjects = normalizedKeyword
    ? projects.filter((project) =>
        [project.name, project.description].some((value) =>
          value.toLowerCase().includes(normalizedKeyword)
        )
      )
    : projects

  async function handleCreateProject(name: string, groupIds: string[]) {
    const project = await createProject(name, groupIds)
    navigate(`/projects/${encodeURIComponent(project.id)}`)
  }

  async function handleLoadMore() {
    try {
      await loadMoreProjects()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载更多项目失败")
    }
  }

  async function handleProjectDeleted() {
    navigate("/projects", { replace: true })
    await Promise.allSettled([refreshProjects()])
  }

  return (
    <SidebarProvider
      className="min-h-0 min-w-0 flex-1"
      style={
        {
          "--sidebar-width": "18rem",
        } as React.CSSProperties
      }
    >
      <Sidebar className="border-r bg-background" collapsible="none">
        <SidebarHeader className="gap-0 p-0">
          <div className="flex h-14 items-center justify-between px-4">
            <h1 className="text-base font-medium">项目</h1>
            <Button
              aria-label="新建项目"
              onClick={() => setCreateDialogOpen(true)}
              size="icon-sm"
              title="新建项目"
              type="button"
              variant="ghost"
            >
              <Plus className="size-4" />
            </Button>
          </div>
          <div className="px-4 pb-3">
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 z-10 size-4 -translate-y-1/2 text-muted-foreground" />
              <SidebarInput
                aria-label="搜索项目"
                className="pl-8"
                onChange={(event) => setKeyword(event.target.value)}
                placeholder="搜索项目"
                type="search"
                value={keyword}
              />
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent className="gap-0">
          {visiblePersonalWorkspace && (
            <ProjectListSection>
              <ProjectListButton
                active={projectId === personalProject.id}
                onSelect={() =>
                  navigate(
                    `/projects/${encodeURIComponent(personalProject.id)}`
                  )
                }
                project={personalProject}
                user={me}
              />
            </ProjectListSection>
          )}
          {visibleProjects.length > 0 && (
            <ProjectListSection title="协作项目">
              {visibleProjects.map((project) => (
                <ProjectListButton
                  active={projectId === project.id}
                  key={project.id}
                  onSelect={() =>
                    navigate(`/projects/${encodeURIComponent(project.id)}`)
                  }
                  project={project}
                />
              ))}
            </ProjectListSection>
          )}
          {!visiblePersonalWorkspace && visibleProjects.length === 0 && (
            <div className="px-3 py-8 text-center text-sm text-muted-foreground">
              没有匹配的项目
            </div>
          )}
          {projectsNextCursor && !normalizedKeyword && (
            <div className="px-3 py-2">
              <Button
                className="w-full"
                disabled={projectsLoadingMore}
                onClick={() => void handleLoadMore()}
                variant="ghost"
              >
                {projectsLoadingMore ? "正在加载" : "加载更多"}
              </Button>
            </div>
          )}
        </SidebarContent>
      </Sidebar>

      {projectId ? (
        <SelectedProjectPanel
          groups={groupConversations}
          key={projectId}
          onProjectDeleted={handleProjectDeleted}
          projectId={projectId}
          refreshConversations={refreshConversations}
          refreshProjects={refreshProjects}
          user={me}
        />
      ) : (
        <ProjectEmptyState />
      )}
      <CreateProjectDialog
        groups={groupConversations}
        onCreate={handleCreateProject}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
      />
    </SidebarProvider>
  )
}

function ProjectListSection({
  children,
  title,
}: {
  children: React.ReactNode
  title?: string
}) {
  return (
    <SidebarGroup className="py-1">
      {title && <SidebarGroupLabel>{title}</SidebarGroupLabel>}
      <SidebarGroupContent>
        <SidebarMenu>{children}</SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

function ProjectListButton({
  active,
  onSelect,
  project,
  user,
}: {
  active: boolean
  onSelect: () => void
  project: ClientProjectSummary
  user?: ClientUser
}) {
  const updatedAt = formatActivityTime(project.updatedAt)

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        aria-pressed={active}
        className="h-16 gap-3 py-2 data-active:bg-teal-200 data-active:hover:bg-teal-200 dark:data-active:bg-teal-800 dark:data-active:hover:bg-teal-800"
        isActive={active}
        onClick={onSelect}
        size="lg"
        type="button"
      >
        <ProjectAvatar className="size-9" project={project} user={user} />
        <span className="min-w-0 flex-1 overflow-hidden">
          <span className="flex w-full min-w-0 items-center justify-between gap-2 overflow-hidden text-sm leading-snug font-medium">
            <span
              className={cn(
                "block min-w-0 flex-1 truncate",
                active && "text-teal-600 dark:text-teal-400"
              )}
            >
              {project.name}
            </span>
            {updatedAt && (
              <span
                className={cn(
                  "shrink-0 pr-2 text-xs font-normal text-muted-foreground",
                  active && "text-teal-600 dark:text-teal-500"
                )}
              >
                {updatedAt}
              </span>
            )}
          </span>
          <span className="mt-0.5 block truncate text-xs font-normal text-muted-foreground">
            {project.description.trim() || "暂无说明"}
          </span>
        </span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function ProjectEmptyState() {
  return (
    <SidebarInset className="min-w-0 overflow-hidden bg-muted">
      <div className="flex flex-1 items-center justify-center self-stretch text-sm text-muted-foreground">
        选择一个项目查看详情
      </div>
    </SidebarInset>
  )
}

function SelectedProjectPanel({
  groups,
  onProjectDeleted,
  projectId,
  refreshConversations,
  refreshProjects,
  user,
}: {
  groups: ClientConversation[]
  onProjectDeleted: () => Promise<void>
  projectId: string
  refreshConversations: () => Promise<void>
  refreshProjects: () => Promise<void>
  user: ClientUser
}) {
  const [error, setError] = React.useState("")
  const [loading, setLoading] = React.useState(true)
  const [members, setMembers] = React.useState<ClientProjectMember[]>([])
  const [project, setProject] = React.useState<ClientProjectDetail | null>(null)
  const requestIdRef = React.useRef(0)

  const loadProject = React.useCallback(async () => {
    const requestId = ++requestIdRef.current

    try {
      const [nextProject, nextMembers] = await loadSelectedProject(projectId)
      if (requestId === requestIdRef.current) {
        setProject(nextProject)
        setMembers(nextMembers)
        setError("")
      }
    } catch {
      // Keep the current detail visible when a background refresh fails.
    }
  }, [projectId])

  React.useEffect(() => {
    const requestId = ++requestIdRef.current
    void loadSelectedProject(projectId)
      .then(([nextProject, nextMembers]) => {
        if (requestId === requestIdRef.current) {
          setProject(nextProject)
          setMembers(nextMembers)
        }
      })
      .catch((loadError: unknown) => {
        if (requestId === requestIdRef.current) {
          setError(
            loadError instanceof Error ? loadError.message : "加载项目详情失败"
          )
        }
      })
      .finally(() => {
        if (requestId === requestIdRef.current) {
          setLoading(false)
        }
      })
    return () => {
      requestIdRef.current += 1
    }
  }, [projectId])

  async function handleProjectUpdated() {
    await Promise.allSettled([loadProject(), refreshProjects()])
  }

  async function handleRelationsChanged() {
    await Promise.allSettled([
      loadProject(),
      refreshConversations(),
      refreshProjects(),
    ])
  }

  if (loading) {
    return <ProjectPanelState loading message="正在加载项目" />
  }

  if (error || !project) {
    return <ProjectPanelState message={error || "项目不存在或无法访问"} />
  }

  return (
    <ProjectPanel
      groups={groups}
      members={members}
      onProjectDeleted={onProjectDeleted}
      onProjectUpdated={handleProjectUpdated}
      onRelationsChanged={handleRelationsChanged}
      project={project}
      user={user}
    />
  )
}

async function loadSelectedProject(projectId: string) {
  const [project, memberPage] = await Promise.all([
    getClientProject(projectId),
    listClientProjectMembers(projectId, { limit: 3 }),
  ])
  return [project, memberPage.members] as const
}

function ProjectPanelState({
  loading = false,
  message,
}: {
  loading?: boolean
  message: string
}) {
  return (
    <SidebarInset className="min-w-0 overflow-hidden bg-muted">
      <div className="flex flex-1 items-center justify-center gap-2 self-stretch text-sm text-muted-foreground">
        {loading && <Loader2 className="size-4 animate-spin" />}
        {message}
      </div>
    </SidebarInset>
  )
}

function ProjectPanel({
  groups,
  members,
  onProjectDeleted,
  onProjectUpdated,
  onRelationsChanged,
  project,
  user,
}: {
  groups: ClientConversation[]
  members: ClientProjectMember[]
  onProjectDeleted: () => Promise<void>
  onProjectUpdated: () => Promise<void>
  onRelationsChanged: () => Promise<void>
  project: ClientProjectDetail
  user: ClientUser
}) {
  const extraMemberCount = Math.max(project.memberCount - members.length, 0)

  return (
    <SidebarInset className="min-w-0 overflow-hidden">
      <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b px-4">
        <div className="flex min-w-0 items-center gap-2.5">
          <ProjectAvatar className="size-8" project={project} user={user} />
          <div className="min-w-0">
            <h1 className="truncate text-sm font-semibold">{project.name}</h1>
            <p className="truncate text-xs text-muted-foreground">
              {project.description.trim() || "暂无说明"}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <AvatarGroup className="hidden md:flex">
            {members.map((member) => {
              const initial =
                Array.from(member.displayName.trim())[0]?.toUpperCase() ?? "?"

              return (
                <Avatar className="size-6" key={member.id}>
                  {member.avatar && (
                    <AvatarImage alt={member.displayName} src={member.avatar} />
                  )}
                  <AvatarFallback>{initial}</AvatarFallback>
                </Avatar>
              )
            })}
            {extraMemberCount > 0 && (
              <AvatarGroupCount className="size-6 text-[10px]">
                +{extraMemberCount}
              </AvatarGroupCount>
            )}
          </AvatarGroup>
          <ProjectSettingsMenu
            groups={groups}
            onProjectDeleted={onProjectDeleted}
            onProjectUpdated={onProjectUpdated}
            onRelationsChanged={onRelationsChanged}
            project={project}
            user={user}
          />
        </div>
      </header>

      <Tabs
        className="min-h-0 flex-1 gap-0 overflow-hidden"
        defaultValue="tasks"
      >
        <ProjectNavigation />
        <TabsContent
          className="flex min-h-0 flex-1 overflow-hidden"
          value="tasks"
        >
          <ProjectTasksTab
            key={project.id}
            onTasksChanged={onProjectUpdated}
            projectId={project.id}
          />
        </TabsContent>
        <TabsContent
          className="flex min-h-0 flex-1 overflow-hidden"
          value="topics"
        >
          <ProjectTopicsTab />
        </TabsContent>
        <TabsContent
          className="flex min-h-0 flex-1 overflow-hidden"
          value="documents"
        >
          <ProjectDocumentsTab />
        </TabsContent>
        <TabsContent
          className="flex min-h-0 flex-1 overflow-hidden"
          value="members"
        >
          <ProjectMembersTab
            key={`${project.id}-${project.updatedAt}`}
            projectId={project.id}
          />
        </TabsContent>
      </Tabs>
    </SidebarInset>
  )
}

function CreateProjectDialog({
  groups,
  onCreate,
  onOpenChange,
  open,
}: {
  groups: ClientConversation[]
  onCreate: (name: string, groupIds: string[]) => Promise<void>
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const [creating, setCreating] = React.useState(false)
  const [groupKeyword, setGroupKeyword] = React.useState("")
  const [name, setName] = React.useState("")
  const [selectedGroupIds, setSelectedGroupIds] = React.useState<Set<string>>(
    () => new Set()
  )

  function resetForm() {
    setCreating(false)
    setGroupKeyword("")
    setName("")
    setSelectedGroupIds(new Set())
  }

  const filteredGroups = React.useMemo(() => {
    const keyword = groupKeyword.trim().toLowerCase()

    if (!keyword) {
      return groups
    }

    return groups.filter((group) => group.name.toLowerCase().includes(keyword))
  }, [groupKeyword, groups])
  const trimmedName = name.trim()
  const canCreate = trimmedName.length > 0 && !creating

  function handleOpenChange(nextOpen: boolean) {
    if (!creating) {
      if (!nextOpen) {
        resetForm()
      }
      onOpenChange(nextOpen)
    }
  }

  function toggleGroup(groupId: string, checked: boolean | string) {
    setSelectedGroupIds((currentIds) => {
      if (
        checked === true &&
        !currentIds.has(groupId) &&
        currentIds.size >= maxProjectGroupCount
      ) {
        return currentIds
      }

      const nextIds = new Set(currentIds)

      if (checked === true) {
        nextIds.add(groupId)
      } else {
        nextIds.delete(groupId)
      }

      return nextIds
    })
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!canCreate) {
      return
    }

    setCreating(true)
    try {
      await onCreate(trimmedName, Array.from(selectedGroupIds))
      resetForm()
      onOpenChange(false)
      toast.success("项目已创建")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "创建项目失败")
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="gap-5 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-base">新建项目</DialogTitle>
          <DialogDescription className="sr-only">
            输入项目名称并选择要关联的群聊
          </DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={handleSubmit}>
          <div className="grid gap-2">
            <Label htmlFor="create-project-name">项目名称</Label>
            <Input
              autoFocus
              disabled={creating}
              id="create-project-name"
              maxLength={120}
              onChange={(event) => setName(event.target.value)}
              placeholder="输入项目名称"
              value={name}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="create-project-group-search">
              关联群聊
              {selectedGroupIds.size > 0 && (
                <span className="font-normal text-muted-foreground">
                  已选择 {selectedGroupIds.size}/{maxProjectGroupCount} 个
                </span>
              )}
            </Label>
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-8"
                disabled={creating}
                id="create-project-group-search"
                onChange={(event) => setGroupKeyword(event.target.value)}
                placeholder="搜索群聊"
                type="search"
                value={groupKeyword}
              />
            </div>
          </div>
          <ScrollArea className="h-64 rounded-md border">
            <div className="grid gap-1 p-2">
              {filteredGroups.length === 0 && (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  {groupKeyword.trim() ? "没有匹配的群聊" : "暂无可关联群聊"}
                </div>
              )}
              {filteredGroups.map((group) => {
                const checkboxId = `create-project-group-${group.id}`
                const selected = selectedGroupIds.has(group.id)
                const selectionDisabled =
                  !selected && selectedGroupIds.size >= maxProjectGroupCount

                return (
                  <Label
                    className={cn(
                      "cursor-pointer rounded-md px-2 py-2 font-normal hover:bg-muted",
                      selectionDisabled &&
                        "cursor-not-allowed opacity-50 hover:bg-transparent"
                    )}
                    htmlFor={checkboxId}
                    key={group.id}
                  >
                    <Checkbox
                      checked={selected}
                      disabled={creating || selectionDisabled}
                      id={checkboxId}
                      onCheckedChange={(checked) =>
                        toggleGroup(group.id, checked)
                      }
                    />
                    <GroupAvatar
                      avatar={group.avatar}
                      members={group.members}
                      name={group.name}
                    />
                    <span className="min-w-0 flex-1 truncate">
                      {group.name}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {group.memberCount} 人
                    </span>
                  </Label>
                )
              })}
            </div>
          </ScrollArea>
          <DialogFooter>
            <Button
              disabled={creating}
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              取消
            </Button>
            <Button disabled={!canCreate} type="submit">
              {creating ? "正在创建" : "确定"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ProjectNavigation() {
  const items = [
    { value: "tasks", label: "任务" },
    { value: "topics", label: "话题" },
    { value: "documents", label: "文档" },
    { value: "members", label: "成员" },
  ]

  return (
    <div className="shrink-0 border-b px-4">
      <TabsList aria-label="项目内容" variant="line">
        {items.map((item) => (
          <TabsTrigger key={item.value} value={item.value}>
            {item.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </div>
  )
}
