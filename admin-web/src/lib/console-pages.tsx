import {
  AppWindowIcon,
  LayoutDashboardIcon,
  SettingsIcon,
  UsersRoundIcon,
} from "lucide-react"

type ConsoleChildPage = {
  path: string
  title: string
}

type ConsolePage = {
  label: string
  path: string
  title: string
  icon: React.ReactNode
  children?: ConsoleChildPage[]
}

export const consolePages: ConsolePage[] = [
  {
    label: "仪表盘",
    path: "/dashboard",
    title: "仪表盘",
    icon: <LayoutDashboardIcon />,
  },
  {
    label: "成员",
    path: "/members",
    title: "成员管理",
    icon: <UsersRoundIcon />,
  },
  {
    label: "应用",
    path: "/apps",
    title: "应用管理",
    icon: <AppWindowIcon />,
  },
  {
    label: "设置",
    path: "/settings",
    title: "系统设置",
    icon: <SettingsIcon />,
  },
] as const

export const defaultConsolePage = consolePages[0].path

export function getConsolePage(pathname: string) {
  for (const page of consolePages) {
    if (page.path === pathname) {
      return {
        page,
        parent: undefined,
      }
    }

    const child = page.children?.find((item) => item.path === pathname)
    if (child) {
      return {
        page: child,
        parent: page,
      }
    }
  }

  return {
    page: consolePages[0],
    parent: undefined,
  }
}
