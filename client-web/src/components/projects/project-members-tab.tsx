import * as React from "react"
import { Loader2 } from "lucide-react"

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  type ClientProjectMember,
  listClientProjectMembers,
} from "@/lib/project-data-api"

export function ProjectMembersTab({ projectId }: { projectId: string }) {
  const [error, setError] = React.useState("")
  const [loading, setLoading] = React.useState(true)
  const [members, setMembers] = React.useState<ClientProjectMember[]>([])

  React.useEffect(() => {
    let active = true

    void listAllProjectMembers(projectId)
      .then((nextMembers) => {
        if (active) {
          setMembers(nextMembers)
        }
      })
      .catch((loadError) => {
        if (active) {
          setError(
            loadError instanceof Error ? loadError.message : "加载项目成员失败"
          )
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
  }, [projectId])

  if (loading) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center gap-2 bg-muted/10 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" />
        正在加载成员
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center bg-muted/10 text-sm text-destructive">
        {error}
      </div>
    )
  }

  if (members.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center bg-muted/10 text-sm text-muted-foreground">
        暂无项目成员
      </div>
    )
  }

  return (
    <ScrollArea className="min-h-0 flex-1 bg-muted/10">
      <div className="grid w-full gap-2 p-4">
        {members.map((member) => {
          const initial =
            Array.from(member.displayName.trim())[0]?.toUpperCase() ?? "?"

          return (
            <Item
              className="cursor-default px-3 py-2.5 hover:bg-muted"
              key={member.id}
              size="sm"
              variant="outline"
            >
              <ItemMedia>
                <Avatar className="size-9 rounded-sm bg-muted after:rounded-sm">
                  {member.avatar && (
                    <AvatarImage
                      alt={member.displayName}
                      className="rounded-sm"
                      src={member.avatar}
                    />
                  )}
                  <AvatarFallback className="rounded-sm">
                    {initial}
                  </AvatarFallback>
                </Avatar>
              </ItemMedia>
              <ItemContent className="min-w-0">
                <ItemTitle className="truncate">{member.displayName}</ItemTitle>
                <ItemDescription className="truncate text-xs">
                  {member.email}
                </ItemDescription>
              </ItemContent>
              <ItemActions>
                <Badge variant="secondary">
                  {member.role === "owner" ? "所有者" : "成员"}
                </Badge>
              </ItemActions>
            </Item>
          )
        })}
      </div>
    </ScrollArea>
  )
}

async function listAllProjectMembers(projectId: string) {
  const members: ClientProjectMember[] = []
  const seenCursors = new Set<string>()
  let cursor: string | undefined

  do {
    const page = await listClientProjectMembers(projectId, {
      cursor,
      limit: 100,
    })
    members.push(...page.members)
    if (!page.nextCursor || seenCursors.has(page.nextCursor)) {
      break
    }
    seenCursors.add(page.nextCursor)
    cursor = page.nextCursor
  } while (cursor)

  return members
}
