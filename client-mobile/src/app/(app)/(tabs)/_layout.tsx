import { Tabs } from "expo-router"
import { DrawerActions } from "expo-router/react-navigation"
import {
  BriefcaseBusiness,
  ContactRound,
  MessageCircleMore,
  Plus,
  UserPlus,
} from "lucide-react-native"
import { Alert } from "react-native"

import { AppHeader } from "@/components/navigation/app-header"

function showComingSoon(feature: string) {
  Alert.alert(feature, "当前为基础界面版本，这项功能会在后续接入。")
}

export default function AppTabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: true,
        tabBarHideOnKeyboard: true,
      }}
    >
      <Tabs.Screen
        name="messages"
        options={{
          header: ({ navigation }) => (
            <AppHeader
              actions={[
                {
                  icon: Plus,
                  label: "新建会话",
                  onPress: () => showComingSoon("新建会话"),
                },
              ]}
              onMenuPress={() =>
                navigation
                  .getParent()
                  ?.dispatch(DrawerActions.openDrawer())
              }
              title="消息"
            />
          ),
          tabBarIcon: ({ color, size }) => (
            <MessageCircleMore color={color} size={size} />
          ),
          title: "消息",
        }}
      />
      <Tabs.Screen
        name="contacts"
        options={{
          header: ({ navigation }) => (
            <AppHeader
              actions={[
                {
                  icon: UserPlus,
                  label: "添加联系人",
                  onPress: () => showComingSoon("添加联系人"),
                },
              ]}
              onMenuPress={() =>
                navigation
                  .getParent()
                  ?.dispatch(DrawerActions.openDrawer())
              }
              title="通讯录"
            />
          ),
          tabBarIcon: ({ color, size }) => (
            <ContactRound color={color} size={size} />
          ),
          title: "通讯录",
        }}
      />
      <Tabs.Screen
        name="projects"
        options={{
          header: ({ navigation }) => (
            <AppHeader
              actions={[
                {
                  icon: Plus,
                  label: "新建项目",
                  onPress: () => showComingSoon("新建项目"),
                },
              ]}
              onMenuPress={() =>
                navigation
                  .getParent()
                  ?.dispatch(DrawerActions.openDrawer())
              }
              title="项目"
            />
          ),
          tabBarIcon: ({ color, size }) => (
            <BriefcaseBusiness color={color} size={size} />
          ),
          title: "项目",
        }}
      />
    </Tabs>
  )
}
