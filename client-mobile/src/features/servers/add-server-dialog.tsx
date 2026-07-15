import { useRef, useState } from "react"
import {
  Button,
  Dialog,
  Input,
  Label,
  Paragraph,
  Sheet,
  type TamaguiElement,
  XStack,
  YStack,
} from "tamagui"

import { useServers } from "@/features/servers/server-context"
import { isValidServerUrl } from "@/features/servers/server-model"

const SERVER_NAME_INPUT_ID = "new-server-name"
const SERVER_URL_INPUT_ID = "new-server-url"

export function AddServerDialog({
  onOpenChange,
  open,
}: {
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const { addServer } = useServers()
  const urlInputRef = useRef<TamaguiElement>(null)
  const [name, setName] = useState("")
  const [url, setUrl] = useState("")
  const [errorMessage, setErrorMessage] = useState("")
  const canAdd = name.trim().length > 0 && isValidServerUrl(url)

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setName("")
      setUrl("")
      setErrorMessage("")
    }

    onOpenChange(nextOpen)
  }

  function handleAdd() {
    if (!canAdd) {
      setErrorMessage("请填写服务器名称和有效的 HTTP 或 HTTPS 地址")
      return
    }

    const result = addServer(name, url)

    if (result.status === "duplicate") {
      setErrorMessage("该服务器地址已经存在")
      return
    }

    if (result.status === "invalid") {
      setErrorMessage("请填写服务器名称和有效的 HTTP 或 HTTPS 地址")
      return
    }

    handleOpenChange(false)
  }

  return (
    <Dialog modal onOpenChange={handleOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay bg="$shadow6" opacity={0.5} />
        <Dialog.Content bordered elevate gap="$4" maxW={440} width="90%">
          <Dialog.Title>添加服务器</Dialog.Title>
          <Dialog.Description>
            添加一个可供 MagicChat 登录使用的服务器。
          </Dialog.Description>

          <YStack gap="$2">
            <Label htmlFor={SERVER_NAME_INPUT_ID}>服务器名称</Label>
            <Input
              autoFocus
              id={SERVER_NAME_INPUT_ID}
              onChangeText={(value) => {
                setName(value)
                setErrorMessage("")
              }}
              onSubmitEditing={() => urlInputRef.current?.focus()}
              placeholder="例如：公司服务器"
              returnKeyType="next"
              value={name}
            />
          </YStack>

          <YStack gap="$2">
            <Label htmlFor={SERVER_URL_INPUT_ID}>服务器地址</Label>
            <Input
              autoCapitalize="none"
              autoComplete="url"
              id={SERVER_URL_INPUT_ID}
              keyboardType="url"
              onChangeText={(value) => {
                setUrl(value)
                setErrorMessage("")
              }}
              onSubmitEditing={handleAdd}
              placeholder="https://chat.example.com"
              ref={urlInputRef}
              returnKeyType="done"
              value={url}
            />
          </YStack>

          {errorMessage ? (
            <Paragraph color="$red10" size="$2">
              {errorMessage}
            </Paragraph>
          ) : null}

          <XStack gap="$3" justify="flex-end">
            <Dialog.Close asChild>
              <Button variant="outlined">取消</Button>
            </Dialog.Close>
            <Button disabled={!canAdd} onPress={handleAdd} theme="blue">
              添加
            </Button>
          </XStack>
        </Dialog.Content>
      </Dialog.Portal>

      <Dialog.Adapt when="max-sm">
        <Sheet dismissOnSnapToBottom modal snapPointsMode="fit">
          <Sheet.Frame gap="$4" p="$4">
            <Sheet.Handle />
            <Dialog.Adapt.Contents />
          </Sheet.Frame>
          <Sheet.Overlay />
        </Sheet>
      </Dialog.Adapt>
    </Dialog>
  )
}
