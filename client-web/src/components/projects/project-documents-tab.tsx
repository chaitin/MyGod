import * as React from "react"
import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import {
  BrainCircuit,
  File,
  FileCode2,
  FileSpreadsheet,
  FileText,
  Folder,
  FolderOpen,
  Plus,
  Search,
  type LucideIcon,
} from "lucide-react"
import { createPortal } from "react-dom"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

type ProjectDocumentType =
  "document" | "file" | "markdown" | "mindmap" | "spreadsheet"

type ProjectDocumentCreator = {
  id: string
  name: string
}

type ProjectDocumentNodeBase = {
  creator: ProjectDocumentCreator
  id: string
  name: string
  updatedAt: string
  updatedBy: ProjectDocumentCreator
}

type ProjectDocumentFile = ProjectDocumentNodeBase & {
  kind: "document"
  type: ProjectDocumentType
}

type ProjectDocumentFolder = ProjectDocumentNodeBase & {
  children: ProjectDocumentNode[]
  kind: "folder"
}

type ProjectDocumentNode = ProjectDocumentFile | ProjectDocumentFolder

type DocumentDropTarget =
  | { folderId: string; kind: "folder" }
  | { index: number; kind: "position"; parentId: string | null }

const documentTypeMetadata = {
  document: {
    icon: FileText,
    iconClassName: "text-sky-600 dark:text-sky-300",
    label: "文档",
  },
  markdown: {
    icon: FileCode2,
    iconClassName: "text-violet-600 dark:text-violet-300",
    label: "Markdown",
  },
  file: {
    icon: File,
    iconClassName: "text-zinc-600 dark:text-zinc-300",
    label: "文件",
  },
  mindmap: {
    icon: BrainCircuit,
    iconClassName: "text-orange-600 dark:text-orange-300",
    label: "脑图",
  },
  spreadsheet: {
    icon: FileSpreadsheet,
    iconClassName: "text-emerald-600 dark:text-emerald-300",
    label: "表格",
  },
} satisfies Record<
  ProjectDocumentType,
  { icon: LucideIcon; iconClassName: string; label: string }
>

const initialDocumentTree: ProjectDocumentNode[] = [
  {
    children: [
      {
        creator: { id: "user-1", name: "林晓" },
        id: "document-1",
        kind: "document",
        name: "产品需求文档",
        type: "document",
        updatedAt: "2026-07-21 16:42",
        updatedBy: { id: "user-2", name: "陈默" },
      },
      {
        creator: { id: "user-1", name: "林晓" },
        id: "document-4",
        kind: "document",
        name: "视觉设计规范",
        type: "file",
        updatedAt: "2026-07-18 15:07",
        updatedBy: { id: "user-1", name: "林晓" },
      },
    ],
    creator: { id: "user-1", name: "林晓" },
    id: "folder-product",
    kind: "folder",
    name: "产品资料",
    updatedAt: "2026-07-21 16:42",
    updatedBy: { id: "user-2", name: "陈默" },
  },
  {
    children: [
      {
        creator: { id: "user-2", name: "陈默" },
        id: "document-2",
        kind: "document",
        name: "API 接入说明",
        type: "markdown",
        updatedAt: "2026-07-20 18:15",
        updatedBy: { id: "user-3", name: "顾然" },
      },
      {
        children: [
          {
            creator: { id: "user-3", name: "顾然" },
            id: "document-6",
            kind: "document",
            name: "消息分区设计",
            type: "mindmap",
            updatedAt: "2026-07-19 14:25",
            updatedBy: { id: "user-1", name: "林晓" },
          },
        ],
        creator: { id: "user-3", name: "顾然" },
        id: "folder-technical",
        kind: "folder",
        name: "技术方案",
        updatedAt: "2026-07-19 14:25",
        updatedBy: { id: "user-1", name: "林晓" },
      },
    ],
    creator: { id: "user-2", name: "陈默" },
    id: "folder-development",
    kind: "folder",
    name: "开发文档",
    updatedAt: "2026-07-20 18:15",
    updatedBy: { id: "user-3", name: "顾然" },
  },
  {
    creator: { id: "user-3", name: "顾然" },
    id: "document-3",
    kind: "document",
    name: "项目排期与里程碑",
    type: "spreadsheet",
    updatedAt: "2026-07-19 11:36",
    updatedBy: { id: "user-2", name: "陈默" },
  },
  {
    creator: { id: "user-2", name: "陈默" },
    id: "document-5",
    kind: "document",
    name: "项目会议纪要",
    type: "document",
    updatedAt: "2026-07-17 17:28",
    updatedBy: { id: "user-2", name: "陈默" },
  },
]

