import { BriefcaseBusiness } from "lucide-react"

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import type { ClientUser } from "@/lib/client-data-api"
import type { ClientProjectSummary } from "@/lib/project-data-api"
import { cn } from "@/lib/utils"

export function ProjectAvatar({
  avatarOverride,
  className,
  project,
  user,
}: {
  avatarOverride?: string
  className?: string
  project: ClientProjectSummary
  user?: ClientUser
}) {
  const displayName = project.isPersonal
    ? user?.nickname || user?.name || project.name
    : project.name
  const avatar =
    avatarOverride ?? (project.isPersonal ? user?.avatar : project.avatar)
  const initial = Array.from(displayName.trim())[0]?.toUpperCase() ?? "?"

  if (!project.isPersonal && !avatar) {
    return (
      <span
        className={cn(
          "flex shrink-0 items-center justify-center rounded-md bg-amber-600 text-background dark:bg-amber-600",
          className
        )}
      >
        <BriefcaseBusiness aria-hidden="true" className="size-4" />
      </span>
    )
  }

  return (
    <Avatar
      className={cn("shrink-0 rounded-md bg-muted after:rounded-md", className)}
    >
      {avatar && (
        <AvatarImage alt={displayName} className="rounded-md" src={avatar} />
      )}
      <AvatarFallback
        className={cn(
          "rounded-md text-xs",
          !project.isPersonal &&
            "bg-amber-600 text-background dark:bg-amber-600"
        )}
      >
        {project.isPersonal ? (
          initial
        ) : (
          <BriefcaseBusiness aria-hidden="true" className="size-4" />
        )}
      </AvatarFallback>
    </Avatar>
  )
}
