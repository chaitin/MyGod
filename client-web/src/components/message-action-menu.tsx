import * as React from "react"
import { ContextMenu as ContextMenuPrimitive } from "radix-ui"
import { Copy, Forward, Reply, Undo2 } from "lucide-react"

import { cn } from "@/lib/utils"

type MessageActionMenuProps = {
  children: React.ReactNode
  canRevoke?: boolean
  copyDisabled?: boolean
  onCopy?: () => void
  onReply?: () => void
  onRevoke?: () => void
}

const messageActions = [
  { label: "复制", icon: Copy, type: "copy" },
  { label: "回复", icon: Reply, type: "reply" },
  { label: "转发", icon: Forward, type: "forward" },
] as const

export function MessageActionMenu({
  canRevoke = false,
  children,
  copyDisabled = false,
  onCopy,
  onReply,
  onRevoke,
}: MessageActionMenuProps) {
  return (
    <ContextMenuPrimitive.Root>
      <ContextMenuPrimitive.Trigger asChild>
        {children}
      </ContextMenuPrimitive.Trigger>
      <ContextMenuPrimitive.Portal>
        <ContextMenuPrimitive.Content
          className={cn(
            "z-50 min-w-32 overflow-hidden rounded-md bg-popover p-1 text-popover-foreground shadow-lg ring-1 ring-foreground/10",
            "origin-(--radix-context-menu-content-transform-origin) duration-100",
            "data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
            "data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95",
            "data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
          )}
          data-slot="message-action-menu"
        >
          {messageActions.map((action) => (
            <MessageActionMenuItem
              disabled={action.type === "copy" ? copyDisabled : false}
              key={action.label}
              onSelect={
                action.type === "copy"
                  ? onCopy
                  : action.type === "reply"
                    ? onReply
                    : undefined
              }
            >
              <action.icon aria-hidden="true" className="size-4" />
              <span>{action.label}</span>
            </MessageActionMenuItem>
          ))}
          {canRevoke && (
            <>
              <ContextMenuPrimitive.Separator className="-mx-1 my-1 h-px bg-border" />
              <MessageActionMenuItem
                disabled={!onRevoke}
                onSelect={onRevoke}
                variant="destructive"
              >
                <Undo2 aria-hidden="true" className="size-4" />
                <span>撤回</span>
              </MessageActionMenuItem>
            </>
          )}
        </ContextMenuPrimitive.Content>
      </ContextMenuPrimitive.Portal>
    </ContextMenuPrimitive.Root>
  )
}

function MessageActionMenuItem({
  children,
  className,
  disabled = false,
  onSelect,
  variant = "default",
}: {
  children: React.ReactNode
  className?: string
  disabled?: boolean
  onSelect?: () => void
  variant?: "default" | "destructive"
}) {
  return (
    <ContextMenuPrimitive.Item
      className={cn(
        "flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none",
        "focus:bg-accent focus:text-accent-foreground focus:**:text-accent-foreground",
        "data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0",
        variant === "destructive" &&
          "text-destructive focus:bg-destructive/10 focus:text-destructive dark:focus:bg-destructive/20",
        className
      )}
      data-slot="message-action-menu-item"
      data-variant={variant}
      disabled={disabled}
      onSelect={onSelect}
    >
      {children}
    </ContextMenuPrimitive.Item>
  )
}
