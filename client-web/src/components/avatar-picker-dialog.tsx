import { useState } from "react"
import { Loader2Icon, X } from "lucide-react"

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog"

const builtinAvatars = Array.from({ length: 64 }, (_, index) => {
  const id = String(index + 1).padStart(2, "0")

  return {
    id,
    src: `/assets/avatars/builtin/${id}.webp`,
  }
})

type AvatarPickerDialogProps = {
  onOpenChange: (open: boolean) => void
  onSaveAvatar: (avatar: string) => Promise<void> | void
  open: boolean
  selectedAvatar: string
}

export function AvatarPickerDialog({
  onOpenChange,
  onSaveAvatar,
  open,
  selectedAvatar,
}: AvatarPickerDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open && (
        <AvatarPickerDialogContent
          onOpenChange={onOpenChange}
          onSaveAvatar={onSaveAvatar}
          selectedAvatar={selectedAvatar}
        />
      )}
    </Dialog>
  )
}

function AvatarPickerDialogContent({
  onOpenChange,
  onSaveAvatar,
  selectedAvatar,
}: Omit<AvatarPickerDialogProps, "open">) {
  const [draftAvatar, setDraftAvatar] = useState(selectedAvatar)
  const [saving, setSaving] = useState(false)

  async function handleSave() {
    setSaving(true)

    try {
      await onSaveAvatar(draftAvatar)
      onOpenChange(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <DialogContent
      showCloseButton={false}
      className="flex w-[calc(100vw-2rem)] max-w-md flex-col gap-4 rounded-md border bg-background p-5 text-foreground shadow-lg ring-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <DialogTitle className="text-base font-medium">选择头像</DialogTitle>
          <DialogDescription className="sr-only">
            选择一个头像作为个人头像
          </DialogDescription>
        </div>
        <DialogClose asChild>
          <Button
            aria-label="关闭头像选择"
            size="icon-sm"
            type="button"
            variant="ghost"
          >
            <X className="size-4" />
          </Button>
        </DialogClose>
      </div>

      <div className="grid max-h-72 grid-cols-4 gap-2 overflow-y-auto rounded-md border bg-muted/30 p-2 sm:grid-cols-8">
        {builtinAvatars.map((item) => {
          const selected = draftAvatar === item.src

          return (
            <Button
              aria-label={`选择头像 ${item.id}`}
              aria-pressed={selected}
              className="h-auto rounded-sm bg-background p-0.5 hover:bg-background data-[pressed=true]:ring-2 data-[pressed=true]:ring-ring"
              data-pressed={selected}
              disabled={saving}
              key={item.id}
              onClick={() => setDraftAvatar(item.src)}
              type="button"
              variant="ghost"
            >
              <Avatar className="size-8 rounded-sm bg-muted after:rounded-sm">
                <AvatarImage alt="" className="rounded-sm" src={item.src} />
                <AvatarFallback className="rounded-sm text-xs">
                  {item.id}
                </AvatarFallback>
              </Avatar>
            </Button>
          )
        })}
      </div>

      <div className="flex justify-end">
        <Button
          disabled={saving}
          onClick={() => void handleSave()}
          type="button"
        >
          {saving && (
            <Loader2Icon aria-hidden="true" className="animate-spin" />
          )}
          保存
        </Button>
      </div>
    </DialogContent>
  )
}
