import { useRouter } from "expo-router"
import { Fragment, useState } from "react"
import { SafeAreaView } from "react-native-safe-area-context"
import {
  AlertDialog,
  Button,
  Paragraph,
  ScrollView,
  Separator,
  XStack,
  YGroup,
  YStack,
} from "tamagui"

import { PageHeader } from "@/components/navigation/page-header"
import { AddServerDialog } from "@/features/servers/add-server-dialog"
import { useServers } from "@/features/servers/server-context"
import { ServerListItem } from "@/features/servers/server-list-item"
import type { ServerConfig } from "@/features/servers/server-model"

export function ServerManagementScreen() {
  const router = useRouter()
  const { removeServer, selectedServer, selectServer, servers } = useServers()
  const [isAddDialogOpen, setIsAddDialogOpen] = useState(false)
  const [serverToDelete, setServerToDelete] = useState<ServerConfig | null>(null)

  function returnToLogin() {
    if (router.canGoBack()) {
      router.back()
      return
    }

    router.replace("/login")
  }

  function handleSelect(server: ServerConfig) {
    selectServer(server.id)
    router.replace("/init")
  }

  function handleDelete() {
    if (!serverToDelete) {
      return
    }

    removeServer(serverToDelete.id)
    setServerToDelete(null)
  }

  return (
    <YStack bg="$background" flex={1}>
      <PageHeader
        actionLabel="添加"
        onActionPress={() => setIsAddDialogOpen(true)}
        onBackPress={returnToLogin}
        title="服务器管理"
      />

      <SafeAreaView edges={["bottom"]} style={{ flex: 1 }}>
        <ScrollView bg="$background">
          <YStack gap="$2" p="$4">
            <Paragraph color="$color10" px="$2" size="$2">
              服务器
            </Paragraph>

            <YGroup
              borderColor="$borderColor"
              borderWidth={1}
              overflow="hidden"
              rounded="$4"
              size="$5"
            >
              {servers.map((server, index) => (
                <Fragment key={server.id}>
                  <ServerListItem
                    isSelected={server.id === selectedServer.id}
                    onDelete={() => setServerToDelete(server)}
                    onSelect={() => handleSelect(server)}
                    server={server}
                  />
                  {index < servers.length - 1 ? <Separator /> : null}
                </Fragment>
              ))}
            </YGroup>

            <Paragraph color="$color10" px="$2" size="$2">
              点击一项即可切换。向左滑动自定义服务器可以删除。
            </Paragraph>
          </YStack>
        </ScrollView>
      </SafeAreaView>

      <AddServerDialog
        onOpenChange={setIsAddDialogOpen}
        open={isAddDialogOpen}
      />

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) {
            setServerToDelete(null)
          }
        }}
        open={serverToDelete !== null}
      >
        <AlertDialog.Portal>
          <AlertDialog.Overlay bg="$shadow6" opacity={0.5} />
          <AlertDialog.Content bordered elevate gap="$4" maxW={440} width="90%">
            <AlertDialog.Title>删除服务器</AlertDialog.Title>
            <AlertDialog.Description>
              确定删除“{serverToDelete?.name}”吗？此操作无法撤销。
            </AlertDialog.Description>
            <XStack gap="$3" justify="flex-end">
              <AlertDialog.Cancel asChild>
                <Button variant="outlined">取消</Button>
              </AlertDialog.Cancel>
              <AlertDialog.Action asChild>
                <Button onPress={handleDelete} theme="red">
                  删除
                </Button>
              </AlertDialog.Action>
            </XStack>
          </AlertDialog.Content>
        </AlertDialog.Portal>
      </AlertDialog>
    </YStack>
  )
}
