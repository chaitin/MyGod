import { CircleCheck, type LucideIcon } from "lucide-react-native"
import {
  Avatar,
  Card,
  H3,
  ListItem,
  Paragraph,
  Separator,
  XStack,
  YStack,
} from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"

export function FeaturePlaceholder({
  description,
  icon,
  kicker,
  title,
}: {
  description: string
  icon: LucideIcon
  kicker: string
  title: string
}) {
  return (
    <YStack bg="$background" flex={1} items="center" justify="center" p="$5">
      <Card maxW={440} size="$5" width="100%">
        <Card.Header gap="$4">
          <XStack gap="$3" items="center">
            <Avatar circular size="$6" theme="blue">
              <Avatar.Fallback>
                <ThemedIcon icon={icon} size={24} />
              </Avatar.Fallback>
            </Avatar>
            <Paragraph color="$color10" size="$2">
              {kicker}
            </Paragraph>
          </XStack>
          <YStack gap="$2">
            <H3>{title}</H3>
            <Paragraph color="$color10">{description}</Paragraph>
          </YStack>
        </Card.Header>

        <Separator />

        <Card.Footer>
          <ListItem
            icon={<ThemedIcon icon={CircleCheck} />}
            subTitle="数据将在接入服务端后显示。"
            title="基础界面已准备好"
          />
        </Card.Footer>
      </Card>
    </YStack>
  )
}
