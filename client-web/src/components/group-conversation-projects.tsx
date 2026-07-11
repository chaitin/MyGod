import { useMemo, useState } from "react"
import { Link } from "react-router"
import { BriefcaseBusiness, Loader2, Plus, Search, Unlink } from "lucide-react"
import { toast } from "sonner"

import {
  bindGroupConversationProject,
  type ClientProjectSummary,
  unbindGroupConversationProject,
} from "@/lib/project-data-api"
import type { ClientConversationProject } from "@/lib/client-data-api"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

type GroupConversationProjectsProps = {
  availableProjects: ClientProjectSummary[]
  canManage: boolean
  conversationId: string
  linkedProjects: ClientConversationProject[]
  onConversationsChanged: () => Promise<void>
  onProjectsChanged: () => Promise<void>
}

export function GroupConversationProjects({
  availableProjects,
  canManage,
  conversationId,
  linkedProjects,
  onConversationsChanged,
  onProjectsChanged,
}: GroupConversationProjectsProps) {
  const [bindDialogOpen, setBindDialogOpen] = useState(false)
  const [binding, setBinding] = useState(false)
  const [keyword, setKeyword] = useState("")
  const [selectedProjectId, setSelectedProjectId] = useState("")
  const [unbindTarget, setUnbindTarget] =
    useState<ClientConversationProject | null>(null)
  const [unbinding, setUnbinding] = useState(false)

  const linkedProjectIds = useMemo(
    () => new Set(linkedProjects.map((project) => project.id)),
    [linkedProjects]
  )
  const candidates = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLocaleLowerCase()

    return availableProjects.filter(
      (project) =>
        !project.isPersonal &&
        !linkedProjectIds.has(project.id) &&
        (!normalizedKeyword ||
          project.name.toLocaleLowerCase().includes(normalizedKeyword))
    )
  }, [availableProjects, keyword, linkedProjectIds])

  function handleBindDialogOpenChange(open: boolean) {
    if (binding) {
      return
    }
    setBindDialogOpen(open)
    if (open) {
      setKeyword("")
      setSelectedProjectId("")
    }
  }

  async function handleBind() {
    if (!canManage || !selectedProjectId || binding) {
      return
    }

    setBinding(true)
    try {
      await bindGroupConversationProject(conversationId, selectedProjectId)
      setBindDialogOpen(false)
      toast.success("项目已关联")
      await Promise.allSettled([onConversationsChanged(), onProjectsChanged()])
    } catch (error) {
      toast.error(getErrorMessage(error, "关联项目失败"))
    } finally {
      setBinding(false)
    }
  }

  async function handleUnbind() {
    if (!canManage || !unbindTarget || unbinding) {
      return
    }

    setUnbinding(true)
    try {
      await unbindGroupConversationProject(conversationId, unbindTarget.id)
      setUnbindTarget(null)
      toast.success("已解除项目关联")
      await Promise.allSettled([onConversationsChanged(), onProjectsChanged()])
    } catch (error) {
      toast.error(getErrorMessage(error, "解除项目关联失败"))
    } finally {
      setUnbinding(false)
    }
  }

  return (
    <>
      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-2">
          <Label>项目（{linkedProjects.length}）</Label>
          {canManage && (
            <Button
              aria-label="关联项目"
              onClick={() => handleBindDialogOpenChange(true)}
              size="icon-sm"
              title="关联项目"
              type="button"
              variant="ghost"
            >
              <Plus />
            </Button>
          )}
        </div>
        <div className="grid gap-1">
          {linkedProjects.length === 0 ? (
            <div className="rounded-md border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
              暂无项目
            </div>
          ) : (
            linkedProjects.map((project) => (
              <ProjectItem
                canUnbind={canManage}
                key={project.id}
                onUnbind={() => setUnbindTarget(project)}
                project={project}
              />
            ))
          )}
        </div>
      </div>

      <Dialog open={bindDialogOpen} onOpenChange={handleBindDialogOpenChange}>
        <DialogContent className="gap-4 sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>关联项目</DialogTitle>
            <DialogDescription>
              选择一个当前可访问的协作项目。
            </DialogDescription>
          </DialogHeader>
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              autoFocus
              className="pl-8"
              disabled={binding}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder="搜索项目"
              type="search"
              value={keyword}
            />
          </div>
          <ScrollArea className="h-64 rounded-md border">
            <div className="p-2">
              {candidates.length === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  {keyword.trim() ? "没有匹配的项目" : "暂无可关联项目"}
                </div>
              ) : (
                <RadioGroup
                  className="gap-1"
                  disabled={binding}
                  onValueChange={setSelectedProjectId}
                  value={selectedProjectId}
                >
                  {candidates.map((project) => (
                    <ProjectSelectionItem
                      key={project.id}
                      project={project}
                      selected={selectedProjectId === project.id}
                    />
                  ))}
                </RadioGroup>
              )}
            </div>
          </ScrollArea>
          <DialogFooter>
            <DialogClose asChild>
              <Button disabled={binding} type="button" variant="outline">
                取消
              </Button>
            </DialogClose>
            <Button
              disabled={!selectedProjectId || binding}
              onClick={() => void handleBind()}
              type="button"
            >
              {binding && <Loader2 className="animate-spin" />}
              确定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={unbindTarget !== null}
        onOpenChange={(open) => {
          if (!open && !unbinding) {
            setUnbindTarget(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>解除项目关联</AlertDialogTitle>
            <AlertDialogDescription>
              {`确定解除群聊与“${unbindTarget?.name ?? ""}”的关联吗？解除后，该群成员可能失去项目访问权限。`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={unbinding}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={!canManage || unbinding}
              onClick={(event) => {
                event.preventDefault()
                void handleUnbind()
              }}
              variant="destructive"
            >
              {unbinding && <Loader2 className="animate-spin" />}
              解除关联
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function ProjectSelectionItem({
  project,
  selected,
}: {
  project: ClientProjectSummary
  selected: boolean
}) {
  const radioId = `bind-project-${project.id}`

  return (
    <Item
      asChild
      className={cn(
        "cursor-pointer px-2 py-1.5 hover:bg-muted",
        selected && "bg-foreground/10 hover:bg-foreground/10"
      )}
      size="sm"
    >
      <Label htmlFor={radioId}>
        <ItemMedia>
          <ProjectAvatar project={project} />
        </ItemMedia>
        <ItemContent className="min-w-0">
          <ItemTitle className="truncate">{project.name}</ItemTitle>
        </ItemContent>
        <ItemActions>
          <RadioGroupItem
            aria-label={project.name}
            id={radioId}
            value={project.id}
          />
        </ItemActions>
      </Label>
    </Item>
  )
}

function ProjectItem({
  canUnbind,
  onUnbind,
  project,
}: {
  canUnbind: boolean
  onUnbind: () => void
  project: ClientConversationProject
}) {
  return (
    <div className="group/project flex min-w-0 items-center gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted">
      <ProjectAvatar project={project} />
      <div className="min-w-0 flex-1">
        <Link
          className="block w-fit max-w-full truncate transition-colors group-hover/project:text-sky-600 focus-visible:text-sky-600 focus-visible:outline-none"
          to={`/projects/${encodeURIComponent(project.id)}`}
        >
          {project.name}
        </Link>
        <div className="truncate text-xs text-muted-foreground">
          {project.description.trim() || "暂无说明"}
        </div>
      </div>
      {canUnbind && (
        <Button
          aria-label={`解除 ${project.name} 的关联`}
          className="transition-opacity sm:opacity-0 sm:group-hover/project:opacity-100 sm:focus-visible:opacity-100"
          onClick={onUnbind}
          size="icon-sm"
          title="解除关联"
          type="button"
          variant="ghost"
        >
          <Unlink />
        </Button>
      )}
    </div>
  )
}

function ProjectAvatar({
  project,
}: {
  project: Pick<ClientProjectSummary, "avatar" | "name">
}) {
  if (!project.avatar) {
    return (
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-amber-600 text-background dark:bg-amber-600">
        <BriefcaseBusiness aria-hidden="true" className="size-4" />
      </span>
    )
  }

  return (
    <Avatar className="size-8 shrink-0 rounded-md after:rounded-md">
      <AvatarImage
        alt={project.name}
        className="rounded-md"
        src={project.avatar}
      />
      <AvatarFallback className="rounded-md bg-amber-600 text-background dark:bg-amber-600">
        <BriefcaseBusiness aria-hidden="true" className="size-4" />
      </AvatarFallback>
    </Avatar>
  )
}

function getErrorMessage(error: unknown, fallbackMessage: string) {
  return error instanceof Error ? error.message : fallbackMessage
}
