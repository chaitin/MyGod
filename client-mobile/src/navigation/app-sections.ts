import { type Href } from "expo-router"
import {
  BriefcaseBusiness,
  ContactRound,
  MessageCircleMore,
  type LucideIcon,
} from "lucide-react-native"

export type AppSection = {
  href: Href
  icon: LucideIcon
  label: string
  routeName: "messages" | "contacts" | "projects"
}

export const appSections: readonly AppSection[] = [
  {
    href: "/(app)/(tabs)/messages",
    icon: MessageCircleMore,
    label: "消息",
    routeName: "messages",
  },
  {
    href: "/(app)/(tabs)/contacts",
    icon: ContactRound,
    label: "通讯录",
    routeName: "contacts",
  },
  {
    href: "/(app)/(tabs)/projects",
    icon: BriefcaseBusiness,
    label: "项目",
    routeName: "projects",
  },
]
