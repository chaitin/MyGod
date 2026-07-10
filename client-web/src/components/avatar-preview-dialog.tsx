import type { ReactNode } from "react"

import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"

type AvatarPreviewDialogProps = {
  children: ReactNode
  label: string
  onOpenChange: (open: boolean) => void
  open: boolean
}

export function AvatarPreviewDialog({
  children,
  label,
  onOpenChange,
  open,
}: AvatarPreviewDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="w-auto max-w-none gap-0 p-4 sm:max-w-none">
        <DialogTitle className="sr-only">{label}</DialogTitle>
        <div
          className="size-64 overflow-hidden rounded-sm"
          data-slot="avatar-preview"
          data-testid="avatar-preview"
        >
          {children}
        </div>
      </DialogContent>
    </Dialog>
  )
}
