import { Link as LinkIcon } from "lucide-react"

import type { ClientLinkMessageBody } from "@/lib/client-data-api"

type MessageLinkProps = {
  link: ClientLinkMessageBody
}

export function MessageLink({ link }: MessageLinkProps) {
  return (
    <a
      className="flex w-72 max-w-full items-center gap-3 rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      href={link.url}
      rel="noopener noreferrer"
      target="_blank"
    >
      <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-background/50 text-muted-foreground">
        <LinkIcon className="size-5" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm leading-snug font-medium">
          {link.title}
        </div>
        <div className="truncate text-xs leading-snug text-muted-foreground">
          {link.url}
        </div>
      </div>
    </a>
  )
}
