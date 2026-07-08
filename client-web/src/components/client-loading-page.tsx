import { ClientDocumentTitle } from "@/components/client-document-title"

export function ClientLoadingPage() {
  return (
    <>
      <ClientDocumentTitle title="正在加载" disableMessageAlert />
      <div className="flex h-svh items-center justify-center bg-background text-foreground">
        <div className="flex w-56 flex-col items-center gap-3">
          <div className="text-sm text-muted-foreground">正在为你加载数据</div>
          <div
            aria-label="加载进度"
            aria-valuetext="加载中"
            className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
            role="progressbar"
          >
            <div className="client-loading-progress-indicator h-full w-1/3 rounded-full bg-primary" />
          </div>
        </div>
      </div>
    </>
  )
}
