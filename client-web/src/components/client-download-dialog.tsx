import {
  Download,
  Laptop,
  Monitor,
  Smartphone,
  TabletSmartphone,
  X,
  type LucideIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"

type ClientPlatform = {
  description: string
  downloadUrl?: string
  icon: LucideIcon
  name: string
  released: boolean
}

const clientPlatforms: ClientPlatform[] = [
  {
    description: "Windows 桌面客户端",
    icon: Monitor,
    name: "Windows",
    released: false,
  },
  {
    description: "macOS 桌面客户端",
    icon: Laptop,
    name: "macOS",
    released: false,
  },
  {
    description: "Android 手机和平板",
    downloadUrl:
      "https://chat-public-1450770193.cos.ap-guangzhou.myqcloud.com/releases/magic-chat-2026-07-17-1.apk",
    icon: Smartphone,
    name: "Android",
    released: true,
  },
  {
    description: "iPhone 与 iPad",
    icon: TabletSmartphone,
    name: "iOS",
    released: false,
  },
]

export function ClientDownloadDialog() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          aria-label="下载客户端"
          className="rounded-md hover:bg-transparent hover:text-teal-500 aria-expanded:bg-transparent aria-expanded:text-teal-500 data-[state=open]:bg-transparent data-[state=open]:text-teal-500 dark:hover:bg-transparent"
          size="icon-sm"
          title="下载客户端"
          type="button"
          variant="ghost"
        >
          <Download className="size-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="gap-5 sm:max-w-lg" showCloseButton={false}>
        <DialogHeader className="pr-10">
          <DialogTitle>下载客户端</DialogTitle>
          <DialogDescription>选择适合当前设备的客户端</DialogDescription>
        </DialogHeader>
        <DialogClose asChild>
          <Button
            aria-label="关闭下载客户端"
            className="absolute top-4 right-4"
            size="icon-sm"
            type="button"
            variant="ghost"
          >
            <X className="size-4" />
          </Button>
        </DialogClose>

        <div className="grid gap-3 sm:grid-cols-2">
          {clientPlatforms.map((platform) => (
            <ClientPlatformCard key={platform.name} platform={platform} />
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ClientPlatformCard({ platform }: { platform: ClientPlatform }) {
  const Icon = platform.icon

  return (
    <section className="flex min-h-36 flex-col rounded-lg border bg-muted/20 p-4">
      <div className="flex items-center gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-background text-foreground ring-1 ring-foreground/10">
          <Icon aria-hidden="true" className="size-5" />
        </div>
        <div className="min-w-0">
          <h3 className="font-medium">{platform.name}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {platform.description}
          </p>
        </div>
      </div>

      <div className="mt-auto pt-5">
        {platform.released ? (
          platform.downloadUrl ? (
            <Button asChild className="w-full">
              <a
                aria-label={`下载 ${platform.name} 客户端`}
                href={platform.downloadUrl}
                rel="noreferrer"
                target="_blank"
              >
                <Download aria-hidden="true" />
                立即下载
              </a>
            </Button>
          ) : (
            <Button
              aria-label={`下载 ${platform.name} 客户端`}
              className="w-full"
              disabled
              type="button"
            >
              <Download aria-hidden="true" />
              下载地址待配置
            </Button>
          )
        ) : (
          <Button className="w-full" disabled type="button" variant="secondary">
            敬请期待
          </Button>
        )}
      </div>
    </section>
  )
}
