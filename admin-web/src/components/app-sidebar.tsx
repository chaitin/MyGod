import * as React from "react"

import { NavConsolePages } from "@/components/nav-console-pages"
import { consolePages } from "@/lib/console-pages"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { HomeIcon, MessageSquareIcon } from "lucide-react"

const data = {
  user: {
    name: "shadcn",
    email: "m@example.com",
    avatar: "/avatars/shadcn.jpg",
  },
  navSecondary: [
    {
      title: "长亭百智云",
      url: "https://baizhi.cloud/",
      icon: <HomeIcon />,
    },
    {
      title: "技术论坛",
      url: "https://bbs.baizhi.cloud/",
      icon: <MessageSquareIcon />,
    },
  ],
}
export function AppSidebar({
  appName,
  ...props
}: React.ComponentProps<typeof Sidebar> & { appName: string }) {
  return (
    <Sidebar variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={<a href="https://baizhi.cloud/apps" />}
            >
              <div className="flex aspect-square size-8 items-center justify-center overflow-hidden rounded-lg bg-background">
                <img
                  alt="应用 Logo"
                  className="size-full object-contain"
                  src="/logo.png"
                />
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{appName}</span>
                <span className="truncate text-xs text-sidebar-foreground/70">
                  管理控制面板
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavConsolePages
          pages={consolePages.map((page) => ({
            name: page.title,
            url: page.path,
            icon: page.icon,
            children: page.children?.map((child) => ({
              name: child.title,
              url: child.path,
            })),
          }))}
        />
      </SidebarContent>
      <SidebarFooter>
        <SidebarGroup className="mt-auto">
          <SidebarGroupContent>
            <SidebarMenu>
              {data.navSecondary.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton
                    size="sm"
                    render={
                      <a
                        href={item.url}
                        rel="noreferrer noopener"
                        target="_blank"
                      />
                    }
                  >
                    {item.icon}
                    <span>{item.title}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarFooter>
    </Sidebar>
  )
}
