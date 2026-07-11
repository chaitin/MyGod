import * as React from "react"
import { Loader2, Plus, Search, Unlink } from "lucide-react"
import { toast } from "sonner"

import { GroupAvatar } from "@/components/group-avatar"
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
import { Button } from "@/components/ui/button"
import {
  Dialog,
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
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import type { ClientConversation } from "@/lib/client-data-api"
import {
  bindClientProjectGroup,
  type ClientProjectDetail,
  type ClientProjectGroup,
  listClientProjectGroups,
  unbindClientProjectGroup,
} from "@/lib/project-data-api"
import { cn } from "@/lib/utils"

const maxProjectGroupCount = 100

export function ProjectGroupAssociationsDialog({
  groups,
  onOpenChange,
  onRelationsChanged,
  open,
  project,
}: {
  groups: ClientConversation[]
  onOpenChange: (open: boolean) => void
  onRelationsChanged: () => Promise<void>
  open: boolean
  project: ClientProjectDetail
}) {
  const [addOpen, setAddOpen] = React.useState(false)
  const [adding, setAdding] = React.useState(false)
  const [keyword, setKeyword] = React.useState("")
  const [linkedGroups, setLinkedGroups] = React.useState<ClientProjectGroup[]>(
    []
  )
  const [loading, setLoading] = React.useState(true)
  const [loadError, setLoadError] = React.useState("")
  const [selectedGroupId, setSelectedGroupId] = React.useState("")
  const [unlinking, setUnlinking] = React.useState(false)
  const [unlinkTarget, setUnlinkTarget] =
    React.useState<ClientProjectGroup | null>(null)

  React.useEffect(() => {
    if (!open) {
      return
    }

    let active = true
    void listAllProjectGroups(project.id)
      .then((nextGroups) => {
        if (active) {
          setLinkedGroups(nextGroups)
        }
      })
      .catch((error) => {
        if (active) {
          setLoadError(getErrorMessage(error, "加载授权群组失败"))
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false)
        }
      })

    return () => {
      active = false
    }
  }, [open, project.id])

  const linkedGroupIds = React.useMemo(
    () => new Set(linkedGroups.map((group) => group.id)),
    [linkedGroups]
  )
  const conversationsById = React.useMemo(
    () => new Map(groups.map((group) => [group.id, group])),
    [groups]
  )
  const candidates = React.useMemo(() => {
    const normalizedKeyword = keyword.trim().toLocaleLowerCase()
    return groups
      .filter(
        (group) =>
          !linkedGroupIds.has(group.id) &&
          (!normalizedKeyword ||
            group.name.toLocaleLowerCase().includes(normalizedKeyword))
      )
      .sort((left, right) => left.name.localeCompare(right.name, "zh-CN"))
  }, [groups, keyword, linkedGroupIds])

  function handleAddOpenChange(nextOpen: boolean) {
    if (adding) {
      return
    }
    if (nextOpen) {
      setKeyword("")
      setSelectedGroupId("")
    }
    setAddOpen(nextOpen)
  }

  function openAddDialog() {
    handleAddOpenChange(true)
  }

  async function handleAdd() {
    if (!selectedGroupId || adding) {
      return
    }

    const selectedGroup = groups.find((group) => group.id === selectedGroupId)
    if (!selectedGroup) {
      return
    }

    setAdding(true)
    try {
      await bindClientProjectGroup(project.id, selectedGroup.id)
      setLinkedGroups((current) => [toProjectGroup(selectedGroup), ...current])
      setAddOpen(false)
      toast.success("群组已授权")
      await onRelationsChanged()
    } catch (error) {
      toast.error(getErrorMessage(error, "授权群组失败"))
    } finally {
      setAdding(false)
    }
  }

  async function handleUnlink() {
    if (!unlinkTarget || unlinking) {
      return
    }

    setUnlinking(true)
    try {
      await unbindClientProjectGroup(project.id, unlinkTarget.id)
      setLinkedGroups((current) =>
        current.filter((group) => group.id !== unlinkTarget.id)
      )
      setUnlinkTarget(null)
      toast.success("已取消群组授权")
      await onRelationsChanged()
    } catch (error) {
      toast.error(getErrorMessage(error, "取消群组授权失败"))
    } finally {
      setUnlinking(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>授权群组</DialogTitle>
          <DialogDescription>
            已授权群组的成员可以访问这个项目。
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center justify-between gap-2">
          <Label>已授权群组（{linkedGroups.length}）</Label>
          <Button
            aria-label="添加授权群组"
            disabled={loading || linkedGroups.length >= maxProjectGroupCount}
            onClick={openAddDialog}
            size="icon-sm"
            title="添加授权群组"
            type="button"
            variant="ghost"
          >
            <Plus />
          </Button>
        </div>
        <ScrollArea className="h-72 rounded-md border">
          <div className="grid gap-1 p-2">
            {loading && (
              <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                正在加载
              </div>
            )}
            {!loading && loadError && (
              <div className="py-12 text-center text-sm text-destructive">
                {loadError}
              </div>
            )}
            {!loading && !loadError && linkedGroups.length === 0 && (
              <div className="py-12 text-center text-sm text-muted-foreground">
                暂无授权群组
              </div>
            )}
            {!loading &&
              !loadError &&
              linkedGroups.map((group) => {
                const conversation = conversationsById.get(group.id)

                return (
                  <Item
                    className="px-2 py-1.5 hover:bg-muted"
                    key={group.id}
                    size="sm"
                  >
                    <ItemMedia>
                      <GroupAvatar
                        avatar={group.avatar}
                        members={conversation?.members}
                        name={group.name}
                      />
                    </ItemMedia>
                    <ItemContent className="min-w-0">
                      <ItemTitle className="truncate">{group.name}</ItemTitle>
                      <ItemDescription className="truncate text-xs">
                        {group.memberCount} 人
                      </ItemDescription>
                    </ItemContent>
                    <ItemActions>
                      <Button
                        aria-label={`取消 ${group.name} 的授权`}
                        onClick={() => setUnlinkTarget(group)}
                        size="icon-sm"
                        title="取消授权"
                        type="button"
                        variant="ghost"
                      >
                        <Unlink />
                      </Button>
                    </ItemActions>
                  </Item>
                )
              })}
          </div>
        </ScrollArea>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button">
            关闭
          </Button>
        </DialogFooter>

        <Dialog open={addOpen} onOpenChange={handleAddOpenChange}>
          <DialogContent className="gap-4 sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>添加授权群组</DialogTitle>
              <DialogDescription>
                选择群组，点击确定后才会完成授权。
              </DialogDescription>
            </DialogHeader>
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                autoFocus
                className="pl-8"
                disabled={adding}
                onChange={(event) => setKeyword(event.target.value)}
                placeholder="搜索群组"
                type="search"
                value={keyword}
              />
            </div>
            <ScrollArea className="h-72 rounded-md border">
              <div className="grid gap-1 p-2">
                {candidates.length === 0 && (
                  <div className="py-12 text-center text-sm text-muted-foreground">
                    {keyword.trim() ? "没有匹配的群组" : "暂无可授权群组"}
                  </div>
                )}
                {candidates.length > 0 && (
                  <RadioGroup
                    className="gap-1"
                    disabled={adding}
                    onValueChange={setSelectedGroupId}
                    value={selectedGroupId}
                  >
                    {candidates.map((group) => {
                      const radioId = `authorize-project-group-${project.id}-${group.id}`
                      const selected = selectedGroupId === group.id

                      return (
                        <Item
                          asChild
                          className={cn(
                            "cursor-pointer px-2 py-1.5 hover:bg-muted",
                            selected &&
                              "bg-foreground/10 hover:bg-foreground/10"
                          )}
                          key={group.id}
                          size="sm"
                        >
                          <Label htmlFor={radioId}>
                            <ItemMedia>
                              <GroupAvatar
                                avatar={group.avatar}
                                members={group.members}
                                name={group.name}
                              />
                            </ItemMedia>
                            <ItemContent className="min-w-0">
                              <ItemTitle className="truncate">
                                {group.name}
                              </ItemTitle>
                              <ItemDescription className="truncate text-xs">
                                {group.memberCount} 人
                              </ItemDescription>
                            </ItemContent>
                            <ItemActions>
                              <RadioGroupItem
                                aria-label={group.name}
                                id={radioId}
                                value={group.id}
                              />
                            </ItemActions>
                          </Label>
                        </Item>
                      )
                    })}
                  </RadioGroup>
                )}
              </div>
            </ScrollArea>
            <DialogFooter>
              <Button
                disabled={adding}
                onClick={() => handleAddOpenChange(false)}
                type="button"
                variant="outline"
              >
                取消
              </Button>
              <Button
                disabled={adding || !selectedGroupId}
                onClick={() => void handleAdd()}
                type="button"
              >
                {adding && <Loader2 className="animate-spin" />}
                确定
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <AlertDialog
          open={unlinkTarget !== null}
          onOpenChange={(nextOpen) => {
            if (!nextOpen && !unlinking) {
              setUnlinkTarget(null)
            }
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>取消群组授权</AlertDialogTitle>
              <AlertDialogDescription>
                {`确定取消“${unlinkTarget?.name ?? ""}”的项目授权吗？该群组成员可能失去项目访问权限。`}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={unlinking}>取消</AlertDialogCancel>
              <AlertDialogAction
                disabled={unlinking}
                onClick={(event) => {
                  event.preventDefault()
                  void handleUnlink()
                }}
                variant="destructive"
              >
                {unlinking && <Loader2 className="animate-spin" />}
                取消授权
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DialogContent>
    </Dialog>
  )
}

function toProjectGroup(group: ClientConversation): ClientProjectGroup {
  return {
    avatar: group.avatar,
    createdAt: new Date().toISOString(),
    id: group.id,
    memberCount: group.memberCount,
    name: group.name,
    status: "active",
  }
}

async function listAllProjectGroups(projectId: string) {
  const groups: ClientProjectGroup[] = []
  const seenCursors = new Set<string>()
  let cursor: string | undefined

  do {
    const page = await listClientProjectGroups(projectId, {
      cursor,
      limit: 100,
    })
    groups.push(...page.groups)
    if (!page.nextCursor || seenCursors.has(page.nextCursor)) {
      break
    }
    seenCursors.add(page.nextCursor)
    cursor = page.nextCursor
  } while (cursor)

  return groups
}

function getErrorMessage(error: unknown, fallbackMessage: string) {
  return error instanceof Error ? error.message : fallbackMessage
}
