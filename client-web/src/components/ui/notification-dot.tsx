import * as React from "react"

import { cn } from "@/lib/utils"

function NotificationDot({
  className,
  "aria-hidden": ariaHidden = true,
  ...props
}: React.ComponentProps<"span">) {
  return (
    <span
      aria-hidden={ariaHidden}
      className={cn(
        "pointer-events-none inline-flex size-2.5 shrink-0 rounded-full bg-red-500 ring-2 ring-background",
        className
      )}
      data-slot="notification-dot"
      {...props}
    />
  )
}

export { NotificationDot }
