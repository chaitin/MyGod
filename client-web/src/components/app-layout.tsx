import {
  Cable,
  CircleCheckBig,
  CircleUserRound,
  Loader2Icon,
  LogOut,
  MessageCircleMore,
  Moon,
  Settings,
  Sun,
  SunMoon,
} from "lucide-react"
import { useState } from "react"
import { NavLink, Outlet, useMatch, useNavigate } from "react-router"
import { toast } from "sonner"

import { ProfileSettingsDialog } from "@/components/profile-settings-dialog"
import { useTheme } from "@/components/theme-provider"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { clientLogout } from "@/lib/client-auth"
import { updateCurrentClientUser } from "@/lib/client-data-api"
import type { ClientUser } from "@/lib/client-data-api"
import { useClientData } from "@/lib/client-data-context"

const navItems = [
  { label: "聊天", to: "/chat", icon: MessageCircleMore },
  { label: "通讯录", to: "/contacts", icon: CircleUserRound },
  { label: "任务", to: "/tasks", icon: CircleCheckBig },
  { label: "连接", to: "/connections", icon: Cable },
]

const themeItems = [
  { value: "system", label: "跟随系统", icon: SunMoon },
  { value: "light", label: "明亮模式", icon: Sun },
  { value: "dark", label: "黑暗模式", icon: Moon },
] as const

type ThemeValue = (typeof themeItems)[number]["value"]

export function AppLayout() {
  const { me, refreshMe } = useClientData()

  return (
    <div className="flex h-svh min-h-0 bg-background text-foreground">
      <aside className="flex w-12 shrink-0 flex-col items-center border-r bg-sidebar py-3">
        <UserAvatarMenu user={me} refreshMe={refreshMe} />
        <nav aria-label="主导航" className="flex flex-1 flex-col gap-2">
          {navItems.map((item) => (
            <MainNavItem key={item.to} item={item} />
          ))}
        </nav>
        <ThemeSwitcher />
      </aside>
      <Outlet />
    </div>
  )
}