export function ProjectDocumentsTab() {
  const [activeId, setActiveId] = React.useState<string | null>(null)
  const [documentTree, setDocumentTree] =
    React.useState<ProjectDocumentNode[]>(initialDocumentTree)
  const [expandedFolderIds, setExpandedFolderIds] = React.useState<Set<string>>(
    () => new Set(["folder-product", "folder-development"])
  )
  const [keyword, setKeyword] = React.useState("")
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 8 },
    })
  )
  const normalizedKeyword = keyword.trim().toLocaleLowerCase()
  const searching = normalizedKeyword.length > 0
  const visibleTree = searching
    ? filterDocumentTree(documentTree, normalizedKeyword)
    : documentTree
  const activeNode = activeId ? findDocumentNode(documentTree, activeId) : null
  const blockedParentIds = activeNode
    ? collectDocumentNodeIds(activeNode)
    : new Set<string>()

  function handleDragEnd(event: DragEndEvent) {
    const target = parseDocumentDropTarget(event.over?.data.current)
    const draggedId = String(event.active.id)
    setActiveId(null)
    if (!target) return

    setDocumentTree((current) => moveDocumentNode(current, draggedId, target))
    if (target.kind === "folder") {
      setExpandedFolderIds((current) => {
        const next = new Set(current)
        next.add(target.folderId)
        return next
      })
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-muted/10">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 p-4">
        <DocumentToolbar keyword={keyword} onKeywordChange={setKeyword} />
        <DndContext
          collisionDetection={pointerWithin}
          onDragCancel={() => setActiveId(null)}
          onDragEnd={handleDragEnd}
          onDragStart={(event) => setActiveId(String(event.active.id))}
          sensors={sensors}
        >
          <div className="min-h-0 flex-1 overflow-auto rounded-md border bg-background shadow-xs">
            <div className="min-w-240">
              <div className="sticky top-0 z-20 grid h-10 grid-cols-[minmax(20rem,1fr)_20rem] items-center border-b bg-muted/70 text-sm font-medium text-muted-foreground backdrop-blur-sm">
                <div className="pl-11">名称</div>
                <div>最近修改</div>
              </div>
              {visibleTree.length === 0 ? (
                <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
                  没有匹配的文档
                </div>
              ) : (
                <div role="tree">
                  <DocumentTree
                    activeId={activeId}
                    blockedParentIds={blockedParentIds}
                    depth={0}
                    draggingDisabled={searching}
                    expandedFolderIds={expandedFolderIds}
                    items={visibleTree}
                    onFolderOpenChange={(folderId, open) =>
                      setExpandedFolderIds((current) => {
                        const next = new Set(current)
                        if (open) next.add(folderId)
                        else next.delete(folderId)
                        return next
                      })
                    }
                    parentId={null}
                    searching={searching}
                  />
                </div>
              )}
            </div>
          </div>
          {typeof document !== "undefined" &&
            createPortal(
              <DragOverlay dropAnimation={null}>
                {activeNode && <DocumentDragOverlay node={activeNode} />}
              </DragOverlay>,
              document.body
            )}
        </DndContext>
      </div>
    </div>
  )
}

