import {
  ActivityIcon,
  MessageSquareIcon,
  MessagesSquareIcon,
  RefreshCwIcon,
  UserRoundCheckIcon,
  UsersRoundIcon,
} from "lucide-react"
import { useEffect, useState, type ReactNode } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  AdminDashboardRequestError,
  getAdminDashboardStats,
  type AdminDashboardStats,
} from "@/lib/admin-dashboard"

type StatCardConfig = {
  icon: ReactNode
  label: string
  value: keyof AdminDashboardStats
}

const userStats: StatCardConfig[] = [
  { icon: <UsersRoundIcon />, label: "总用户量", value: "totalUsers" },
  {
    icon: <UserRoundCheckIcon />,
    label: "24 小时访问用户",
    value: "visitedUsers24Hours",
  },
  {
    icon: <UserRoundCheckIcon />,
    label: "7 天访问用户",
    value: "visitedUsers7Days",
  },
  { icon: <ActivityIcon />, label: "当前在线用户", value: "onlineUsers" },
]

const activityStats: StatCardConfig[] = [
  {
    icon: <MessageSquareIcon />,
    label: "24 小时消息",
    value: "messages24Hours",
  },
  {
    icon: <MessageSquareIcon />,
    label: "7 天消息",
    value: "messages7Days",
  },
  {
    icon: <MessagesSquareIcon />,
    label: "24 小时活跃会话",
    value: "activeConversations24Hours",
  },
  {
    icon: <MessagesSquareIcon />,
    label: "7 天活跃会话",
    value: "activeConversations7Days",
  },
]

export default function DashboardPage() {
  const [isLoading, setIsLoading] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const [stats, setStats] = useState<AdminDashboardStats | null>(null)

  useEffect(() => {
    let ignore = false

    async function loadStats() {
      setIsLoading(true)
      try {
        const nextStats = await getAdminDashboardStats()
        if (!ignore) {
          setStats(nextStats)
        }
      } catch (error) {
        if (!ignore) {
          toast.error(
            error instanceof AdminDashboardRequestError
              ? error.message
              : "加载仪表盘统计失败"
          )
        }
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void loadStats()
    return () => {
      ignore = true
    }
  }, [reloadKey])

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-6 p-4 pt-0">
      <div className="flex justify-end">
        <Button
          disabled={isLoading}
          onClick={() => setReloadKey((current) => current + 1)}
          type="button"
          variant="outline"
        >
          <RefreshCwIcon className={isLoading ? "animate-spin" : undefined} />
          刷新
        </Button>
      </div>

      <DashboardSection
        isLoading={isLoading}
        items={userStats}
        stats={stats}
        title="用户概览"
      />
      <DashboardSection
        isLoading={isLoading}
        items={activityStats}
        stats={stats}
        title="消息与会话"
      />
    </div>
  )
}

function DashboardSection({
  isLoading,
  items,
  stats,
  title,
}: {
  isLoading: boolean
  items: StatCardConfig[]
  stats: AdminDashboardStats | null
  title: string
}) {
  return (
    <section className="grid gap-3">
      <h2 className="text-sm font-medium text-muted-foreground">{title}</h2>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {items.map((item) => (
          <Card key={item.value} size="sm">
            <CardHeader>
              <CardTitle className="text-muted-foreground">
                {item.label}
              </CardTitle>
              <CardAction className="text-muted-foreground [&_svg]:size-4">
                {item.icon}
              </CardAction>
            </CardHeader>
            <CardContent>
              {isLoading && !stats ? (
                <Skeleton className="h-9 w-24" />
              ) : (
                <div className="text-3xl font-semibold tracking-tight tabular-nums">
                  {stats ? formatStatValue(stats[item.value]) : "-"}
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  )
}

function formatStatValue(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value)
}