function UserAvatarMenu({
  refreshMe,
  user,
}: {
  refreshMe: () => Promise<void>
  user: ClientUser
}) {
  const navigate = useNavigate()
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false)
  const [logoutPending, setLogoutPending] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const displayName = getUserDisplayName(user)

  async function handleLogout() {
    setLogoutPending(true)

    try {
      await clientLogout()
      navigate("/login", { replace: true })
    } catch (error) {
      toast.error(getLogoutErrorMessage(error))
    } finally {
      setLogoutPending(false)
    }
  }

  async function handleAvatarSave(avatar: string) {
    try {
      await updateCurrentClientUser({ avatar })
      await refreshMe()
      toast.success("头像已保存")
    } catch (error) {
      toast.error(getProfileUpdateErrorMessage(error))
      throw error
    }
  }

  async function handleNicknameSave(nickname: string) {
    try {
      await updateCurrentClientUser({ nickname })
      await refreshMe()
      toast.success("昵称已保存")
    } catch (error) {
      toast.error(getProfileUpdateErrorMessage(error))
      throw error
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label="用户菜单"
            className="group/avatar-trigger mb-6 rounded-sm bg-muted transition-colors outline-none hover:bg-background focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 data-[state=open]:bg-background"
            size="icon-sm"
            title={displayName}
            type="button"
            variant="ghost"
          >
            <Avatar className="size-8 rounded-sm bg-muted group-hover/avatar-trigger:bg-background group-data-[state=open]/avatar-trigger:bg-background after:rounded-sm after:transition-colors group-hover/avatar-trigger:after:border-ring group-data-[state=open]/avatar-trigger:after:border-ring">
              {user.avatar && (
                <AvatarImage
                  alt={displayName}
                  className="rounded-sm"
                  src={user.avatar}
                />
              )}
              <AvatarFallback className="rounded-sm text-xs">
                {getAvatarInitial(displayName)}
              </AvatarFallback>
            </Avatar>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="right" align="start" className="w-64">
          <UserMenuProfileSummary user={user} />
          <DropdownMenuItem onSelect={() => setSettingsOpen(true)}>
            <Settings className="size-4" />
            设置
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={logoutPending}
            onSelect={() => setLogoutConfirmOpen(true)}
            variant="destructive"
          >
            <LogOut className="size-4" />
            退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
        <ProfileSettingsDialog
          open={settingsOpen}
          onOpenChange={setSettingsOpen}
          onAvatarSave={handleAvatarSave}
          onNicknameSave={handleNicknameSave}
          user={user}
        />
      </DropdownMenu>

      <AlertDialog
        open={logoutConfirmOpen}
        onOpenChange={setLogoutConfirmOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认退出登录</AlertDialogTitle>
            <AlertDialogDescription>
              当前会话将结束，你可以稍后重新登录。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={logoutPending}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={logoutPending}
              onClick={(event) => {
                event.preventDefault()
                void handleLogout()
              }}
              variant="destructive"
            >
              {logoutPending && (
                <Loader2Icon aria-hidden="true" className="animate-spin" />
              )}
              退出登录
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function UserMenuProfileSummary({ user }: { user: ClientUser }) {
  const displayName = getUserDisplayName(user)
  const contactText = user.email || user.phone || "未设置"

  return (
    <div
      aria-label="用户信息"
      className="grid grid-cols-[3rem_minmax(0,1fr)] items-center gap-x-3 px-2 py-3"
      role="group"
    >
      <Avatar className="row-span-2 size-12 rounded-full bg-muted after:rounded-full">
        {user.avatar && (
          <AvatarImage
            alt={displayName}
            className="rounded-full"
            src={user.avatar}
          />
        )}
        <AvatarFallback className="rounded-full text-base">
          {getAvatarInitial(displayName)}
        </AvatarFallback>
      </Avatar>

      <div
        aria-label="姓名信息"
        className="flex min-w-0 items-center gap-1.5 text-sm font-semibold"
        role="group"
      >
        <span className="min-w-0 truncate">{displayName}</span>
      </div>

      <div
        aria-label="联系方式"
        className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground"
        role="group"
      >
        <span className="min-w-0 truncate">{contactText}</span>
      </div>
    </div>
  )
}

function MainNavItem({ item }: { item: (typeof navItems)[number] }) {
  const active = Boolean(useMatch({ path: item.to, end: true }))
  const Icon = item.icon

  return (
    <Button
      asChild
      variant={active ? "default" : "ghost"}
      size="icon-sm"
      className={active ? "rounded-full" : "rounded-full text-muted-foreground"}
    >
      <NavLink to={item.to} aria-label={item.label} title={item.label}>
        <Icon className="size-4" strokeWidth={active ? 2.5 : 2} />
      </NavLink>
    </Button>
  )
}

function ThemeSwitcher() {
  const { theme, setTheme } = useTheme()
  const currentTheme =
    themeItems.find((item) => item.value === theme) ?? themeItems[0]
  const CurrentIcon = currentTheme.icon

  function handleThemeChange(value: string) {
    if (isThemeValue(value)) {
      setTheme(value)
    }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="rounded-md"
          aria-label={`配色：${currentTheme.label}`}
          title={`配色：${currentTheme.label}`}
        >
          <CurrentIcon className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="end" className="w-36">
        <DropdownMenuRadioGroup value={theme} onValueChange={handleThemeChange}>
          {themeItems.map((item) => {
            const Icon = item.icon

            return (
              <DropdownMenuRadioItem key={item.value} value={item.value}>
                <Icon className="size-4" />
                {item.label}
              </DropdownMenuRadioItem>
            )
          })}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function isThemeValue(value: string): value is ThemeValue {
  return themeItems.some((item) => item.value === value)
}

function getUserDisplayName(user: { name: string; nickname: string }) {
  return user.nickname || user.name
}

function getAvatarInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}

function getLogoutErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }

  return "退出登录失败，请稍后重试"
}

function getProfileUpdateErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }

  return "更新个人信息失败，请稍后重试"
}