function DocumentToolbar({
  keyword,
  onKeywordChange,
}: {
  keyword: string
  onKeywordChange: (keyword: string) => void
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-3">
      <div className="relative min-w-52 sm:min-w-64">
        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label="搜索文档"
          className="pl-8"
          onChange={(event) => onKeywordChange(event.target.value)}
          placeholder="搜索文档"
          type="search"
          value={keyword}
        />
      </div>
      <Button type="button">
        <Plus />
        创建文档
      </Button>
    </div>
  )
}

function DocumentTree({
  activeId,
  blockedParentIds,
  depth,
  draggingDisabled,
  expandedFolderIds,
  items,
  onFolderOpenChange,
  parentId,
  searching,
}: {
  activeId: string | null
  blockedParentIds: Set<string>
  depth: number
  draggingDisabled: boolean
  expandedFolderIds: Set<string>
  items: ProjectDocumentNode[]
  onFolderOpenChange: (folderId: string, open: boolean) => void
  parentId: string | null
  searching: boolean
}) {
  return (
    <>
      {items.map((node, index) => (
        <React.Fragment key={node.id}>
          <DocumentDropPosition
            depth={depth}
            disabled={
              activeId === null ||
              draggingDisabled ||
              (parentId !== null && blockedParentIds.has(parentId))
            }
            index={index}
            parentId={parentId}
          />
          <DocumentTreeItem
            activeId={activeId}
            blockedParentIds={blockedParentIds}
            depth={depth}
            draggingDisabled={draggingDisabled}
            expandedFolderIds={expandedFolderIds}
            node={node}
            onFolderOpenChange={onFolderOpenChange}
            rowDropTarget={
              node.kind === "folder"
                ? { folderId: node.id, kind: "folder" }
                : { index: index + 1, kind: "position", parentId }
            }
            searching={searching}
          />
        </React.Fragment>
      ))}
      <DocumentDropPosition
        depth={depth}
        disabled={
          activeId === null ||
          draggingDisabled ||
          (parentId !== null && blockedParentIds.has(parentId))
        }
        index={items.length}
        parentId={parentId}
      />
    </>
  )
}

function DocumentTreeItem({
  activeId,
  blockedParentIds,
  depth,
  draggingDisabled,
  expandedFolderIds,
  node,
  onFolderOpenChange,
  rowDropTarget,
  searching,
}: {
  activeId: string | null
  blockedParentIds: Set<string>
  depth: number
  draggingDisabled: boolean
  expandedFolderIds: Set<string>
  node: ProjectDocumentNode
  onFolderOpenChange: (folderId: string, open: boolean) => void
  rowDropTarget: DocumentDropTarget
  searching: boolean
}) {
  const open =
    node.kind === "folder" && (searching || expandedFolderIds.has(node.id))

  if (node.kind === "document") {
    return (
      <DocumentTreeRow
        activeId={activeId}
        depth={depth}
        draggingDisabled={draggingDisabled}
        folderDropDisabled={false}
        node={node}
        open={false}
        rowDropTarget={rowDropTarget}
      />
    )
  }

  return (
    <Collapsible
      onOpenChange={(nextOpen) => {
        if (!searching) onFolderOpenChange(node.id, nextOpen)
      }}
      open={open}
    >
      <DocumentTreeRow
        activeId={activeId}
        depth={depth}
        draggingDisabled={draggingDisabled}
        folderDropDisabled={blockedParentIds.has(node.id)}
        node={node}
        open={open}
        rowDropTarget={rowDropTarget}
      />
      <CollapsibleContent role="group">
        <DocumentTree
          activeId={activeId}
          blockedParentIds={blockedParentIds}
          depth={depth + 1}
          draggingDisabled={draggingDisabled}
          expandedFolderIds={expandedFolderIds}
          items={node.children}
          onFolderOpenChange={onFolderOpenChange}
          parentId={node.id}
          searching={searching}
        />
      </CollapsibleContent>
    </Collapsible>
  )
}

function DocumentTreeRow({
  activeId,
  depth,
  draggingDisabled,
  folderDropDisabled,
  node,
  open,
  rowDropTarget,
}: {
  activeId: string | null
  depth: number
  draggingDisabled: boolean
  folderDropDisabled: boolean
  node: ProjectDocumentNode
  open: boolean
  rowDropTarget: DocumentDropTarget
}) {
  const [testDocumentAlertOpen, setTestDocumentAlertOpen] =
    React.useState(false)
  const {
    attributes,
    isDragging,
    listeners,
    setNodeRef: setDraggableNodeRef,
  } = useDraggable({ disabled: draggingDisabled, id: node.id })
  const { isOver: isRowDropTarget, setNodeRef: setDroppableNodeRef } =
    useDroppable({
      data: rowDropTarget,
      disabled:
        activeId === null ||
        activeId === node.id ||
        (node.kind === "folder" && folderDropDisabled) ||
        draggingDisabled,
      id: `${node.kind === "folder" ? "folder" : "document"}:${node.id}`,
    })
  const setRowRef = React.useCallback(
    (element: HTMLDivElement | null) => {
      setDraggableNodeRef(element)
      setDroppableNodeRef(element)
    },
    [setDraggableNodeRef, setDroppableNodeRef]
  )
  const metadata =
    node.kind === "document" ? documentTypeMetadata[node.type] : null
  const NodeIcon =
    node.kind === "folder" ? (open ? FolderOpen : Folder) : metadata!.icon
  const nameContent = (
    <>
      <span
        className={cn(
          "flex size-6 shrink-0 items-center justify-center",
          node.kind === "folder"
            ? "text-amber-600 dark:text-amber-300"
            : metadata!.iconClassName
        )}
      >
        <NodeIcon className="size-5" />
      </span>
      <span className="min-w-0 truncate font-medium transition-colors group-hover:text-sky-600">
        {node.name}
      </span>
    </>
  )

  return (
    <>
      <div
        ref={setRowRef}
        {...attributes}
        {...listeners}
        aria-expanded={node.kind === "folder" ? open : undefined}
        aria-level={depth + 1}
        className={cn(
          "group grid min-h-14 touch-pan-y grid-cols-[minmax(20rem,1fr)_20rem] items-center border-y border-transparent text-sm transition-colors select-none hover:bg-muted/50",
          draggingDisabled
            ? "cursor-default"
            : "cursor-grab active:cursor-grabbing",
          isDragging && "opacity-30",
          isRowDropTarget && "border-teal-500 bg-teal-50 dark:bg-teal-950/40"
        )}
        role="treeitem"
      >
        <div className="min-w-0 pr-4" style={{ paddingLeft: depth * 24 + 12 }}>
          {node.kind === "folder" ? (
            <CollapsibleTrigger asChild>
              <button
                aria-label={open ? `收起${node.name}` : `展开${node.name}`}
                className="flex max-w-full min-w-0 items-center gap-2 rounded-sm text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                type="button"
              >
                {nameContent}
              </button>
            </CollapsibleTrigger>
          ) : (
            <button
              className="flex max-w-full min-w-0 items-center gap-2 rounded-sm text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              onClick={() => {
                if (!isDragging) setTestDocumentAlertOpen(true)
              }}
              type="button"
            >
              {nameContent}
            </button>
          )}
        </div>
        <div className="flex min-w-0 items-center gap-2 pr-3 text-muted-foreground">
          <Avatar className="size-6 rounded-sm bg-muted after:rounded-sm">
            <AvatarFallback className="rounded-sm text-[9px]">
              {getCreatorInitial(node.updatedBy.name)}
            </AvatarFallback>
          </Avatar>
          <div className="truncate">
            {node.updatedBy.name} 修改于{" "}
            <time dateTime={node.updatedAt}>{node.updatedAt}</time>
          </div>
        </div>
      </div>
      <AlertDialog
        onOpenChange={setTestDocumentAlertOpen}
        open={testDocumentAlertOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>暂不支持打开</AlertDialogTitle>
            <AlertDialogDescription>
              目前是测试文档，暂不支持打开。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogAction>知道了</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function DocumentDropPosition({
  depth,
  disabled,
  index,
  parentId,
}: {
  depth: number
  disabled: boolean
  index: number
  parentId: string | null
}) {
  const { isOver, setNodeRef } = useDroppable({
    data: { index, kind: "position", parentId } satisfies DocumentDropTarget,
    disabled,
    id: `position:${parentId ?? "root"}:${index}`,
  })

  return (
    <div className="relative z-10 h-0">
      <div
        ref={setNodeRef}
        className={cn(
          "absolute top-0 right-3 h-3 -translate-y-1/2",
          disabled && "pointer-events-none"
        )}
        style={{ left: depth * 24 + 12 }}
      >
        <div
          className={cn(
            "pointer-events-none absolute top-1/2 right-0 left-0 h-0.5 -translate-y-1/2 rounded-full bg-transparent",
            isOver && "bg-teal-500"
          )}
        />
      </div>
    </div>
  )
}

function DocumentDragOverlay({ node }: { node: ProjectDocumentNode }) {
  const metadata =
    node.kind === "document" ? documentTypeMetadata[node.type] : null
  const NodeIcon = node.kind === "folder" ? Folder : metadata!.icon

  return (
    <div className="flex w-80 items-center gap-3 rounded-md border bg-background px-3 py-2 shadow-lg">
      <div
        className={cn(
          "flex size-6 shrink-0 items-center justify-center",
          node.kind === "folder"
            ? "text-amber-600 dark:text-amber-300"
            : metadata!.iconClassName
        )}
      >
        <NodeIcon className="size-5" />
      </div>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{node.name}</div>
      </div>
    </div>
  )
}

function parseDocumentDropTarget(value: unknown): DocumentDropTarget | null {
  if (!value || typeof value !== "object" || !("kind" in value)) return null
  if (value.kind === "folder" && "folderId" in value) {
    return { folderId: String(value.folderId), kind: "folder" }
  }
  if (
    value.kind === "position" &&
    "index" in value &&
    typeof value.index === "number" &&
    "parentId" in value
  ) {
    return {
      index: value.index,
      kind: "position",
      parentId: value.parentId === null ? null : String(value.parentId),
    }
  }
  return null
}

function moveDocumentNode(
  tree: ProjectDocumentNode[],
  nodeId: string,
  target: DocumentDropTarget
) {
  const location = findDocumentNodeLocation(tree, nodeId)
  if (!location) return tree

  const targetParentId =
    target.kind === "folder" ? target.folderId : target.parentId
  if (
    targetParentId === nodeId ||
    (targetParentId !== null &&
      collectDocumentNodeIds(location.node).has(targetParentId))
  ) {
    return tree
  }

  let targetIndex =
    target.kind === "folder"
      ? getFolderChildren(tree, target.folderId)?.length
      : target.index
  if (targetIndex === undefined) return tree
  if (location.parentId === targetParentId && location.index < targetIndex) {
    targetIndex -= 1
  }
  if (location.parentId === targetParentId && location.index === targetIndex) {
    return tree
  }

  const removal = removeDocumentNode(tree, nodeId)
  if (!removal.node) return tree
  const insertion = insertDocumentNode(
    removal.tree,
    targetParentId,
    targetIndex,
    removal.node
  )
  return insertion.inserted ? insertion.tree : tree
}

function findDocumentNode(
  tree: ProjectDocumentNode[],
  nodeId: string
): ProjectDocumentNode | null {
  for (const node of tree) {
    if (node.id === nodeId) return node
    if (node.kind === "folder") {
      const match = findDocumentNode(node.children, nodeId)
      if (match) return match
    }
  }
  return null
}

function findDocumentNodeLocation(
  tree: ProjectDocumentNode[],
  nodeId: string,
  parentId: string | null = null
): {
  index: number
  node: ProjectDocumentNode
  parentId: string | null
} | null {
  for (const [index, node] of tree.entries()) {
    if (node.id === nodeId) return { index, node, parentId }
    if (node.kind === "folder") {
      const match = findDocumentNodeLocation(node.children, nodeId, node.id)
      if (match) return match
    }
  }
  return null
}

function collectDocumentNodeIds(node: ProjectDocumentNode) {
  const ids = new Set<string>([node.id])
  if (node.kind === "folder") {
    for (const child of node.children) {
      for (const id of collectDocumentNodeIds(child)) ids.add(id)
    }
  }
  return ids
}

function getFolderChildren(tree: ProjectDocumentNode[], folderId: string) {
  const folder = findDocumentNode(tree, folderId)
  return folder?.kind === "folder" ? folder.children : null
}

function removeDocumentNode(
  tree: ProjectDocumentNode[],
  nodeId: string
): { node: ProjectDocumentNode | null; tree: ProjectDocumentNode[] } {
  const index = tree.findIndex((node) => node.id === nodeId)
  if (index >= 0) {
    return {
      node: tree[index],
      tree: [...tree.slice(0, index), ...tree.slice(index + 1)],
    }
  }

  for (const [folderIndex, node] of tree.entries()) {
    if (node.kind !== "folder") continue
    const removal = removeDocumentNode(node.children, nodeId)
    if (!removal.node) continue
    const nextTree = [...tree]
    nextTree[folderIndex] = { ...node, children: removal.tree }
    return { node: removal.node, tree: nextTree }
  }
  return { node: null, tree }
}

function insertDocumentNode(
  tree: ProjectDocumentNode[],
  parentId: string | null,
  index: number,
  nodeToInsert: ProjectDocumentNode
): { inserted: boolean; tree: ProjectDocumentNode[] } {
  if (parentId === null) {
    const safeIndex = Math.max(0, Math.min(index, tree.length))
    return {
      inserted: true,
      tree: [
        ...tree.slice(0, safeIndex),
        nodeToInsert,
        ...tree.slice(safeIndex),
      ],
    }
  }

  for (const [folderIndex, node] of tree.entries()) {
    if (node.kind !== "folder") continue
    if (node.id === parentId) {
      const safeIndex = Math.max(0, Math.min(index, node.children.length))
      const nextTree = [...tree]
      nextTree[folderIndex] = {
        ...node,
        children: [
          ...node.children.slice(0, safeIndex),
          nodeToInsert,
          ...node.children.slice(safeIndex),
        ],
      }
      return { inserted: true, tree: nextTree }
    }
    const insertion = insertDocumentNode(
      node.children,
      parentId,
      index,
      nodeToInsert
    )
    if (!insertion.inserted) continue
    const nextTree = [...tree]
    nextTree[folderIndex] = { ...node, children: insertion.tree }
    return { inserted: true, tree: nextTree }
  }
  return { inserted: false, tree }
}

function filterDocumentTree(
  tree: ProjectDocumentNode[],
  keyword: string
): ProjectDocumentNode[] {
  return tree.flatMap<ProjectDocumentNode>((node) => {
    const metadataLabel =
      node.kind === "folder" ? "文件夹" : documentTypeMetadata[node.type].label
    const matches = [
      node.name,
      node.creator.name,
      node.updatedBy.name,
      metadataLabel,
    ].some((value) => value.toLocaleLowerCase().includes(keyword))
    if (node.kind === "document") return matches ? [node] : []

    const children = filterDocumentTree(node.children, keyword)
    return matches || children.length > 0 ? [{ ...node, children }] : []
  })
}

function getCreatorInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
